# Spese (Go + HTMX)

Simple expense tracking application that saves to Google Spreadsheets with hierarchical categories.
- Automatic date (day and month) pre-filled in the form
- Description and expense amount input
- **Hierarchical categories**: Primary categories with dynamic secondary category loading
- Categories and subcategories read from Spreadsheet with intelligent mapping

Stack: Go, HTMX, SQLite, Google Sheets API, Docker (multistage), Docker Compose, Makefile, pre-commit.

## Requirements

- Go 1.22+
- Docker + Docker Compose (for containers)
- Google Sheets API access via OAuth (client + token)

## Local Execution

1) Configure environment variables (see below).
   - Copy the example: `cp .env.example .env`
   - Edit `.env` with your values. Docker Compose loads `.env` and injects it into containers.

Example `.env` (base names without year; the app automatically prefixes the current year):

```bash
GOOGLE_SPREADSHEET_ID=...
GOOGLE_SHEET_NAME=Expenses
GOOGLE_CATEGORIES_SHEET_NAME=Dashboard
GOOGLE_SUBCATEGORIES_SHEET_NAME=Dashboard
# Base name of the dashboard sheet used for monthly summary
# The app builds "<year> <name>", e.g.: "2025 Dashboard"
DASHBOARD_SHEET_NAME=Dashboard
# (legacy fallback) Pattern with %d: e.g. "%d Dashboard"
# DASHBOARD_SHEET_PREFIX="%d Dashboard"

DATA_BACKEND=sqlite # use 'sheets' to integrate Google Sheets directly or 'sqlite' for local storage + async sync
PORT=8080
# Google Service Account
# GOOGLE_SERVICE_ACCOUNT_FILE=/path/to/service-account.json
```

2) Start the app:

- `make run` for local development (with graceful shutdown)
- `make docker-up` for execution via Docker Compose

App available at `http://localhost:8080` (`PORT` variable).

The app supports two backends:
- `DATA_BACKEND=sqlite`: Uses local SQLite database with async Google Sheets sync
- `DATA_BACKEND=sheets`: Direct Google Sheets integration

**Security and Performance:**
- Rate limiting: 60 requests per minute per IP
- Timeouts: 10s read/write, 60s idle
- Security headers: CSP, XSS protection, CSRF mitigation
- Input sanitization and comprehensive validation

## Supported Environment Variables

See `.env.example` for defaults. Main variables:
- `PORT`: HTTP port (default: 8080)
- `BASE_URL`: public base URL (for absolute links)
- `GOOGLE_SPREADSHEET_ID`: Google Sheets document ID
- `GOOGLE_SHEET_NAME`: base name of expenses sheet (without year), default `Expenses` → resolved to `"<year> Expenses"`
- `GOOGLE_CATEGORIES_SHEET_NAME`: base name categories sheet, default `Dashboard` → `"<year> Dashboard"`
- `GOOGLE_SUBCATEGORIES_SHEET_NAME`: base name subcategories sheet, default `Dashboard` → `"<year> Dashboard"`
- `DATA_BACKEND`: `sqlite` (default), or `sheets`
- `DASHBOARD_SHEET_NAME`: base name of annual dashboard sheet to read totals from (preferred). Result: `"<year> <name>"`.
- `DASHBOARD_SHEET_PREFIX`: (legacy) pattern or prefix of annual dashboard sheet (e.g. `%d Dashboard`). Used only if `DASHBOARD_SHEET_NAME` is not set.

SQLite Configuration (backend `sqlite`):
- `SQLITE_DB_PATH`: SQLite database path (default: `./data/spese.db`)
- `SYNC_BATCH_SIZE`: sync processor batch size (default: `10`)
- `SYNC_INTERVAL`: periodic sync interval (default: `30s`)
- `RECURRING_PROCESSOR_INTERVAL`: recurring expenses check interval (default: `1h`)

Google Service Account:
- `GOOGLE_SERVICE_ACCOUNT_JSON`: Service account credentials as JSON string
- `GOOGLE_SERVICE_ACCOUNT_FILE`: Path to service account credentials file
- `GOOGLE_APPLICATION_CREDENTIALS`: Standard Google Cloud credentials file path

## Useful Makefile Commands

- `make setup`: setup dev tools (pre-commit, linters)
- `make tidy`: manage Go modules
- `make build`: compile binary
- `make run`: run app locally
- `make sqlc-generate`: regenerate sqlc code after schema changes
- `make test`: unit tests with race/coverage
- `make lint`: lints and vet
- `make fmt`: format code
- `make docker-build`: build Docker image
- `make docker-up`: start stack with Compose
- `make docker-logs`: follow logs

## Architecture

The application runs as a single binary with integrated processors:

1. **HTTP Server**: Handles web requests with HTMX frontend
2. **Sync Processor**: Periodically syncs pending expenses to Google Sheets
3. **Recurring Processor**: Creates expenses from recurring configurations when due

Benefits:
- **Simplicity**: Single deployment unit
- **Performance**: Immediate HTTP responses (SQLite only)
- **Reliability**: SQLite queue with automatic retries
- **Resilience**: Continues working even if Google Sheets is unavailable

## Docker

- Multistage Dockerfile for small images (builder + scratch runner).
- `docker compose up -d` for local execution; configuration reads `.env` and injects it into containers (`env_file`).

## Google Sheets Setup (Quick)

1) Create document and sheets:
- Expenses sheet (e.g. `2025 Expenses`) with headers in row 1:
  - A: Month, B: Day, C: Expense, D: Amount, E: Currency, F: EUR, G: Primary, H: Secondary
- Categories sheet (e.g. `2025 Dashboard` column `A2:A65`)
- Subcategories sheet (e.g. `2025 Dashboard` column `B2:B65`)

2) Service Account setup:
- Create a service account in Google Cloud Console
- Enable Google Sheets API for your project
- Generate JSON credentials for the service account
- Share your spreadsheet with the service account email address
- Set `GOOGLE_SERVICE_ACCOUNT_FILE=/path/to/service-account.json`
- Start the app with `DATA_BACKEND=sheets` and service account variables set.

**Service Account Security:**
- Credentials file should be stored securely with restricted permissions (e.g., 0600)
- Service account should have minimum required permissions (Google Sheets API access)
- No Service Account keys committed to repo

Troubleshooting Service Account (Docker):
- Place your service account file at `./configs/service-account.json` or set `GOOGLE_SERVICE_ACCOUNT_FILE` to a path inside the container and bind-mount it.
- Ensure the service account email has been granted access to your Google Spreadsheet.

## Health & Readiness

- `GET /healthz`: quick health check (always 200 if process is alive)
- `GET /readyz`: readiness check (includes dependency verification)
- `GET /metrics`: application and security metrics (Prometheus format)

## Deploy

- Container-first: build and push image to registry; run on container runtime (Fly.io, Render, k8s, ECS, etc.).
- Environment variables provided by platform secret manager.

## Commit Message Template (Conventional Commits)

We use the Conventional Commits standard for clear and automatable messages.

Basic format:

```
<type>(<scope>)<!>: <subject>

<body>

<footer>
```

- type: type of change. Common examples: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`.
- scope: (optional) affected area, e.g. `templates`, `encounters`, `router`.
- !: (optional) indicates breaking change.
- subject: (required) present tense summary, lowercase, no final period.
- body: (optional) context/motivation, technical details if useful.
- footer: (optional) references to issues/PRs or `BREAKING CHANGE:` with explanation.

Examples:

```
fix(templates): align HTMX routes to /encounters/*

docs(readme): add Conventional Commits template
```

Notes:
- Present imperative in subject (e.g. "add", "fix").
- Keep subject within ~72 characters when possible.
- One commit should do one thing well.

## Pre-commit Hook

Pre-commit to maintain quality and consistency:
- gofmt/goimports
- golangci-lint (if configured)
- yamllint/hadolint (for YAML/Dockerfile)
- prettier (only for static/ or templates optionally)

Install and activate:
- `pipx install pre-commit` (or `pip install pre-commit`)
- `pre-commit install`

## ADR (Architectural Decision Records)

Architectural decision documentation is available in `docs/adrs`.
ADR Index: [docs/adrs/README.md](./docs/adrs/README.md)
