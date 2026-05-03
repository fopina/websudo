package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func TestHandleRequestLoginReplacesConfiguredCredentials(t *testing.T) {
	t.Setenv("UPSTREAM_USER", "boss")
	t.Setenv("UPSTREAM_PASS", "swordfish")
	t.Setenv("COOKIE_SECRET", "secret-key")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			RoutePrefix:         "/github",
			BaseURL:             "https://upstream.internal",
			CookieEncryptionKey: "env:COOKIE_SECRET",
			Login: config.LoginConfig{
				Path:          "/session",
				UsernameField: "login",
				PasswordField: "password",
				Username:      "env:UPSTREAM_USER",
				Password:      "env:UPSTREAM_PASS",
			},
		},
	}})

	req := httptest.NewRequest(http.MethodPost, "http://websudo.local/github/session", strings.NewReader("login=fake&password=wrong&remember_me=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "https", outReq.URL.Scheme)
	require.Equal(t, "upstream.internal", outReq.URL.Host)
	require.Equal(t, "/session", outReq.URL.Path)
	body, err := io.ReadAll(outReq.Body)
	require.NoError(t, err)
	require.Equal(t, "login=boss&password=swordfish&remember_me=1", string(body))
	require.Empty(t, outReq.Header.Get("Authorization"))
}

func TestHandleRequestDecryptsEncryptedCookiesAndLeavesInvalidOnes(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	svc := config.Service{
		MatchHost:                "api.github.com",
		BaseURL:                  "https://upstream.internal/api",
		PlaceholderAuth:          "Authorization",
		InjectAuth:               "env:GITHUB_TOKEN",
		RequirePlaceholderPrefix: "Bearer ph_",
		CookieEncryptionKey:      "env:COOKIE_SECRET",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user"},
	}
	key, err := svc.CookieCipherKey()
	require.NoError(t, err)
	encrypted, err := encryptCookieValue(key, "user_session", "live_session")
	require.NoError(t, err)

	srv := New(&config.Config{Services: map[string]config.Service{"github": svc}})
	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")
	req.AddCookie(&http.Cookie{Name: "user_session", Value: encrypted})
	req.AddCookie(&http.Cookie{Name: "plain", Value: "wsenc:not-valid"})

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Contains(t, outReq.Header.Get("Cookie"), "user_session=live_session")
	require.Contains(t, outReq.Header.Get("Cookie"), "plain=wsenc:not-valid")
}

func TestHandleResponseEncryptsUpstreamSetCookie(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")

	svc := config.Service{CookieEncryptionKey: "env:COOKIE_SECRET"}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Set-Cookie": []string{"user_session=live_session; Path=/; HttpOnly"},
		},
		Request: httptest.NewRequest(http.MethodGet, "http://websudo.local/user", nil),
	}

	srv := NewWithLogger(testServerConfig(t), slog.Default())
	out := srv.handleResponse(resp, &goproxy.ProxyCtx{UserData: routeContext{serviceName: "github", service: svc}})
	require.NotNil(t, out)
	cookies := out.Cookies()
	require.Len(t, cookies, 1)
	require.NotEqual(t, "live_session", cookies[0].Value)
	key, err := svc.CookieCipherKey()
	require.NoError(t, err)
	decrypted, ok := decryptCookieValue(key, cookies[0].Name, cookies[0].Value)
	require.True(t, ok)
	require.Equal(t, "live_session", decrypted)
}
