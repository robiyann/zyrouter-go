BINARY_NAME := zyrouter
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "1.0.0")
PORT ?= 20128
DATA_DIR ?= $(HOME)/.9router
RTK ?=
CAVEMAN ?=
PONYTAIL ?=
AUTO_UPDATE ?= false

LDFLAGS := -s -w -X 'zyrouter/backend/internal/updater.CurrentVersion=$(VERSION)'

.PHONY: build run dev version update test test-short vet bench bench-go cross docker docker-build clean help

## build — compile binary with version embedding
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/zyrouter/

## run — start proxy (PORT=20128)
run: build
	PORT=$(PORT) DATA_DIR=$(DATA_DIR) ./$(BINARY_NAME) $(if $(RTK),--rtk=$(RTK)) $(if $(CAVEMAN),--caveman=$(CAVEMAN)) $(if $(PONYTAIL),--ponytail=$(PONYTAIL)) --auto-update=$(AUTO_UPDATE)

## dev — start with go run (auto-rebuild)
dev:
	PORT=$(PORT) DATA_DIR=$(DATA_DIR) go run -ldflags="$(LDFLAGS)" ./cmd/zyrouter/ $(if $(RTK),--rtk=$(RTK)) $(if $(CAVEMAN),--caveman=$(CAVEMAN)) $(if $(PONYTAIL),--ponytail=$(PONYTAIL)) --auto-update=$(AUTO_UPDATE)

## version — display binary version info
version: build
	./$(BINARY_NAME) version

## update — check and install binary self-update
update: build
	./$(BINARY_NAME) update

## test — run all unit tests
test:
	go test ./... -v

## test-short — run tests (quiet)
test-short:
	go test ./...

## vet — run go vet static analysis
vet:
	go vet ./...

## bench — run bash comparison benchmark
bench: build
	bash benchmark/run_comparison.sh

## bench-go — run native Go high-throughput benchmark
bench-go:
	go run ./benchmark/runner.go

## cross — cross-compile Linux/macOS/Windows release binaries
cross:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64 ./cmd/zyrouter/
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-linux-arm64 ./cmd/zyrouter/
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-darwin-amd64 ./cmd/zyrouter/
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-darwin-arm64 ./cmd/zyrouter/
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME)-windows-amd64.exe ./cmd/zyrouter/
	@ls -lh $(BINARY_NAME)-*

## docker — docker compose up
docker:
	docker compose up -d

## docker-build — build Docker image only
docker-build:
	docker build -t $(BINARY_NAME) -f Dockerfile ..

## clean — remove build artifacts
clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*

## help — show targets
help:
	@echo "zyrouter — Makefile targets:"
	@grep -E '^## ' Makefile | sed 's/## /  make /' | sed 's/ — /  /'
	@echo ""
	@echo "Options:"
	@echo "  make run PORT=3000 VERSION=1.1.0"
	@echo "  make run DATA_DIR=/path/to/data"
	@echo "  make run CAVEMAN=true PONYTAIL=true AUTO_UPDATE=true"
	@echo "  make run RTK=false"
