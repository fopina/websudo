package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

type stubServer struct {
	run func(context.Context) error
}

func (s stubServer) Run(ctx context.Context) error {
	return s.run(ctx)
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "websudo.yaml")

	err := os.WriteFile(configPath, []byte(`listen: 127.0.0.1:0
tls:
  generate_on_boot: true
services:
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
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	return configPath
}

func TestServeCommandLoadsConfig(t *testing.T) {
	configPath := writeTestConfig(t)

	cmd := newServeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath, "-t"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "loaded config for 1 services")
}

func TestServeCommandLongTestFlag(t *testing.T) {
	configPath := writeTestConfig(t)

	cmd := newServeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath, "--test"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "loaded config for 1 services")
}

func TestServeCommandInvalidConfig(t *testing.T) {
	cmd := newServeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", "/does/not/exist", "-t"})

	err := cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "read config")
}

func TestServeCommandRejectsExtraArgs(t *testing.T) {
	configPath := writeTestConfig(t)

	cmd := newServeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath, "-t", "extra"})

	err := cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown command \"extra\"")
}

func TestServeCommandStartsServerWithoutTestFlag(t *testing.T) {
	configPath := writeTestConfig(t)
	caDir := t.TempDir()
	caCertPath := filepath.Join(caDir, "custom-ca.pem")
	caKeyPath := filepath.Join(caDir, "custom-ca-key.pem")

	originalNewServer := newServer
	defer func() { newServer = originalNewServer }()

	called := false
	newServer = func(cfg *config.Config) runnableServer {
		require.Equal(t, "127.0.0.1:0", cfg.Listen)
		require.Equal(t, caCertPath, cfg.TLS.CAcertPath)
		require.Equal(t, caKeyPath, cfg.TLS.CAkeyPath)
		require.FileExists(t, cfg.TLS.CAcertPath)
		require.FileExists(t, cfg.TLS.CAkeyPath)
		return stubServer{run: func(context.Context) error {
			called = true
			return nil
		}}
	}

	cmd := newServeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath, "--ca-cert", caCertPath, "--ca-key", caKeyPath})

	err := cmd.Execute()
	require.NoError(t, err)
	require.True(t, called)
}

func TestServeCommandPropagatesServerError(t *testing.T) {
	configPath := writeTestConfig(t)

	originalNewServer := newServer
	defer func() { newServer = originalNewServer }()

	newServer = func(cfg *config.Config) runnableServer {
		return stubServer{run: func(context.Context) error {
			return fmt.Errorf("boom")
		}}
	}

	cmd := newServeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath})

	err := cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "boom")
}

func TestServeCommandFailsWhenTLSAssetsMissingAndAutogenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "websudo.yaml")
	missingDir := filepath.Join(tmpDir, "missing")
	missingCertPath := filepath.Join(missingDir, "missing-cert.pem")
	missingKeyPath := filepath.Join(missingDir, "missing-key.pem")

	err := os.WriteFile(configPath, []byte(`listen: 127.0.0.1:0
tls:
  generate_on_boot: false
services:
  github:
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    inject_auth: env:GITHUB_TOKEN
`), 0o600)
	require.NoError(t, err)
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	cmd := newServeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", configPath, "--ca-cert", missingCertPath, "--ca-key", missingKeyPath, "-t"})

	err = cmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "CA files are missing")
}
