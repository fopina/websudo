projectname?=websudo

default: help

.PHONY: help
help: ## list makefile targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## build golang binary
	@go build -ldflags "-X main.version=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo dev)" -o $(projectname)

.PHONY: install
install: ## install golang binary
	@go install -ldflags "-X main.version=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo dev)"

.PHONY: run
run: ## run the app
	@go run -ldflags "-X main.version=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo dev)" main.go serve --config config/websudo.yaml

.PHONY: bootstrap
bootstrap: ## install build deps
	go generate -tags tools tools/tools.go

.PHONY: test
test: clean ## display test coverage
	go test --cover -parallel=1 -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | sort -rnk3

.PHONY: e2e
e2e: ## run end-to-end tests
ifdef WEBSUDO_E2E_COVERAGE_OUT
	WEBSUDO_E2E_COVERAGE_OUT=$(WEBSUDO_E2E_COVERAGE_OUT).forward ./tests/e2e/test_forward_proxy.py
	WEBSUDO_E2E_COVERAGE_OUT=$(WEBSUDO_E2E_COVERAGE_OUT).reverse ./tests/e2e/test_reverse_proxy.py
	WEBSUDO_E2E_COVERAGE_OUT=$(WEBSUDO_E2E_COVERAGE_OUT).defectdojo-forward ./tests/e2e/test_forward_proxy_defectdojo.py
	WEBSUDO_E2E_COVERAGE_OUT=$(WEBSUDO_E2E_COVERAGE_OUT).defectdojo-reverse ./tests/e2e/test_reverse_proxy_defectdojo.py
	@cat /dev/null > $(WEBSUDO_E2E_COVERAGE_OUT)
	@for f in $(WEBSUDO_E2E_COVERAGE_OUT).forward $(WEBSUDO_E2E_COVERAGE_OUT).reverse $(WEBSUDO_E2E_COVERAGE_OUT).defectdojo-forward $(WEBSUDO_E2E_COVERAGE_OUT).defectdojo-reverse; do \
		if [ -f "$$f" ]; then awk 'FNR == 1 && NR != 1 { next } { print }' "$$f" >> $(WEBSUDO_E2E_COVERAGE_OUT); fi; \
	done
else
	./tests/e2e/test_forward_proxy.py
	./tests/e2e/test_reverse_proxy.py
	./tests/e2e/test_forward_proxy_defectdojo.py
	./tests/e2e/test_reverse_proxy_defectdojo.py
endif

.PHONY: clean
clean: ## clean up environment
	@rm -rf coverage.out dist/ $(projectname)

.PHONY: race
race: ## display test coverage with race
	go test -v -race $(shell go list ./... | grep -v /vendor/) -coverprofile=coverage.out
	go tool cover -func=coverage.out

.PHONY: snapshot
snapshot: ## goreleaser snapshot
	goreleaser release --snapshot --clean
