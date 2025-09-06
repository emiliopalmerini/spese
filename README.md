# Spese (Go + HTMX)

Simple expense tracking application that saves to Google Spreadsheets.
- Automatic date (day and month) pre-filled in the form
- Description and expense amount input
- Categories and subcategories read from Spreadsheet

Stack: Go, HTMX, SQLite, RabbitMQ, Google Sheets API, Docker (multistage), Docker Compose, Makefile, pre-commit.

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

DATA_BACKEND=memory # use 'sheets' to integrate Google Sheets directly or 'sqlite' for local storage + async sync
PORT=8080
# OAuth
# GOOGLE_OAUTH_CLIENT_FILE=/path/to/client.json
# GOOGLE_OAUTH_TOKEN_FILE=token.json
```

2) Start the app:

- `make run` for local development (with graceful shutdown)
- `make docker-up` for execution via Docker Compose

App available at `http://localhost:8080` (`PORT` variable).

With `DATA_BACKEND=memory`, the app loads development data from `./data`:
- `data/seed_categories.txt` and `data/seed_subcategories.txt`
- optional `data/seed_expenses.csv` (Month,Day,Description,Amount,Primary,Secondary)
This data is also used to populate the monthly overview below the form.

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
- `DATA_BACKEND`: `memory` (default), `sheets`, or `sqlite`
- `DASHBOARD_SHEET_NAME`: base name of annual dashboard sheet to read totals from (preferred). Result: `"<year> <name>"`.
- `DASHBOARD_SHEET_PREFIX`: (legacy) pattern or prefix of annual dashboard sheet (e.g. `%d Dashboard`). Used only if `DASHBOARD_SHEET_NAME` is not set.

SQLite + AMQP (backend `sqlite`):
- `SQLITE_DB_PATH`: SQLite database path (default: `./data/spese.db`)
- `AMQP_URL`: RabbitMQ connection (default: `amqp://guest:guest@localhost:5672/`)
- `AMQP_EXCHANGE`: AMQP exchange name (default: `spese`)
- `AMQP_QUEUE`: AMQP queue name (default: `sync_expenses`)
- `SYNC_BATCH_SIZE`: worker batch size (default: `10`)
- `SYNC_INTERVAL`: periodic sync interval (default: `30s`)

OAuth:
- `GOOGLE_OAUTH_CLIENT_JSON` or `GOOGLE_OAUTH_CLIENT_FILE`: OAuth client credentials (JSON)
- `GOOGLE_OAUTH_TOKEN_JSON` or `GOOGLE_OAUTH_TOKEN_FILE`: user token generated via consent

## Useful Makefile Commands

- `make setup`: setup dev tools (pre-commit, linters)
- `make tidy`: manage Go modules
- `make build`: compile main binary
- `make build-worker`: compile synchronization worker
- `make build-all`: compile both services
- `make run`: run main app locally
- `make run-worker`: run worker locally
- `make sqlc-generate`: regenerate sqlc code after schema changes
- `make test`: unit tests with race/coverage
- `make lint`: lints and vet
- `make fmt`: format code
- `make docker-build`: build Docker image
- `make docker-up`: start stack with Compose
- `make docker-logs`: follow logs

## SQLite + AMQP Architecture (Recommended)

To improve performance and scalability, the app supports an asynchronous architecture:

1. **Main app** (`cmd/spese`): saves expenses to local SQLite (fast)
2. **AMQP message**: publishes expense ID + version (lightweight, ~100 bytes)
3. **Background worker** (`cmd/spese-worker`): consumes messages, reads full expense from SQLite, syncs with Google Sheets
4. **Graceful fallback**: works even without RabbitMQ (SQLite-only mode)

Benefits:
- **Performance**: immediate HTTP responses (SQLite only)
- **Reliability**: persistent messages, automatic retries
- **Scalability**: independent workers, batch processing
- **Resilience**: continues working even if Google Sheets is unavailable

To use this mode:
```bash
# Start RabbitMQ
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management

# Start main app with SQLite
DATA_BACKEND=sqlite AMQP_URL=amqp://localhost:5672 make run

# In another terminal, start the worker
DATA_BACKEND=sqlite AMQP_URL=amqp://localhost:5672 make run-worker
```

Or more simply with Docker Compose (includes RabbitMQ, app and worker):
```bash
make docker-up
```

## Docker

- Multistage Dockerfile for small images (builder + distroless/alpine runner).
- `docker compose up -d` for local execution; configuration reads `.env` and injects it into containers (`env_file`).
- Includes RabbitMQ with management UI at http://localhost:15672 (admin/admin).

## Google Sheets Setup (Quick)

1) Create document and sheets:
- Expenses sheet (e.g. `2025 Expenses`) with headers in row 1:
  - A: Month, B: Day, C: Expense, D: Amount, E: Currency, F: EUR, G: Primary, H: Secondary
- Categories sheet (e.g. `2025 Dashboard` column `A2:A65`) 
- Subcategories sheet (e.g. `2025 Dashboard` column `B2:B65`)

2) OAuth user consent:
- Create an OAuth Client (Type: Desktop app or Web with redirect `http://localhost:8085/callback`).
- Export `GOOGLE_OAUTH_CLIENT_FILE=/path/to/client.json` (or `GOOGLE_OAUTH_CLIENT_JSON`).
- **Option A** - Local: Run `make oauth-init` and complete consent in browser
- **Option B** - Docker: Run `make oauth-init-docker` for containerized OAuth flow
- Generates `token.json` (configure `GOOGLE_OAUTH_TOKEN_FILE` if you want a different path).
- Start the app with `DATA_BACKEND=sheets` and OAuth variables set.

**OAuth Security:**
- Tokens saved with 0600 permissions (owner only)
- Auto-refresh of expired tokens
- No Service Account keys committed to repo

Troubleshooting OAuth (Docker):
- Place your OAuth client file at `./configs/client.json` or set `GOOGLE_OAUTH_CLIENT_FILE` to a path inside the container and bind-mount it.
- The compose profile `oauth-init` binds `./configs/client.json` to `${GOOGLE_OAUTH_CLIENT_FILE:-/client.json}` so the default works out-of-the-box.
- If the token lacks `refresh_token`, revoke previous consent in your Google Account and rerun `make oauth-init-docker` (the flow forces `prompt=consent`).

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
