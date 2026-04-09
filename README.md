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
- host-based service matching
- method/path policy checks
- placeholder credential validation
- upstream credential injection from environment variables
- unit tests for credential validation and replacement

## Usage

```sh
websudo serve --config config/websudo.yaml
websudo version
```

## Example configuration

```yaml
listen: 127.0.0.1:8080

services:
  github:
    match_host: api.github.com
    base_url: https://api.github.com
    placeholder_auth: Authorization
    require_placeholder_prefix: "Bearer ph_"
    inject_auth: env:GITHUB_TOKEN
    allowed_methods: [GET, POST]
    allowed_paths:
      - /user
      - /repos/*
    denied_paths:
      - /user/emails
```

## Validation covered by tests

- requests without placeholder credentials are rejected
- requests with non-placeholder credentials are rejected
- valid placeholder credentials are replaced with the configured upstream credentials
- unknown hosts are rejected

## Next steps

- tighten request header forwarding rules
- add structured audit records for allow/deny decisions
- add integration tests against a live upstream test server
- refine config for multiple credential strategies

## Build

Check out [CONTRIBUTING.md](CONTRIBUTING.md)
