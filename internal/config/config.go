package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level websudo configuration.
type Config struct {
	Listen   string             `yaml:"listen"`
	Services map[string]Service `yaml:"services"`
}

// Service describes one upstream service and its policy.
type Service struct {
	BaseURL        string   `yaml:"base_url"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedPaths   []string `yaml:"allowed_paths"`
	DeniedPaths    []string `yaml:"denied_paths"`
	HeadersAllow   []string `yaml:"headers_allow"`
	InjectAuth     string   `yaml:"inject_auth"`
}

// Load reads and validates configuration from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}

	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("config %q defines no services", path)
	}

	for name, svc := range cfg.Services {
		if svc.BaseURL == "" {
			return nil, fmt.Errorf("service %q is missing base_url", name)
		}
	}

	return &cfg, nil
}
