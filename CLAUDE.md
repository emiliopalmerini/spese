# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build
make build              # Build main app (bin/spese)
make build-worker       # Build sync worker (bin/spese-worker)
make build-recurring-worker  # Build recurring worker (bin/recurring-worker)
make build-all          # Build all binaries

# Run locally
make run                # Run all apps (spese + workers)
make run-spese          # Run main app only
make run-worker         # Run sync worker
make run-recurring-worker    # Run recurring expenses worker

# Testing
make test               # Run tests with race detector and coverage
make cover              # Run coverage (requires 100% for core/http packages)
make smoke              # Run smoke tests (scripts/smoke.sh)

# Code quality
make fmt                # Format code (gofmt -s -w .)
make vet                # Run go vet
make lint               # Run golangci-lint (optional)

# Docker
make docker-up          # Start full stack (app + workers + RabbitMQ)
make docker-down        # Stop containers
make docker-logs        # Follow logs

# Database
make sqlc-generate      # Regenerate sqlc code after modifying queries.sql or schema.sql
```

## Architecture

### Three Services Architecture

1. **Main App** (`cmd/spese`): HTTP server with HTMX frontend for expense/income tracking. Saves to SQLite and publishes AMQP messages.

2. **Sync Worker** (`cmd/spese-worker`): Consumes AMQP messages and syncs expenses to Google Sheets. Handles category caching from Sheets.

3. **Recurring Worker** (`cmd/recurring-worker`): Periodically processes recurrent expense configurations, creating actual expenses when due.

### Data Flow

```
User → HTTP Server → SQLite → AMQP Message → Sync Worker → Google Sheets
                                    ↓
                         Recurring Worker (creates expenses from recurrent configs)
```

### Key Packages

- `internal/core`: Domain entities (Expense, Income, RecurrentExpenses, Money, Date) with validation
- `internal/sheets/ports.go`: Port interfaces (ExpenseWriter, TaxonomyReader, DashboardReader, etc.)
- `internal/sheets/google`: Google Sheets adapter implementation
- `internal/storage`: SQLite repository, sqlc-generated queries, migrations
- `internal/adapters`: SQLiteAdapter implementing port interfaces
- `internal/http`: HTTP server with HTMX handlers
- `internal/amqp`: RabbitMQ client for async messaging
- `internal/services`: ExpenseService (creates + publishes), RecurringProcessor
- `internal/worker`: SyncWorker (consumes messages, syncs to Sheets)

### SQLite Schema

Key tables: `expenses`, `incomes`, `recurrent_expenses`, `primary_categories`, `secondary_categories`, `income_categories`

Expenses use cents for amounts (`amount_cents`) and track sync status (`pending`, `synced`, `error`).

### Backend Modes

- `DATA_BACKEND=sqlite`: Local SQLite with async Google Sheets sync (recommended)
- `DATA_BACKEND=sheets`: Direct Google Sheets integration

## Configuration

Copy `.env.example` to `.env`. Key variables:
- `GOOGLE_SPREADSHEET_ID`: Target spreadsheet
- `GOOGLE_SHEET_NAME`: Base name (year prefixed automatically, e.g., "Expenses" → "2025 Expenses")
- `DATA_BACKEND`: `sqlite` or `sheets`
- `SQLITE_DB_PATH`: Default `./data/spese.db`
- `AMQP_URL`: RabbitMQ connection string
