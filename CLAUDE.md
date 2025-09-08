# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture Overview

This is a Go expense tracking application with HTMX frontend and Google Sheets integration. The architecture follows DDD principles with hexagonal architecture:

### Key Components

- **cmd/spese**: Main web application (HTTP server + HTMX frontend)
- **cmd/spese-worker**: Background worker for async Google Sheets synchronization
- **cmd/oauth-init**: OAuth flow utility for Google Sheets API setup

### Data Backends

The application supports multiple data backends via `DATA_BACKEND` environment variable:

- **sheets**: Direct Google Sheets integration (legacy, synchronous)
- **sqlite**: Local SQLite storage with async Google Sheets sync via AMQP (recommended)

### SQLite + AMQP Architecture (Recommended)

When using `DATA_BACKEND=sqlite`:

1. **Main app** saves expenses to SQLite (fast local storage)
2. **AMQP message** published with expense ID + version (lightweight)
3. **Background worker** consumes messages, fetches full expense data from SQLite, syncs to Google Sheets
4. **Graceful fallback** when AMQP unavailable (SQLite-only mode)

Key packages:
- `internal/storage`: SQLite repository with sqlc-generated queries and go-migrate migrations
- `internal/amqp`: AMQP client and message structures
- `internal/services`: ExpenseService orchestrates storage + messaging
- `internal/worker`: SyncWorker handles background synchronization
- `internal/adapters`: SQLiteAdapter implements sheets.* interfaces for HTTP compatibility

## Core Domain (internal/core)

- **Expense**: Main entity with DateParts, Money, categories
- **Money**: Value object using cents to avoid float precision issues  
- **MonthOverview**: Aggregated monthly data with category breakdowns
- **Domain validation**: All business rules enforced in domain layer, not controllers

## Development Commands

### Building and Running
```bash
# Build both main app and worker
make build-all

# Run main app locally
make run
DATA_BACKEND=sqlite SQLITE_DB_PATH=./data/spese.db make run

# Run background worker
make run-worker

# Build individual services
make build        # main app only
make build-worker # worker only
```

### Development Workflow
```bash
# Setup development environment
make setup
make tidy

# Code quality
make fmt          # Format code
make vet          # Go vet
make lint         # golangci-lint (if available)
make test         # Run tests with race detector

# Database operations (SQLite backend)
make sqlc-generate # Regenerate sqlc code after schema changes
```

### Docker Development
```bash
# Start full stack (app + worker + RabbitMQ)
make docker-up

# View logs
make docker-logs

# OAuth setup in Docker
make oauth-init-docker

# Cleanup
make docker-down
```

### Testing Individual Components
```bash
# Test specific packages
go test ./internal/core
go test -race ./internal/http
go test -v ./internal/sheets/google

# Coverage for core packages  
make cover
```

## Configuration

The app uses environment-based configuration via `internal/config`. Key variables:

### Core Settings
- `PORT`: HTTP server port (default: 8081)
- `DATA_BACKEND`: Backend type (sheets|sqlite)

### SQLite + AMQP (Recommended)
- `SQLITE_DB_PATH`: SQLite database path (default: ./data/spese.db)
- `AMQP_URL`: RabbitMQ connection (default: amqp://guest:guest@localhost:5672/)
- `AMQP_EXCHANGE`: AMQP exchange name (default: spese)
- `AMQP_QUEUE`: AMQP queue name (default: sync_expenses)
- `SYNC_BATCH_SIZE`: Worker batch size (default: 10)
- `SYNC_INTERVAL`: Periodic sync interval (default: 30s)

### Google Sheets OAuth
- `GOOGLE_SPREADSHEET_ID`: Target spreadsheet ID
- `GOOGLE_SHEET_NAME`: Base sheet name (year auto-prefixed, e.g., "2025 Expenses")
- `GOOGLE_OAUTH_CLIENT_FILE` or `GOOGLE_OAUTH_CLIENT_JSON`: OAuth client credentials
- `GOOGLE_OAUTH_TOKEN_FILE` or `GOOGLE_OAUTH_TOKEN_JSON`: User access token

## Database Schema (SQLite)

Managed via go-migrate with embedded migrations in `internal/storage/migrations/`:

```sql
-- expenses table with versioning for sync tracking
CREATE TABLE expenses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    day INTEGER NOT NULL CHECK (day >= 1 AND day <= 31),
    month INTEGER NOT NULL CHECK (month >= 1 AND month <= 12),  
    description TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
    primary_category TEXT NOT NULL,
    secondary_category TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    synced_at DATETIME NULL,
    sync_status TEXT DEFAULT 'pending' -- 'pending'|'synced'|'error'
);
```

### Code Generation

The project uses **sqlc** for type-safe database queries:

```bash
# After modifying internal/storage/schema.sql or queries.sql:
make sqlc-generate

# Or directly:
sqlc generate
```

Schema and queries are in:
- `internal/storage/schema.sql`: Database schema for sqlc
- `internal/storage/queries.sql`: SQL queries with sqlc annotations
- `migrations/`: Actual migration files for go-migrate

## Hierarchical Categories System

The application implements a hierarchical category system with dynamic frontend loading for improved user experience.

### Database Structure

Categories are stored in separate tables with parent-child relationships:

```sql
-- Primary categories (13 main categories in Italian)
CREATE TABLE primary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Secondary categories linked to primary categories
CREATE TABLE secondary_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    primary_category_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (primary_category_id) REFERENCES primary_categories(id)
);
```

### Category Structure

**Primary Categories (13):**
- Casa, Salute, Spesa, Trasporti, Fuori (come fuori a cena...), Viaggi, Bimbi, Vestiti, Divertimento, Regali, Tasse e Percentuali, Altre spese, Lavoro

**Secondary Categories (47 total):**
- **Casa**: Mutuo, Spese condominiali, Internet, Mobili, Assicurazioni, Pulizia, Elettricità, Telefono
- **Salute**: Assicurazione sanitaria, Dottori, Medicine, Personale, Sport  
- **Trasporti**: Trasporto locale, Car sharing, Spese automobile, Servizi taxi
- And so on...

### Frontend Integration

The UI uses **HTMX** for dynamic category loading:

1. **Primary Selection**: User selects a primary category
2. **Dynamic Loading**: HTMX calls `/api/categories/secondary?primary=X`
3. **Filtered Options**: Only relevant secondary categories are shown
4. **Better UX**: Reduces selection errors and improves usability

### API Endpoints

- `GET /api/categories/secondary?primary={category}`: Returns HTML `<option>` elements for secondary categories filtered by primary category

### Google Sheets Sync Compatibility

The system maintains compatibility with Google Sheets sync through intelligent mapping:

- **Primary categories**: Managed via database migrations (not synced from sheets)  
- **Secondary categories**: Synced from Google Sheets with automatic mapping to primary categories
- **Legacy support**: Old category names are automatically mapped to the new hierarchical structure

### Repository Methods

New methods support hierarchical operations:

```go
// Get secondary categories for a specific primary category
func (r *SQLiteRepository) GetSecondariesByPrimary(ctx context.Context, primaryCategory string) ([]string, error)

// SQLiteAdapter exposes hierarchical methods
func (a *SQLiteAdapter) GetSecondariesByPrimary(ctx context.Context, primaryCategory string) ([]string, error)
```

## Ports & Adapters Pattern

The application follows hexagonal architecture with clear port definitions:

### Ports (internal/sheets/ports.go)
- `ExpenseWriter`: Save expenses
- `TaxonomyReader`: Read categories/subcategories  
- `DashboardReader`: Read aggregated monthly data
- `ExpenseLister`: List detailed expenses

### Adapters
- `internal/sheets/google`: Google Sheets API adapter
- `internal/adapters/sqlite_adapter.go`: SQLite adapter with AMQP integration

## HTTP Layer (internal/http)

HTMX-based frontend with server-side rendering:
- `server.go`: Main HTTP server with middleware, rate limiting
- Templates in `web/templates/`: Base layout + HTMX partials
- Security headers, input sanitization, structured logging
- Direct SQLite queries with no caching (SQLite is fast for local storage)

## Google Sheets Integration

### Sheet Structure
Year-prefixed sheet names (e.g., "2025 Expenses", "2025 Dashboard"):
- **Expenses sheet**: Month, Day, Expense, Amount, Currency, EUR, Primary, Secondary
- **Categories**: Column A in Dashboard sheet
- **Subcategories**: Column B in Dashboard sheet

### OAuth Setup
```bash
# Local OAuth flow
make oauth-init

# Docker OAuth flow  
make oauth-init-docker
```

## Architecture Decision Records (ADRs)

Architectural decisions documented in `docs/adrs/`:
- 0001: Languages and stack selection
- 0002: DDD and hexagonal architecture principles  
- 0003: Domain and data models design
- 0004: Monthly expense visualization
- 0005: Year-prefixed sheet names strategy

## Commit Conventions

Use Conventional Commits format: `feat(scope): description`

Examples:
- `feat(storage): add SQLite backend with AMQP integration`  
- `fix(http): handle rate limit edge case`
- `docs(readme): update Docker setup instructions`

## Development Notes

- **No CGO**: Pure Go builds for easy deployment (`CGO_ENABLED=0`)
- **Embedded assets**: Templates and static files embedded via `go:embed`
- **Graceful shutdown**: Both services handle SIGTERM/SIGINT properly
- **Error handling**: Structured logging with context, error wrapping with `%w`
- **Security**: Rate limiting, input sanitization, security headers, OAuth token management
- **Performance**: Local SQLite storage, async background sync, HTTP response caching