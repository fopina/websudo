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
    inject_auth_target: cookie:_session
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
	require.Equal(t, "cookie:_session", cfg.Services["github"].InjectAuthTarget)
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
		PlaceholderAuth: "cookie:websudo_ph",
		InjectAuth:      "env:GITHUB_SESSION",
	})
	require.NoError(t, err)
	require.Equal(t, "cookie:websudo_ph", svc.InjectAuthTarget)
}

func TestEffectiveServiceVariantCanOverrideInjectAuthTarget(t *testing.T) {
	service := Service{
		PlaceholderAuth:  "Authorization",
		InjectAuth:       "env:BASE_TOKEN",
		InjectAuthTarget: "Authorization",
		Variants: []Variant{{
			Name:                "browser",
			PlaceholderContains: "browser",
			InjectAuthTarget:    "cookie:_session",
		}},
	}

	effective, variantName := service.EffectiveService("Bearer ph_browser_123")
	require.Equal(t, "browser", variantName)
	require.Equal(t, "cookie:_session", effective.InjectAuthTarget)
}
