# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Nix (recommended)
nix develop             # Enter development shell with all tools
nix build               # Build the spese binary (result/bin/spese)
nix build .#docker      # Build OCI image

# Make (inside nix develop or with Go installed)
make build              # Build main app (bin/spese)
make run                # Run application locally
make test               # Run tests with race detector and coverage
make cover              # Run coverage (requires 100% for core/http packages)
make smoke              # Run smoke tests (scripts/smoke.sh)

# Code quality
make fmt                # Format code (gofmt -s -w .)
make vet                # Run go vet
make lint               # Run golangci-lint (optional)

# Database
make sqlc-generate      # Regenerate sqlc code after modifying queries.sql or schema.sql
```

## Architecture

### Single Application Architecture

The application is a single binary (`cmd/spese`) that runs:
- HTTP server with HTMX frontend for expense/income tracking
- SQLite sync queue processor (syncs to Google Sheets)
- Recurring expense processor (creates expenses from recurrent configs)

### Data Flow

```
User → HTTP Server → SQLite → Sync Queue Processor → Google Sheets
                         ↓
              Recurring Processor (creates expenses from recurrent configs)
```

### Key Packages

- `internal/core`: Domain entities (Expense, Income, RecurrentExpenses, Money, Date) with validation
- `internal/sheets/ports.go`: Port interfaces (ExpenseWriter, TaxonomyReader, DashboardReader, etc.)
- `internal/sheets/google`: Google Sheets adapter implementation
- `internal/storage`: SQLite repository, sqlc-generated queries, migrations
- `internal/adapters`: SQLiteAdapter implementing port interfaces
- `internal/http`: HTTP server with HTMX handlers
- `internal/services`: ExpenseService, RecurringProcessor, SyncQueueProcessor

### SQLite Schema

Key tables: `expenses`, `incomes`, `recurrent_expenses`, `primary_categories`, `secondary_categories`, `income_categories`, `sync_queue`

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
- `SYNC_INTERVAL`: Interval for sync processor (default `30s`)
- `RECURRING_PROCESSOR_INTERVAL`: Interval for recurring processor (default `1h`)

## NixOS Deployment

The flake provides a NixOS module for deployment:

```nix
{
  imports = [ inputs.spese.nixosModules.default ];

  services.spese = {
    enable = true;
    port = 8081;
    googleSpreadsheetId = "your-spreadsheet-id";
    googleServiceAccountFile = "/run/secrets/google-sa.json";
    syncInterval = "1m";
  };
}
```
