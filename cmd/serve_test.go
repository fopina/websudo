package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServeCommandLoadsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "websudo.yaml")

	err := os.WriteFile(configPath, []byte(`listen: 127.0.0.1:0
services:
  github:
    base_url: https://api.github.com
    allowed_methods: [GET]
    allowed_paths:
      - /user
`), 0o600)
	require.NoError(t, err)

	cmd := newServeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath})

	err = cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "loaded config for 1 services")
}
