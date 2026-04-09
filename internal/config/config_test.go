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
    base_url: https://api.github.com
    allowed_methods: [GET]
    allowed_paths:
      - /user
`), 0o600)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8080", cfg.Listen)
	require.Contains(t, cfg.Services, "github")
}
