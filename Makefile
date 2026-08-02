APP_NAME := spese
PKG := ./...
BIN := bin/$(APP_NAME)
WORKER_BIN := bin/$(APP_NAME)-worker

.PHONY: all help setup tidy fmt vet lint test test-go test-frontend test-e2e frontend-install frontend-build build run run-demo demo-data run-local run-worker run-worker-local run-e2e smoke-local clean dev nix-build nix-docker

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
	@echo "  frontend-build Build the embedded React SPA"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Run:"
	@echo "  run            Run application locally"
	@echo "  run-demo       Reset demo data and run the local-only application"
	@echo "  demo-data      Reset tmp/spese-demo.db with rolling example data"
	@echo "  run-local      Run web app with Rabbit sheet-sync publisher"
	@echo "  run-worker     Run Rabbit sheet-sync worker"
	@echo "  run-worker-local Run worker mirroring to tmp/local-sheet.json"
	@echo "  smoke-local    Smoke-test local worker output"
	@echo ""
	@echo "Code quality:"
	@echo "  fmt            Format Go code"
	@echo "  vet            Run go vet"
	@echo "  lint           Run linter (golangci-lint)"
	@echo "  test           Run tests with race detector"
	@echo "  test-e2e       Run Playwright browser flows"
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
	npm --prefix frontend run lint

test: test-go test-frontend

test-go:
	go test -race -cover $(PKG)

test-frontend:
	npm --prefix frontend run typecheck
	npm --prefix frontend test

test-e2e:
	npm --prefix frontend run test:e2e

frontend-install:
	npm --prefix frontend ci

frontend-build:
	npm --prefix frontend run build

build: frontend-build
	mkdir -p bin
	go build -ldflags='-s -w' -o $(BIN) ./cmd/spese
	go build -ldflags='-s -w' -o $(WORKER_BIN) ./cmd/spese-worker
	go build -ldflags='-s -w' -o bin/spese-migrate ./cmd/spese-migrate

run:
	go run ./cmd/spese

demo-data:
	go run ./cmd/spese-demo -db tmp/spese-demo.db

run-demo: demo-data
	SPESE_DB_PATH=tmp/spese-demo.db SPESE_SHEET_MIRROR_BACKEND=none go run ./cmd/spese

run-e2e:
	mkdir -p tmp
	rm -f tmp/spese-e2e-demo.db tmp/spese-e2e-demo.db-shm tmp/spese-e2e-demo.db-wal
	go run ./cmd/spese-demo -db tmp/spese-e2e-demo.db
	SPESE_HOST=127.0.0.1 SPESE_PORT=8085 SPESE_DB_PATH=tmp/spese-e2e-demo.db SPESE_SHEET_MIRROR_BACKEND=none go run ./cmd/spese

run-local:
	mkdir -p tmp
	SPESE_DB_PATH=$${SPESE_DB_PATH:-tmp/spese-local.db} SPESE_SHEET_MIRROR_BACKEND=local SPESE_LOCAL_SHEET_PATH=$${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json} SPESE_RABBITMQ_URL=$${SPESE_RABBITMQ_URL:-amqp://guest:guest@localhost:5672/} go run ./cmd/spese

run-worker:
	go run ./cmd/spese-worker

run-worker-local:
	mkdir -p tmp
	SPESE_DB_PATH=$${SPESE_DB_PATH:-tmp/spese-local.db} SPESE_SHEET_MIRROR_BACKEND=local SPESE_LOCAL_SHEET_PATH=$${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json} SPESE_RABBITMQ_URL=$${SPESE_RABBITMQ_URL:-amqp://guest:guest@localhost:5672/} go run ./cmd/spese-worker

smoke-local:
	SPESE_SHEET_MIRROR_BACKEND=local SPESE_LOCAL_SHEET_PATH=$${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json} SPESE_RABBITMQ_URL=$${SPESE_RABBITMQ_URL:-amqp://guest:guest@localhost:5672/} scripts/smoke.sh

clean:
	rm -rf bin result result-*
