APP_NAME := spese
PKG := ./...
BIN := bin/$(APP_NAME)

.PHONY: all help setup tidy fmt vet lint test build run run-local smoke-local clean dev nix-build nix-docker

all: help

help: ## Show this help message
	@echo "Spese - Net Worth + Expense Tracking"
	@echo ""
	@echo "Available commands:"
	@echo ""
	@echo "Nix:"
	@echo "  dev            Enter nix development shell"
	@echo "  nix-build      Build binary with nix (result/bin/spese)"
	@echo "  nix-docker     Build OCI image with nix"
	@echo ""
	@echo "Build:"
	@echo "  build          Build main application (bin/spese)"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Run:"
	@echo "  run            Run application locally"
	@echo "  run-local      Run with Honker worker mirroring to tmp/local-sheet.json"
	@echo "  smoke-local    Smoke-test local worker output"
	@echo ""
	@echo "Code quality:"
	@echo "  fmt            Format Go code"
	@echo "  vet            Run go vet"
	@echo "  lint           Run linter (golangci-lint)"
	@echo "  test           Run tests with race detector"
	@echo "  tidy           Run go mod tidy"

setup:
	@echo "Run 'nix develop' or 'make dev' to enter development shell"

dev:
	nix develop

nix-build:
	nix build

nix-docker:
	nix build .#docker

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

vet:
	go vet $(PKG)

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed; skipping"

test: fmt
	go test -race -cover $(PKG)

build: fmt
	go build -ldflags='-s -w' -o $(BIN) ./cmd/spese

run:
	go run ./cmd/spese

run-local:
	mkdir -p tmp
	SPESE_DB_PATH=$${SPESE_DB_PATH:-tmp/spese-local.db} SPESE_SHEET_MIRROR_BACKEND=local SPESE_LOCAL_SHEET_PATH=$${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json} go run ./cmd/spese

smoke-local:
	SPESE_SHEET_MIRROR_BACKEND=local SPESE_LOCAL_SHEET_PATH=$${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json} scripts/smoke.sh

clean:
	rm -rf bin result result-*
