package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "websudo.yaml")

	err := os.WriteFile(path, []byte(`services:
  github:
    match_host: api.github.com
    route_prefix: /github
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
    inject_auth_target: header:X-GitHub-Auth
    allowed_methods: [GET]
    allowed_paths:
      - /user
    variants:
      - name: repo-write
        placeholder_contains: repo_write
        allowed_paths:
          - /repos/*/*
`), 0o600)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", cfg.Listen)
	require.False(t, cfg.BlockUnconfiguredDestinations)
	require.Contains(t, cfg.Services, "github")
	require.Equal(t, "Bearer ph_", cfg.Services["github"].RequirePlaceholderPrefix)
	require.Equal(t, "/github", cfg.Services["github"].RoutePrefix)
	require.Equal(t, "header:X-GitHub-Auth", cfg.Services["github"].InjectAuthTarget)
	require.Empty(t, cfg.Services["github"].CookieEncryptionKeyPath)
	require.Len(t, cfg.Services["github"].Variants, 1)
}

func TestInjectedAuthValue(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	value, err := Service{InjectAuth: "env:GITHUB_TOKEN"}.InjectedAuthValue()
	require.NoError(t, err)
	require.Equal(t, "Bearer live_token", value)
}

func TestEffectiveServiceVariantOverride(t *testing.T) {
	service := Service{
		AllowedPaths:             []string{"/user"},
		InjectAuth:               "env:BASE_TOKEN",
		RequirePlaceholderPrefix: "Bearer ph_",
		Variants: []Variant{{
			Name:                "repo-write",
			PlaceholderContains: "repo_write",
			AllowedPaths:        []string{"/repos/*/*"},
			InjectAuth:          "env:REPO_TOKEN",
		}},
	}

	effective, variantName := service.EffectiveService("Bearer ph_repo_write_123")
	require.Equal(t, "repo-write", variantName)
	require.Equal(t, []string{"/repos/*/*"}, effective.AllowedPaths)
	require.Equal(t, "env:REPO_TOKEN", effective.InjectAuth)
}

func TestNormalizeServiceDefaultsInjectAuthTargetToPlaceholderAuth(t *testing.T) {
	svc, err := normalizeService("github", Service{
		MatchHost:       "api.github.com",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "header:X-Placeholder-Auth",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "header:X-Placeholder-Auth", svc.InjectAuthTarget)
}

func TestNormalizeServiceRejectsCookieAuthTargets(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "cookie:websudo_ph",
		InjectAuth:      "env:GITHUB_SESSION",
	}, t.TempDir())
	require.ErrorContains(t, err, "unsupported auth target")
}

func TestNormalizeServiceExpandsHomeCookieEncryptionKeyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc, err := normalizeService("browser", Service{
		RoutePrefix:             "/app",
		BaseURL:                 "https://app.internal",
		CookieEncryptionKey:     "static-secret",
		CookieEncryptionKeyPath: "~/websudo/app.cookie-key",
		Login: LoginConfig{
			Path:          "/session",
			UsernameField: "username",
			PasswordField: "password",
			Username:      "env:APP_USER",
			Password:      "env:APP_PASS",
		},
	}, t.TempDir())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "websudo", "app.cookie-key"), svc.CookieEncryptionKeyPath)
}

func TestEffectiveServiceVariantCanOverrideInjectAuthTarget(t *testing.T) {
	service := Service{
		PlaceholderAuth:  "Authorization",
		InjectAuth:       "env:BASE_TOKEN",
		InjectAuthTarget: "Authorization",
		Variants: []Variant{{
			Name:                "browser",
			PlaceholderContains: "browser",
			InjectAuthTarget:    "header:X-Upstream-Auth",
		}},
	}

	effective, variantName := service.EffectiveService("Bearer ph_browser_123")
	require.Equal(t, "browser", variantName)
	require.Equal(t, "header:X-Upstream-Auth", effective.InjectAuthTarget)
}

func TestLoginCredentialsResolveEnvSources(t *testing.T) {
	t.Setenv("UPSTREAM_USER", "boss")
	t.Setenv("UPSTREAM_PASS", "swordfish")

	username, password, err := (LoginConfig{Username: "env:UPSTREAM_USER", Password: "env:UPSTREAM_PASS"}).LoginCredentials()
	require.NoError(t, err)
	require.Equal(t, "boss", username)
	require.Equal(t, "swordfish", password)
}

func TestCookieCipherKeyUsesResolvedSecret(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")
	key, err := (Service{CookieEncryptionKey: "env:COOKIE_SECRET"}).CookieCipherKey()
	require.NoError(t, err)
	require.Len(t, key, 32)
}

func TestCookieCipherKeyUsesPersistedSecretFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.key")
	require.NoError(t, os.WriteFile(path, []byte("persisted-secret\n"), 0o600))
	key, err := (Service{CookieEncryptionKeyPath: path}).CookieCipherKey()
	require.NoError(t, err)
	require.Len(t, key, 32)
}
