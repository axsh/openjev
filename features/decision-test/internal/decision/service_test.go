package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

// syncBuffer is a log sink that workers may still write to after Run returns.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// fakeEngine records calls per question so tests can assert per-question
// ordering while questions of one request run in parallel.
type fakeEngine struct {
	mu        sync.Mutex
	calls     []string
	text      string
	emptyTop  bool
	blockOn   string
	unblocked chan struct{}
	once      sync.Once
	delay     time.Duration
}

// questionOf extracts the instructions text from the user message built by prompt.Messages.
func questionOf(messages []prompt.Message) string {
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		_, after, ok := strings.Cut(message.Content, "Question:\n")
		if !ok {
			return ""
		}
		before, _, _ := strings.Cut(after, "\n\n")
		return before
	}
	return ""
}

func (f *fakeEngine) record(kind string, messages []prompt.Message) string {
	q := questionOf(messages)
	f.mu.Lock()
	f.calls = append(f.calls, kind+":"+q)
	f.mu.Unlock()
	return q
}

func (f *fakeEngine) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeEngine) ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []engine.Label) (engine.ReadResult, error) {
	q := f.record("read", messages)
	if f.blockOn != "" && q == f.blockOn {
		<-ctx.Done()
		f.once.Do(func() { close(f.unblocked) })
		return engine.ReadResult{}, ctx.Err()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
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
	f.record("generate", messages)
	return engine.GenRaw{Text: f.text, TTFTMs: 1, TotalMs: 2310.7, PromptTokens: 12, CompletionTokens: 61}, nil
}

func (f *fakeEngine) Health(ctx context.Context) (bool, error) { return true, nil }

func testQuestion() domain.Question {
	return questionWithID("queue", "Which queue?")
}

func questionWithID(id, text string) domain.Question {
	return domain.Question{
		ID:           id,
		Type:         "choice",
		Instructions: domain.TextOf(text),
		Criteria: []domain.Criterion{
			{Key: "account_access", Text: "account_access: Account access support"},
			{Key: "billing", Text: "billing: Billing support"},
		},
	}
}

func newTestService(t *testing.T, eng engine.Engine, workers int, stall time.Duration, logs io.Writer) *Service {
	t.Helper()
	svc := &Service{
		Engine:       eng,
		Labels:       []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}},
		Log:          logger.New(logs),
		StallTimeout: stall,
	}
	svc.StartPool(t.Context(), workers)
	return svc
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

func TestServiceDirect(t *testing.T) {
	fake := &fakeEngine{}
	svc := newTestService(t, fake, 2, time.Second, io.Discard)
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion()}, Method: domain.MethodDirect})
	if err != nil {
		t.Fatal(err)
	}
	calls := fake.snapshot()
	if len(calls) != 1 || calls[0] != "read:Which queue?" {
		t.Fatalf("calls %v", calls)
	}
	ans := resp.Answers["queue"]
	if ans.Generation != nil || ans.Choice != "account_access" || ans.Timings.DirectMs <= 0 || resp.Usage.OutputTokens != 1 {
		t.Fatalf("%+v usage %+v", ans, resp.Usage)
	}
}

func TestServiceBothOrder(t *testing.T) {
	fake := &fakeEngine{text: `{"A: account_access: Account access support": 0.2, "B: billing: Billing support": 0.8}`}
	svc := newTestService(t, fake, 2, time.Second, io.Discard)
	questions := []domain.Question{testQuestion(), questionWithID("next", "Which next?")}
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: questions, Method: domain.MethodBoth})
	if err != nil {
		t.Fatal(err)
	}
	calls := fake.snapshot()
	if len(calls) != 4 {
		t.Fatalf("calls %v", calls)
	}
	for _, text := range []string{"Which queue?", "Which next?"} {
		read, generate := indexOf(calls, "read:"+text), indexOf(calls, "generate:"+text)
		if read < 0 || generate < 0 || read > generate {
			t.Fatalf("order for %q in %v", text, calls)
		}
	}
	for _, id := range []string{"queue", "next"} {
		ans, ok := resp.Answers[id]
		if !ok {
			t.Fatalf("missing answer %s", id)
		}
		if ans.Choice != "account_access" || ans.Generation.Choice != "billing" {
			t.Fatalf("%s direct %s gen %s", id, ans.Choice, ans.Generation.Choice)
		}
		if ans.Timings.Ratio == nil || *ans.Timings.Ratio != ans.Timings.GenerationMs/ans.Timings.DirectMs {
			t.Fatalf("%s ratio %+v", id, ans.Timings)
		}
	}
}

func TestServiceGenerationInvalid(t *testing.T) {
	fake := &fakeEngine{text: "not json"}
	svc := newTestService(t, fake, 1, time.Second, io.Discard)
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
	svc := newTestService(t, fake, 2, time.Second, io.Discard)
	questions := []domain.Question{testQuestion(), questionWithID("next", "Which next?")}
	_, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: questions, Method: domain.MethodDirect})
	if !errors.Is(err, ErrEngine) || !strings.Contains(err.Error(), "missing option logit for A") {
		t.Fatal(err)
	}
}

func TestServiceStallReturnsPartial(t *testing.T) {
	fake := &fakeEngine{blockOn: "Block me", unblocked: make(chan struct{})}
	logs := &syncBuffer{}
	svc := newTestService(t, fake, 3, 200*time.Millisecond, logs)
	questions := []domain.Question{questionWithID("a", "Which a?"), questionWithID("b", "Block me"), questionWithID("c", "Which c?")}
	start := time.Now()
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: questions, Method: domain.MethodDirect})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("returned after %s, before the stall timeout", elapsed)
	}
	if len(resp.Answers) != 2 || resp.Answers["a"].Choice != "account_access" || resp.Answers["c"].Choice != "account_access" {
		t.Fatalf("answers %+v", resp.Answers)
	}
	if _, ok := resp.Answers["b"]; ok {
		t.Fatal("blocked question answered")
	}
	if resp.Usage.OutputTokens != 2 {
		t.Fatalf("usage %+v", resp.Usage)
	}
	text := logs.String()
	for _, want := range []string{"systemone stalled", "questions=3", "submitted=3", "received=2", "missing_ids=b", "stall_timeout_ms=200", "level=WARN"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %q:\n%s", want, text)
		}
	}
	select {
	case <-fake.unblocked:
	case <-time.After(time.Second):
		t.Fatal("blocked task was not cancelled after Run returned")
	}
}

func TestServiceProgressResetsTimer(t *testing.T) {
	fake := &fakeEngine{delay: 150 * time.Millisecond}
	logs := &syncBuffer{}
	svc := newTestService(t, fake, 1, 200*time.Millisecond, logs)
	questions := make([]domain.Question, 5)
	for i := range questions {
		questions[i] = questionWithID("q"+string(rune('1'+i)), "Which q"+string(rune('1'+i))+"?")
	}
	start := time.Now()
	resp, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: questions, Method: domain.MethodDirect})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Answers) != 5 {
		t.Fatalf("answers %d", len(resp.Answers))
	}
	if elapsed < 750*time.Millisecond {
		t.Fatalf("elapsed %s", elapsed)
	}
	text := logs.String()
	if strings.Contains(text, "systemone stalled") {
		t.Fatalf("stalled although progress continued:\n%s", text)
	}
	if !strings.Contains(text, "systemone completed") || !strings.Contains(text, "answered=5") || !strings.Contains(text, "missing=0") {
		t.Fatalf("completion log:\n%s", text)
	}
}

func TestServiceWithoutPool(t *testing.T) {
	svc := &Service{Engine: &fakeEngine{}, Labels: []engine.Label{{Letter: "A", TokenID: 1}, {Letter: "B", TokenID: 2}}, Log: logger.New(io.Discard)}
	_, err := svc.Run(context.Background(), domain.Request{Model: "m", State: mustState(t, `"hello"`), Questions: []domain.Question{testQuestion()}, Method: domain.MethodDirect})
	if err == nil || !strings.Contains(err.Error(), "worker pool is not started") {
		t.Fatalf("error %v", err)
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
