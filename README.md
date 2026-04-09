# websudo

[![goreference](https://pkg.go.dev/badge/github.com/fopina/websudo.svg)](https://pkg.go.dev/github.com/fopina/websudo)
[![release](https://img.shields.io/github/v/release/fopina/websudo)](https://github.com/fopina/websudo/releases)
[![downloads](https://img.shields.io/github/downloads/fopina/websudo/total.svg)](https://github.com/fopina/websudo/releases)
[![ci](https://github.com/fopina/websudo/actions/workflows/publish-main.yml/badge.svg)](https://github.com/fopina/websudo/actions/workflows/publish-main.yml)
[![test](https://github.com/fopina/websudo/actions/workflows/test.yml/badge.svg)](https://github.com/fopina/websudo/actions/workflows/test.yml)
[![codecov](https://codecov.io/github/fopina/websudo/graph/badge.svg)](https://codecov.io/github/fopina/websudo)

**websudo** is a policy-aware reverse proxy that brokers scoped access to web services without exposing real credentials.

## Status

This repository is initialized from the Go template and now contains the first project-specific scaffold:
- `serve` command
- YAML config loading
- sample service policy config
- placeholder server runtime for the proxy implementation

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
    base_url: https://api.github.com
    allowed_methods: [GET, POST]
    allowed_paths:
      - /user
      - /repos/*
    denied_paths:
      - /user/emails
    headers_allow: [Accept, Content-Type, User-Agent]
    inject_auth: env:GITHUB_TOKEN
```

## Next steps

- implement request routing and upstream proxying
- enforce method/path policy checks
- inject scoped upstream authentication
- add audit logging and response redaction
- add integration tests around proxy behavior

## Build

Check out [CONTRIBUTING.md](CONTRIBUTING.md)

### Makefile targets

```sh
make
make build
make test
make snapshot
```
