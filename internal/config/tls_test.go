package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTLSConfigDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tlsCfg := normalizeTLSConfig(TLSConfig{})
	require.Contains(t, tlsCfg.CAcertPath, filepath.Join(".local", "share", "websudo", "ca.pem"))
	require.Contains(t, tlsCfg.CAkeyPath, filepath.Join(".local", "share", "websudo", "ca-key.pem"))
}

func TestEnsureTLSAssetsUsesDefaultPathsWhenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{TLS: normalizeTLSConfig(TLSConfig{})}
	require.NoError(t, EnsureTLSAssets(context.Background(), cfg))
	require.FileExists(t, cfg.TLS.CAcertPath)
	require.FileExists(t, cfg.TLS.CAkeyPath)
}

func TestEnsureTLSAssetsGeneratesCAOnBoot(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{TLS: TLSConfig{
		GenerateOnBoot: true,
		CAcertPath:     filepath.Join(tmpDir, "ca.pem"),
		CAkeyPath:      filepath.Join(tmpDir, "ca-key.pem"),
	}}

	require.NoError(t, EnsureTLSAssets(context.Background(), cfg))
	require.FileExists(t, cfg.TLS.CAcertPath)
	require.FileExists(t, cfg.TLS.CAkeyPath)
}

func TestEnsureTLSAssetsErrorsWhenFilesMissingAndAutogenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{TLS: TLSConfig{
		GenerateOnBoot: false,
		CAcertPath:     filepath.Join(tmpDir, "ca.pem"),
		CAkeyPath:      filepath.Join(tmpDir, "ca-key.pem"),
	}}

	err := EnsureTLSAssets(context.Background(), cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "CA files are missing")
}

func TestGenerateCAWritesPEMFiles(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.pem")
	keyPath := filepath.Join(tmpDir, "ca-key.pem")

	require.NoError(t, GenerateCA(certPath, keyPath))

	certBytes, err := os.ReadFile(certPath)
	require.NoError(t, err)
	keyBytes, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	require.Contains(t, string(certBytes), "BEGIN CERTIFICATE")
	require.Contains(t, string(keyBytes), "BEGIN RSA PRIVATE KEY")
}
