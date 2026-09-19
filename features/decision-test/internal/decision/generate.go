package decision

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strings"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/prompt"
)

var thinkPrefix = regexp.MustCompile(`(?i)^<think>[\s\S]*?</think>\s*`)

type GenResult struct {
	Valid           bool
	ValidationError string
	ChoiceKey       string
	Probabilities   map[string]float64
}

func ValidateGeneration(text string, criteria []domain.Criterion) GenResult {
	cleaned := strings.TrimSpace(thinkPrefix.ReplaceAllString(text, ""))
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return GenResult{ValidationError: "invalid JSON"}
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return GenResult{ValidationError: "expected one JSON object"}
	}
	values := map[string]float64{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return GenResult{ValidationError: "invalid JSON"}
		}
		key, ok := keyTok.(string)
		if !ok {
			return GenResult{ValidationError: "invalid JSON"}
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return GenResult{ValidationError: "invalid JSON"}
		}
		number, ok := parseProbability(raw)
		if !ok {
			return GenResult{ValidationError: "probabilities must be numbers from 0 to 1"}
		}
		values[key] = number
	}
	end, err := dec.Token()
	if err != nil {
		return GenResult{ValidationError: "invalid JSON"}
	}
	if endDelim, ok := end.(json.Delim); !ok || endDelim != '}' {
		return GenResult{ValidationError: "expected one JSON object"}
	}
	expected := prompt.ExpectedGenerationKeys(criteria)
	if len(values) != len(expected) {
		return GenResult{ValidationError: "expected one probability for every exact option key"}
	}
	probs := make([]float64, len(expected))
	total := 0.0
	mapped := map[string]float64{}
	for i, key := range expected {
		value, ok := values[key]
		if !ok {
			return GenResult{ValidationError: "expected one probability for every exact option key"}
		}
		probs[i] = value
		total += value
		mapped[criteria[i].Key] = value
	}
	if math.Abs(total-1) > 0.02+1e-9 {
		return GenResult{ValidationError: "probabilities must sum to 1"}
	}
	return GenResult{
		Valid:         true,
		ChoiceKey:     criteria[ArgMax(probs)].Key,
		Probabilities: mapped,
	}
}

func parseProbability(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] == '"' || raw[0] == '{' || raw[0] == '[' {
		return 0, false
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, false
	}
	value, err := number.Float64()
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return 0, false
	}
	return value, true
}
