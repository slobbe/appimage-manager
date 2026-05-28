BIN := ./bin/aim
VERSION ?= 0.0.0-dev
LDFLAGS := -X main.version=$(VERSION)

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

.PHONY: test-architecture
test-architecture:
	go test ./internal/architecture

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal

.PHONY: check
check: build test vet

.PHONY: clean
clean:
	rm -rf ./bin ./dist
