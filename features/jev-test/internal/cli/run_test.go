package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openjev/features/jev-test/internal/logger"
)

func TestRunSuccessText(t *testing.T) {
	var hits int
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"is_urgent":{"type":"noul","noul":0.95}},"elapsedMs":1200}`))
	}))
	t.Cleanup(srv.Close)
	input, key := writeFixtures(t, "k\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--input", input, "--key-file", key, "--url", srv.URL, "--timeout", "5s"}, &stdout, &stderr, logger.New(io.Discard))
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	if hits != 1 {
		t.Fatalf("hits %d", hits)
	}
	if sent["model"] != "typesafe/jev-1.13" {
		t.Fatalf("model %#v", sent["model"])
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("lines %#v", lines)
	}
	if lines[0] != "status: 200" || !strings.HasPrefix(lines[1], "wall_ms: ") || lines[2] != "server_elapsed_ms: 1200" || lines[3] != "model: jev-1.13.0" || lines[4] != "questions: 1" || lines[5] != "content_ok: true" || lines[6] != "is_urgent type=noul noul=0.95" {
		t.Fatalf("lines %#v", lines)
	}
}

func TestRunSuccessJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","elapsedMs":1200,"usage":{"input_tokens":3,"output_tokens":1},"answers":{"is_urgent":{"type":"noul","noul":0.95}}}`))
	}))
	t.Cleanup(srv.Close)
	input, key := writeFixtures(t, "k\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--input", input, "--key-file", key, "--url", srv.URL, "--timeout", "5s", "--json"}, &stdout, &stderr, logger.New(io.Discard))
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout %s: %v", stdout.String(), err)
	}
	if doc["wall_ms"].(float64) <= 0 || doc["questions"].(float64) != 1 || doc["content_ok"] != true || doc["status"].(float64) != 200 || doc["server_elapsed_ms"].(float64) != 1200 {
		t.Fatalf("doc %#v", doc)
	}
	usage := doc["usage"].(map[string]any)
	if usage["input_tokens"].(float64) != 3 {
		t.Fatalf("usage %#v", usage)
	}
	answers := doc["answers"].(map[string]any)
	urgent := answers["is_urgent"].(map[string]any)
	if urgent["noul"].(float64) != 0.95 {
		t.Fatalf("answer %#v", urgent)
	}
	if strings.Contains(stdout.String(), "Bearer") {
		t.Fatal("token leaked")
	}
}

func TestRunHTTPError(t *testing.T) {
	const token = "sekret-token-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)
	input, key := writeFixtures(t, token+"\n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--input", input, "--key-file", key, "--url", srv.URL, "--timeout", "5s"}, &stdout, &stderr, logger.New(io.Discard))
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stderr.String(), "401") || !strings.Contains(stderr.String(), "denied") {
		t.Fatalf("stderr %s", stderr.String())
	}
	if strings.Contains(stdout.String(), token) || strings.Contains(stderr.String(), token) {
		t.Fatal("token leaked")
	}
	if strings.Contains(stdout.String(), "type=noul") {
		t.Fatalf("stdout %s", stdout.String())
	}
}

func TestRunEmptyKey(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	input, key := writeFixtures(t, " \n")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--input", input, "--key-file", key, "--url", srv.URL, "--timeout", "5s"}, &stdout, &stderr, logger.New(io.Discard))
	if code != 2 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	if hits != 0 {
		t.Fatalf("hits %d", hits)
	}
}

func TestRunBadInput(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	input := filepath.Join(dir, "in.json")
	if err := os.WriteFile(input, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(key, []byte("k\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := Run([]string{"--input", input, "--key-file", key, "--url", srv.URL, "--timeout", "5s"}, io.Discard, &stderr, logger.New(io.Discard))
	if code != 2 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	if hits != 0 {
		t.Fatalf("hits %d", hits)
	}
}

func writeFixtures(t *testing.T, keyBody string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "in.json")
	body := `{"state":"hello","questions":{"is_urgent":{"type":"noul","instructions":"now?"}}}`
	if err := os.WriteFile(input, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(key, []byte(keyBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return input, key
}
