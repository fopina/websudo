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
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
    allowed_methods: [GET]
    allowed_paths:
      - /user
`), 0o600)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", cfg.Listen)
	require.Contains(t, cfg.Services, "github")
	require.Equal(t, "Bearer ph_", cfg.Services["github"].RequirePlaceholderPrefix)
}

func TestInjectedAuthValue(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	value, err := Service{InjectAuth: "env:GITHUB_TOKEN"}.InjectedAuthValue()
	require.NoError(t, err)
	require.Equal(t, "Bearer live_token", value)
}
