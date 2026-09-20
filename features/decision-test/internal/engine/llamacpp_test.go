package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"openjev/features/decision-test/internal/prompt"
)

func TestDirectRequestShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		_, _ = io.WriteString(w, `{"choices":[{"logprobs":{"content":[{"top_logprobs":[{"id":54,"logprob":-0.1},{"id":55,"logprob":-1},{"token":"x","logprob":-2}]}]}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	client := NewClient(srv.URL, nil, 1)
	result, err := client.ReadLabelLogprobs(context.Background(), []prompt.Message{{Role: "user", Content: "x"}}, []Label{{Letter: "A", TokenID: 54}, {Letter: "B", TokenID: 55}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Top) != 2 || result.Top[0].ID != 54 {
		t.Fatalf("top %+v", result.Top)
	}
	assertFloat(t, got, "max_tokens", 1)
	assertFloat(t, got, "temperature", 1)
	assertFloat(t, got, "top_k", 0)
	assertFloat(t, got, "top_p", 1)
	assertFloat(t, got, "top_logprobs", 64)
	if got["logprobs"] != true || got["cache_prompt"] != false {
		t.Fatalf("flags %+v", got)
	}
	if got["grammar"] != `root ::= "A" | "B"` {
		t.Fatalf("grammar %v", got["grammar"])
	}
	bias := got["logit_bias"].(map[string]any)
	if bias["54"] != float64(100) || bias["55"] != float64(100) {
		t.Fatalf("bias %+v", bias)
	}
	kwargs := got["chat_template_kwargs"].(map[string]any)
	if kwargs["enable_thinking"] != false {
		t.Fatalf("thinking %+v", kwargs)
	}
}

func TestTopLogprobsScales(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = io.WriteString(w, `{"choices":[{"logprobs":{"content":[{"top_logprobs":[]}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	labels := make([]Label, 20)
	for i := range labels {
		labels[i] = Label{Letter: string(rune('A' + i)), TokenID: 100 + i}
	}
	if _, err := NewClient(srv.URL, nil, 1).ReadLabelLogprobs(context.Background(), nil, labels); err != nil {
		t.Fatal(err)
	}
	assertFloat(t, got, "top_logprobs", 80)
}

func TestGenerateSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":""}}]}`)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"{"}}]}`)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"\"A: x\": 1}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
		_, _ = io.WriteString(w, "data: [DONE]\n")
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL, nil, 1).Generate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != `{"A: x": 1}` || got.CompletionTokens != 4 || got.TTFTMs <= 0 {
		t.Fatalf("%+v", got)
	}
}

func TestGenerateGrammar(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"{"}}]}`)
		_, _ = io.WriteString(w, "data: [DONE]\n")
	}))
	defer srv.Close()
	_, err := NewClient(srv.URL, nil, 1).Generate(context.Background(), []prompt.Message{{
		Role:    "user",
		Content: "Allowed options:\nA. north: Route north\nB. south: Route south\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, _ := got["grammar"].(string)
	if !strings.Contains(grammar, `A: north: Route north`) || !strings.Contains(grammar, `B: south: Route south`) {
		t.Fatalf("grammar %s", grammar)
	}
}

func TestResolveLabels(t *testing.T) {
	t.Run("two tokens", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"tokens":[{"id":1},{"id":2}]}`)
		}))
		defer srv.Close()
		if _, err := ResolveLabels(context.Background(), NewClient(srv.URL, nil, 1)); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("single", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req tokenizeRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			id := int(req.Content[0])
			_, _ = io.WriteString(w, `{"tokens":[{"id":`+strconvItoa(id)+`}]}`)
		}))
		defer srv.Close()
		labels, err := ResolveLabels(context.Background(), NewClient(srv.URL, nil, 1))
		if err != nil {
			t.Fatal(err)
		}
		if len(labels) != 20 || labels[0].Letter != "A" || labels[0].TokenID != int('A') {
			t.Fatalf("%+v", labels[0])
		}
	})
}

// TestConcurrentReadsNotSerialized fixes that the client no longer holds a
// semaphore: four concurrent reads overlap inside llama, and health is answered
// while they are in flight.
func TestConcurrentReadsNotSerialized(t *testing.T) {
	const parallel = 4
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = io.WriteString(w, `{"status":"ok"}`)
			return
		}
		n := inFlight.Add(1)
		for {
			old := maxSeen.Load()
			if n <= old || maxSeen.CompareAndSwap(old, n) {
				break
			}
		}
		// Hold the request until every caller has entered (maxSeen is
		// monotonic, so early leavers cannot stall late arrivals), bounded so
		// a regression fails instead of hanging.
		deadline := time.Now().Add(2 * time.Second)
		for maxSeen.Load() < parallel && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		inFlight.Add(-1)
		_, _ = io.WriteString(w, `{"choices":[{"logprobs":{"content":[{"top_logprobs":[{"id":1,"logprob":0}]}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	client := NewClient(srv.URL, nil, parallel)
	labels := []Label{{Letter: "A", TokenID: 1}}
	var wg sync.WaitGroup
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.ReadLabelLogprobs(context.Background(), nil, labels); err != nil {
				t.Error(err)
			}
		}()
	}
	healthStart := time.Now()
	ok, err := client.Health(context.Background())
	if err != nil || !ok || time.Since(healthStart) > 100*time.Millisecond {
		t.Fatalf("health %v %v elapsed %s", ok, err, time.Since(healthStart))
	}
	wg.Wait()
	if maxSeen.Load() != parallel {
		t.Fatalf("max concurrent %d, want %d", maxSeen.Load(), parallel)
	}
}

func TestClientIdleConns(t *testing.T) {
	seven := NewClient("http://127.0.0.1:1", nil, 7).http.Transport.(*http.Transport)
	if seven.MaxIdleConnsPerHost != 7 || seven.MaxIdleConns < 7 {
		t.Fatalf("idle conns %d / %d", seven.MaxIdleConnsPerHost, seven.MaxIdleConns)
	}
	zero := NewClient("http://127.0.0.1:1", nil, 0).http.Transport.(*http.Transport)
	if zero.MaxIdleConnsPerHost != 1 {
		t.Fatalf("idle conns %d", zero.MaxIdleConnsPerHost)
	}
}

func assertFloat(t *testing.T, got map[string]any, key string, want float64) {
	t.Helper()
	value, ok := got[key].(float64)
	if !ok || value != want {
		t.Fatalf("%s = %v, want %v", key, got[key], want)
	}
}

func strconvItoa(n int) string {
	return jsonNumber(n)
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
