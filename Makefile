BIN := ./bin/aim
VERSION ?= 0.0.0-dev
LDFLAGS := -X main.version=$(VERSION)
GOVULNCHECK ?= $(shell go env GOPATH)/bin/govulncheck

.PHONY: build
build:
	go build -o $(BIN) ./cmd/aim

.PHONY: build-version
build-version:
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

.PHONY: test-architecture
test-architecture:
	scripts/check-architecture.sh

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal

.PHONY: vulncheck
vulncheck:
	"$(GOVULNCHECK)" ./...

.PHONY: shellcheck
shellcheck:
	shellcheck scripts/*.sh

.PHONY: check
check: build test vet

.PHONY: verify
verify: check test-race test-architecture

.PHONY: audit
audit: vulncheck shellcheck

.PHONY: clean
clean:
	rm -rf ./bin ./dist
