package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"openjev/features/decision-test/internal/decision"
	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

type countingEngine struct {
	calls atomic.Int32
	miss  bool
}

func (e *countingEngine) ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []engine.Label) (engine.ReadResult, error) {
	e.calls.Add(1)
	if e.miss {
		return engine.ReadResult{CompletionTokens: 1, ElapsedMs: 10}, nil
	}
	top := make([]engine.TokenLogprob, len(labels))
	for i, label := range labels {
		top[i] = engine.TokenLogprob{ID: label.TokenID, Logprob: float64(len(labels) - i)}
	}
	return engine.ReadResult{Top: top, PromptTokens: 4, CompletionTokens: 1, ElapsedMs: 10}, nil
}

func (e *countingEngine) Generate(ctx context.Context, messages []prompt.Message) (engine.GenRaw, error) {
	e.calls.Add(1)
	return engine.GenRaw{}, nil
}

func (e *countingEngine) Health(ctx context.Context) (bool, error) { return true, nil }

func labels() []engine.Label {
	out := make([]engine.Label, 20)
	for i := range out {
		out[i] = engine.Label{Letter: string(rune('A' + i)), TokenID: 50 + i}
	}
	return out
}

func newService(t *testing.T, eng *countingEngine) *decision.Service {
	t.Helper()
	svc := &decision.Service{
		Engine:       eng,
		Labels:       labels(),
		Log:          logger.New(io.Discard),
		ModelID:      "minicpm5-2b-q4_k_m",
		StallTimeout: time.Second,
	}
	svc.StartPool(t.Context(), 4)
	return svc
}

func TestDirectAccount(t *testing.T) {
	eng := &countingEngine{}
	_, api := humatest.New(t)
	Register(api, newService(t, eng), func(context.Context) domain.Health {
		return domain.Health{Ready: true, Model: "minicpm5-2b-q4_k_m"}
	})
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(raw)))
	if resp.Code != 200 {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
	var body struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var ans map[string]any
	if err := json.Unmarshal(body.Answers["queue"], &ans); err != nil {
		t.Fatal(err)
	}
	if _, ok := ans["generation"]; ok {
		t.Fatal("generation key present")
	}
	if ans["choice"] != "account_access" {
		t.Fatalf("choice %v", ans["choice"])
	}
	probs := ans["probabilities"].(map[string]any)
	sum := 0.0
	for _, value := range probs {
		sum += value.(float64)
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("sum %v", sum)
	}
	conf := ans["confidence"].(float64)
	if conf < 0 || conf > 1 {
		t.Fatalf("confidence %v", conf)
	}
	spec, err := api.OpenAPI().YAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), "1 - H(p) / ln(n)") {
		t.Fatal("confidence doc missing from OpenAPI")
	}
}

func TestRejectsBeforeEngine(t *testing.T) {
	cases := []string{
		`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"only":"One"}}}}`,
		criteriaN(21),
		`{"state":"hello","questions":{"q":{"type":"rank","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`,
		`{"state":"","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`,
		`{"state":"hello","questions":{"q":{"type":"choice","instructions":123,"criteria":{"a":"A","b":"B"}}}}`,
	}
	for _, body := range cases {
		eng := &countingEngine{}
		_, api := humatest.New(t)
		Register(api, newService(t, eng), func(context.Context) domain.Health {
			return domain.Health{Ready: true}
		})
		resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(body))
		if resp.Code != 422 {
			t.Fatalf("status %d body %s for %s", resp.Code, resp.Body.String(), body)
		}
		if eng.calls.Load() != 0 {
			t.Fatalf("engine calls %d", eng.calls.Load())
		}
	}
}

func TestMissingLogit502(t *testing.T) {
	eng := &countingEngine{miss: true}
	_, api := humatest.New(t)
	Register(api, newService(t, eng), func(context.Context) domain.Health {
		return domain.Health{Ready: true}
	})
	body := `{"state":"hello","questions":{"q1":{"type":"choice","instructions":"Pick one","criteria":{"a":"A","b":"B"}},"q2":{"type":"choice","instructions":"Pick two","criteria":{"a":"A","b":"B"}}}}`
	resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(body))
	if resp.Code != 502 || !strings.Contains(resp.Body.String(), "missing option logit for A") || !strings.Contains(resp.Body.String(), "detail") {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
}

func TestHealthStatus(t *testing.T) {
	_, api := humatest.New(t)
	ready := true
	Register(api, newService(t, &countingEngine{}), func(context.Context) domain.Health {
		return domain.Health{Ready: ready, Model: "minicpm5-2b-q4_k_m", Workers: 16, QueueDepth: 0}
	})
	ok := api.Get("/health")
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), `"ready":true`) {
		t.Fatalf("ready status %d body %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), `"workers":16`) || !strings.Contains(ok.Body.String(), `"queue_depth":0`) {
		t.Fatalf("pool fields missing: %s", ok.Body.String())
	}
	ready = false
	down := api.Get("/health")
	if down.Code != 503 || !strings.Contains(down.Body.String(), `"ready":false`) {
		t.Fatalf("down status %d body %s", down.Code, down.Body.String())
	}
}

func TestPlaygroundFixture(t *testing.T) {
	eng := &countingEngine{}
	_, api := humatest.New(t)
	Register(api, newService(t, eng), func(context.Context) domain.Health { return domain.Health{Ready: true} })
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "playground.json"))
	if err != nil {
		t.Fatal(err)
	}
	resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(raw)))
	if resp.Code != 200 {
		t.Fatalf("status %d %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	answers := body["answers"].(map[string]any)
	if len(answers) != 3 {
		t.Fatalf("answers %d", len(answers))
	}
	for id, want := range map[string]string{"department": "choice", "urgency": "score", "wants_refund": "noul"} {
		ans, ok := answers[id].(map[string]any)
		if !ok || ans["type"] != want {
			t.Fatalf("%s = %#v", id, answers[id])
		}
	}
	if eng.calls.Load() != 3 {
		t.Fatalf("engine calls %d", eng.calls.Load())
	}
}

func TestScoreAndNoulDirect(t *testing.T) {
	eng := &countingEngine{}
	_, api := humatest.New(t)
	Register(api, newService(t, eng), func(context.Context) domain.Health { return domain.Health{Ready: true} })
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "score.json"))
	if err != nil {
		t.Fatal(err)
	}
	resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(raw)))
	if resp.Code != 200 {
		t.Fatalf("status %d %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ans := body["answers"].(map[string]any)["frustration"].(map[string]any)
	if _, ok := ans["choice"]; ok {
		t.Fatal("choice present")
	}
	probs := ans["probabilities"].(map[string]any)
	score := ans["score"].(float64)
	sum := 0.0
	weighted := 0.0
	for _, key := range []string{"0", "1", "2"} {
		value := probs[key].(float64)
		sum += value
		index, _ := json.Number(key).Float64()
		weighted += index * value
	}
	if math.Abs(sum-1) > 1e-6 || math.Abs(score-weighted) > 1e-6 {
		t.Fatalf("score %v weighted %v sum %v", score, weighted, sum)
	}
	noulRaw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "noul.json"))
	if err != nil {
		t.Fatal(err)
	}
	nresp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(noulRaw)))
	if nresp.Code != 200 || strings.Contains(nresp.Body.String(), "confidence") || strings.Contains(nresp.Body.String(), `"choice"`) {
		t.Fatalf("noul %d %s", nresp.Code, nresp.Body.String())
	}
}

func TestRejectFixtures(t *testing.T) {
	names := []string{"score-one.json", "score-eleven.json", "score-object.json", "score-empty.json", "noul-true-only.json", "noul-extra-key.json", "type-rank.json"}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		eng := &countingEngine{}
		_, api := humatest.New(t)
		Register(api, newService(t, eng), func(context.Context) domain.Health { return domain.Health{Ready: true} })
		resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(raw)))
		if resp.Code != 422 || eng.calls.Load() != 0 {
			t.Fatalf("%s status %d calls %d", name, resp.Code, eng.calls.Load())
		}
	}
}

func criteriaN(n int) string {
	var b strings.Builder
	b.WriteString(`{"state":"hello","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		key := string(rune('a' + i))
		if i >= 26 {
			key = "k" + string(rune('a'+(i-26)))
		}
		b.WriteString(`"` + key + `":"` + key + `"`)
	}
	b.WriteString("}}}}")
	return b.String()
}
