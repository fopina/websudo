package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func testServerConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "ca.pem")
	keyPath := filepath.Join(tmpDir, "ca-key.pem")
	require.NoError(t, config.GenerateCA(certPath, keyPath))

	return &config.Config{
		Listen: "127.0.0.1:0",
		TLS: config.TLSConfig{
			CAcertPath: certPath,
			CAkeyPath:  keyPath,
		},
		Services: map[string]config.Service{
			"github": {
				MatchHost:                "api.github.com",
				BaseURL:                  "https://api.github.com",
				PlaceholderAuth:          "Authorization",
				InjectAuth:               "env:GITHUB_TOKEN",
				RequirePlaceholderPrefix: "Bearer ph_",
			},
		},
	}
}

func TestHandleResponseReturnsResponse(t *testing.T) {
	srv := NewWithLogger(testServerConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	resp := &http.Response{StatusCode: http.StatusOK}
	out := srv.handleResponse(resp, nil)
	require.Same(t, resp, out)
}

func TestRunReturnsServerErrors(t *testing.T) {
	srv := &Server{httpServer: &http.Server{Addr: "127.0.0.1:-1"}}
	err := srv.Run(context.Background())
	require.Error(t, err)
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	srv := &Server{httpServer: &http.Server{Addr: "127.0.0.1:0", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
