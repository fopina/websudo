package server

import (
	"encoding/json"
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

func TestHandleRequestLoginCanRequirePlaceholderCredentials(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			RoutePrefix:         "/github",
			BaseURL:             "https://upstream.internal",
			CookieEncryptionKey: "env:COOKIE_SECRET",
			Login: config.LoginConfig{
				Path:                "/session",
				UsernameField:       "login",
				PasswordField:       "password",
				PlaceholderUsername: "app",
				PlaceholderPassword: "app",
				Username:            "boss",
				Password:            "swordfish",
			},
		},
	}})

	req := httptest.NewRequest(http.MethodPost, "http://websudo.local/github/session", strings.NewReader("login=fake&password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHandleRequestLoginWithPlaceholderCredentialsRewrites(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			RoutePrefix:         "/github",
			BaseURL:             "https://upstream.internal",
			CookieEncryptionKey: "env:COOKIE_SECRET",
			Login: config.LoginConfig{
				Path:                "/session",
				UsernameField:       "login",
				PasswordField:       "password",
				PlaceholderUsername: "app",
				PlaceholderPassword: "app",
				Username:            "boss",
				Password:            "swordfish",
			},
		},
	}})

	req := httptest.NewRequest(http.MethodPost, "http://websudo.local/github/session", strings.NewReader("login=app&password=app"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Empty(t, outReq.Header.Get("Authorization"))
	body, err := io.ReadAll(outReq.Body)
	require.NoError(t, err)
	require.Equal(t, "login=boss&password=swordfish", string(body))
}

func TestHandleRequestLoginReplacesConfiguredJSONCredentials(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "http://websudo.local/github/session", strings.NewReader(`{"login":"fake","password":"wrong","remember_me":true}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "https", outReq.URL.Scheme)
	require.Equal(t, "upstream.internal", outReq.URL.Host)
	require.Equal(t, "/session", outReq.URL.Path)

	var body map[string]any
	require.NoError(t, json.NewDecoder(outReq.Body).Decode(&body))
	require.Equal(t, "boss", body["login"])
	require.Equal(t, "swordfish", body["password"])
	require.Equal(t, true, body["remember_me"])
}

func TestHandleRequestLoginJSONFieldsAreTopLevelKeysOnly(t *testing.T) {
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
				UsernameField: "user.login",
				PasswordField: "user.password",
				Username:      "env:UPSTREAM_USER",
				Password:      "env:UPSTREAM_PASS",
			},
		},
	}})

	req := httptest.NewRequest(http.MethodPost, "http://websudo.local/github/session", strings.NewReader(`{"user":{"login":"fake","password":"wrong"}}`))
	req.Header.Set("Content-Type", "application/json")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)

	var body map[string]any
	require.NoError(t, json.NewDecoder(outReq.Body).Decode(&body))
	require.Equal(t, "boss", body["user.login"])
	require.Equal(t, "swordfish", body["user.password"])
	require.Equal(t, map[string]any{"login": "fake", "password": "wrong"}, body["user"])
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

func TestHeaderLoginEncryptsResponseTokenAndDecryptsRequestToken(t *testing.T) {
	t.Setenv("COOKIE_SECRET", "secret-key")

	svc := config.Service{
		AuthMode:            config.AuthModeHeader,
		RoutePrefix:         "/api",
		BaseURL:             "https://upstream.internal",
		PlaceholderAuth:     "Authorization",
		CookieEncryptionKey: "env:COOKIE_SECRET",
		AllowedMethods:      []string{http.MethodGet},
		AllowedPaths:        []string{"/profile"},
		Login: config.LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			TokenField:          "access_token",
			PlaceholderUsername: "app",
			PlaceholderPassword: "app",
			Username:            "boss",
			Password:            "swordfish",
		},
	}
	srv := New(&config.Config{Services: map[string]config.Service{"app": svc}})

	loginReq := httptest.NewRequest(http.MethodPost, "http://websudo.local/api/session", strings.NewReader("login=app&password=app"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := &goproxy.ProxyCtx{}
	outReq, resp := srv.handleRequest(loginReq, ctx)
	require.Nil(t, resp)
	require.Equal(t, "/session", outReq.URL.Path)
	require.Empty(t, outReq.Header.Get("Authorization"))
	body, err := io.ReadAll(outReq.Body)
	require.NoError(t, err)
	require.Equal(t, "login=boss&password=swordfish", string(body))

	upstreamResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    io.NopCloser(strings.NewReader(`{"access_token":"live_token","token_type":"Bearer"}`)),
		Request: outReq,
	}
	loginResp := srv.handleResponse(upstreamResp, ctx)
	require.NotNil(t, loginResp)

	var loginPayload map[string]string
	require.NoError(t, json.NewDecoder(loginResp.Body).Decode(&loginPayload))
	encryptedToken := loginPayload["access_token"]
	require.NotEmpty(t, encryptedToken)
	require.NotEqual(t, "live_token", encryptedToken)
	key, err := svc.CookieCipherKey()
	require.NoError(t, err)
	decrypted, ok := decryptCookieValue(key, "Authorization", encryptedToken)
	require.True(t, ok)
	require.Equal(t, "live_token", decrypted)

	apiReq := httptest.NewRequest(http.MethodGet, "http://websudo.local/api/profile", nil)
	apiReq.Header.Set("Authorization", "Bearer "+encryptedToken)
	outReq, resp = srv.handleRequest(apiReq, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "/profile", outReq.URL.Path)
	require.Equal(t, "Bearer live_token", outReq.Header.Get("Authorization"))
}
