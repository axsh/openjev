package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultWorkers is the worker count when the settings file omits workers.
	DefaultWorkers = 16
	// DefaultStallTimeoutMs is the per-request stall cutoff when the settings file omits it.
	DefaultStallTimeoutMs = 15000
)

type File struct {
	APIHost        string `yaml:"api_host"`
	APIPort        int    `yaml:"api_port"`
	LlamaURL       string `yaml:"llama_url"`
	LlamaHost      string `yaml:"llama_host"`
	LlamaPort      int    `yaml:"llama_port"`
	LlamaBinary    string `yaml:"llama_binary"`
	LlamaBuild     string `yaml:"llama_build"`
	LlamaZip       string `yaml:"llama_zip"`
	CudaRTZip      string `yaml:"cudart_zip"`
	ModelPath      string `yaml:"model_path"`
	ModelID        string `yaml:"model_id"`
	ModelURL       string `yaml:"model_url"`
	LogPath        string `yaml:"log_path"`
	Workers        int    `yaml:"workers"`
	StallTimeoutMs int    `yaml:"stall_timeout_ms"`
}

func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var cfg File
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return File{}, err
	}
	if cfg.APIPort == 0 || cfg.LlamaURL == "" || cfg.ModelID == "" || cfg.ModelPath == "" || cfg.LlamaBinary == "" {
		return File{}, fmt.Errorf("settings file %s is missing required fields", path)
	}
	if cfg.Workers < 0 {
		return File{}, fmt.Errorf("workers must be >= 1")
	}
	if cfg.Workers == 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.StallTimeoutMs < 0 {
		return File{}, fmt.Errorf("stall_timeout_ms must be >= 1")
	}
	if cfg.StallTimeoutMs == 0 {
		cfg.StallTimeoutMs = DefaultStallTimeoutMs
	}
	return cfg, nil
}
