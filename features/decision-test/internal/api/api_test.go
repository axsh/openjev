package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"openjev/features/decision-test/internal/decision"
	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

type countingEngine struct {
	calls int
	miss  bool
}

func (e *countingEngine) ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []engine.Label) (engine.ReadResult, error) {
	e.calls++
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
	e.calls++
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

func newService(eng *countingEngine) *decision.Service {
	return &decision.Service{
		Engine:  eng,
		Labels:  labels(),
		Log:     logger.New(io.Discard),
		ModelID: "minicpm5-2b-q4_k_m",
	}
}

func TestDirectAccount(t *testing.T) {
	eng := &countingEngine{}
	_, api := humatest.New(t)
	Register(api, newService(eng), func(context.Context) domain.Health {
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
		`{"state":"hello","questions":{"q":{"type":"score","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`,
		`{"state":"","questions":{"q":{"type":"choice","instructions":"Pick","criteria":{"a":"A","b":"B"}}}}`,
	}
	for _, body := range cases {
		eng := &countingEngine{}
		_, api := humatest.New(t)
		Register(api, newService(eng), func(context.Context) domain.Health {
			return domain.Health{Ready: true}
		})
		resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(body))
		if resp.Code != 422 {
			t.Fatalf("status %d body %s for %s", resp.Code, resp.Body.String(), body)
		}
		if eng.calls != 0 {
			t.Fatalf("engine calls %d", eng.calls)
		}
	}
}

func TestMissingLogit502(t *testing.T) {
	eng := &countingEngine{miss: true}
	_, api := humatest.New(t)
	Register(api, newService(eng), func(context.Context) domain.Health {
		return domain.Health{Ready: true}
	})
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	resp := api.Post("/v1/systemone", "Content-Type: application/json", strings.NewReader(string(raw)))
	if resp.Code != 502 || !strings.Contains(resp.Body.String(), "missing option logit for A") || !strings.Contains(resp.Body.String(), "detail") {
		t.Fatalf("status %d body %s", resp.Code, resp.Body.String())
	}
}

func TestHealthStatus(t *testing.T) {
	_, api := humatest.New(t)
	ready := true
	Register(api, newService(&countingEngine{}), func(context.Context) domain.Health {
		return domain.Health{Ready: ready, Model: "minicpm5-2b-q4_k_m"}
	})
	ok := api.Get("/health")
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), `"ready":true`) {
		t.Fatalf("ready status %d body %s", ok.Code, ok.Body.String())
	}
	ready = false
	down := api.Get("/health")
	if down.Code != 503 || !strings.Contains(down.Body.String(), `"ready":false`) {
		t.Fatalf("down status %d body %s", down.Code, down.Body.String())
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
