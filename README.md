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
- optional blocking of unconfigured destinations across both HTTP and HTTPS CONNECT
- method/path policy checks
- placeholder credential validation
- upstream credential injection from environment variables
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
- `allow_unconfigured_destinations`: defaults to `true`; when `false`, requests for destinations that do not match any configured service are rejected for both plain HTTP requests and HTTPS CONNECT

Both modes use the same per-service fields:
- `placeholder_auth`
- `require_placeholder_prefix`
- `inject_auth`
- `allowed_methods`
- `allowed_paths`
- `denied_paths`
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

## Validation covered by tests

- requests without placeholder credentials are rejected
- requests with non-placeholder credentials are rejected
- forward proxy requests are matched by hostname and rewritten to the configured upstream
- reverse proxy requests are matched by route prefix and rewritten to the configured upstream
- valid placeholder credentials are replaced with the configured upstream credentials
- unconfigured HTTP and HTTPS destinations pass through by default
- unconfigured HTTP and HTTPS destinations are blocked when `allow_unconfigured_destinations: false`
- placeholder token variants can select different allowed paths and injected credentials for the same service
- reverse mode also honors variant-specific path and credential overrides

## Next steps

- tighten request header forwarding rules
- add structured audit records for allow/deny decisions
- refine passthrough controls for direct reverse-proxy-only deployments
- refine config for multiple credential strategies

## Build

Check out [CONTRIBUTING.md](CONTRIBUTING.md)
