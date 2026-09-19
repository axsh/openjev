package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *State) UnmarshalJSON(b []byte) error {
	if len(bytes.TrimSpace(b)) == 0 {
		return fmt.Errorf("state is empty")
	}
	s.raw = append(json.RawMessage(nil), bytes.TrimSpace(b)...)
	return nil
}

func (s State) PromptText() (string, error) {
	raw := bytes.TrimSpace(s.raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", fmt.Errorf("state is empty")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", fmt.Errorf("state is empty")
		}
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("state is empty")
		}
		return text, nil
	}
	if raw[0] == '{' || raw[0] == '[' {
		return string(raw), nil
	}
	return "", fmt.Errorf("state is empty")
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
		if q.Type != "choice" {
			return Request{}, fmt.Errorf("question type %q is not supported", q.Type)
		}
		if strings.TrimSpace(q.Instructions) == "" {
			return Request{}, fmt.Errorf("instructions must not be empty")
		}
		if len(q.Criteria) < 2 || len(q.Criteria) > 20 {
			return Request{}, fmt.Errorf("choice requires 2 to 20 options")
		}
		for ci := range q.Criteria {
			c := &q.Criteria[ci]
			if c.Key == "" {
				return Request{}, fmt.Errorf("empty key")
			}
			if c.Description == nil {
				c.Text = c.Key
			} else {
				c.Text = c.Key + ": " + *c.Description
			}
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
	return marshalRequest(req)
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
			if err := dec.Decode(&q.Instructions); err != nil {
				return Question{}, err
			}
		case "criteria":
			criteria, err := parseCriteria(dec)
			if err != nil {
				return Question{}, err
			}
			q.Criteria = criteria
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
		writeJSON(&buf, q.Instructions)
		buf.WriteString(`,"criteria":{`)
		for j, c := range q.Criteria {
			if j > 0 {
				buf.WriteByte(',')
			}
			writeJSON(&buf, c.Key)
			buf.WriteByte(':')
			if c.Description == nil {
				buf.WriteString("null")
			} else {
				writeJSON(&buf, *c.Description)
			}
		}
		buf.WriteString("}}")
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
