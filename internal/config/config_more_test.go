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

func TestNormalizeServiceRequiresCookieKeyForLogin(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Login: LoginConfig{
			Path:          "/session",
			UsernameField: "login",
			PasswordField: "password",
			Username:      "env:UPSTREAM_USER",
			Password:      "env:UPSTREAM_PASS",
		},
	})
	require.ErrorContains(t, err, "requires cookie_encryption_key")
}

func TestNormalizeServiceRequiresLoginPathWhenFieldsPresent(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Login: LoginConfig{
			UsernameField: "login",
		},
	})
	require.ErrorContains(t, err, "login fields require login.path")
}

func TestNormalizeServiceAllowsLoginWithoutInjectedAuth(t *testing.T) {
	svc, err := normalizeService("github", Service{
		MatchHost:           "github.com",
		BaseURL:             "https://github.com",
		CookieEncryptionKey: "env:COOKIE_SECRET",
		Login: LoginConfig{
			Path:          "/session",
			UsernameField: "login",
			PasswordField: "password",
			Username:      "env:UPSTREAM_USER",
			Password:      "env:UPSTREAM_PASS",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "/session", svc.Login.Path)
}
