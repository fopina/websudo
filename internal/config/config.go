package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level websudo configuration.
type Config struct {
	Listen   string             `yaml:"listen"`
	TLS      TLSConfig          `yaml:"tls"`
	Services map[string]Service `yaml:"services"`
}

// TLSConfig controls CA storage and TLS handling.
type TLSConfig struct {
	CAcertPath        string `yaml:"ca_cert_path"`
	CAkeyPath         string `yaml:"ca_key_path"`
	GenerateOnBoot    bool   `yaml:"generate_on_boot"`
	GenerateOnBootSet bool   `yaml:"-"`
}

// Service describes one upstream service and its policy.
type Service struct {
	BaseURL                  string    `yaml:"base_url"`
	MatchHost                string    `yaml:"match_host"`
	RoutePrefix              string    `yaml:"route_prefix"`
	AllowedMethods           []string  `yaml:"allowed_methods"`
	AllowedPaths             []string  `yaml:"allowed_paths"`
	DeniedPaths              []string  `yaml:"denied_paths"`
	HeadersAllow             []string  `yaml:"headers_allow"`
	PlaceholderAuth          string    `yaml:"placeholder_auth"`
	InjectAuth               string    `yaml:"inject_auth"`
	RequirePlaceholderPrefix string    `yaml:"require_placeholder_prefix"`
	Variants                 []Variant `yaml:"variants"`
}

// Variant is a placeholder-token-specific override for a service.
type Variant struct {
	Name                     string   `yaml:"name"`
	PlaceholderContains      string   `yaml:"placeholder_contains"`
	AllowedMethods           []string `yaml:"allowed_methods"`
	AllowedPaths             []string `yaml:"allowed_paths"`
	DeniedPaths              []string `yaml:"denied_paths"`
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
	cfg.TLS.GenerateOnBootSet = strings.Contains(string(data), "generate_on_boot:")

	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	cfg.TLS = normalizeTLSConfig(cfg.TLS)
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("config %q defines no services", path)
	}

	for name, svc := range cfg.Services {
		normalized, err := normalizeService(name, svc)
		if err != nil {
			return nil, err
		}
		cfg.Services[name] = normalized
	}

	return &cfg, nil
}

func normalizeTLSConfig(tls TLSConfig) TLSConfig {
	baseDir := filepath.Join(userHomeDir(), ".local", "share", "websudo")
	if tls.CAcertPath == "" {
		tls.CAcertPath = filepath.Join(baseDir, "ca.pem")
	}
	if tls.CAkeyPath == "" {
		tls.CAkeyPath = filepath.Join(baseDir, "ca-key.pem")
	}
	if !tls.GenerateOnBootSet {
		tls.GenerateOnBoot = true
	}
	return tls
}

func normalizeService(name string, svc Service) (Service, error) {
	if svc.BaseURL == "" {
		return Service{}, fmt.Errorf("service %q is missing base_url", name)
	}
	if svc.MatchHost == "" && svc.RoutePrefix == "" {
		return Service{}, fmt.Errorf("service %q must define either match_host or route_prefix", name)
	}
	if svc.PlaceholderAuth == "" {
		return Service{}, fmt.Errorf("service %q is missing placeholder_auth", name)
	}
	if svc.InjectAuth == "" {
		return Service{}, fmt.Errorf("service %q is missing inject_auth", name)
	}
	if svc.RequirePlaceholderPrefix == "" {
		svc.RequirePlaceholderPrefix = "Bearer ph_"
	}
	if svc.RoutePrefix != "" && !strings.HasPrefix(svc.RoutePrefix, "/") {
		svc.RoutePrefix = "/" + svc.RoutePrefix
	}

	for i, variant := range svc.Variants {
		if variant.Name == "" {
			return Service{}, fmt.Errorf("service %q variant %d is missing name", name, i)
		}
		if variant.PlaceholderContains == "" {
			return Service{}, fmt.Errorf("service %q variant %q is missing placeholder_contains", name, variant.Name)
		}
		if variant.InjectAuth == "" {
			variant.InjectAuth = svc.InjectAuth
		}
		if variant.RequirePlaceholderPrefix == "" {
			variant.RequirePlaceholderPrefix = svc.RequirePlaceholderPrefix
		}
		svc.Variants[i] = variant
	}

	return svc, nil
}

// EffectiveService returns the base service merged with the matching variant, if any.
func (s Service) EffectiveService(placeholder string) (Service, string) {
	for _, variant := range s.Variants {
		if strings.Contains(placeholder, variant.PlaceholderContains) {
			effective := s
			effective.AllowedMethods = chooseStrings(variant.AllowedMethods, s.AllowedMethods)
			effective.AllowedPaths = chooseStrings(variant.AllowedPaths, s.AllowedPaths)
			effective.DeniedPaths = chooseStrings(variant.DeniedPaths, s.DeniedPaths)
			effective.InjectAuth = chooseString(variant.InjectAuth, s.InjectAuth)
			effective.RequirePlaceholderPrefix = chooseString(variant.RequirePlaceholderPrefix, s.RequirePlaceholderPrefix)
			return effective, variant.Name
		}
	}

	return s, ""
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

func chooseString(primary string, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func chooseStrings(primary []string, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}
