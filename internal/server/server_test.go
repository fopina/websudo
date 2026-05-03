package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func TestValidateRequestRequiresPlaceholder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	err := validateRequest(req, config.Service{
		PlaceholderAuth:          "Authorization",
		RequirePlaceholderPrefix: "Bearer ph_",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user"},
	})
	require.ErrorContains(t, err, "missing placeholder credentials")
}

func TestValidateRequestRejectsWrongPlaceholderPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer real_token")

	err := validateRequest(req, config.Service{
		PlaceholderAuth:          "Authorization",
		RequirePlaceholderPrefix: "Bearer ph_",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user"},
	})
	require.ErrorContains(t, err, "placeholder credentials do not match required prefix")
}

func TestValidateRequestAcceptsCookiePlaceholder(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.AddCookie(&http.Cookie{Name: "websudo_ph", Value: "ph_demo"})

	err := validateRequest(req, config.Service{
		PlaceholderAuth:          "cookie:websudo_ph",
		RequirePlaceholderPrefix: "ph_",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user"},
	})
	require.NoError(t, err)
}

func TestHandleRequestCanInjectCookieCredentials(t *testing.T) {
	t.Setenv("GITHUB_SESSION", "live_session")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "cookie:websudo_ph",
			InjectAuth:               "env:GITHUB_SESSION",
			InjectAuthTarget:         "cookie:_session",
			RequirePlaceholderPrefix: "ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.AddCookie(&http.Cookie{Name: "websudo_ph", Value: "ph_demo"})
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "https", outReq.URL.Scheme)
	require.Equal(t, "upstream.internal", outReq.URL.Host)
	require.Equal(t, "/api/user", outReq.URL.Path)
	require.Contains(t, outReq.Header.Get("Cookie"), "_session=live_session")
	require.Contains(t, outReq.Header.Get("Cookie"), "theme=dark")
	require.NotContains(t, outReq.Header.Get("Cookie"), "websudo_ph=")
}

func TestHandleRequestCanMoveHeaderPlaceholderToCookieInjection(t *testing.T) {
	t.Setenv("GITHUB_SESSION", "live_session")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_SESSION",
			InjectAuthTarget:         "cookie:_session",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Empty(t, outReq.Header.Get("Authorization"))
	require.Equal(t, "_session=live_session", outReq.Header.Get("Cookie"))
}

func TestHandleRequestForwardProxyReplacesPlaceholderCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "https", outReq.URL.Scheme)
	require.Equal(t, "upstream.internal", outReq.URL.Host)
	require.Equal(t, "/api/user", outReq.URL.Path)
	require.Equal(t, "Bearer live_token", outReq.Header.Get("Authorization"))
}

func TestHandleRequestPassesThroughUnknownHostByDefault(t *testing.T) {
	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://gitlab.com/user", nil)
	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Same(t, req, outReq)
	require.Equal(t, "gitlab.com", outReq.URL.Host)
}

func TestHandleRequestRejectsUnknownHostWhenUnconfiguredDestinationsDisabled(t *testing.T) {
	srv := New(&config.Config{
		BlockUnconfiguredDestinations: true,
		Services: map[string]config.Service{
			"github": {
				MatchHost:                "api.github.com",
				BaseURL:                  "https://upstream.internal",
				PlaceholderAuth:          "Authorization",
				InjectAuth:               "env:GITHUB_TOKEN",
				RequirePlaceholderPrefix: "Bearer ph_",
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gitlab.com/user", nil)
	_, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHandleRequestMatchesVariantByPlaceholderAndOverridesAllowedPaths(t *testing.T) {
	t.Setenv("GITHUB_TOKEN_REPO", "Bearer repo_token")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
			Variants: []config.Variant{{
				Name:                "repo-write",
				PlaceholderContains: "repo_write",
				AllowedPaths:        []string{"/repos/*/*"},
				InjectAuth:          "env:GITHUB_TOKEN_REPO",
			}},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/repos/fopina/websudo", nil)
	req.Header.Set("Authorization", "Bearer ph_repo_write_123")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "Bearer repo_token", outReq.Header.Get("Authorization"))
	require.Equal(t, "/api/repos/fopina/websudo", outReq.URL.Path)
}

func TestHandleRequestRejectsPathWhenPlaceholderDoesNotMatchVariant(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
			Variants: []config.Variant{{
				Name:                "repo-write",
				PlaceholderContains: "repo_write",
				AllowedPaths:        []string{"/repos/*/*"},
			}},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/repos/fopina/websudo", nil)
	req.Header.Set("Authorization", "Bearer ph_readonly_123")

	_, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHandleRequestReverseProxyModeUsesRoutePrefix(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "Bearer live_token")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			RoutePrefix:              "/github",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://websudo.local/github/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "https", outReq.URL.Scheme)
	require.Equal(t, "upstream.internal", outReq.URL.Host)
	require.Equal(t, "/api/user", outReq.URL.Path)
	require.Equal(t, "Bearer live_token", outReq.Header.Get("Authorization"))
}

func TestHandleRequestReverseProxyVariantUsesDifferentAllowedPathAndCredential(t *testing.T) {
	t.Setenv("GITHUB_REPO_TOKEN", "Bearer repo_token")

	srv := New(&config.Config{Services: map[string]config.Service{
		"github": {
			RoutePrefix:              "/github",
			BaseURL:                  "https://upstream.internal/api",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:GITHUB_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			AllowedMethods:           []string{http.MethodGet},
			AllowedPaths:             []string{"/user"},
			Variants: []config.Variant{{
				Name:                "repo-write",
				PlaceholderContains: "repo_write",
				AllowedPaths:        []string{"/repos/*/*"},
				InjectAuth:          "env:GITHUB_REPO_TOKEN",
			}},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://websudo.local/github/repos/fopina/websudo", nil)
	req.Header.Set("Authorization", "Bearer ph_repo_write_123")

	outReq, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.Nil(t, resp)
	require.Equal(t, "/api/repos/fopina/websudo", outReq.URL.Path)
	require.Equal(t, "Bearer repo_token", outReq.Header.Get("Authorization"))
}
