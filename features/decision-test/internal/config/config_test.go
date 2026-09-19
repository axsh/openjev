package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decision-test.yaml")
	body := []byte(`api_host: 127.0.0.1
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
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
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
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
