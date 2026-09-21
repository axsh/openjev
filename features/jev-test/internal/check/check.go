package check

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
)

type Question struct {
	ID          string
	Type        string
	ChoiceKeys  []string
	ScoreLevels int
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Answer struct {
	ID         string
	Type       string
	Choice     string
	Score      float64
	Noul       float64
	Confidence float64
	HasScore   bool
	HasNoul    bool
	HasConf    bool
}

type Report struct {
	Model           string
	ServerElapsedMs *float64
	Usage           *Usage
	ContentOK       bool
	Reasons         []string
	Answers         []Answer
}

func Check(questions []Question, body []byte) Report {
	var report Report
	top, ok := asMap(body)
	if !ok {
		report.Reasons = []string{"response: not a JSON object"}
		return report
	}
	result := top
	if data, enveloped := envelopeData(top); enveloped {
		result = data
	}
	if ms, present, bad := elapsed(result, top); bad {
		report.Reasons = append(report.Reasons, "elapsedMs")
	} else if present {
		report.ServerElapsedMs = &ms
	}
	if raw, exists := result["model"]; !exists {
		report.Reasons = append(report.Reasons, "model")
	} else if s, isString := asString(raw); !isString || s == "" {
		report.Reasons = append(report.Reasons, "model")
	} else {
		report.Model = s
	}
	answers, answersOK := asMap(result["answers"])
	if !answersOK {
		report.Reasons = append(report.Reasons, "answers")
		answers = map[string]json.RawMessage{}
	}
	known := map[string]bool{}
	for _, q := range questions {
		known[q.ID] = true
		raw, exists := answers[q.ID]
		if !exists {
			report.Reasons = append(report.Reasons, q.ID+": missing answer")
			report.Answers = append(report.Answers, Answer{ID: q.ID})
			continue
		}
		report.Answers = append(report.Answers, checkAnswer(&report, q, raw))
	}
	for id := range answers {
		if !known[id] {
			report.Reasons = append(report.Reasons, id+": extra answer")
		}
	}
	if raw, exists := result["usage"]; exists {
		report.Usage = checkUsage(&report, raw)
	}
	report.ContentOK = len(report.Reasons) == 0
	return report
}

func checkAnswer(report *Report, q Question, raw json.RawMessage) Answer {
	answer := Answer{ID: q.ID}
	obj, ok := asMap(raw)
	if !ok {
		report.Reasons = append(report.Reasons, q.ID+": field type")
		return answer
	}
	if s, isString := asString(obj["type"]); isString {
		answer.Type = s
	}
	if answer.Type != q.Type {
		report.Reasons = append(report.Reasons, q.ID+": field type")
	}
	switch q.Type {
	case "choice":
		checkChoice(report, &answer, q, obj)
	case "score":
		checkScore(report, &answer, q, obj)
	case "noul":
		checkNoul(report, &answer, q, obj)
	default:
		report.Reasons = append(report.Reasons, q.ID+": field type")
	}
	return answer
}

func checkChoice(report *Report, answer *Answer, q Question, obj map[string]json.RawMessage) {
	choice, ok := asString(obj["choice"])
	if !ok || !contains(q.ChoiceKeys, choice) {
		report.Reasons = append(report.Reasons, q.ID+": field choice")
	} else {
		answer.Choice = choice
	}
	probs, ok := asMap(obj["probabilities"])
	if !ok || !sameKeys(probs, q.ChoiceKeys) || !unitValues(probs) {
		report.Reasons = append(report.Reasons, q.ID+": field probabilities")
	}
	readConfidence(report, answer, q.ID, obj, true)
}

func checkScore(report *Report, answer *Answer, q Question, obj map[string]json.RawMessage) {
	score, ok := asFloat(obj["score"])
	maxLevel := float64(q.ScoreLevels - 1)
	if !ok || score < 0 || score > maxLevel {
		report.Reasons = append(report.Reasons, q.ID+": field score")
	} else {
		answer.Score = score
		answer.HasScore = true
	}
	probs, ok := asMap(obj["probabilities"])
	if !ok || !sameKeys(probs, scoreKeyList(q.ScoreLevels)) || !unitValues(probs) {
		report.Reasons = append(report.Reasons, q.ID+": field probabilities")
	}
	legend, exists := obj["legend"]
	if !exists || !isObject(legend) {
		report.Reasons = append(report.Reasons, q.ID+": field legend")
	}
	readConfidence(report, answer, q.ID, obj, true)
}

func checkNoul(report *Report, answer *Answer, q Question, obj map[string]json.RawMessage) {
	noul, ok := asFloat(obj["noul"])
	if !ok || noul < 0 || noul > 1 {
		report.Reasons = append(report.Reasons, q.ID+": field noul")
		return
	}
	answer.Noul = noul
	answer.HasNoul = true
}

func readConfidence(report *Report, answer *Answer, id string, obj map[string]json.RawMessage, required bool) {
	raw, exists := obj["confidence"]
	if !exists {
		if required {
			report.Reasons = append(report.Reasons, id+": field confidence")
		}
		return
	}
	v, ok := asFloat(raw)
	if !ok || v < 0 || v > 1 {
		report.Reasons = append(report.Reasons, id+": field confidence")
		return
	}
	answer.Confidence = v
	answer.HasConf = true
}

func checkUsage(report *Report, raw json.RawMessage) *Usage {
	obj, ok := asMap(raw)
	if !ok {
		report.Reasons = append(report.Reasons, "usage.input_tokens")
		report.Reasons = append(report.Reasons, "usage.output_tokens")
		return nil
	}
	in, inOK := asWhole(obj["input_tokens"])
	out, outOK := asWhole(obj["output_tokens"])
	if !inOK {
		report.Reasons = append(report.Reasons, "usage.input_tokens")
	}
	if !outOK {
		report.Reasons = append(report.Reasons, "usage.output_tokens")
	}
	if !inOK || !outOK {
		return nil
	}
	return &Usage{InputTokens: in, OutputTokens: out}
}

func envelopeData(top map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	if _, ok := top["code"]; !ok {
		return nil, false
	}
	if _, ok := top["message"]; !ok {
		return nil, false
	}
	data, ok := asMap(top["data"])
	if !ok {
		return nil, false
	}
	if _, ok := data["answers"]; !ok {
		return nil, false
	}
	return data, true
}

func elapsed(result, top map[string]json.RawMessage) (float64, bool, bool) {
	for _, src := range []map[string]json.RawMessage{result, top} {
		raw, ok := src["elapsedMs"]
		if !ok {
			continue
		}
		v, isNum := asFloat(raw)
		if !isNum {
			return 0, false, true
		}
		return v, true, false
	}
	return 0, false, false
}

func asMap(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if !isObject(raw) {
		return nil, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, false
	}
	return m, true
}

func asString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func asFloat(raw json.RawMessage) (float64, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, false
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func asWhole(raw json.RawMessage) (int, bool) {
	v, ok := asFloat(raw)
	if !ok || v < 0 || v != math.Trunc(v) || v > math.MaxInt {
		return 0, false
	}
	return int(v), true
}

func isObject(raw json.RawMessage) bool {
	trim := bytes.TrimSpace(raw)
	return len(trim) > 0 && trim[0] == '{'
}

func sameKeys(got map[string]json.RawMessage, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			return false
		}
	}
	return true
}

func unitValues(m map[string]json.RawMessage) bool {
	for _, raw := range m {
		v, ok := asFloat(raw)
		if !ok || v < 0 || v > 1 {
			return false
		}
	}
	return true
}

func scoreKeyList(levels int) []string {
	keys := make([]string, levels)
	for i := range levels {
		keys[i] = strconv.Itoa(i)
	}
	return keys
}

func contains(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}
