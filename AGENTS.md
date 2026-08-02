# Repository Guidelines

## Project Structure & Module Organization

Spese is a Go single-binary React application backed by a canonical SQLite ledger. The entry point is `cmd/spese/main.go`; the versioned JSON API is in `internal/api`, ledger behavior is in `internal/features/ledger`, and the SPA source is in `frontend/`. The Vite build is embedded from `web/dist`. Google Sheets is only a derived mirror through the SQLite outbox, RabbitMQ, and `spese-worker`. Operational scripts are in `scripts/`, and architectural decisions are recorded in `docs/adr/`.

## Build, Test, and Development Commands

- `nix develop` or `make dev`: enter the recommended development shell.
- `make run`: run the app locally with `go run ./cmd/spese`.
- `make frontend-install`: install the pinned npm dependency graph.
- `make build`: build the SPA, runtime, worker, and migration tool.
- `make test`: run Go race tests plus frontend typecheck and unit/component tests.
- `make test-e2e`: run Playwright against the embedded SPA and API.
- `make vet`: run `go vet ./...`.
- `make lint`: run `go vet`, then `golangci-lint run` when installed.
- `make nix-build`: build through Nix, producing `result/bin/spese`.
- `scripts/smoke.sh`: smoke-test a running local app via `/healthz` and a form post.

## Coding Style & Naming Conventions

Use `gofmt -s` for Go and TypeScript strict mode for the SPA. Keep packages small and domain-oriented. Preserve ledger transaction boundaries and use stable IDs rather than names as foreign keys. Sheet writes are complete derived exports and must never become a read dependency. Add comments only for non-obvious intent, invariants, or external constraints.

## Testing Guidelines

Use Go's standard `testing` package and Vitest/Testing Library in `frontend`. Place tests beside implementation and prefer table-driven tests for parsing, validation, and value types. New behavior should include a failing test first when practical. Run `make test` before opening a PR, `make test-e2e` for UI/API changes, and `scripts/smoke.sh` after outbox or mirror changes.

## Commit & Pull Request Guidelines

History uses Conventional Commits, for example `fix(sheets): treat missing tab as empty range` and `feat(dashboard): unfold pick months + proiezioni, italianize UI`. Keep commits atomic and scoped by intent. Pull requests should describe the behavior change, list verification commands, link related issues or ADRs, and include screenshots for visible UI changes.

## Security & Configuration Tips

Do not commit `.env` files, Google service-account JSON, spreadsheet IDs from private environments, or local database artifacts. Configure local runs with `GOOGLE_SPREADSHEET_ID`, `GOOGLE_SERVICE_ACCOUNT_FILE`, optional `SPESE_PORT`, `SPESE_DB_PATH`, and `SPESE_RABBITMQ_URL`.
