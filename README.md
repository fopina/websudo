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
- upstream credential injection from environment variables into headers
- encrypted upstream session-cookie round-tripping for browser-style form and JSON logins
- encrypted upstream login-token round-tripping for API logins
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
- `tls.require_existing_ca`: defaults to `false`; when `true`, startup fails if CA files are missing instead of generating a local CA certificate and key for TLS interception
- `tls.ca_cert_path` and `tls.ca_key_path`: optional CA certificate/key paths; default to `~/.local/share/websudo/ca.pem` and `~/.local/share/websudo/ca-key.pem`

Path values such as `tls.ca_cert_path`, `tls.ca_key_path`, and `cookie_encryption_key_path` support `~` and `~/...` expansion to the current user's home directory. Relative `cookie_encryption_key_path` values are resolved relative to the config file.

Each service must choose one authentication mode with `auth_mode`.

`auth_mode: header` validates a client placeholder header on every request, then replaces it with the configured upstream header value. If the placeholder header is missing or does not match `require_placeholder_prefix`, the request is rejected before upstream auth is injected.

When `auth_mode: header` includes `login.path`, it uses the same login request rewrite as cookie auth but captures a token from the JSON login response instead of requiring `inject_auth`. The response token is encrypted before returning to the client, and later encrypted client auth headers are decrypted before forwarding upstream.

Header injection fields:
- `placeholder_auth` (`Authorization` is shorthand for `header:Authorization`)
- `require_placeholder_prefix`
- `inject_auth` (required unless `login.path` is configured)
- `inject_auth_target` (defaults to the same target as `placeholder_auth`)
- `variants`

Header login-token fields:
- `login.path`, `login.username_field`, `login.password_field`, `login.placeholder_username`, `login.placeholder_password`, `login.username`, `login.password`
- `login.token_field` (optional; defaults to `token`; accepts `gjson`/`sjson` path syntax such as `data.access_token` or `tokens.0.value`; escape literal dots as `data\\.access_token`)
- `cookie_encryption_key` (optional explicit secret or `env:...` override for login-token encryption)
- `cookie_encryption_key_path` (optional override for the persisted login-token secret file; defaults to a generated file next to the config for login services)

Header login-token services cannot be combined with `inject_auth` or `variants`; the upstream token comes from the login response instead of from a configured static credential or placeholder-token selector.

`auth_mode: cookie` requires `login.path`, `login.placeholder_username`, and `login.placeholder_password`. The login request validates those placeholder form/JSON credentials, rewrites only that login body to the configured upstream username/password, and encrypts upstream session cookies before returning them to the client. Later requests rely only on those encrypted cookies being decrypted before forwarding upstream; no header auth is injected outside the login request.

Cookie auth fields:
- `login.path`, `login.username_field`, `login.password_field`, `login.placeholder_username`, `login.placeholder_password`, `login.username`, `login.password`
- `cookie_encryption_key` (optional explicit secret or `env:...` override for login session-cookie encryption)
- `cookie_encryption_key_path` (optional override for the persisted login cookie secret file; defaults to a generated file next to the config for login services)

Cookie auth cannot be combined with header injection fields: `placeholder_auth`, `require_placeholder_prefix`, `inject_auth`, `inject_auth_target`, or `variants`.

Policy fields supported by both modes:
- `allowed_methods`
- `allowed_paths`
- `denied_paths`

## Example: forward proxy by hostname

```yaml
services:
  github-forward:
    auth_mode: header
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
    auth_mode: header
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

## Example: upstream login capture for browser sessions

```yaml
services:
  app-browser:
    auth_mode: cookie
    route_prefix: /app
    base_url: https://internal.example.com
    cookie_encryption_key_path: ./app-browser.cookie-key
    allowed_methods: [GET, POST]
    allowed_paths:
      - /dashboard
    login:
      path: /session
      username_field: username
      password_field: password
      placeholder_username: app
      placeholder_password: app
      username: env:APP_LOGIN_USER
      password: env:APP_LOGIN_PASS
```

Behavior:
- POST `/app/session` is intentionally exempt from header placeholder-auth gating so the login flow can establish the upstream session, and the rest of the service can rely on the encrypted session cookies instead
- `username` and `password` form fields, or top-level JSON keys with those names, must match `placeholder_username` and `placeholder_password`
- after placeholder credential validation, those fields are replaced with the configured upstream `username` and `password`
- JSON login bodies must be objects; nested JSON fields are not supported, so configured field names are treated as literal top-level keys
- upstream `Set-Cookie` headers are encrypted before they reach the client
- if `cookie_encryption_key` is omitted, websudo generates and persists a default key file next to the config (or at `cookie_encryption_key_path` if set)
- later client `Cookie` headers are decrypted before forwarding upstream
- if a client cookie cannot be decrypted, it is forwarded as-is

## Example: upstream login capture for API tokens

```yaml
services:
  app-api:
    auth_mode: header
    route_prefix: /api
    base_url: https://internal.example.com
    placeholder_auth: Authorization
    cookie_encryption_key_path: ./app-api.token-key
    allowed_methods: [GET, POST]
    allowed_paths:
      - /profile
    login:
      path: /session
      username_field: username
      password_field: password
      token_field: access_token
      placeholder_username: app
      placeholder_password: app
      username: env:APP_LOGIN_USER
      password: env:APP_LOGIN_PASS
```

Behavior:
- POST `/api/session` validates the placeholder username/password and rewrites them to the configured upstream credentials
- the upstream JSON response field selected by `login.token_field` is encrypted before returning to the client
- later requests can send the encrypted value in `Authorization`, for example `Authorization: Bearer wsenc:...`
- websudo decrypts that header before forwarding upstream, preserving the header prefix

## Validation covered by tests

- requests without placeholder credentials are rejected
- requests with non-placeholder credentials are rejected
- forward proxy requests are matched by hostname and rewritten to the configured upstream
- reverse proxy requests are matched by route prefix and rewritten to the configured upstream
- valid placeholder credentials are replaced with the configured upstream credentials in the configured header target
- unconfigured HTTP and HTTPS destinations pass through by default
- unconfigured HTTP and HTTPS destinations are blocked when `block_unconfigured_destinations: true`
- placeholder token variants can select different allowed paths and injected credentials for the same service
- reverse mode also honors variant-specific path and credential overrides
- upstream login form fields or top-level JSON keys can be validated against configured placeholder credentials and replaced with configured upstream credentials on a specific login endpoint
- nested JSON login fields are not supported
- upstream Set-Cookie headers can be encrypted and client cookies decrypted on the way back in
- undecryptable client cookies are intentionally forwarded as-is
- upstream login response tokens can be encrypted and client auth headers decrypted on the way back in
- login endpoints configured under `login.path` are intentionally allowed without header placeholder-auth gating

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
