package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeServiceRequiresMatcher(t *testing.T) {
	_, err := normalizeService("github", Service{
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.ErrorContains(t, err, "must define either match_host or route_prefix")
}

func TestNormalizeServiceNormalizesRoutePrefix(t *testing.T) {
	svc, err := normalizeService("github", Service{
		RoutePrefix:     "github",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
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

func TestNormalizeServiceRequiresLoginPathWhenFieldsPresent(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Login: LoginConfig{
			UsernameField: "login",
		},
	}, t.TempDir())
	require.ErrorContains(t, err, "login fields require login.path")
}

func TestNormalizeServiceAllowsLoginWithoutInjectedAuth(t *testing.T) {
	svc, err := normalizeService("github", Service{
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
		Login: LoginConfig{
			Path:          "/session",
			UsernameField: "login",
			PasswordField: "password",
			Username:      "env:UPSTREAM_USER",
			Password:      "env:UPSTREAM_PASS",
		},
	}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "/session", svc.Login.Path)
	require.NotEmpty(t, svc.CookieEncryptionKeyPath)
}

func TestNormalizeServiceGeneratesDefaultCookieKeyPath(t *testing.T) {
	tmp := t.TempDir()
	svc, err := normalizeService("github", Service{
		MatchHost:        "github.com",
		BaseURL:          "https://github.com",
		PlaceholderAuth:  "cookie:websudo_ph",
		InjectAuth:       "env:GITHUB_SESSION",
		InjectAuthTarget: "cookie:user_session",
	}, tmp)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmp, ".github.cookie-encryption.key"), svc.CookieEncryptionKeyPath)
	data, err := os.ReadFile(svc.CookieEncryptionKeyPath)
	require.NoError(t, err)
	require.NotEmpty(t, string(data))
}

func TestNormalizeServiceResolvesRelativeCookieKeyPath(t *testing.T) {
	tmp := t.TempDir()
	svc, err := normalizeService("github", Service{
		MatchHost:               "github.com",
		BaseURL:                 "https://github.com",
		PlaceholderAuth:         "cookie:websudo_ph",
		InjectAuth:              "env:GITHUB_SESSION",
		InjectAuthTarget:        "cookie:user_session",
		CookieEncryptionKeyPath: "secrets/websudo.key",
	}, tmp)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmp, "secrets", "websudo.key"), svc.CookieEncryptionKeyPath)
}
