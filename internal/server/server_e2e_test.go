package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func newProxyTestServer(t *testing.T, allowUnconfigured bool) *httptest.Server {
	t.Helper()
	cfg := testServerConfig(t)
	cfg.AllowUnconfiguredDestinations = allowUnconfigured
	srv := NewWithLogger(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return httptest.NewServer(srv.httpServer.Handler)
}

func newHTTPClientWithProxy(t *testing.T, proxyURL string) *http.Client {
	t.Helper()
	parsedProxyURL, err := url.Parse(proxyURL)
	require.NoError(t, err)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	return &http.Client{Transport: transport}
}

func newHTTPSClientWithProxy(t *testing.T, proxyURL string, upstream *httptest.Server) *http.Client {
	t.Helper()
	parsedProxyURL, err := url.Parse(proxyURL)
	require.NoError(t, err)

	transport := upstream.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	return &http.Client{Transport: transport}
}

func TestE2EAllowsUnknownHTTPDestinationByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("http passthrough ok"))
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, true)
	defer proxy.Close()

	resp, err := newHTTPClientWithProxy(t, proxy.URL).Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Equal(t, "http passthrough ok", string(body))
}

func TestE2EBlocksUnknownHTTPDestinationWhenDisabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not reach upstream")
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, false)
	defer proxy.Close()

	resp, err := newHTTPClientWithProxy(t, proxy.URL).Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Contains(t, string(body), "no configured service matches request")
}

func TestE2EAllowsUnknownHTTPSDestinationByDefault(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("https passthrough ok"))
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, true)
	defer proxy.Close()

	resp, err := newHTTPSClientWithProxy(t, proxy.URL, upstream).Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, "https passthrough ok", string(body))
}

func TestE2EBlocksUnknownHTTPSDestinationWhenDisabled(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not reach upstream")
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, false)
	defer proxy.Close()

	_, err := newHTTPSClientWithProxy(t, proxy.URL, upstream).Get(upstream.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "EOF")
}
