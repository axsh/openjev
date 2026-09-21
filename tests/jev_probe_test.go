//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func jevProbeBin(t *testing.T, root string) string {
	t.Helper()
	name := "jev-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(root, "bin", name)
	requireFile(t, bin)
	return bin
}

func TestJevProbe_BinaryPostsBankOnce(t *testing.T) {
	root := repoRoot(t)
	bin := jevProbeBin(t, root)
	var hits int
	var auth string
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		auth = r.Header.Get("Authorization")
		body, _ := readAll(r)
		_ = json.Unmarshal(body, &sent)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(validJevResponse(body))
	}))
	t.Cleanup(srv.Close)
	key := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(key, []byte("probe-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--url", srv.URL, "--key-file", key, "--json", "--timeout", "30s")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("exit %v stderr %s stdout %s", err, stderr.String(), stdout.String())
	}
	if hits != 1 {
		t.Fatalf("hits %d", hits)
	}
	if auth != "Bearer probe-token" {
		t.Fatalf("auth %q", auth)
	}
	qs, _ := sent["questions"].(map[string]any)
	if len(qs) != 30 || sent["model"] != "jev-latest" {
		t.Fatalf("sent questions %d model %#v", len(qs), sent["model"])
	}
	bank, err := os.ReadFile(filepath.Join(root, "features", "decision-test", "testdata", "bank.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]json.RawMessage
	if err := json.Unmarshal(bank, &onDisk); err != nil {
		t.Fatal(err)
	}
	if _, ok := onDisk["model"]; ok {
		t.Fatal("model key written to bank.json")
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout %s stderr %s: %v", stdout.String(), stderr.String(), err)
	}
	if doc["status"].(float64) != 200 || doc["questions"].(float64) != 30 || doc["content_ok"] != true || doc["wall_ms"].(float64) <= 0 {
		t.Fatalf("doc %#v", doc)
	}
	if strings.Contains(stdout.String()+stderr.String(), "probe-token") {
		t.Fatal("token leaked")
	}
}

func TestJevProbe_BinaryRejectsUnauthorized(t *testing.T) {
	root := repoRoot(t)
	bin := jevProbeBin(t, root)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("denied"))
	}))
	t.Cleanup(srv.Close)
	key := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(key, []byte("probe-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--url", srv.URL, "--key-file", key, "--timeout", "30s")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 {
		t.Fatalf("exit %v", err)
	}
	text := stdout.String() + stderr.String()
	if strings.Contains(text, "probe-token") {
		t.Fatal("token leaked")
	}
	if !strings.Contains(stderr.String(), "401") || !strings.Contains(stderr.String(), "denied") {
		t.Fatalf("stderr %s", stderr.String())
	}
}

func TestJevProbe_BinaryUsage(t *testing.T) {
	root := repoRoot(t)
	bin := jevProbeBin(t, root)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	key := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(key, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--url", srv.URL, "--key-file", key, "--timeout", "30s")
	cmd.Dir = root
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("exit %v", err)
	}
	if hits != 0 {
		t.Fatalf("hits %d", hits)
	}
}

func readAll(r *http.Request) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func validJevResponse(raw []byte) []byte {
	var doc struct {
		Questions map[string]struct {
			Type     string          `json:"type"`
			Criteria json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(raw, &doc)
	answers := map[string]any{}
	for id, q := range doc.Questions {
		switch q.Type {
		case "choice":
			var criteria map[string]json.RawMessage
			_ = json.Unmarshal(q.Criteria, &criteria)
			keys := make([]string, 0, len(criteria))
			for key := range criteria {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			probs := map[string]float64{}
			for i, key := range keys {
				if i == 0 {
					probs[key] = 1
				} else {
					probs[key] = 0
				}
			}
			choice := ""
			if len(keys) > 0 {
				choice = keys[0]
			}
			answers[id] = map[string]any{
				"type":          "choice",
				"choice":        choice,
				"confidence":    1,
				"probabilities": probs,
			}
		case "score":
			var levels []json.RawMessage
			_ = json.Unmarshal(q.Criteria, &levels)
			probs := map[string]float64{}
			for i := range levels {
				if i == 0 {
					probs[strconv.Itoa(i)] = 1
				} else {
					probs[strconv.Itoa(i)] = 0
				}
			}
			answers[id] = map[string]any{
				"type":          "score",
				"score":         0,
				"confidence":    1,
				"probabilities": probs,
				"legend":        map[string]any{},
			}
		default:
			answers[id] = map[string]any{"type": "noul", "noul": 0.5}
		}
	}
	out := map[string]any{
		"model":     "jev-1.13.0",
		"elapsedMs": 10,
		"usage":     map[string]int{"input_tokens": 1, "output_tokens": 1},
		"answers":   answers,
	}
	encoded, _ := json.Marshal(out)
	return encoded
}
