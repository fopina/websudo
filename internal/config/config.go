package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level websudo configuration.
type Config struct {
	Listen                        string             `yaml:"listen"`
	TLS                           TLSConfig          `yaml:"tls"`
	BlockUnconfiguredDestinations bool               `yaml:"block_unconfigured_destinations"`
	Services                      map[string]Service `yaml:"services"`
}

// TLSConfig controls CA storage and TLS handling.
type TLSConfig struct {
	CAcertPath        string `yaml:"ca_cert_path"`
	CAkeyPath         string `yaml:"ca_key_path"`
	RequireExistingCA bool   `yaml:"require_existing_ca"`
}

// LoginConfig defines a special upstream login request that should receive configured credentials.
type LoginConfig struct {
	Path                string `yaml:"path"`
	UsernameField       string `yaml:"username_field"`
	PasswordField       string `yaml:"password_field"`
	PlaceholderUsername string `yaml:"placeholder_username"`
	PlaceholderPassword string `yaml:"placeholder_password"`
	Username            string `yaml:"username"`
	Password            string `yaml:"password"`
}

// Service describes one upstream service and its policy.
type Service struct {
	AuthMode                 string      `yaml:"auth_mode"`
	BaseURL                  string      `yaml:"base_url"`
	MatchHost                string      `yaml:"match_host"`
	RoutePrefix              string      `yaml:"route_prefix"`
	AllowedMethods           []string    `yaml:"allowed_methods"`
	AllowedPaths             []string    `yaml:"allowed_paths"`
	DeniedPaths              []string    `yaml:"denied_paths"`
	HeadersAllow             []string    `yaml:"headers_allow"`
	PlaceholderAuth          string      `yaml:"placeholder_auth"`
	InjectAuth               string      `yaml:"inject_auth"`
	InjectAuthTarget         string      `yaml:"inject_auth_target"`
	RequirePlaceholderPrefix string      `yaml:"require_placeholder_prefix"`
	CookieEncryptionKey      string      `yaml:"cookie_encryption_key"`
	CookieEncryptionKeyPath  string      `yaml:"cookie_encryption_key_path"`
	Login                    LoginConfig `yaml:"login"`
	Variants                 []Variant   `yaml:"variants"`
}

const (
	AuthModeHeader = "header"
	AuthModeCookie = "cookie"
)

// Variant is a placeholder-token-specific override for a service.
type Variant struct {
	Name                     string   `yaml:"name"`
	PlaceholderContains      string   `yaml:"placeholder_contains"`
	AllowedMethods           []string `yaml:"allowed_methods"`
	AllowedPaths             []string `yaml:"allowed_paths"`
	DeniedPaths              []string `yaml:"denied_paths"`
	InjectAuth               string   `yaml:"inject_auth"`
	InjectAuthTarget         string   `yaml:"inject_auth_target"`
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
	cfg.TLS = normalizeTLSConfig(cfg.TLS)
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("config %q defines no services", path)
	}

	configDir := filepath.Dir(path)
	for name, svc := range cfg.Services {
		normalized, err := normalizeService(name, svc, configDir)
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
	} else {
		tls.CAcertPath = expandHomePath(tls.CAcertPath)
	}
	if tls.CAkeyPath == "" {
		tls.CAkeyPath = filepath.Join(baseDir, "ca-key.pem")
	} else {
		tls.CAkeyPath = expandHomePath(tls.CAkeyPath)
	}
	return tls
}

func normalizeService(name string, svc Service, configDir string) (Service, error) {
	if svc.BaseURL == "" {
		return Service{}, fmt.Errorf("service %q is missing base_url", name)
	}
	if svc.MatchHost == "" && svc.RoutePrefix == "" {
		return Service{}, fmt.Errorf("service %q must define either match_host or route_prefix", name)
	}

	svc.AuthMode = strings.ToLower(svc.AuthMode)
	switch svc.AuthMode {
	case AuthModeHeader:
		if err := validateHeaderAuthMode(name, svc); err != nil {
			return Service{}, err
		}
	case AuthModeCookie:
		if err := validateCookieAuthMode(name, svc); err != nil {
			return Service{}, err
		}
	case "":
		return Service{}, fmt.Errorf("service %q is missing auth_mode", name)
	default:
		return Service{}, fmt.Errorf("service %q has unsupported auth_mode %q", name, svc.AuthMode)
	}

	if svc.PlaceholderAuth != "" {
		if _, err := parseHeaderAuthTarget(svc.PlaceholderAuth); err != nil {
			return Service{}, fmt.Errorf("service %q placeholder_auth: %w", name, err)
		}
		if svc.InjectAuthTarget == "" {
			svc.InjectAuthTarget = svc.PlaceholderAuth
		}
	}
	if svc.InjectAuthTarget != "" {
		if _, err := parseHeaderAuthTarget(svc.InjectAuthTarget); err != nil {
			return Service{}, fmt.Errorf("service %q inject_auth_target: %w", name, err)
		}
	}
	if svc.RequirePlaceholderPrefix == "" {
		svc.RequirePlaceholderPrefix = "Bearer ph_"
	}
	if svc.RoutePrefix != "" && !strings.HasPrefix(svc.RoutePrefix, "/") {
		svc.RoutePrefix = "/" + svc.RoutePrefix
	}
	if svc.CookieEncryptionKeyPath != "" {
		svc.CookieEncryptionKeyPath = expandHomePath(svc.CookieEncryptionKeyPath)
		if !filepath.IsAbs(svc.CookieEncryptionKeyPath) {
			svc.CookieEncryptionKeyPath = filepath.Join(configDir, svc.CookieEncryptionKeyPath)
		}
	}
	if svc.needsCookieEncryption() {
		if svc.CookieEncryptionKeyPath == "" {
			svc.CookieEncryptionKeyPath = filepath.Join(configDir, fmt.Sprintf(".%s.cookie-encryption.key", name))
		}
		if svc.CookieEncryptionKey == "" {
			if err := ensureSecretFile(svc.CookieEncryptionKeyPath); err != nil {
				return Service{}, fmt.Errorf("service %q cookie_encryption_key_path: %w", name, err)
			}
		}
	}
	if svc.Login.Path != "" {
		if !strings.HasPrefix(svc.Login.Path, "/") {
			svc.Login.Path = "/" + svc.Login.Path
		}
		if svc.Login.UsernameField == "" {
			return Service{}, fmt.Errorf("service %q login is missing username_field", name)
		}
		if svc.Login.PasswordField == "" {
			return Service{}, fmt.Errorf("service %q login is missing password_field", name)
		}
		if (svc.Login.PlaceholderUsername == "") != (svc.Login.PlaceholderPassword == "") {
			return Service{}, fmt.Errorf("service %q login placeholder_username and placeholder_password must be configured together", name)
		}
		if svc.Login.Username == "" {
			return Service{}, fmt.Errorf("service %q login is missing username", name)
		}
		if svc.Login.Password == "" {
			return Service{}, fmt.Errorf("service %q login is missing password", name)
		}
	} else if svc.Login.UsernameField != "" || svc.Login.PasswordField != "" || svc.Login.PlaceholderUsername != "" || svc.Login.PlaceholderPassword != "" || svc.Login.Username != "" || svc.Login.Password != "" {
		return Service{}, fmt.Errorf("service %q login fields require login.path", name)
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
		if variant.InjectAuthTarget == "" {
			variant.InjectAuthTarget = svc.InjectAuthTarget
		}
		if variant.InjectAuthTarget != "" {
			if _, err := parseHeaderAuthTarget(variant.InjectAuthTarget); err != nil {
				return Service{}, fmt.Errorf("service %q variant %q inject_auth_target: %w", name, variant.Name, err)
			}
		}
		if variant.RequirePlaceholderPrefix == "" {
			variant.RequirePlaceholderPrefix = svc.RequirePlaceholderPrefix
		}
		svc.Variants[i] = variant
	}

	return svc, nil
}

func validateHeaderAuthMode(name string, svc Service) error {
	if hasLoginConfig(svc.Login) {
		return fmt.Errorf("service %q auth_mode header cannot be combined with login fields", name)
	}
	if svc.CookieEncryptionKey != "" || svc.CookieEncryptionKeyPath != "" {
		return fmt.Errorf("service %q auth_mode header cannot be combined with cookie_encryption_key or cookie_encryption_key_path", name)
	}
	if svc.PlaceholderAuth == "" {
		return fmt.Errorf("service %q is missing placeholder_auth", name)
	}
	if svc.InjectAuth == "" {
		return fmt.Errorf("service %q is missing inject_auth", name)
	}
	return nil
}

func validateCookieAuthMode(name string, svc Service) error {
	if svc.PlaceholderAuth != "" || svc.InjectAuth != "" || svc.InjectAuthTarget != "" || svc.RequirePlaceholderPrefix != "" || len(svc.Variants) > 0 {
		return fmt.Errorf("service %q auth_mode cookie cannot be combined with placeholder_auth, inject_auth, inject_auth_target, require_placeholder_prefix, or variants", name)
	}
	if svc.Login.Path == "" {
		return fmt.Errorf("service %q auth_mode cookie requires login.path", name)
	}
	if svc.Login.PlaceholderUsername == "" && svc.Login.PlaceholderPassword == "" {
		return fmt.Errorf("service %q auth_mode cookie requires login.placeholder_username and login.placeholder_password", name)
	}
	return nil
}

func hasLoginConfig(login LoginConfig) bool {
	return login.Path != "" ||
		login.UsernameField != "" ||
		login.PasswordField != "" ||
		login.PlaceholderUsername != "" ||
		login.PlaceholderPassword != "" ||
		login.Username != "" ||
		login.Password != ""
}

func (s Service) needsCookieEncryption() bool {
	return s.AuthMode == AuthModeCookie
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
			effective.InjectAuthTarget = chooseString(variant.InjectAuthTarget, s.InjectAuthTarget)
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

// LoginCredentials resolves the configured upstream login credentials.
func (l LoginConfig) LoginCredentials() (string, string, error) {
	username, err := resolveValue(l.Username)
	if err != nil {
		return "", "", fmt.Errorf("resolve login username: %w", err)
	}
	password, err := resolveValue(l.Password)
	if err != nil {
		return "", "", fmt.Errorf("resolve login password: %w", err)
	}
	return username, password, nil
}

// PlaceholderCredentials resolves optional client-facing login credentials.
func (l LoginConfig) PlaceholderCredentials() (string, string, bool, error) {
	if l.PlaceholderUsername == "" && l.PlaceholderPassword == "" {
		return "", "", false, nil
	}
	username, err := resolveValue(l.PlaceholderUsername)
	if err != nil {
		return "", "", false, fmt.Errorf("resolve login placeholder_username: %w", err)
	}
	password, err := resolveValue(l.PlaceholderPassword)
	if err != nil {
		return "", "", false, fmt.Errorf("resolve login placeholder_password: %w", err)
	}
	return username, password, true, nil
}

// CookieCipherKey resolves the cookie encryption secret into a stable AES-256 key.
func (s Service) CookieCipherKey() ([]byte, error) {
	var value string
	var err error
	if s.CookieEncryptionKey != "" {
		value, err = resolveValue(s.CookieEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("resolve cookie_encryption_key: %w", err)
		}
	} else if s.CookieEncryptionKeyPath != "" {
		value, err = readSecretFile(s.CookieEncryptionKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read cookie_encryption_key_path: %w", err)
		}
	} else {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:], nil
}

func resolveValue(raw string) (string, error) {
	const envPrefix = "env:"
	if strings.HasPrefix(raw, envPrefix) {
		value := os.Getenv(strings.TrimPrefix(raw, envPrefix))
		if value == "" {
			return "", fmt.Errorf("source %q resolved empty value", raw)
		}
		return value, nil
	}
	if raw == "" {
		return "", fmt.Errorf("source resolved empty value")
	}
	return raw, nil
}

func ensureSecretFile(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	return os.WriteFile(path, []byte(encoded), 0o600)
}

func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("secret file %q is empty", path)
	}
	return value, nil
}

func parseHeaderAuthTarget(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("auth target cannot be empty")
	}
	if !strings.Contains(raw, ":") {
		return raw, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid auth target %q", raw)
	}
	if !strings.EqualFold(parts[0], "header") {
		return "", fmt.Errorf("unsupported auth target %q", raw)
	}
	return parts[1], nil
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

func expandHomePath(path string) string {
	if path == "~" {
		return userHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(userHomeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}
