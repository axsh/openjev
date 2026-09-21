package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"openjev/features/jev-test/internal/check"
	"openjev/features/jev-test/internal/logger"
)

type Loaded struct {
	Body      []byte
	Questions []check.Question
}

type PostResult struct {
	Status int
	Body   []byte
	WallMs float64
}

func ReadKey(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("api key is empty")
	}
	return token, nil
}

func Load(path, defaultModel string) (Loaded, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Loaded{}, fmt.Errorf("input json: %w", err)
	}
	if doc == nil {
		return Loaded{}, errors.New("state is required")
	}
	if !statePresent(doc["state"]) {
		return Loaded{}, errors.New("state is required")
	}
	qraw, ok := doc["questions"]
	if !ok {
		return Loaded{}, errors.New("questions is required")
	}
	questions, err := parseQuestions(qraw)
	if err != nil {
		return Loaded{}, err
	}
	if len(questions) == 0 {
		return Loaded{}, errors.New("questions is required")
	}
	if modelMissing(doc["model"]) {
		encoded, err := json.Marshal(defaultModel)
		if err != nil {
			return Loaded{}, err
		}
		doc["model"] = encoded
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return Loaded{}, err
	}
	return Loaded{Body: body, Questions: questions}, nil
}

func Post(ctx context.Context, url, token string, body []byte, timeout time.Duration, log *logger.Logger) (PostResult, error) {
	if log != nil {
		log.Debug("posting", "url", url, "body_size", len(body), "timeout_ms", timeout.Milliseconds())
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return PostResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		wall := float64(time.Since(start).Microseconds()) / 1000
		return PostResult{WallMs: wall}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	wall := float64(time.Since(start).Microseconds()) / 1000
	if log != nil {
		log.Debug("post completed", "status", resp.StatusCode, "duration_ms", wall, "body_size", len(respBody))
		log.Trace("response body", "body", string(respBody))
	}
	if err != nil {
		return PostResult{Status: resp.StatusCode, Body: respBody, WallMs: wall}, err
	}
	return PostResult{Status: resp.StatusCode, Body: respBody, WallMs: wall}, nil
}

func statePresent(raw json.RawMessage) bool {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return false
	}
	trim := bytes.TrimSpace(raw)
	switch string(trim) {
	case `""`, "{}", "[]":
		return false
	}
	return true
}

func modelMissing(raw json.RawMessage) bool {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || string(trim) == "null" {
		return true
	}
	var s string
	if err := json.Unmarshal(trim, &s); err != nil {
		return false
	}
	return strings.TrimSpace(s) == ""
}

func parseQuestions(raw json.RawMessage) ([]check.Question, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, errors.New("questions is required")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, errors.New("questions is required")
	}
	var out []check.Question
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("input json: %w", err)
		}
		id, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("questions is required")
		}
		var obj map[string]json.RawMessage
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("input json: %w", err)
		}
		q, err := interpretQuestion(id, obj)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("input json: %w", err)
	}
	return out, nil
}

func interpretQuestion(id string, obj map[string]json.RawMessage) (check.Question, error) {
	var q check.Question
	q.ID = id
	var typ string
	if err := json.Unmarshal(obj["type"], &typ); err != nil {
		return check.Question{}, fmt.Errorf("%s: type must be choice, score, or noul", id)
	}
	q.Type = typ
	switch typ {
	case "choice":
		keys, err := objectKeys(obj["criteria"])
		if err != nil || len(keys) == 0 {
			return check.Question{}, fmt.Errorf("%s: field criteria", id)
		}
		q.ChoiceKeys = keys
	case "score":
		n, err := arrayLen(obj["criteria"])
		if err != nil || n < 2 || n > 10 {
			return check.Question{}, fmt.Errorf("%s: field criteria", id)
		}
		q.ScoreLevels = n
	case "noul":
		if raw, ok := obj["criteria"]; ok && len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
			if _, err := objectKeys(raw); err != nil {
				return check.Question{}, fmt.Errorf("%s: field criteria", id)
			}
		}
	default:
		return check.Question{}, fmt.Errorf("%s: type must be choice, score, or noul", id)
	}
	return q, nil
}

func objectKeys(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, errors.New("criteria must be an object")
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("criteria must be an object")
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return keys, nil
}

func arrayLen(raw json.RawMessage) (int, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return 0, errors.New("criteria must be an array")
	}
	n := 0
	for dec.More() {
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return 0, err
		}
		n++
	}
	if _, err := dec.Token(); err != nil {
		return 0, err
	}
	return n, nil
}
