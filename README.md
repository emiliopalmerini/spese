# Spese (Go + HTMX)

Simple expense and net-worth tracker backed by local SQLite, with Google Sheets as an external mirror.
- Automatic date (day and month) pre-filled in the form
- Description and expense amount input
- **Hierarchical categories**: Primary categories with dynamic secondary category loading
- Categories and subcategories suggested from local transaction history

Stack: Go, HTMX, SQLite, RabbitMQ, Google Sheets API, Docker, Docker Compose, Makefile, pre-commit.

## Requirements

- Go 1.22+
- Docker + Docker Compose (for containers)
- RabbitMQ or AMQPCloud for the sheet-sync queue
- Google Sheets API access via service account when Google sheet mirroring is enabled

## Local Execution

1) Configure environment variables (see below).
   - Copy the example: `cp .env.example .env`
   - Edit `.env` with your values. Docker Compose loads `.env` and injects it into containers.

Example `.env` (base names without year; the app automatically prefixes the current year):

```bash
SPESE_PORT=8080
SPESE_DB_PATH=tmp/spese-local.db
SPESE_RABBITMQ_URL=amqp://guest:guest@localhost:5672/
SPESE_RABBITMQ_QUEUE=spese.sheet-sync
SPESE_WORKER_MODE=daemon
SPESE_SHEETS_WRITE_RATE_PER_MINUTE=10
SPESE_SHEET_MIRROR_BACKEND=local
SPESE_LOCAL_SHEET_PATH=tmp/local-sheet.json

# For Google mirroring instead:
# SPESE_SHEET_MIRROR_BACKEND=google
# SPESE_RABBITMQ_URL=amqps://...
# GOOGLE_SPREADSHEET_ID=...
# GOOGLE_SERVICE_ACCOUNT_FILE=/path/to/service-account.json
```

2) Start the app:

- `make run` for local development (with graceful shutdown)
- `make run-local` to run the web app with the Rabbit sheet-sync publisher
- `make run-worker-local` in another shell to consume Rabbit messages and mirror into `tmp/local-sheet.json`
- `make docker-up` for execution via Docker Compose

App available at `http://localhost:8080` (`SPESE_PORT` variable).

### Demo data

To inspect the UI with representative data and no external services:

```bash
make run-demo
```

This resets the disposable `tmp/spese-demo.db` database with five accounts,
twelve rolling months of transactions, transfers, and balance snapshots, then
starts the app with sheet mirroring disabled. Run `make demo-data` to regenerate
the database without starting the server. The demo seeder refuses database
filenames that do not contain `demo`.

To exercise the HTTP write path, SQLite outbox, Rabbit queue, worker, and local mirror output:

```bash
make run-local
# in another shell
make run-worker-local
# in another shell
make smoke-local
```

**Security and Performance:**
- Rate limiting: 60 requests per minute per IP
- Timeouts: 10s read/write, 60s idle
- Security headers: CSP, XSS protection, CSRF mitigation
- Input sanitization and comprehensive validation

## Supported Environment Variables

See `.env.example` for defaults. Main variables:
- `SPESE_PORT`: HTTP port (default: 8080)
- `SPESE_DB_PATH`: SQLite database path (default: `./spese.db`)
- `SPESE_SHEET_MIRROR_BACKEND`: `auto`, `google`, `local`, or `none` (default: `auto`)
- `SPESE_LOCAL_SHEET_PATH`: JSON output path for `local` mirror mode (default: `tmp/local-sheet.json`)
- `SPESE_RABBITMQ_URL`: RabbitMQ/AMQPCloud URL required when sheet mirroring is enabled
- `SPESE_RABBITMQ_QUEUE`: RabbitMQ queue name (default: `spese.sheet-sync`)
- `SPESE_WORKER_MODE`: `daemon` to keep consuming, or `once` to drain currently queued messages and exit (default: `daemon`)
- `SPESE_SHEETS_WRITE_RATE_PER_MINUTE`: Google Sheets write request limit in the worker; `10` spaces writes 6 seconds apart, `0` disables it (default: `10`)
- `GOOGLE_SPREADSHEET_ID`: Google Sheets document ID

Google Service Account:
- `GOOGLE_SERVICE_ACCOUNT_FILE`: Path to service account credentials file

## Useful Makefile Commands

- `make setup`: setup dev tools (pre-commit, linters)
- `make tidy`: manage Go modules
- `make build`: compile binary
- `make run`: run app locally
- `make run-demo`: reset rolling demo data and run without external services
- `make demo-data`: regenerate the disposable demo database
- `make run-local`: run web app locally with Rabbit publisher enabled
- `make run-worker-local`: run local Rabbit worker mirroring to `tmp/local-sheet.json`
- `make smoke-local`: post smoke data and verify the local sheet mirror
- `make test`: unit tests with race/coverage
- `make lint`: lints and vet
- `make fmt`: format code
- `make docker-build`: build Docker image
- `make docker-up`: start stack with Compose
- `make docker-logs`: follow logs

## Architecture

The application runs as two cooperating processes when mirroring is enabled:

1. **HTTP Server**: Handles web requests with HTMX frontend
2. **SQLite Outbox + Rabbit Publisher**: Records durable sheet-sync work in the same SQLite transaction as each write, then publishes confirmed RabbitMQ messages in the background
3. **Sheet Mirror Worker**: Consumes RabbitMQ messages and rebuilds source tabs from SQLite, writing either Google Sheets or a local JSON sheet file. Google writes are rate-limited in the worker; production can run the worker in `once` mode from an hourly scheduler.

Benefits:
- **Simplicity**: Single codebase and container image
- **Performance**: Immediate HTTP responses (SQLite only)
- **Reliability**: SQLite outbox, persistent RabbitMQ messages, manual acknowledgements, and worker retries
- **Resilience**: Continues working even if Google Sheets is unavailable

## Docker

- Multistage Dockerfile for small images (builder + Alpine runner).
- `docker compose up -d` for local execution; configuration reads `.env` and injects it into containers (`env_file`).

## Google Sheets Setup (Quick)

1) Create document and sheets:
- Source tabs matching the app model: `accounts`, `transactions`, and `snapshots`.
- Derived `v_*` tabs are optional now; the app reads reports from SQLite.

2) Service Account setup:
- Create a service account in Google Cloud Console
- Enable Google Sheets API for your project
- Generate JSON credentials for the service account
- Share your spreadsheet with the service account email address
- Set `GOOGLE_SERVICE_ACCOUNT_FILE=/path/to/service-account.json`
- Start `spese` and `spese-worker` with `SPESE_SHEET_MIRROR_BACKEND=google`, `SPESE_DB_PATH`, `SPESE_RABBITMQ_URL`, and service account variables set.

**Service Account Security:**
- Credentials file should be stored securely with restricted permissions (e.g., 0600)
- Service account should have minimum required permissions (Google Sheets API access)
- No Service Account keys committed to repo

Troubleshooting Service Account (Docker):
- Place your service account file at `./configs/service-account.json` or set `GOOGLE_SERVICE_ACCOUNT_FILE` to a path inside the container and bind-mount it.
- Ensure the service account email has been granted access to your Google Spreadsheet.

## Health & Readiness

- `GET /healthz`: quick health check (always 200 if process is alive)

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
