package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideJSON(t *testing.T) {
	const body = `{"model":"m","answers":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"x","questions":{},"options":{"method":"direct"}}`)
	var stdout, stderr bytes.Buffer
	err := Decide(context.Background(), Options{Input: path, Server: srv.URL, JSON: true}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != body {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestDecideOverridesMethod(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"answers":{}}`)
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"hello","questions":{"queue":{"type":"choice","instructions":"Which?","criteria":{"a":"A","b":"B"}}},"options":{"method":"direct"}}`)
	var stdout, stderr bytes.Buffer
	if err := Decide(context.Background(), Options{Input: path, Server: srv.URL, Method: "both", JSON: true}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	options := got["options"].(map[string]any)
	if options["method"] != "both" {
		t.Fatalf("method %v", options["method"])
	}
}

func TestDecideHuman(t *testing.T) {
	const body = `{"answers":{"queue":{"choice":"account_access","confidence":0.71,"probabilities":{"account_access":0.91,"billing":0.03,"close":0.06},"timings":{"direct_ms":95.2,"generation_ms":2310.7,"ratio":24.27}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"hello","questions":{"queue":{"type":"choice","instructions":"Which?","criteria":{"account_access":"A","billing":"B","close":"C"}}}}`)
	var stdout, stderr bytes.Buffer
	if err := Decide(context.Background(), Options{Input: path, Server: srv.URL}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	for _, part := range []string{"account_access", "billing", "close", "0.910", "0.030", "0.060", "direct_ms", "generation_ms", "ratio"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %s in %s", part, text)
		}
	}
}

func TestDecideStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()
	path := writeInput(t, `{"state":"x"}`)
	var stdout, stderr bytes.Buffer
	err := Decide(context.Background(), Options{Input: path, Server: srv.URL, JSON: true}, &stdout, &stderr)
	if err == nil || !strings.Contains(stderr.String(), "status") {
		t.Fatalf("err %v stderr %s", err, stderr.String())
	}
}

func writeInput(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
