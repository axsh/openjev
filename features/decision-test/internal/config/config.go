package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type File struct {
	APIHost       string `yaml:"api_host"`
	APIPort       int    `yaml:"api_port"`
	LlamaURL      string `yaml:"llama_url"`
	LlamaHost     string `yaml:"llama_host"`
	LlamaPort     int    `yaml:"llama_port"`
	LlamaBinary   string `yaml:"llama_binary"`
	LlamaBuild    string `yaml:"llama_build"`
	LlamaZip      string `yaml:"llama_zip"`
	CudaRTZip     string `yaml:"cudart_zip"`
	ModelPath     string `yaml:"model_path"`
	ModelID       string `yaml:"model_id"`
	ModelURL      string `yaml:"model_url"`
	LogPath       string `yaml:"log_path"`
	LlamaParallel int    `yaml:"llama_parallel"`
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
	if cfg.LlamaParallel < 0 {
		return File{}, fmt.Errorf("llama_parallel must be >= 1")
	}
	if cfg.LlamaParallel == 0 {
		cfg.LlamaParallel = 1
	}
	return cfg, nil
}
