package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeServiceRequiresMatcher(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.ErrorContains(t, err, "must define either match_host or route_prefix")
}

func TestNormalizeServiceNormalizesRoutePrefix(t *testing.T) {
	svc, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		RoutePrefix:     "github",
		BaseURL:         "https://api.github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "/github", svc.RoutePrefix)
	require.Equal(t, "Bearer ph_", svc.RequirePlaceholderPrefix)
}

func TestNormalizeServiceRequiresAuthMode(t *testing.T) {
	_, err := normalizeService("github", Service{
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.ErrorContains(t, err, "missing auth_mode")
}

func TestNormalizeServiceRejectsUnsupportedAuthMode(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        "magic",
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
	}, t.TempDir())
	require.ErrorContains(t, err, `unsupported auth_mode "magic"`)
}

func TestInjectedAuthValueRejectsUnsupportedSource(t *testing.T) {
	_, err := Service{InjectAuth: "literal:token"}.InjectedAuthValue()
	require.ErrorContains(t, err, "unsupported inject_auth source")
}

func TestInjectedAuthValueRejectsEmptyEnv(t *testing.T) {
	_, err := Service{InjectAuth: "env:MISSING_TOKEN"}.InjectedAuthValue()
	require.ErrorContains(t, err, "resolved empty value")
}

func TestNormalizeServiceRejectsHeaderAuthWithLoginFields(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:        AuthModeHeader,
		MatchHost:       "github.com",
		BaseURL:         "https://github.com",
		PlaceholderAuth: "Authorization",
		InjectAuth:      "env:GITHUB_TOKEN",
		Login: LoginConfig{
			UsernameField: "login",
		},
	}, t.TempDir())
	require.ErrorContains(t, err, "auth_mode header cannot be combined with login fields")
}

func TestNormalizeServiceRejectsHeaderAuthWithCookieEncryption(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:                AuthModeHeader,
		MatchHost:               "github.com",
		BaseURL:                 "https://github.com",
		PlaceholderAuth:         "Authorization",
		InjectAuth:              "env:GITHUB_TOKEN",
		CookieEncryptionKeyPath: "cookie.key",
	}, t.TempDir())
	require.ErrorContains(t, err, "auth_mode header cannot be combined with cookie_encryption_key")
}

func TestNormalizeServiceRejectsCookieAuthWithoutLoginPath(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:  AuthModeCookie,
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
	}, t.TempDir())
	require.ErrorContains(t, err, "auth_mode cookie requires login.path")
}

func TestNormalizeServiceRejectsCookieAuthWithoutPlaceholderCredentials(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:  AuthModeCookie,
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
	require.ErrorContains(t, err, "auth_mode cookie requires login.placeholder_username and login.placeholder_password")
}

func TestNormalizeServiceRejectsPartialLoginPlaceholderCredentials(t *testing.T) {
	_, err := normalizeService("github", Service{
		AuthMode:  AuthModeCookie,
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
		Login: LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			PlaceholderUsername: "app",
			Username:            "env:UPSTREAM_USER",
			Password:            "env:UPSTREAM_PASS",
		},
	}, t.TempDir())
	require.ErrorContains(t, err, "placeholder_username and placeholder_password")
}

func TestNormalizeServiceAllowsLoginWithoutInjectedAuth(t *testing.T) {
	svc, err := normalizeService("github", Service{
		AuthMode:  AuthModeCookie,
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
		Login: LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			PlaceholderUsername: "app",
			PlaceholderPassword: "app",
			Username:            "env:UPSTREAM_USER",
			Password:            "env:UPSTREAM_PASS",
		},
	}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "/session", svc.Login.Path)
	require.NotEmpty(t, svc.CookieEncryptionKeyPath)
}

func TestNormalizeServiceRejectsMixedLoginAndHeaderAuth(t *testing.T) {
	base := Service{
		AuthMode:  AuthModeCookie,
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
		Login: LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			PlaceholderUsername: "app",
			PlaceholderPassword: "app",
			Username:            "env:UPSTREAM_USER",
			Password:            "env:UPSTREAM_PASS",
		},
	}

	tests := map[string]func(*Service){
		"placeholder_auth": func(svc *Service) {
			svc.PlaceholderAuth = "Authorization"
		},
		"inject_auth": func(svc *Service) {
			svc.InjectAuth = "env:GITHUB_TOKEN"
		},
		"inject_auth_target": func(svc *Service) {
			svc.InjectAuthTarget = "Authorization"
		},
		"require_placeholder_prefix": func(svc *Service) {
			svc.RequirePlaceholderPrefix = "Bearer ph_"
		},
		"variants": func(svc *Service) {
			svc.Variants = []Variant{{
				Name:                "repo-write",
				PlaceholderContains: "repo_write",
			}}
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			svc := base
			mutate(&svc)

			_, err := normalizeService("github", svc, t.TempDir())
			require.ErrorContains(t, err, "auth_mode cookie cannot be combined")
		})
	}
}

func TestNormalizeServiceGeneratesDefaultLoginCookieKeyPath(t *testing.T) {
	tmp := t.TempDir()
	svc, err := normalizeService("github", Service{
		AuthMode:  AuthModeCookie,
		MatchHost: "github.com",
		BaseURL:   "https://github.com",
		Login: LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			PlaceholderUsername: "app",
			PlaceholderPassword: "app",
			Username:            "env:UPSTREAM_USER",
			Password:            "env:UPSTREAM_PASS",
		},
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
		AuthMode:                AuthModeCookie,
		MatchHost:               "github.com",
		BaseURL:                 "https://github.com",
		CookieEncryptionKeyPath: "secrets/websudo.key",
		Login: LoginConfig{
			Path:                "/session",
			UsernameField:       "login",
			PasswordField:       "password",
			PlaceholderUsername: "app",
			PlaceholderPassword: "app",
			Username:            "env:UPSTREAM_USER",
			Password:            "env:UPSTREAM_PASS",
		},
	}, tmp)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(tmp, "secrets", "websudo.key"), svc.CookieEncryptionKeyPath)
}
