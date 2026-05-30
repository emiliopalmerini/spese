# Repository Guidelines

## Project Structure & Module Organization

Spese is a Go single-binary HTMX application backed by Google Sheets. The entry point is `cmd/spese/main.go`. Feature code lives in vertical slices under `internal/features/<feature>/`, where handlers, sheet access, and types stay together. Shared packages live in `internal/config`, `internal/kernel`, `internal/render`, and `internal/sheets`. Templates and browser assets are embedded from `web/templates` and `web/static`. Seed data is in `data/`, operational scripts are in `scripts/`, and architectural decisions are recorded in `docs/adr/`.

## Build, Test, and Development Commands

- `nix develop` or `make dev`: enter the recommended development shell.
- `make run`: run the app locally with `go run ./cmd/spese`.
- `make build`: format and build `bin/spese`.
- `make test`: run `gofmt` and `go test -race -cover ./...`.
- `make vet`: run `go vet ./...`.
- `make lint`: run `go vet`, then `golangci-lint run` when installed.
- `make nix-build`: build through Nix, producing `result/bin/spese`.
- `scripts/smoke.sh`: smoke-test a running local app via `/healthz` and a form post.

## Coding Style & Naming Conventions

Use `gofmt -s` for all Go code; `make fmt` applies it repository-wide. Keep packages small and domain-oriented. Prefer existing vertical-slice boundaries over technical-layer directories. Name feature packages by domain nouns such as `accounts`, `transactions`, and `snapshots`. Keep sheet writes append-only unless the architecture decision records say otherwise. Add comments only for non-obvious intent, invariants, or external constraints.

## Testing Guidelines

Use Go's standard `testing` package. Place tests beside implementation files with `_test.go` names and prefer table-driven tests for parsing, validation, and value types. New behavior should include a failing test first when practical. Run `make test` before opening a PR; use `scripts/smoke.sh` after changes that affect routes, templates, or Google Sheets writes.

## Commit & Pull Request Guidelines

History uses Conventional Commits, for example `fix(sheets): treat missing tab as empty range` and `feat(dashboard): unfold pick months + proiezioni, italianize UI`. Keep commits atomic and scoped by intent. Pull requests should describe the behavior change, list verification commands, link related issues or ADRs, and include screenshots for visible UI changes.

## Security & Configuration Tips

Do not commit `.env` files, Google service-account JSON, spreadsheet IDs from private environments, or local database artifacts. Configure local runs with `GOOGLE_SPREADSHEET_ID`, `GOOGLE_SERVICE_ACCOUNT_FILE`, optional `SPESE_PORT`, `SPESE_DB_PATH`, and `HONKER_EXTENSION_PATH`.
