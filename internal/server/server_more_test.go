package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMatchRoutePrefersRoutePrefixOverHostMatch(t *testing.T) {
	srv := New(&config.Config{Services: map[string]config.Service{
		"forward": {
			MatchHost:                "api.github.com",
			BaseURL:                  "https://forward.internal",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:FORWARD_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
		},
		"reverse": {
			RoutePrefix:              "/github",
			BaseURL:                  "https://reverse.internal",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:REVERSE_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/github/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	matched, err := srv.matchRoute(req)
	require.NoError(t, err)
	require.Equal(t, "reverse", matched.serviceName)
	require.Equal(t, "/user", matched.path)
}

func TestMatchRouteUsesVariantInReverseMode(t *testing.T) {
	srv := New(&config.Config{Services: map[string]config.Service{
		"reverse": {
			RoutePrefix:              "/github",
			BaseURL:                  "https://reverse.internal",
			PlaceholderAuth:          "Authorization",
			InjectAuth:               "env:BASE_TOKEN",
			RequirePlaceholderPrefix: "Bearer ph_",
			Variants: []config.Variant{{
				Name:                "admin",
				PlaceholderContains: "admin",
				AllowedPaths:        []string{"/orgs/*"},
				InjectAuth:          "env:ADMIN_TOKEN",
			}},
		},
	}})

	req := httptest.NewRequest(http.MethodGet, "http://websudo.local/github/orgs/fopina", nil)
	req.Header.Set("Authorization", "Bearer ph_admin_123")

	matched, err := srv.matchRoute(req)
	require.NoError(t, err)
	require.Equal(t, "admin", matched.variantName)
	require.Equal(t, "env:ADMIN_TOKEN", matched.service.InjectAuth)
	require.Equal(t, "/orgs/fopina", matched.path)
}

func TestValidateRequestRejectsDeniedPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	err := validateRequest(req, config.Service{
		PlaceholderAuth:          "Authorization",
		RequirePlaceholderPrefix: "Bearer ph_",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user*"},
		DeniedPaths:              []string{"/user/emails"},
	})
	require.ErrorContains(t, err, "denied")
}

func TestValidateRequestRejectsMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer ph_demo")

	err := validateRequest(req, config.Service{
		PlaceholderAuth:          "Authorization",
		RequirePlaceholderPrefix: "Bearer ph_",
		AllowedMethods:           []string{http.MethodGet},
		AllowedPaths:             []string{"/user"},
	})
	require.ErrorContains(t, err, "method POST is not allowed")
}

func TestJoinURLPath(t *testing.T) {
	require.Equal(t, "/user", joinURLPath("", "/user"))
	require.Equal(t, "/api", joinURLPath("/api", "/"))
	require.Equal(t, "/api/user", joinURLPath("/api", "/user"))
}
