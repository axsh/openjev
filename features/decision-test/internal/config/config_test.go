package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseYAMLText = `api_host: 127.0.0.1
api_port: 8090
llama_url: http://127.0.0.1:18080
llama_host: 127.0.0.1
llama_port: 18080
llama_binary: third_party/llama.cpp/b11056/llama-server.exe
llama_build: b11056
llama_zip: llama-b11056-bin-win-cuda-12.4-x64.zip
cudart_zip: cudart-llama-bin-win-cuda-12.4-x64.zip
model_path: models/MiniCPM5-2B-Q4_K_M.gguf
model_id: minicpm5-2b-q4_k_m
model_url: https://example.invalid/model.gguf
log_path: ""
`

func writeYAML(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decision-test.yaml")
	if err := os.WriteFile(path, []byte(baseYAMLText+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	cfg, err := Load(writeYAML(t, "workers: 16\nstall_timeout_ms: 15000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIHost != "127.0.0.1" || cfg.APIPort != 8090 || cfg.LlamaURL != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.ModelID != "minicpm5-2b-q4_k_m" || cfg.ModelPath != "models/MiniCPM5-2B-Q4_K_M.gguf" {
		t.Fatalf("model fields: %+v", cfg)
	}
	if cfg.LlamaBinary != "third_party/llama.cpp/b11056/llama-server.exe" || cfg.LlamaBuild != "b11056" {
		t.Fatalf("llama fields: %+v", cfg)
	}
	if cfg.Workers != 16 || cfg.StallTimeoutMs != 15000 {
		t.Fatalf("pool fields: %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeYAML(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workers != DefaultWorkers || cfg.StallTimeoutMs != DefaultStallTimeoutMs {
		t.Fatalf("defaults: %+v", cfg)
	}
	explicit, err := Load(writeYAML(t, "workers: 30\nstall_timeout_ms: 200\n"))
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Workers != 30 || explicit.StallTimeoutMs != 200 {
		t.Fatalf("explicit: %+v", explicit)
	}
}

func TestLoadRejectsNegative(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{name: "workers", extra: "workers: -1\n", wantErr: "workers must be >= 1"},
		{name: "stall", extra: "stall_timeout_ms: -5\n", wantErr: "stall_timeout_ms must be >= 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeYAML(t, tt.extra))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadIgnoresLlamaParallel(t *testing.T) {
	cfg, err := Load(writeYAML(t, "llama_parallel: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workers != DefaultWorkers {
		t.Fatalf("workers %d", cfg.Workers)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
