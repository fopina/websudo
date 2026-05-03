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
		MatchHost:       "api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing base_url")
}

func TestNormalizeServiceRejectsMissingPlaceholderAuth(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:  "api.github.com",
		BaseURL:    "https://api.github.com",
		InjectAuth: "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing placeholder_auth")
}

func TestNormalizeServiceRejectsMissingInjectAuth(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "api.github.com",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
	}, t.TempDir())
	require.Error(t, err)
	require.ErrorContains(t, err, "missing inject_auth")
}

func TestNormalizeServiceRejectsVariantWithoutName(t *testing.T) {
	_, err := normalizeService("github", Service{
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
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.True(t, cfg.BlockUnconfiguredDestinations)
}
