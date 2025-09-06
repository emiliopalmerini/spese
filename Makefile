APP_NAME := spese
WORKER_NAME := spese-worker
PKG := ./...
BIN := bin/$(APP_NAME)
WORKER_BIN := bin/$(WORKER_NAME)

.PHONY: all setup tidy fmt vet lint test build build-worker build-all run run-worker clean docker-build docker-up docker-logs docker-down up smoke cover oauth-init sqlc-generate

all: build-all

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

test:
	go test -race -cover $(PKG)

build:
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(BIN) ./cmd/spese

build-worker:
	CGO_ENABLED=0 go build -ldflags='-s -w' -o $(WORKER_BIN) ./cmd/spese-worker

build-all: build build-worker

run:
	go run ./cmd/spese

run-worker:
	go run ./cmd/spese-worker

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
	go test -coverprofile=coverage.out ./internal/core ./internal/http ./internal/sheets/memory
	go tool cover -func=coverage.out | tail -n1 | awk '{print $$3}' | grep -qx '100.0%' && echo "Coverage 100%" || (echo "Coverage below 100%" && go tool cover -func=coverage.out && exit 1)

oauth-init:
	@echo "Starting OAuth flow (redirect to http://localhost:$${OAUTH_REDIRECT_PORT:-8085}/callback)"
	@echo "Token will be saved to: $${GOOGLE_OAUTH_TOKEN_FILE:-token.json}"
	go run ./cmd/oauth-init

oauth-init-docker:
	@echo "Starting OAuth flow in Docker container"
	@echo "The OAuth callback server will run on port $${OAUTH_REDIRECT_PORT:-8085}"
	@echo "Open the displayed URL in your browser on the HOST machine"
	@echo "Token will be saved to .secrets/token.json"
	@mkdir -p .secrets
	docker compose --profile oauth up --build oauth-init

oauth-init-docker-clean:
	@echo "Cleaning up OAuth container"
	docker compose --profile oauth down oauth-init
