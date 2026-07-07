package server

import (
	"bytes"
	"log/slog"
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

	require.NoError(t, applyTLSConfig(proxy, cfg, slog.Default()))
}

func TestApplyTLSConfigErrorsWhenCAIsMissing(t *testing.T) {
	proxy := goproxy.NewProxyHttpServer()
	err := applyTLSConfig(proxy, &config.Config{TLS: config.TLSConfig{CAcertPath: "/missing/ca.pem", CAkeyPath: "/missing/ca-key.pem"}}, slog.Default())
	require.Error(t, err)
}

func TestLogForwardConnectRecordsHostnameAndAction(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	logForwardConnect(logger, "gitlab.com:443", "passthrough")

	require.Contains(t, logs.String(), "forward CONNECT unmatched")
	require.Contains(t, logs.String(), "host=gitlab.com")
	require.Contains(t, logs.String(), "connect_host=gitlab.com:443")
	require.Contains(t, logs.String(), "action=passthrough")
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
