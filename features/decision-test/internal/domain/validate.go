package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	errTextEmpty = errors.New("text is empty")
	errTextType  = errors.New("text must be a string, object, or array")
)

func (t *Text) UnmarshalJSON(b []byte) error {
	if len(bytes.TrimSpace(b)) == 0 {
		return errTextEmpty
	}
	t.raw = append(json.RawMessage(nil), bytes.TrimSpace(b)...)
	return nil
}

// PromptText renders the value for the prompt. Strings are returned as-is,
// objects and arrays as their JSON text. Empty, blank, and null values return
// errTextEmpty; numbers and booleans return errTextType.
func (t Text) PromptText() (string, error) {
	raw := bytes.TrimSpace(t.raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", errTextEmpty
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", errTextEmpty
		}
		if strings.TrimSpace(text) == "" {
			return "", errTextEmpty
		}
		return text, nil
	}
	if raw[0] == '{' || raw[0] == '[' {
		return string(raw), nil
	}
	return "", errTextType
}

// Raw returns the trimmed JSON of the value, or nil when it was never set.
func (t Text) Raw() json.RawMessage {
	if len(t.raw) == 0 {
		return nil
	}
	return bytes.TrimSpace(t.raw)
}

func Validate(raw []byte, configuredModelID string) (Request, error) {
	req, err := parse(raw)
	if err != nil {
		return Request{}, err
	}
	if _, err := req.State.PromptText(); err != nil {
		return Request{}, fmt.Errorf("state is empty")
	}
	if len(req.Questions) == 0 {
		return Request{}, fmt.Errorf("questions must not be empty")
	}
	for qi := range req.Questions {
		q := &req.Questions[qi]
		if _, err := q.Instructions.PromptText(); err != nil {
			if errors.Is(err, errTextType) {
				return Request{}, fmt.Errorf("instructions must be a string, object, or array")
			}
			return Request{}, fmt.Errorf("instructions must not be empty")
		}
		if err := interpretQuestion(q); err != nil {
			return Request{}, err
		}
	}
	if req.Model == "" {
		req.Model = configuredModelID
	} else if req.Model != configuredModelID {
		return Request{}, fmt.Errorf("unknown model %q", req.Model)
	}
	if req.Method == "" {
		req.Method = MethodDirect
	} else if req.Method != MethodDirect && req.Method != MethodGeneration && req.Method != MethodBoth {
		return Request{}, fmt.Errorf("unknown method %q", req.Method)
	}
	return req, nil
}

func ApplyMethod(raw []byte, method string) ([]byte, error) {
	if method == "" {
		return append([]byte(nil), raw...), nil
	}
	req, err := parse(raw)
	if err != nil {
		return nil, err
	}
	req.Method = Method(method)
	for i := range req.Questions {
		if err := interpretQuestion(&req.Questions[i]); err != nil {
			return nil, err
		}
	}
	return marshalRequest(req)
}

// TakeQuestions keeps the first n questions of a request body in appearance
// order. n == 0 returns a copy of raw unchanged.
func TakeQuestions(raw []byte, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("questions must be >= 0")
	}
	if n == 0 {
		return append([]byte(nil), raw...), nil
	}
	req, err := parse(raw)
	if err != nil {
		return nil, err
	}
	if len(req.Questions) < n {
		return nil, fmt.Errorf("input has %d questions; --questions %d exceeds it", len(req.Questions), n)
	}
	req.Questions = req.Questions[:n]
	for i := range req.Questions {
		if err := interpretQuestion(&req.Questions[i]); err != nil {
			return nil, err
		}
	}
	return marshalRequest(req)
}

// QuestionCount returns the number of questions in a request body.
func QuestionCount(raw []byte) (int, error) {
	req, err := parse(raw)
	if err != nil {
		return 0, err
	}
	return len(req.Questions), nil
}

func parse(raw []byte) (Request, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return Request{}, fmt.Errorf("invalid JSON")
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return Request{}, fmt.Errorf("invalid JSON")
	}
	var req Request
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return Request{}, err
		}
		switch key {
		case "model":
			if err := dec.Decode(&req.Model); err != nil {
				return Request{}, err
			}
		case "state":
			var body json.RawMessage
			if err := dec.Decode(&body); err != nil {
				return Request{}, err
			}
			req.State.raw = body
		case "questions":
			questions, err := parseQuestions(dec)
			if err != nil {
				return Request{}, err
			}
			req.Questions = questions
		case "options":
			method, err := parseMethod(dec)
			if err != nil {
				return Request{}, err
			}
			req.Method = method
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return Request{}, err
			}
		}
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return Request{}, err
	}
	return req, nil
}

func parseQuestions(dec *json.Decoder) ([]Question, error) {
	if _, err := expectDelim(dec, '{'); err != nil {
		return nil, fmt.Errorf("questions must be an object")
	}
	var out []Question
	seen := map[string]struct{}{}
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate key %s", key)
		}
		seen[key] = struct{}{}
		q, err := parseQuestion(dec)
		if err != nil {
			return nil, err
		}
		q.ID = key
		out = append(out, q)
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return nil, err
	}
	return out, nil
}

func parseQuestion(dec *json.Decoder) (Question, error) {
	if _, err := expectDelim(dec, '{'); err != nil {
		return Question{}, err
	}
	var q Question
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return Question{}, err
		}
		switch key {
		case "type":
			if err := dec.Decode(&q.Type); err != nil {
				return Question{}, err
			}
		case "instructions":
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return Question{}, err
			}
			q.Instructions.raw = raw
		case "criteria":
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return Question{}, err
			}
			q.criteriaRaw = raw
			q.sawCriteria = true
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return Question{}, err
			}
		}
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return Question{}, err
	}
	return q, nil
}

func interpretQuestion(q *Question) error {
	switch q.Type {
	case "choice":
		return interpretChoice(q)
	case "score":
		return interpretScore(q)
	case "noul":
		return interpretNoul(q)
	default:
		return fmt.Errorf("question type %q is not supported", q.Type)
	}
}

func interpretChoice(q *Question) error {
	if !q.sawCriteria {
		return fmt.Errorf("choice requires 2 to 20 options")
	}
	dec := json.NewDecoder(bytes.NewReader(q.criteriaRaw))
	criteria, err := parseCriteria(dec)
	if err != nil {
		return err
	}
	if len(criteria) < 2 || len(criteria) > 20 {
		return fmt.Errorf("choice requires 2 to 20 options")
	}
	for i := range criteria {
		if criteria[i].Description == nil {
			criteria[i].Text = criteria[i].Key
		} else {
			criteria[i].Text = criteria[i].Key + ": " + *criteria[i].Description
		}
	}
	q.Criteria = criteria
	return nil
}

func interpretScore(q *Question) error {
	if !q.sawCriteria {
		return fmt.Errorf("score requires 2 to 10 levels")
	}
	dec := json.NewDecoder(bytes.NewReader(q.criteriaRaw))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("score criteria must be an array")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("score criteria must be an array")
	}
	var levels []string
	seen := map[string]struct{}{}
	for dec.More() {
		var text string
		if err := dec.Decode(&text); err != nil {
			return fmt.Errorf("score criteria must be an array")
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("score level must not be blank")
		}
		if _, ok := seen[text]; ok {
			return fmt.Errorf("duplicate score level")
		}
		seen[text] = struct{}{}
		levels = append(levels, text)
	}
	if _, err := expectDelim(dec, ']'); err != nil {
		return fmt.Errorf("score criteria must be an array")
	}
	if len(levels) < 2 || len(levels) > 10 {
		return fmt.Errorf("score requires 2 to 10 levels")
	}
	q.Criteria = make([]Criterion, len(levels))
	for i, text := range levels {
		q.Criteria[i] = Criterion{Key: strconv.Itoa(i), Text: text}
	}
	return nil
}

func interpretNoul(q *Question) error {
	if !q.sawCriteria || bytes.Equal(bytes.TrimSpace(q.criteriaRaw), []byte("null")) {
		q.Criteria = []Criterion{{Key: "true", Text: "true"}, {Key: "false", Text: "false"}}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(q.criteriaRaw))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("noul criteria must be an object")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("noul criteria must be an object")
	}
	var trueText, falseText string
	var sawTrue, sawFalse bool
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return err
		}
		var text string
		if err := dec.Decode(&text); err != nil {
			return fmt.Errorf("noul criteria must be an object")
		}
		switch key {
		case "true":
			if sawTrue {
				return fmt.Errorf("duplicate key true")
			}
			sawTrue = true
			trueText = text
		case "false":
			if sawFalse {
				return fmt.Errorf("duplicate key false")
			}
			sawFalse = true
			falseText = text
		default:
			return fmt.Errorf("noul criteria has unexpected key")
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("noul criteria must not be blank")
		}
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return err
	}
	if !sawTrue || !sawFalse {
		return fmt.Errorf("noul criteria requires true and false")
	}
	q.Criteria = []Criterion{{Key: "true", Text: trueText}, {Key: "false", Text: falseText}}
	return nil
}

func parseCriteria(dec *json.Decoder) ([]Criterion, error) {
	if _, err := expectDelim(dec, '{'); err != nil {
		return nil, fmt.Errorf("criteria must be an object")
	}
	var out []Criterion
	seen := map[string]struct{}{}
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return nil, err
		}
		if key == "" {
			var skip json.RawMessage
			_ = dec.Decode(&skip)
			return nil, fmt.Errorf("empty key")
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate key %s", key)
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		item := Criterion{Key: key}
		trimmed := bytes.TrimSpace(raw)
		if bytes.Equal(trimmed, []byte("null")) {
			out = append(out, item)
			continue
		}
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil, fmt.Errorf("criteria description must be a string or null")
		}
		item.Description = &text
		out = append(out, item)
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return nil, err
	}
	return out, nil
}

func parseMethod(dec *json.Decoder) (Method, error) {
	if _, err := expectDelim(dec, '{'); err != nil {
		return "", err
	}
	var method Method
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return "", err
		}
		if key == "method" {
			var value string
			if err := dec.Decode(&value); err != nil {
				return "", err
			}
			method = Method(value)
			continue
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return "", err
		}
	}
	if _, err := expectDelim(dec, '}'); err != nil {
		return "", err
	}
	return method, nil
}

func objectKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected object key")
	}
	return key, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) (json.Delim, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != want {
		return 0, fmt.Errorf("expected %q", string(want))
	}
	return delim, nil
}

func marshalRequest(req Request) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	comma := func() {
		if !first {
			buf.WriteByte(',')
		}
		first = false
	}
	if req.Model != "" {
		comma()
		buf.WriteString(`"model":`)
		writeJSON(&buf, req.Model)
	}
	comma()
	buf.WriteString(`"state":`)
	if len(req.State.raw) == 0 {
		buf.WriteString("null")
	} else {
		buf.Write(bytes.TrimSpace(req.State.raw))
	}
	comma()
	buf.WriteString(`"questions":{`)
	for i, q := range req.Questions {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeJSON(&buf, q.ID)
		buf.WriteString(`:{"type":`)
		writeJSON(&buf, q.Type)
		buf.WriteString(`,"instructions":`)
		if instructions := q.Instructions.Raw(); instructions == nil {
			buf.WriteString("null")
		} else {
			buf.Write(instructions)
		}
		buf.WriteString(`,"criteria":`)
		switch q.Type {
		case "score":
			buf.WriteByte('[')
			for j, c := range q.Criteria {
				if j > 0 {
					buf.WriteByte(',')
				}
				writeJSON(&buf, c.Text)
			}
			buf.WriteByte(']')
		default:
			buf.WriteByte('{')
			for j, c := range q.Criteria {
				if j > 0 {
					buf.WriteByte(',')
				}
				key := c.Key
				if q.Type == "noul" && key == "" {
					key = c.Text
				}
				writeJSON(&buf, key)
				buf.WriteByte(':')
				if q.Type == "noul" {
					writeJSON(&buf, c.Text)
				} else if c.Description == nil {
					buf.WriteString("null")
				} else {
					writeJSON(&buf, *c.Description)
				}
			}
			buf.WriteByte('}')
		}
		buf.WriteByte('}')
	}
	buf.WriteByte('}')
	if req.Method != "" {
		comma()
		buf.WriteString(`"options":{"method":`)
		writeJSON(&buf, string(req.Method))
		buf.WriteByte('}')
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func writeJSON(buf *bytes.Buffer, value string) {
	raw, err := json.Marshal(value)
	if err != nil {
		buf.WriteString(`""`)
		return
	}
	buf.Write(raw)
}
