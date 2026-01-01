APP_NAME := spese
WORKER_NAME := spese-worker
RECURRING_WORKER_NAME := recurring-worker
PKG := ./...
BIN := bin/$(APP_NAME)
WORKER_BIN := bin/$(WORKER_NAME)
RECURRING_WORKER_BIN := bin/$(RECURRING_WORKER_NAME)

.PHONY: all help setup tidy fmt vet lint test build build-worker build-recurring-worker build-all run run-spese run-worker run-recurring-worker logs clean docker-build docker-up docker-logs docker-down up smoke cover sqlc-generate refresh-categories

all: help

help: ## Show this help message
	@echo "🏷️  Spese - Expense Tracking Application"
	@echo ""
	@echo "📋 Available commands:"
	@echo ""
	@echo "🏗️  Build Commands:"
	@echo "  build                  Build main application"
	@echo "  build-worker           Build background sync worker"
	@echo "  build-recurring-worker Build recurring expenses worker"
	@echo "  build-all              Build all (app + both workers)"
	@echo "  clean                  Remove build artifacts"
	@echo ""
	@echo "🚀 Development Commands:"
	@echo "  run                    Run all applications locally"
	@echo "  run-spese              Run main application locally"
	@echo "  run-worker             Run background sync worker locally"
	@echo "  run-recurring-worker   Run recurring expenses worker locally"
	@echo "  logs                   Watch logs for all services (Docker)"
	@echo "  setup         Setup development environment"
	@echo "  tidy          Run go mod tidy"
	@echo ""
	@echo "🧪 Code Quality Commands:"
	@echo "  fmt           Format Go code"
	@echo "  vet           Run go vet"
	@echo "  lint          Run linter (golangci-lint)"
	@echo "  test          Run tests with race detector"
	@echo "  cover         Run coverage tests"
	@echo "  smoke         Run smoke tests"
	@echo ""
	@echo "🐳 Docker Commands:"
	@echo "  docker-build  Build Docker image"
	@echo "  docker-up     Start services with Docker Compose"
	@echo "  docker-logs   View Docker logs"
	@echo "  docker-down   Stop Docker services"
	@echo "  up            Format, build, test, and start containers"
	@echo ""
	@echo "🗄️  Database Commands:"
	@echo "  sqlc-generate Generate sqlc code from queries"
	@echo "  refresh-categories  Clear and reload category cache from Google Sheets"
	@echo ""
	@echo "💡 Examples:"
	@echo "  make build-all              # Build everything"
	@echo "  make run                    # Start all apps"
	@echo "  make run-spese              # Start main app only"
	@echo "  make docker-up              # Start with Docker"
	@echo "  make refresh-categories     # Force category refresh"
	@echo ""

setup:
	@echo "Nothing to setup locally yet. Optionally: pre-commit install"

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

build-worker: fmt
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(WORKER_BIN) ./cmd/spese-worker

build-recurring-worker: fmt
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(RECURRING_WORKER_BIN) ./cmd/recurring-worker

build-all: fmt build build-worker build-recurring-worker

run: build-all
	@echo "Starting all applications..."
	@go run ./cmd/spese & go run ./cmd/spese-worker & go run ./cmd/recurring-worker & wait

run-spese:
	go run ./cmd/spese

run-worker:
	go run ./cmd/spese-worker

run-recurring-worker:
	go run ./cmd/recurring-worker

logs:
	docker compose logs -f $(APP_NAME) $(WORKER_NAME) $(RECURRING_WORKER_NAME)

sqlc-generate:
	@echo "Generating sqlc code..."
	sqlc generate

clean:
	rm -rf bin

docker-build:
	docker build -t $(APP_NAME):dev .

docker-up:
	docker compose up -d --build

docker-logs:
	docker compose logs -f $(APP_NAME)

docker-down:
	docker compose down

up: fmt build-all vet test docker-up
	@echo "Formatted, built, vetted, tested, and started containers."

smoke:
	bash scripts/smoke.sh

cover:
	@echo "Running coverage for selected packages..."
	go test -coverprofile=coverage.out ./internal/core ./internal/http
	go tool cover -func=coverage.out | tail -n1 | awk '{print $$3}' | grep -qx '100.0%' && echo "Coverage 100%" || (echo "Coverage below 100%" && go tool cover -func=coverage.out && exit 1)

refresh-categories:
	@echo "Refreshing category cache from Google Sheets"
	@echo "This will clear the SQLite category cache and reload from Google Sheets"
	@echo "Note: Requires worker to be running"
	@sqlite3 $${SQLITE_DB_PATH:-./data/spese.db} "DELETE FROM categories;" || echo "Could not clear cache directly"
	@echo "Categories cache cleared. The worker will reload categories on next sync check."
