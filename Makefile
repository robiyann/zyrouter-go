GO ?= $(shell which /usr/local/go/bin/go 2>/dev/null || which go)
BINARY_NAME := zyrouter
BACKEND_DIR := backend
VERSION ?= $(shell git describe --tags --always 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "1.0.0")
LDFLAGS := -s -w -X 'zyrouter/backend/internal/updater.CurrentVersion=$(VERSION)'

.PHONY: all build clean test run setup help

all: build

## build — compile unified zyrouter binary in backend/zyrouter
build:
	cd $(BACKEND_DIR) && $(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/zyrouter/
	@echo "✓ Build complete: $(BACKEND_DIR)/$(BINARY_NAME) ($(VERSION))"

## run — start zyrouter via PM2 or directly
run: build
	./$(BACKEND_DIR)/$(BINARY_NAME)

## test — run all unit tests across backend packages
test:
	cd $(BACKEND_DIR) && $(GO) test ./...

## setup — install backend Go dependencies and prepare environment
setup:
	cd $(BACKEND_DIR) && $(GO) mod download
	@mkdir -p bin
	@echo "✓ Dependencies ready"

## clean — clean compiled binaries
clean:
	rm -f $(BACKEND_DIR)/$(BINARY_NAME) $(BACKEND_DIR)/$(BINARY_NAME).exe

## help — display available commands
help:
	@echo "Zyrouter All-in-One — Available Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  make /' | sed 's/ — /  - /'
