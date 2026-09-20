package decision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

type fakeEngine struct {
	calls    []string
	text     string
	emptyTop bool
}

func (f *fakeEngine) ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []engine.Label) (engine.ReadResult, error) {
	f.calls = append(f.calls, "read")
	if f.emptyTop {
		return engine.ReadResult{PromptTokens: 10, CompletionTokens: 1, ElapsedMs: 95.2}, nil
	}
	top := make([]engine.TokenLogprob, len(labels))
	for i, label := range labels {
		top[i] = engine.TokenLogprob{ID: label.TokenID, Logprob: 0}
	}
	if len(top) > 0 {
		top[0].Logprob = 1
	}
	return engine.ReadResult{Top: top, PromptTokens: 10, CompletionTokens: 1, ElapsedMs: 95.2}, nil
}

func (f *fakeEngine) Generate(ctx context.Context, messages []prompt.Message) (engine.GenRaw, error) {
	f.calls = append(f.calls, "generate")
	return engine.GenRaw{Text: f.text, TTFTMs: 1, TotalMs: 2310.7, PromptTokens: 12, CompletionTokens: 61}, nil
}

func (f *fakeEngine) Health(ctx context.Context) (bool, error) { return true, nil }

func testQuestion() domain.Question {
	return domain.Question{
		ID:           "queue",
		Type:         "choice",
		Instructions: domain.TextOf("Which queue?"),
		Criteria: []domain.Criterion{
			{Key: "account_access", Text: "account_access: Account access support"},
			{Key: "billing", Text: "billing: Billing support"},
		},
	}
}

func TestServiceDirect(t *testing.T) {
	fake := &fakeEngine{}
	svc := &Service{Engine: fake, Labels: []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}}, Log: logger.New(io.Discard)}
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion()}, Method: domain.MethodDirect})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "read" {
		t.Fatalf("calls %v", fake.calls)
	}
	ans := resp.Answers["queue"]
	if ans.Generation != nil || ans.Choice != "account_access" || ans.Timings.DirectMs <= 0 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("%+v usage %+v", ans, resp.Usage)
	}
}

func TestServiceBothOrder(t *testing.T) {
	fake := &fakeEngine{text: `{"A: account_access: Account access support": 0.2, "B: billing: Billing support": 0.8}`}
	svc := &Service{Engine: fake, Labels: []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}}, Log: logger.New(io.Discard)}
	q2 := testQuestion()
	q2.ID = "next"
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion(), q2}, Method: domain.MethodBoth})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 4 || fake.calls[0] != "read" || fake.calls[1] != "generate" || fake.calls[2] != "read" {
		t.Fatalf("calls %v", fake.calls)
	}
	ans := resp.Answers["queue"]
	if ans.Choice != "account_access" || ans.Generation.Choice != "billing" {
		t.Fatalf("direct %s gen %s", ans.Choice, ans.Generation.Choice)
	}
	if ans.Timings.Ratio == nil || *ans.Timings.Ratio != ans.Timings.GenerationMs/ans.Timings.DirectMs {
		t.Fatalf("ratio %+v", ans.Timings)
	}
}

func TestServiceGenerationInvalid(t *testing.T) {
	fake := &fakeEngine{text: "not json"}
	svc := &Service{Engine: fake, Labels: []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}}, Log: logger.New(io.Discard)}
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion()}, Method: domain.MethodGeneration})
	if err != nil {
		t.Fatal(err)
	}
	ans := resp.Answers["queue"]
	if ans.Choice != "" || ans.Probabilities != nil || ans.Confidence != nil || ans.Generation.Valid {
		t.Fatalf("%+v", ans)
	}
}

func TestServiceMissingLogit(t *testing.T) {
	fake := &fakeEngine{emptyTop: true}
	svc := &Service{Engine: fake, Labels: []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}}, Log: logger.New(io.Discard)}
	_, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion()}, Method: domain.MethodDirect})
	if !errors.Is(err, ErrEngine) || !strings.Contains(err.Error(), "missing option logit for A") {
		t.Fatal(err)
	}
}

func TestApplyNoulOmitsConfidence(t *testing.T) {
	var ans domain.Answer
	ans.Type = "noul"
	applyDirect(&ans, domain.Question{Type: "noul", Criteria: []domain.Criterion{{Key: "true", Text: "true"}, {Key: "false", Text: "false"}}}, []float64{0.95, 0.05})
	if ans.Noul == nil || *ans.Noul != 0.95 || ans.Confidence != nil || ans.Probabilities != nil || ans.Choice != "" {
		t.Fatalf("%+v", ans)
	}
	raw, err := json.Marshal(ans)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "confidence") || strings.Contains(string(raw), "probabilities") || strings.Contains(string(raw), "choice") {
		t.Fatalf("%s", raw)
	}
}

func mustState(t *testing.T, raw string) domain.State {
	t.Helper()
	req, err := domain.Validate([]byte(`{"state":`+raw+`,"questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`), "m")
	if err != nil {
		t.Fatal(err)
	}
	return req.State
}
