APP_NAME := spese
PKG := ./...
BIN := bin/$(APP_NAME)

.PHONY: all help setup tidy fmt vet lint test build run logs clean docker-build docker-up docker-logs docker-down up smoke cover sqlc-generate refresh-categories

all: help

help: ## Show this help message
	@echo "Spese - Expense Tracking Application"
	@echo ""
	@echo "Available commands:"
	@echo ""
	@echo "Build Commands:"
	@echo "  build          Build main application"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Development Commands:"
	@echo "  run            Run application locally"
	@echo "  logs           Watch logs (Docker)"
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
	@echo "Docker Commands:"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-up      Start services with Docker Compose"
	@echo "  docker-logs    View Docker logs"
	@echo "  docker-down    Stop Docker services"
	@echo "  up             Format, build, test, and start containers"
	@echo ""
	@echo "Database Commands:"
	@echo "  sqlc-generate  Generate sqlc code from queries"
	@echo "  refresh-categories  Clear and reload category cache"
	@echo ""
	@echo "Examples:"
	@echo "  make build          # Build the application"
	@echo "  make run            # Start the app"
	@echo "  make docker-up      # Start with Docker"
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

run:
	go run ./cmd/spese

logs:
	docker compose logs -f $(APP_NAME)

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

up: fmt build vet test docker-up
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
	@sqlite3 $${SQLITE_DB_PATH:-./data/spese.db} "DELETE FROM primary_categories; DELETE FROM secondary_categories; DELETE FROM income_categories;" || echo "Could not clear cache directly"
	@echo "Categories cache cleared. The app will reload categories on next sync check."
