package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services: ["), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	require.ErrorContains(t, err, "decode config")
}

func TestLoadRejectsMissingServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, []byte("listen: 127.0.0.1:8080\n"), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	require.ErrorContains(t, err, "defines no services")
}

func TestNormalizeServiceRejectsMissingBaseURL(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		MatchHost:       "api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing base_url")
}

func TestNormalizeServiceRejectsMissingPlaceholderAuth(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:   AuthModeHeader,
		MatchHost:  "api.github.com",
		BaseURL:    "https://api.github.com",
		InjectAuth: "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing placeholder_auth")
}

func TestNormalizeServiceRejectsMissingInjectAuth(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		MatchHost:       "api.github.com",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing inject_auth")
}

func TestNormalizeServiceRejectsVariantWithoutName(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		MatchHost:       "api.github.com",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Variants: []Variant{{
			PlaceholderContains: "repo",
		}},
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing name")
}

func TestNormalizeServiceRejectsVariantWithoutPlaceholderMatch(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		MatchHost:       "api.github.com",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Variants: []Variant{{
			Name: "repo-write",
		}},
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing placeholder_contains")
}

func TestLoadDefaultsBlockUnconfiguredDestinationsToFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default-allow.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`services:
  github:
    auth_mode: header
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.False(t, cfg.BlockUnconfiguredDestinations)
}

func TestLoadAllowsEnablingBlockUnconfiguredDestinations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deny-unknown.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`block_unconfigured_destinations: true
services:
  github:
    auth_mode: header
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.True(t, cfg.BlockUnconfiguredDestinations)
}

func TestLoadRejectsOverlappingMatchHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlapping-hosts.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`services:
  github-read:
    auth_mode: header
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_READ_TOKEN
  github-write:
    auth_mode: header
    match_host: API.GITHUB.COM
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_WRITE_TOKEN
`), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	require.ErrorContains(t, err, "overlapping match_host")
	require.ErrorContains(t, err, "github-read")
	require.ErrorContains(t, err, "github-write")
}

func TestLoadRejectsOverlappingRoutePrefixes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlapping-prefixes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`services:
  api:
    auth_mode: header
    route_prefix: /api
    base_url: https://api.internal
    placeholder_auth: Authorization
    inject_auth: env:API_TOKEN
  api-admin:
    auth_mode: header
    route_prefix: /api/admin
    base_url: https://admin.internal
    placeholder_auth: Authorization
    inject_auth: env:ADMIN_TOKEN
`), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	require.ErrorContains(t, err, "overlapping route_prefix")
	require.ErrorContains(t, err, "/api")
	require.ErrorContains(t, err, "/api/admin")
}

func TestLoadAllowsRoutePrefixesWithSharedTextButDifferentPathSegments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "distinct-prefixes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`services:
  api:
    auth_mode: header
    route_prefix: /api
    base_url: https://api.internal
    placeholder_auth: Authorization
    inject_auth: env:API_TOKEN
  api-v2:
    auth_mode: header
    route_prefix: /api-v2/
    base_url: https://api-v2.internal
    placeholder_auth: Authorization
    inject_auth: env:API_V2_TOKEN
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "/api-v2", cfg.Services["api-v2"].RoutePrefix)
}
