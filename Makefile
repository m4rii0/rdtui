GO ?= go
APP_NAME ?= rdtui
CMD_DIR ?= ./cmd/rdtui
BIN_DIR ?= bin
BIN_PATH := $(BIN_DIR)/$(APP_NAME)

.PHONY: help build run test test-race lint vet fmt tidy clean install check

help:
	@printf '%s\n' \
	  'Common targets:' \
	  '  make build      Build the binary into bin/' \
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
	$(GO) build -o $(BIN_PATH) $(CMD_DIR)

run:
	$(GO) run $(CMD_DIR)

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
	$(GO) install $(CMD_DIR)

clean:
	rm -rf $(BIN_DIR)

check: fmt lint test build
