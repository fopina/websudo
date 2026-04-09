package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeServiceRequiresMatcher(t *testing.T) {
	_, err := normalizeService("github", Service{
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	})
	require.ErrorContains(t, err, "must define either match_host or route_prefix")
}

func TestNormalizeServiceNormalizesRoutePrefix(t *testing.T) {
	svc, err := normalizeService("github", Service{
		RoutePrefix:     "github",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	})
	require.NoError(t, err)
	require.Equal(t, "/github", svc.RoutePrefix)
	require.Equal(t, "Bearer ph_", svc.RequirePlaceholderPrefix)
}

func TestInjectedAuthValueRejectsUnsupportedSource(t *testing.T) {
	_, err := Service{InjectAuth: "literal:token"}.InjectedAuthValue()
	require.ErrorContains(t, err, "unsupported inject_auth source")
}

func TestInjectedAuthValueRejectsEmptyEnv(t *testing.T) {
	_, err := Service{InjectAuth: "env:MISSING_TOKEN"}.InjectedAuthValue()
	require.ErrorContains(t, err, "resolved empty value")
}
