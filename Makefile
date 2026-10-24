.DEFAULT_GOAL := help

BIN := ./bin/aim
VERSION ?= 0.19.0-dev
ARGS ?= --help
RACE ?= 0
LDFLAGS := -X main.version=$(VERSION)
GOVULNCHECK ?= $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)/govulncheck
GOVULNCHECK_VERSION ?= v1.9.0

.PHONY: help
help: ## Show available development commands (default)
	@printf 'Usage: make [target]\n\nTargets:\n'
	@awk 'BEGIN { FS = ":.*## " } /^[a-zA-Z0-9_-]+:.*## / { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\nOptions:\n  VERSION=<version>        Set the version for build and run\n  ARGS="<arguments>"       Pass CLI arguments to run (default: --help)\n  RACE=1                   Enable the race detector for test\n  GOVULNCHECK=<path>       Override the vulnerability scanner path\n'

.PHONY: build
build: ## Build ./bin/aim
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/aim

.PHONY: run
run: ## Run aim from source; accepts ARGS="..."
	go run -ldflags "$(LDFLAGS)" ./cmd/aim $(ARGS)

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w ./cmd ./internal

.PHONY: test
test: ## Run Go tests; use RACE=1 for race detection
	go test $(if $(filter 1,$(RACE)),-race) ./...

.PHONY: check
check: build ## Check formatting, modules, build, static analysis, and architecture
	@files="$$(gofmt -l ./cmd ./internal)" || exit $$?; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		exit 1; \
	fi
	go mod tidy -diff
	go vet ./...
	@! grep -R '"github.com/slobbe/appimage-manager/internal/infra' internal/app internal/cli internal/domain --include='*.go'
	@! grep -R '"github.com/slobbe/appimage-manager/internal/cli' internal/app internal/domain internal/infra --include='*.go'
	@! grep -R '"github.com/slobbe/appimage-manager/internal/app' internal/domain --include='*.go'
	@! grep -R '"github.com/slobbe/appimage-manager/internal/domain' internal/cli --include='*.go'

.PHONY: audit
audit: ## Run vulnerability, shell lint, and installer checks (requires audit tools)
	@if [ ! -x "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found; run: make install-tools"; \
		exit 1; \
	fi
	"$(GOVULNCHECK)" ./...
	@command -v shellcheck >/dev/null 2>&1 || { echo "shellcheck not installed"; exit 1; }
	shellcheck scripts/*.sh
	sh scripts/test-install.sh

.PHONY: install-tools
install-tools: ## Install the pinned govulncheck version (requires network)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: clean
clean: ## Remove build and release output (bin/ and dist/)
	rm -rf ./bin ./dist
