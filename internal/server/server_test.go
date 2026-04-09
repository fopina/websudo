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

func TestHandleRequestReplacesPlaceholderCredentials(t *testing.T) {
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

func TestHandleRequestRejectsUnknownHost(t *testing.T) {
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
	_, resp := srv.handleRequest(req, &goproxy.ProxyCtx{})
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
