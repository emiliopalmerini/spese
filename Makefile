APP_NAME := spese
PKG := ./...
BIN := bin/$(APP_NAME)

.PHONY: all setup tidy fmt vet lint test build run clean docker-build docker-up docker-logs docker-down up

all: build

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

run:
	go run ./cmd/spese

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
