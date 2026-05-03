# websudo

[![goreference](https://pkg.go.dev/badge/github.com/fopina/websudo.svg)](https://pkg.go.dev/github.com/fopina/websudo)
[![release](https://img.shields.io/github/v/release/fopina/websudo)](https://github.com/fopina/websudo/releases)
[![downloads](https://img.shields.io/github/downloads/fopina/websudo/total.svg)](https://github.com/fopina/websudo/releases)
[![ci](https://github.com/fopina/websudo/actions/workflows/publish-main.yml/badge.svg)](https://github.com/fopina/websudo/actions/workflows/publish-main.yml)
[![test](https://github.com/fopina/websudo/actions/workflows/test.yml/badge.svg)](https://github.com/fopina/websudo/actions/workflows/test.yml)
[![codecov](https://codecov.io/github/fopina/websudo/graph/badge.svg)](https://codecov.io/github/fopina/websudo)

**websudo** is a policy-aware proxy that validates placeholder credentials and swaps them for real upstream credentials at the boundary.

## Status

Current v1 scaffold includes:
- proxy runtime built on `goproxy`
- forward proxy mode using hostname matching
- reverse proxy mode using explicit local route prefixes
- default passthrough for destinations that are not explicitly configured
- optional blocking of unconfigured destinations across both HTTP and HTTPS CONNECT with a default-off flag
- method/path policy checks
- placeholder credential validation
- upstream credential injection from environment variables into headers or cookies
- encrypted upstream session-cookie round-tripping for browser-style logins
- per-placeholder-token variants for the same host or route
- unit and e2e tests for credential validation, proxy routing, passthrough, and credential replacement

## Usage

```sh
websudo serve --config config/websudo.yaml
websudo version
```

## Configuration model

Each service can define one or both of:
- `match_host`: for forward proxy requests, matched by requested hostname
- `route_prefix`: for reverse proxy requests, matched by local path prefix such as `/github`

Top-level options:
- `block_unconfigured_destinations`: defaults to `false`; when `true`, requests for destinations that do not match any configured service are rejected for both plain HTTP requests and HTTPS CONNECT

Both modes use the same per-service fields:
- `placeholder_auth` (`Authorization` is shorthand for `header:Authorization`; `cookie:name` is also supported)
- `require_placeholder_prefix`
- `inject_auth`
- `inject_auth_target` (defaults to the same target as `placeholder_auth`)
- `allowed_methods`
- `allowed_paths`
- `denied_paths`
- `cookie_encryption_key` (required when using encrypted upstream cookies or login capture)
- `login.path`, `login.username_field`, `login.password_field`, `login.username`, `login.password`
- `variants`

## Example: forward proxy by hostname

```yaml
services:
  github-forward:
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    require_placeholder_prefix: "Bearer ph_"
    inject_auth: env:GITHUB_TOKEN
    allowed_methods: [GET, POST]
    allowed_paths:
      - /user
    variants:
      - name: repo-write
        placeholder_contains: repo_write
        inject_auth: env:GITHUB_REPO_TOKEN
        allowed_paths:
          - /repos/*/*
```

Behavior:
- request to `https://api.github.com/user` with `Authorization: Bearer ph_demo` uses base policy
- request to `https://api.github.com/repos/fopina/websudo` with `Authorization: Bearer ph_repo_write_123` uses the `repo-write` variant

## Example: reverse proxy by local route

```yaml
services:
  github-reverse:
    route_prefix: /github
    base_url: https://api.github.com
    placeholder_auth: Authorization
    require_placeholder_prefix: "Bearer ph_"
    inject_auth: env:GITHUB_TOKEN
    allowed_methods: [GET, POST]
    allowed_paths:
      - /user
    variants:
      - name: admin
        placeholder_contains: admin
        inject_auth: env:GITHUB_ADMIN_TOKEN
        allowed_paths:
          - /user
          - /repos/*/*
          - /orgs/*
```

Behavior:
- request to `/github/user` maps to upstream `/user`
- request to `/github/repos/fopina/websudo` only works when the placeholder token selects a variant that allows that path

## Example: browser session cookie injection

```yaml
services:
  github-browser:
    match_host: github.com
    base_url: https://github.com
    placeholder_auth: cookie:websudo_ph
    require_placeholder_prefix: "ph_"
    inject_auth: env:GITHUB_SESSION
    inject_auth_target: cookie:user_session
    cookie_encryption_key: env:WEBSUDO_COOKIE_SECRET
    allowed_methods: [GET]
    allowed_paths:
      - /settings/profile
```

Behavior:
- request with cookie `websudo_ph=ph_browser_demo` is accepted
- upstream receives `user_session=<real value from env:GITHUB_SESSION>`
- the placeholder cookie is removed before the request is forwarded
- upstream `Set-Cookie` values are encrypted before they are returned to the client
- encrypted client cookies are decrypted before upstream requests; invalid ciphertext is passed through unchanged

## Example: upstream login capture for browser sessions

```yaml
services:
  app-browser:
    route_prefix: /app
    base_url: https://internal.example.com
    cookie_encryption_key: env:WEBSUDO_COOKIE_SECRET
    allowed_methods: [GET, POST]
    allowed_paths:
      - /dashboard
    login:
      path: /session
      username_field: username
      password_field: password
      username: env:APP_LOGIN_USER
      password: env:APP_LOGIN_PASS
```

Behavior:
- POST `/app/session` does not require placeholder auth, and the rest of the service can rely on the encrypted session cookies instead
- `username` and `password` form fields are replaced with the configured upstream credentials
- upstream `Set-Cookie` headers are encrypted before they reach the client
- later client `Cookie` headers are decrypted before forwarding upstream
- if a client cookie cannot be decrypted, it is forwarded as-is

## Validation covered by tests

- requests without placeholder credentials are rejected
- requests with non-placeholder credentials are rejected
- forward proxy requests are matched by hostname and rewritten to the configured upstream
- reverse proxy requests are matched by route prefix and rewritten to the configured upstream
- valid placeholder credentials are replaced with the configured upstream credentials in the configured header or cookie target
- unconfigured HTTP and HTTPS destinations pass through by default
- unconfigured HTTP and HTTPS destinations are blocked when `block_unconfigured_destinations: true`
- placeholder token variants can select different allowed paths and injected credentials for the same service
- reverse mode also honors variant-specific path and credential overrides
- placeholder cookies can be validated and swapped for upstream session cookies
- upstream login form fields can be replaced with configured credentials on a specific login endpoint
- upstream Set-Cookie headers can be encrypted and client cookies decrypted on the way back in

## Next steps

- tighten request header forwarding rules
- add structured audit records for allow/deny decisions
- refine passthrough controls for direct reverse-proxy-only deployments
- refine config for multiple credential strategies

## Build

Check out [CONTRIBUTING.md](CONTRIBUTING.md)


## Live e2e test credentials

GitHub-backed e2e tests use `WEBSUDO_E2E_GITHUB_AUTH`.

DefectDojo-backed e2e tests use:
- `WEBSUDO_E2E_DEFECTDOJO_USERNAME`
- `WEBSUDO_E2E_DEFECTDOJO_PASSWORD`
- `WEBSUDO_E2E_COOKIE_SECRET` (optional; defaults to a test secret)

When the DefectDojo credentials are not set, those live tests are skipped.
