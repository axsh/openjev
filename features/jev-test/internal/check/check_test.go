package check

import (
	"strings"
	"testing"
)

func sampleQuestions() []Question {
	return []Question{
		{ID: "queue", Type: "choice", ChoiceKeys: []string{"account_access", "billing"}},
		{ID: "frustration", Type: "score", ScoreLevels: 3},
		{ID: "is_urgent", Type: "noul"},
	}
}

func goodBody() string {
	return `{
		"model": "jev-1.13.0",
		"elapsedMs": 1200,
		"usage": {"input_tokens": 1, "output_tokens": 0},
		"answers": {
			"queue": {
				"type": "choice",
				"choice": "account_access",
				"confidence": 1,
				"probabilities": {"account_access": 0.5, "billing": 0.5}
			},
			"frustration": {
				"type": "score",
				"score": 2,
				"confidence": 1,
				"probabilities": {"0": 1, "1": 0, "2": 0},
				"legend": {}
			},
			"is_urgent": {"type": "noul", "noul": 0}
		}
	}`
}

func TestCheckAcceptsFixture(t *testing.T) {
	t.Run("bounds", func(t *testing.T) {
		report := Check(sampleQuestions(), []byte(goodBody()))
		if !report.ContentOK {
			t.Fatalf("reasons %v", report.Reasons)
		}
		if report.Model != "jev-1.13.0" {
			t.Fatalf("model %q", report.Model)
		}
		if report.ServerElapsedMs == nil || *report.ServerElapsedMs != 1200 {
			t.Fatalf("elapsed %#v", report.ServerElapsedMs)
		}
		if report.Usage == nil || report.Usage.InputTokens != 1 || report.Usage.OutputTokens != 0 {
			t.Fatalf("usage %#v", report.Usage)
		}
		if len(report.Answers) != 3 {
			t.Fatalf("answers %d", len(report.Answers))
		}
		want := []string{"queue", "frustration", "is_urgent"}
		for i, id := range want {
			if report.Answers[i].ID != id {
				t.Fatalf("order[%d]=%s", i, report.Answers[i].ID)
			}
		}
		if report.Answers[0].Choice != "account_access" || !report.Answers[0].HasConf || report.Answers[0].Confidence != 1 {
			t.Fatalf("choice %#v", report.Answers[0])
		}
		if !report.Answers[1].HasScore || report.Answers[1].Score != 2 {
			t.Fatalf("score %#v", report.Answers[1])
		}
		if !report.Answers[2].HasNoul || report.Answers[2].Noul != 0 {
			t.Fatalf("noul %#v", report.Answers[2])
		}
	})

	t.Run("noul one", func(t *testing.T) {
		body := strings.Replace(goodBody(), `"noul": 0`, `"noul": 1`, 1)
		report := Check(sampleQuestions(), []byte(body))
		if !report.ContentOK {
			t.Fatalf("reasons %v", report.Reasons)
		}
		if report.Answers[2].Noul != 1 {
			t.Fatalf("noul %v", report.Answers[2].Noul)
		}
	})

	t.Run("usage absent", func(t *testing.T) {
		body := strings.Replace(goodBody(), `"usage": {"input_tokens": 1, "output_tokens": 0},`, "", 1)
		report := Check(sampleQuestions(), []byte(body))
		if !report.ContentOK {
			t.Fatalf("reasons %v", report.Reasons)
		}
		if report.Usage != nil {
			t.Fatalf("usage %#v", report.Usage)
		}
	})
}

func TestCheckRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "missing id",
			body: replaceAnswer(goodBody(), "queue", ""),
			want: []string{"queue", "missing"},
		},
		{
			name: "extra id",
			body: strings.Replace(goodBody(), `"is_urgent": {"type": "noul", "noul": 0}`, `"is_urgent": {"type": "noul", "noul": 0}, "extra": {"type": "noul", "noul": 0}`, 1),
			want: []string{"extra"},
		},
		{
			name: "wrong type",
			body: strings.Replace(goodBody(), `"queue": {
				"type": "choice",`, `"queue": {
				"type": "score",`, 1),
			want: []string{"queue", "type"},
		},
		{
			name: "choice not in criteria",
			body: strings.Replace(goodBody(), `"choice": "account_access"`, `"choice": "nope"`, 1),
			want: []string{"queue", "choice"},
		},
		{
			name: "probability keys",
			body: strings.Replace(goodBody(), `"account_access": 0.5, "billing": 0.5`, `"account_access": 1`, 1),
			want: []string{"queue", "probabilities"},
		},
		{
			name: "confidence high",
			body: strings.Replace(goodBody(), `"confidence": 1,
				"probabilities": {"account_access": 0.5, "billing": 0.5}`, `"confidence": 1.1,
				"probabilities": {"account_access": 0.5, "billing": 0.5}`, 1),
			want: []string{"queue", "confidence"},
		},
		{
			name: "confidence low",
			body: strings.Replace(goodBody(), `"confidence": 1,
				"probabilities": {"account_access": 0.5, "billing": 0.5}`, `"confidence": -0.1,
				"probabilities": {"account_access": 0.5, "billing": 0.5}`, 1),
			want: []string{"queue", "confidence"},
		},
		{
			name: "score below",
			body: strings.Replace(goodBody(), `"score": 2`, `"score": -0.1`, 1),
			want: []string{"frustration", "score"},
		},
		{
			name: "score above",
			body: strings.Replace(goodBody(), `"score": 2`, `"score": 2.1`, 1),
			want: []string{"frustration", "score"},
		},
		{
			name: "legend array",
			body: strings.Replace(goodBody(), `"legend": {}`, `"legend": []`, 1),
			want: []string{"frustration", "legend"},
		},
		{
			name: "noul below",
			body: strings.Replace(goodBody(), `"noul": 0`, `"noul": -0.1`, 1),
			want: []string{"is_urgent", "noul"},
		},
		{
			name: "noul above",
			body: strings.Replace(goodBody(), `"noul": 0`, `"noul": 1.1`, 1),
			want: []string{"is_urgent", "noul"},
		},
		{
			name: "empty model",
			body: strings.Replace(goodBody(), `"model": "jev-1.13.0"`, `"model": ""`, 1),
			want: []string{"model"},
		},
		{
			name: "usage fraction",
			body: strings.Replace(goodBody(), `"input_tokens": 1`, `"input_tokens": 1.5`, 1),
			want: []string{"usage", "input_tokens"},
		},
		{
			name: "usage negative",
			body: strings.Replace(goodBody(), `"output_tokens": 0`, `"output_tokens": -1`, 1),
			want: []string{"usage", "output_tokens"},
		},
		{
			name: "not object",
			body: `[1, 2]`,
			want: []string{"response"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := Check(sampleQuestions(), []byte(tt.body))
			if report.ContentOK {
				t.Fatal("expected content_ok false")
			}
			joined := strings.Join(report.Reasons, "\n")
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Fatalf("reasons %q missing %q", joined, w)
				}
			}
		})
	}
}

func TestCheckEnvelope(t *testing.T) {
	inner := `{
		"model": "jev-1.13.0",
		"answers": {
			"queue": {
				"type": "choice",
				"choice": "account_access",
				"confidence": 1,
				"probabilities": {"account_access": 1, "billing": 0}
			},
			"frustration": {
				"type": "score",
				"score": 0,
				"confidence": 1,
				"probabilities": {"0": 1, "1": 0, "2": 0},
				"legend": {}
			},
			"is_urgent": {"type": "noul", "noul": 0.5}
		}
	}`
	body := `{"code":0,"message":"ok","elapsedMs":50,"data":` + inner + `}`
	report := Check(sampleQuestions(), []byte(body))
	if !report.ContentOK {
		t.Fatalf("reasons %v", report.Reasons)
	}
	if report.ServerElapsedMs == nil || *report.ServerElapsedMs != 50 {
		t.Fatalf("elapsed %#v", report.ServerElapsedMs)
	}
	if report.Model != "jev-1.13.0" {
		t.Fatalf("model %q", report.Model)
	}
}

func TestCheckNoulIgnoresConfidence(t *testing.T) {
	body := strings.Replace(goodBody(), `"noul": 0`, `"noul": 0, "confidence": 5`, 1)
	report := Check(sampleQuestions(), []byte(body))
	if !report.ContentOK {
		t.Fatalf("reasons %v", report.Reasons)
	}
}

func TestCheckProbabilitySumNotRequired(t *testing.T) {
	body := strings.Replace(goodBody(), `"account_access": 0.5, "billing": 0.5`, `"account_access": 0.2, "billing": 0.2`, 1)
	report := Check(sampleQuestions(), []byte(body))
	if !report.ContentOK {
		t.Fatalf("reasons %v", report.Reasons)
	}
}

func replaceAnswer(body, id, replacement string) string {
	// Drop the queue object by replacing from its key through the following comma-newline when replacement is empty.
	start := strings.Index(body, `"`+id+`"`)
	if start < 0 {
		return body
	}
	if replacement != "" {
		return body
	}
	rest := body[start:]
	end := strings.Index(rest, `"frustration"`)
	if end < 0 {
		return body
	}
	return body[:start] + rest[end:]
}
