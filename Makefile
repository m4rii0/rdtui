GO ?= go
APP_NAME ?= rdtui
CMD_DIR ?= ./cmd/rdtui
BIN_DIR ?= bin
BIN_PATH := $(BIN_DIR)/$(APP_NAME)
VHS ?= vhs
VHS_TAPE ?= docs/assets/rdtui-showcase.tape
VHS_BROWSER_LIB_DIR ?= $(HOME)/.cache/rdtui-vhs-libs/usr/lib/x86_64-linux-gnu
VERSION ?= $(shell latest_tag=$$(git tag --sort=-v:refname 2>/dev/null | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); short_hash=$$(git rev-parse --short HEAD 2>/dev/null); dirty=$$(test -z "$$(git status --porcelain 2>/dev/null)" || printf '+dirty'); if [ -n "$$latest_tag" ] && [ -n "$$short_hash" ]; then printf '%s-%s%s' "$$latest_tag" "$$short_hash" "$$dirty"; elif [ -n "$$short_hash" ]; then printf 'v0.0.0-%s%s' "$$short_hash" "$$dirty"; else printf 'dev'; fi)
LDFLAGS := -ldflags "-s -w -X github.com/m4rii0/rdtui/internal/version.Version=$(VERSION)"

.PHONY: help build run run-debug test test-race lint vet fmt fmt-check tidy tidy-check clean install check verify build-all showcase-gif

help:
	@printf '%s\n' \
	  'Common targets:' \
	  '  make build      Build the binary into bin/' \
	  '  make build-all  Build binaries for darwin/linux/windows (amd64+arm64)' \
	  '  make run        Run the TUI directly with go run' \
	  '  make run-debug  Run the TUI with debug logging (RDTUI_DEBUG=1)' \
	  '  make test       Run the Go test suite' \
	  '  make test-race  Run tests with the race detector' \
	  '  make lint       Run go vet and golangci-lint if available' \
	  '  make vet        Run go vet' \
	  '  make fmt        Format Go code' \
	  '  make tidy       Tidy go.mod/go.sum' \
	  '  make install    Install the binary with go install' \
	  '  make showcase-gif  Render the VHS showcase GIF' \
	  '  make clean      Remove build artifacts' \
	  '  make check      Run fmt, lint, test, and build' \
	  '  make verify     Verify fmt, tidy, lint, test, and build without rewriting files'

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

run-debug:
	RDTUI_DEBUG=1 $(GO) run $(LDFLAGS) $(CMD_DIR)

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

fmt-check:
	@files="$$(gofmt -l $$(git ls-files '*.go'))"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' 'Go files need formatting:'; \
		printf '%s\n' "$$files"; \
		exit 1; \
	fi

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

install:
	$(GO) install $(LDFLAGS) $(CMD_DIR)

showcase-gif:
	$(VHS) validate "$(VHS_TAPE)"
	PATH="/usr/bin:/bin:$$PATH" LD_LIBRARY_PATH="$(VHS_BROWSER_LIB_DIR)$${LD_LIBRARY_PATH:+:$$LD_LIBRARY_PATH}" $(VHS) "$(VHS_TAPE)"

clean:
	rm -rf $(BIN_DIR)

check: fmt lint test build

verify: fmt-check tidy-check lint test build
