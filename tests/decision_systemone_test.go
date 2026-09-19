//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDecisionSystemOne_Health(t *testing.T) {
	root := repoRoot(t)
	requireFile(t, filepath.Join(root, "models", "MiniCPM5-2B-Q4_K_M.gguf"))
	requireFile(t, filepath.Join(root, "third_party", "llama.cpp", "b11056", "llama-server.exe"))
	if err := getOK(llamaHealth(t, root)); err != nil {
		t.Fatal(err)
	}
	base := startServer(t, root, 18191, "")
	body := getJSON(t, base+"/health")
	if body["ready"] != true {
		t.Fatalf("ready %#v", body["ready"])
	}
	if body["model"] != "minicpm5-2b-q4_k_m" {
		t.Fatalf("model %#v", body["model"])
	}
	ids := body["label_token_ids"].(map[string]any)
	if len(ids) != 20 {
		t.Fatalf("labels %d", len(ids))
	}
	for letter := 'A'; letter <= 'T'; letter++ {
		value, ok := ids[string(letter)].(float64)
		if !ok || value <= 0 || value != math.Trunc(value) {
			t.Fatalf("label %c = %#v", letter, ids[string(letter)])
		}
	}
}

func TestDecisionSystemOne_DirectAccount(t *testing.T) {
	root := repoRoot(t)
	if err := getOK(llamaHealth(t, root)); err != nil {
		t.Fatal(err)
	}
	base := startServer(t, root, 18192, "")
	raw := readRepo(t, root, filepath.Join("features", "decision-test", "testdata", "account.json"))
	body := postJSON(t, base+"/v1/systemone", withMethod(t, raw, "direct"))
	queue := answer(t, body, "queue")
	if queue["type"] != "choice" {
		t.Fatalf("type %#v", queue["type"])
	}
	choice, _ := queue["choice"].(string)
	if choice != "account_access" && choice != "billing" && choice != "close" {
		t.Fatalf("choice %#v", queue["choice"])
	}
	if _, ok := queue["generation"]; ok {
		t.Fatal("generation present")
	}
	probs := floatMap(t, queue["probabilities"])
	sum := 0.0
	for _, key := range []string{"account_access", "billing", "close"} {
		value, ok := probs[key]
		if !ok || value < 0 || value > 1 {
			t.Fatalf("prob %s %#v", key, probs[key])
		}
		sum += value
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("sum %v", sum)
	}
	conf := queue["confidence"].(float64)
	if conf < 0 || conf > 1 {
		t.Fatalf("confidence %v", conf)
	}
	usage := body["usage"].(map[string]any)
	if usage["output_tokens"] != float64(1) {
		t.Fatalf("output_tokens %#v", usage["output_tokens"])
	}
	timings := queue["timings"].(map[string]any)
	if timings["direct_ms"].(float64) <= 0 {
		t.Fatalf("direct_ms %#v", timings["direct_ms"])
	}
}

func TestDecisionSystemOne_GenerationEmail(t *testing.T) {
	root := repoRoot(t)
	if err := getOK(llamaHealth(t, root)); err != nil {
		t.Fatal(err)
	}
	base := startServer(t, root, 18193, "")
	raw := readRepo(t, root, filepath.Join("features", "decision-test", "testdata", "email.json"))
	body := postJSON(t, base+"/v1/systemone", withMethod(t, raw, "generation"))
	ans := answer(t, body, "classification")
	gen := ans["generation"].(map[string]any)
	if gen["valid"] != true {
		t.Fatalf("generation %#v", gen)
	}
	keys := objectKeys(t, gen["generated_text"].(string))
	want := []string{"A: legitimate: Legitimate", "B: spam: Spam", "C: phishing: Phishing"}
	if strings.Join(keys, "\n") != strings.Join(want, "\n") {
		t.Fatalf("keys %#v", keys)
	}
	probs := floatMap(t, gen["probabilities"])
	sum := 0.0
	best := "legitimate"
	for _, key := range []string{"legitimate", "spam", "phishing"} {
		sum += probs[key]
		if probs[key] > probs[best] {
			best = key
		}
	}
	if math.Abs(sum-1) > 0.02 {
		t.Fatalf("sum %v", sum)
	}
	if ans["choice"] != best {
		t.Fatalf("choice %v best %s", ans["choice"], best)
	}
	ttft := gen["ttft_ms"].(float64)
	total := gen["total_ms"].(float64)
	if ttft <= 0 || total <= 0 || ttft > total {
		t.Fatalf("ttft %v total %v", ttft, total)
	}
}

func TestDecisionSystemOne_BothOrder(t *testing.T) {
	root := repoRoot(t)
	if err := getOK(llamaHealth(t, root)); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "decision.log")
	base := startServer(t, root, 18194, logPath)
	raw := readRepo(t, root, filepath.Join("features", "decision-test", "testdata", "account.json"))
	body := postJSON(t, base+"/v1/systemone", withMethod(t, raw, "both"))
	ans := answer(t, body, "queue")
	if ans["probabilities"] == nil || ans["generation"] == nil {
		t.Fatalf("%#v", ans)
	}
	gen := ans["generation"].(map[string]any)
	if gen["probabilities"] == nil {
		t.Fatal("missing generation probabilities")
	}
	timings := ans["timings"].(map[string]any)
	direct := timings["direct_ms"].(float64)
	generation := timings["generation_ms"].(float64)
	ratio := timings["ratio"].(float64)
	expect := generation / direct
	if expect == 0 || math.Abs(ratio-expect)/math.Abs(expect) > 1e-6 {
		t.Fatalf("ratio %v expect %v", ratio, expect)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(logged)
	directAt := strings.Index(text, "direct completed")
	generationAt := strings.Index(text, "generation started")
	if directAt < 0 || generationAt < 0 || directAt > generationAt {
		t.Fatalf("log order:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "direct completed") || strings.Contains(line, "generation started") {
			if !strings.Contains(line, "level=DEBUG") || !strings.Contains(line, "component=decision") || !strings.Contains(line, "question_id=queue") {
				t.Fatalf("log line %s", line)
			}
		}
		if strings.Contains(line, "level=ERROR") {
			t.Fatalf("error log %s", line)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("missing %s: %v", path, err)
	}
}

func llamaHealth(t *testing.T, root string) string {
	t.Helper()
	raw := readRepo(t, root, filepath.Join("settings", "decision-test.yaml"))
	url := yamlValue(string(raw), "llama_url")
	if url == "" {
		t.Fatal("llama_url missing")
	}
	return strings.TrimRight(url, "/") + "/health"
}

func startServer(t *testing.T, root string, port int, logPath string) string {
	t.Helper()
	bin, err := binaryPath(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, root, port, logPath)
	cmd := exec.Command(bin, "--config", cfg)
	cmd.Dir = root
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(4 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		last = string(body)
		if resp.StatusCode == http.StatusOK && strings.Contains(last, `"ready":true`) {
			return base
		}
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Fatalf("server not ready: %s\n%s", last, logs.String())
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("server did not become ready: %s\n%s", last, logs.String())
	return ""
}

func binaryPath(root string) (string, error) {
	names := []string{"decision-test.exe", "decision-test"}
	if runtime.GOOS != "windows" {
		names = []string{"decision-test", "decision-test.exe"}
	}
	for _, name := range names {
		path := filepath.Join(root, "bin", name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("decision-test binary not found under bin")
}

func writeConfig(t *testing.T, root string, port int, logPath string) string {
	t.Helper()
	raw := readRepo(t, root, filepath.Join("settings", "decision-test.yaml"))
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "api_port:") {
			lines[i] = "api_port: " + strconv.Itoa(port)
		}
		if strings.HasPrefix(strings.TrimSpace(line), "log_path:") {
			if logPath == "" {
				lines[i] = `log_path: ""`
			} else {
				lines[i] = "log_path: " + strconv.Quote(filepath.ToSlash(logPath))
			}
		}
	}
	path := filepath.Join(t.TempDir(), "decision-test.yaml")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func withMethod(t *testing.T, raw []byte, method string) []byte {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '}' {
		t.Fatalf("expected JSON object, got %s", trimmed)
	}
	out := append([]byte{}, trimmed[:len(trimmed)-1]...)
	out = append(out, []byte(`,"options":{"method":"`+method+`"}`)...)
	out = append(out, '}')
	return out
}

func postJSON(t *testing.T, url string, payload []byte) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func getOK(url string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d body %s", resp.StatusCode, raw)
	}
	return nil
}

func answer(t *testing.T, body map[string]any, id string) map[string]any {
	t.Helper()
	answers := body["answers"].(map[string]any)
	return answers[id].(map[string]any)
}

func floatMap(t *testing.T, value any) map[string]float64 {
	t.Helper()
	raw := value.(map[string]any)
	out := map[string]float64{}
	for key, item := range raw {
		out[key] = item.(float64)
	}
	return out
}

func objectKeys(t *testing.T, text string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(text))
	tok, err := dec.Token()
	if err != nil {
		t.Fatal(err)
	}
	if tok != json.Delim('{') {
		t.Fatalf("not object %s", text)
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, keyTok.(string))
		var skip any
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func readRepo(t *testing.T, root, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func yamlValue(raw, key string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		prefix := key + ":"
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, prefix)), `"`)
		}
	}
	return ""
}
