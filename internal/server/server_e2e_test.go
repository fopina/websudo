package server

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func newProxyTestServer(t *testing.T, blockUnconfigured bool) *httptest.Server {
	t.Helper()
	cfg := testServerConfig(t)
	cfg.BlockUnconfiguredDestinations = blockUnconfigured
	srv := NewWithLogger(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return httptest.NewServer(srv.httpServer.Handler)
}

func newProxyTestServerFromConfig(t *testing.T, cfg *config.Config) *httptest.Server {
	t.Helper()
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

func newHTTPSClientWithProxyAndRootCA(t *testing.T, proxyURL string, caCertPath string) *http.Client {
	t.Helper()
	parsedProxyURL, err := url.Parse(proxyURL)
	require.NoError(t, err)

	caPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	} else {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.RootCAs = roots
	return &http.Client{Transport: transport}
}

func TestE2EAllowsUnknownHTTPDestinationByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("http passthrough ok"))
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, false)
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

	proxy := newProxyTestServer(t, true)
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

	proxy := newProxyTestServer(t, false)
	defer proxy.Close()

	resp, err := newHTTPSClientWithProxy(t, proxy.URL, upstream).Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, "https passthrough ok", string(body))
	require.NotNil(t, resp.TLS)
	require.Len(t, resp.TLS.PeerCertificates, 1)
	require.Equal(t, upstream.Certificate().Raw, resp.TLS.PeerCertificates[0].Raw)
}

func TestE2EConfiguredHTTPSInterceptsWithProxyCertificate(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer live_token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("intercepted"))
	}))
	defer upstream.Close()

	cfg := testServerConfig(t)
	cfg.Services = map[string]config.Service{
		"configured": {
			MatchHost:                "configured.test",
			BaseURL:                  upstream.URL,
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
		},
	}

	proxy := newProxyTestServerFromConfig(t, cfg)
	defer proxy.Close()

	req, err := http.NewRequest(http.MethodGet, "https://configured.test/user", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer ph_demo")

	resp, err := newHTTPSClientWithProxyAndRootCA(t, proxy.URL, cfg.TLS.CAcertPath).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "intercepted", string(body))
	require.NotNil(t, resp.TLS)
	require.NotEmpty(t, resp.TLS.PeerCertificates)
	require.Equal(t, "configured.test", resp.TLS.PeerCertificates[0].Subject.CommonName)
	require.Equal(t, "websudo Local CA", resp.TLS.PeerCertificates[0].Issuer.CommonName)
}

func TestE2EBlocksUnknownHTTPSDestinationWhenDisabled(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not reach upstream")
	}))
	defer upstream.Close()

	proxy := newProxyTestServer(t, true)
	defer proxy.Close()

	_, err := newHTTPSClientWithProxy(t, proxy.URL, upstream).Get(upstream.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "EOF")
}
