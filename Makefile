BIN := ./bin/aim
VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOVULNCHECK ?= $(shell go env GOPATH)/bin/govulncheck

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/aim

.PHONY: run
run:
	go run ./cmd/aim --help

.PHONY: test
test:
	go test ./...

.PHONY: test-race
test-race:
	go test -race ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal

.PHONY: fmt-check
fmt-check:
	@files="$$(gofmt -l ./cmd ./internal)"; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		exit 1; \
	fi

.PHONY: test-architecture
test-architecture:
	@! grep -R '"aim/internal/infra' internal/app internal/cli internal/domain --include='*.go'
	@! grep -R '"aim/internal/cli' internal/app internal/domain internal/infra --include='*.go'
	@! grep -R '"aim/internal/app' internal/domain --include='*.go'
	@! grep -R '"aim/internal/domain' internal/cli --include='*.go'

.PHONY: vulncheck
vulncheck:
	@if [ ! -x "$(GOVULNCHECK)" ]; then \
		echo "govulncheck not found; run: make install-tools"; \
		exit 1; \
	fi
	"$(GOVULNCHECK)" ./...

.PHONY: shellcheck
shellcheck:
	@command -v shellcheck >/dev/null 2>&1 || { echo "shellcheck not installed"; exit 1; }
	shellcheck scripts/*.sh

.PHONY: install-tools
install-tools:
	go install golang.org/x/vuln/cmd/govulncheck@latest

.PHONY: check
check: fmt-check build test vet

.PHONY: verify
verify: check test-architecture

.PHONY: verify-heavy
verify-heavy: verify test-race audit

.PHONY: audit
audit: vulncheck shellcheck

.PHONY: clean
clean:
	rm -rf ./bin ./dist
