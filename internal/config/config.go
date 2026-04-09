package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level websudo configuration.
type Config struct {
	Listen   string             `yaml:"listen"`
	Services map[string]Service `yaml:"services"`
}

// Service describes one upstream service and its policy.
type Service struct {
	BaseURL                  string   `yaml:"base_url"`
	MatchHost                string   `yaml:"match_host"`
	AllowedMethods           []string `yaml:"allowed_methods"`
	AllowedPaths             []string `yaml:"allowed_paths"`
	DeniedPaths              []string `yaml:"denied_paths"`
	HeadersAllow             []string `yaml:"headers_allow"`
	PlaceholderAuth          string   `yaml:"placeholder_auth"`
	InjectAuth               string   `yaml:"inject_auth"`
	RequirePlaceholderPrefix string   `yaml:"require_placeholder_prefix"`
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
		if svc.MatchHost == "" {
			return nil, fmt.Errorf("service %q is missing match_host", name)
		}
		if svc.PlaceholderAuth == "" {
			return nil, fmt.Errorf("service %q is missing placeholder_auth", name)
		}
		if svc.InjectAuth == "" {
			return nil, fmt.Errorf("service %q is missing inject_auth", name)
		}
		if svc.RequirePlaceholderPrefix == "" {
			cfg.Services[name] = withDefaultPlaceholderPrefix(svc)
		}
	}

	return &cfg, nil
}

func withDefaultPlaceholderPrefix(svc Service) Service {
	svc.RequirePlaceholderPrefix = "Bearer ph_"
	return svc
}

// InjectedAuthValue resolves the upstream auth value from env:VAR references.
func (s Service) InjectedAuthValue() (string, error) {
	const envPrefix = "env:"
	if !strings.HasPrefix(s.InjectAuth, envPrefix) {
		return "", fmt.Errorf("unsupported inject_auth source %q", s.InjectAuth)
	}

	value := os.Getenv(strings.TrimPrefix(s.InjectAuth, envPrefix))
	if value == "" {
		return "", fmt.Errorf("inject_auth source %q resolved empty value", s.InjectAuth)
	}

	return value, nil
}
