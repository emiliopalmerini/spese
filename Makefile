APP_NAME := spese
PKG := ./...
BIN := bin/$(APP_NAME)

.PHONY: all help setup tidy fmt vet lint test build run clean dev nix-build nix-docker smoke cover sqlc-generate refresh-categories

all: help

help: ## Show this help message
	@echo "Spese - Expense Tracking Application"
	@echo ""
	@echo "Available commands:"
	@echo ""
	@echo "Nix Commands:"
	@echo "  dev            Enter nix development shell"
	@echo "  nix-build      Build binary with nix (result/bin/spese)"
	@echo "  nix-docker     Build OCI image with nix"
	@echo ""
	@echo "Build Commands:"
	@echo "  build          Build main application (bin/spese)"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Development Commands:"
	@echo "  run            Run application locally"
	@echo "  setup          Setup development environment"
	@echo "  tidy           Run go mod tidy"
	@echo ""
	@echo "Code Quality Commands:"
	@echo "  fmt            Format Go code"
	@echo "  vet            Run go vet"
	@echo "  lint           Run linter (golangci-lint)"
	@echo "  test           Run tests with race detector"
	@echo "  cover          Run coverage tests"
	@echo "  smoke          Run smoke tests"
	@echo ""
	@echo "Database Commands:"
	@echo "  sqlc-generate  Generate sqlc code from queries"
	@echo "  refresh-categories  Clear and reload category cache"
	@echo ""
	@echo "Examples:"
	@echo "  make dev            # Enter nix shell"
	@echo "  make build          # Build the application"
	@echo "  make run            # Start the app"
	@echo ""

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
	@echo "golangci-lint optional. Skipping if not installed."
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || true

test: fmt
	go test -race -cover $(PKG)

build: fmt
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(BIN) ./cmd/spese

run:
	go run ./cmd/spese

sqlc-generate:
	@echo "Generating sqlc code..."
	sqlc generate

clean:
	rm -rf bin result result-*

smoke:
	bash scripts/smoke.sh

cover:
	@echo "Running coverage for selected packages..."
	go test -coverprofile=coverage.out ./internal/core ./internal/http
	go tool cover -func=coverage.out | tail -n1 | awk '{print $$3}' | grep -qx '100.0%' && echo "Coverage 100%" || (echo "Coverage below 100%" && go tool cover -func=coverage.out && exit 1)

refresh-categories:
	@echo "Refreshing category cache from Google Sheets"
	@echo "This will clear the SQLite category cache and reload from Google Sheets"
	@sqlite3 $${SQLITE_DB_PATH:-./data/spese.db} "DELETE FROM primary_categories; DELETE FROM secondary_categories; DELETE FROM income_categories;" || echo "Could not clear cache directly"
	@echo "Categories cache cleared. The app will reload categories on next sync check."
