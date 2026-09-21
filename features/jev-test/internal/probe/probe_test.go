package probe

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadBankInjectsModel(t *testing.T) {
	path := filepath.Join("..", "..", "..", "decision-test", "testdata", "bank.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path, "jev-latest")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("input file changed")
	}
	if strings.Contains(string(after), `"model"`) {
		t.Fatal("model key written to input")
	}
	if len(loaded.Questions) != 30 {
		t.Fatalf("questions %d", len(loaded.Questions))
	}
	if loaded.Questions[0].ID != "queue" || loaded.Questions[0].Type != "choice" || len(loaded.Questions[0].ChoiceKeys) == 0 {
		t.Fatalf("first %#v", loaded.Questions[0])
	}
	var doc map[string]any
	if err := json.Unmarshal(loaded.Body, &doc); err != nil {
		t.Fatal(err)
	}
	qs, _ := doc["questions"].(map[string]any)
	if len(qs) != 30 {
		t.Fatalf("sent questions %d", len(qs))
	}
	if doc["model"] != "jev-latest" {
		t.Fatalf("model %#v", doc["model"])
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "no state", body: `{}`, want: "state"},
		{name: "no questions", body: `{"state":"x"}`, want: "questions"},
		{name: "empty questions", body: `{"state":"x","questions":{}}`, want: "questions"},
		{name: "empty state", body: `{"state":"","questions":{"q":{"type":"noul","instructions":"y"}}}`, want: "state"},
		{name: "broken", body: `{`, want: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "in.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path, "jev-latest"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestReadKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(path, []byte("  tok en \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok en" {
		t.Fatalf("token %q", got)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKey(empty); err == nil {
		t.Fatal("expected empty key error")
	}
}

func TestPostOnceMeasuresWall(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer probe-token" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"a":1}` {
			t.Errorf("body %s", body)
		}
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	res, err := Post(t.Context(), srv.URL, "probe-token", []byte(`{"a":1}`), 5*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("hits %d", hits)
	}
	if res.Status != 200 || string(res.Body) != `{"ok":true}` {
		t.Fatalf("status %d body %s", res.Status, res.Body)
	}
	if res.WallMs < 50 {
		t.Fatalf("wall_ms %v", res.WallMs)
	}
}

func TestPostKeepsTokenOutOfError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)
	res, err := Post(t.Context(), srv.URL, "sekret-token-value", []byte(`{}`), 5*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 401 || string(res.Body) != "denied" {
		t.Fatalf("status %d body %s", res.Status, res.Body)
	}
}
