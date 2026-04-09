# golang template

Template based off another template: [FalcoSuessgott/golang-cli-template](https://github.com/FalcoSuessgott/golang-cli-template/)

## Content

* `goreleaser` setup with both binary and docker
* `.github` with actions ready to be used
    * [test](.github/workflows/test.yml) runs unit tests
    * [goreleaser](.github/workflows/goreleaser.yml) publishes semver tags to:
      * binaries to github releases
      * docker image to ghcr.io

## New project checklist

* [ ] Replace every .go file with the actual code :D
* [ ] Replace `github.com/fopina/golang-template` globally with new package name
    * At least `main.go` and `go.mod` should be left after previous step
* [ ] Replace `LICENSE` if MIT does not apply
* [ ] Search the project for `# TODO` to find the (minimum list of) places that need to be changed.
* [ ] Add [codecov](https://app.codecov.io/github/fopina/) token
    * `CODECOV_TOKEN` taken from link above; OR
* [ ] Replace this README.md - template below

## Notes

### Feature branch publishing

`publish-dev` workflow tests and publishes branches:
* Binaries are uploaded as artifacts to the run
* Docker image is tagged as `dev-BRANCH` and pushed to GCHR as well

## ---

# golang-template

[![goreference](https://pkg.go.dev/badge/github.com/fopina/golang-template.svg)](https://pkg.go.dev/github.com/fopina/golang-template)
[![release](https://img.shields.io/github/v/release/fopina/golang-template)](https://github.com/fopina/golang-template/releases)
[![downloads](https://img.shields.io/github/downloads/fopina/golang-template/total.svg)](https://github.com/fopina/golang-template/releases)
[![ci](https://github.com/fopina/golang-template/actions/workflows/publish-main.yml/badge.svg)](https://github.com/fopina/golang-template/actions/workflows/publish-main.yml)
[![test](https://github.com/fopina/golang-template/actions/workflows/test.yml/badge.svg)](https://github.com/fopina/golang-template/actions/workflows/test.yml)
[![codecov](https://codecov.io/github/fopina/golang-template/graph/badge.svg)](https://codecov.io/github/fopina/golang-template)


CLI to add/multiply integers.

## Usage

```sh
➜  golang-template -h
golang project template demo application

Usage:
  golang-template [flags]
  golang-template [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  example     example subcommand which adds or multiplies two given integers
  help        Help about any command
  version     Display version

Flags:
  -h, --help   help for golang-template

Use "golang-template completion [command] --help" for more information about a command.
```

```sh
➜  golang-template example 2 3 --add
5
➜  golang-template example 2 3 --multiply
6
```

## Build

Check out [CONTRIBUTING.md](CONTRIBUTING.md)

### Makefile Targets
```sh
➜  make
bootstrap                      install build deps
build                          build golang binary
clean                          clean up environment
help                           list makefile targets
install                        install golang binary
race                           display test coverage with race
run                            run the app
snapshot                       goreleaser snapshot
test                           display test coverage
```
