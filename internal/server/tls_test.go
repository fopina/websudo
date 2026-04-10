package server

import (
	"path/filepath"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoadCALoadsGeneratedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.pem")
	keyPath := filepath.Join(tmpDir, "ca-key.pem")
	require.NoError(t, config.GenerateCA(certPath, keyPath))

	cert, err := loadCA(certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, cert.Leaf)
}

func TestApplyTLSConfigLoadsCustomCA(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.pem")
	keyPath := filepath.Join(tmpDir, "ca-key.pem")
	require.NoError(t, config.GenerateCA(certPath, keyPath))

	proxy := goproxy.NewProxyHttpServer()
	cfg := &config.Config{TLS: config.TLSConfig{
		CAcertPath: certPath,
		CAkeyPath:  keyPath,
	}}

	require.NoError(t, applyTLSConfig(proxy, cfg))
}

func TestApplyTLSConfigErrorsWhenCAIsMissing(t *testing.T) {
	proxy := goproxy.NewProxyHttpServer()
	err := applyTLSConfig(proxy, &config.Config{TLS: config.TLSConfig{CAcertPath: "/missing/ca.pem", CAkeyPath: "/missing/ca-key.pem"}})
	require.Error(t, err)
}

func TestShouldInterceptTLSMatchesConfiguredHost(t *testing.T) {
	cfg := &config.Config{Services: map[string]config.Service{
		"github": {MatchHost: "api.github.com"},
	}}

	require.True(t, shouldInterceptTLS(cfg, "api.github.com:443"))
	require.False(t, shouldInterceptTLS(cfg, "gitlab.com:443"))
}

func TestTLSInterceptionDecisionOnlyMatchesConfiguredServices(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.Service{
			"github": {MatchHost: "api.github.com"},
		},
	}

	require.False(t, shouldInterceptTLS(cfg, "gitlab.com:443"))
	require.True(t, shouldInterceptTLS(cfg, "api.github.com:443"))
}

func TestStripPort(t *testing.T) {
	require.Equal(t, "api.github.com", stripPort("api.github.com:443"))
	require.Equal(t, "api.github.com", stripPort("api.github.com"))
}
