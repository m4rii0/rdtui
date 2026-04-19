GO ?= go
APP_NAME ?= rdtui
CMD_DIR ?= ./cmd/rdtui
BIN_DIR ?= bin
BIN_PATH := $(BIN_DIR)/$(APP_NAME)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X github.com/m4rii0/rdtui/internal/version.Version=$(VERSION)"

.PHONY: help build run test test-race lint vet fmt tidy clean install check build-all

help:
	@printf '%s\n' \
	  'Common targets:' \
	  '  make build      Build the binary into bin/' \
	  '  make build-all  Build binaries for darwin/linux/windows (amd64+arm64)' \
	  '  make run        Run the TUI directly with go run' \
	  '  make test       Run the Go test suite' \
	  '  make test-race  Run tests with the race detector' \
	  '  make lint       Run go vet and golangci-lint if available' \
	  '  make vet        Run go vet' \
	  '  make fmt        Format Go code' \
	  '  make tidy       Tidy go.mod/go.sum' \
	  '  make install    Install the binary with go install' \
	  '  make clean      Remove build artifacts' \
	  '  make check      Run fmt, lint, test, and build'

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(LDFLAGS) -o $(BIN_PATH) $(CMD_DIR)

build-all:
	@mkdir -p $(BIN_DIR)
	@printf '%s\n' 'Building for all platforms...'
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-linux-arm64 $(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-windows-amd64.exe $(CMD_DIR)
	GOOS=windows GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME)-windows-arm64.exe $(CMD_DIR)
	@printf '%s\n' 'Done. Binaries in $(BIN_DIR)/'

run:
	$(GO) run $(LDFLAGS) $(CMD_DIR)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint: vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		printf '%s\n' 'golangci-lint not installed; go vet completed'; \
	fi

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

install:
	$(GO) install $(LDFLAGS) $(CMD_DIR)

clean:
	rm -rf $(BIN_DIR)

check: fmt lint test build
