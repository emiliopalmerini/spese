# Repository Guidelines

## Project Structure & Module Organization

- Root modules: web app Go con frontend HTMX, SQLite storage, e integrazione Google Sheets via AMQP.
- Layout suggerito:
  - `cmd/spese/`: entrypoint app principale (`main.go`).
  - `cmd/spese-worker/`: entrypoint worker sincronizzazione (`main.go`).
  - `internal/http/`: router, middleware, handlers, glue con templates.
  - `internal/storage/`: repository SQLite con sqlc e go-migrate.
  - `internal/amqp/`: client AMQP e strutture messaggi.
  - `internal/services/`: service layer per orchestrare storage + messaging.
  - `internal/adapters/`: adapter pattern per compatibilità interfacce.
  - `internal/worker/`: worker sincronizzazione background.
  - `internal/sheets/`: client e repository per Google Sheets API.
  - `internal/core/`: modelli dominio (Expense, Category) e validazione.
  - `internal/config/`: configurazione centralizzata.
  - `web/`: `templates/` (frammenti HTMX) e `static/` (css/js/assets).
  - `migrations/`: migrazioni database SQL.
  - `docs/`: ADRs e note architetturali (`docs/adrs`).
  - `scripts/`: script usati da Makefile/CI.
- Binari: 
  - `spese`: servizio principale (UI HTML + API HTMX)
  - `spese-worker`: worker background per sincronizzazione Google Sheets
- Templates: piccoli partials composabili; un layout base + frammenti.

## Build, Test, and Development Commands

- `make setup`: installa dipendenze dev (pre-commit, linters) se previste.
- `make tidy`: `go mod tidy` e vendor se necessario.
- `make build`: compila il binario principale `spese`.
- `make build-worker`: compila il binario worker `spese-worker`.
- `make build-all`: compila entrambi i servizi.
- `make run`: avvia app principale in locale (con `air` se presente, altrimenti `go run`).
- `make run-worker`: avvia worker in locale.
- `make sqlc-generate`: rigenera codice sqlc dopo modifiche schema/query.
- `make test`: esegue unit test con race detector e coverage.
- `make lint`: `gofmt -s`, `go vet`, e `golangci-lint` se configurato.
- `make fmt`: formatta codice e (opzionale) templates/static.
- `make docker-build`: build immagine multi-stage.
- `make docker-up`: `docker compose up -d` per lo stack locale.
- `make docker-logs`: segue i log dell'app.
- `make clean`: rimuove artefatti di build.

Le interfacce dei comandi restano stabili anche se cambiano gli strumenti sottostanti.

## Coding Style & Naming Conventions

- Go idiomatico: funzioni piccole, error-first, niente panics in flussi normali.
- Packages: nomi concisi in minuscolo (`sheets`, `http`, `core`).
- File: raggruppa per feature ove utile (es. `expense_handlers.go`).
- Nomi: tipi esportati come sostantivi (`ExpenseService`), interfacce piccole e comportamentali (`Clock`, `SheetWriter`).
- Errori: avvolgi con `%w`; usa sentinelle nel package, non confronti su testo.
- Logging: strutturato, a livelli; mai loggare dati sensibili.
- Templates: endpoint HTMX restituiscono partials; SSR by default.

## Testing Guidelines

- Unit test per `core` e `sheets` con tabella di casi.
- Handlers HTTP: `httptest`; asserisci status, content-type, marker HTML.
- Niente chiamate reali alle API nei unit test; mocka `sheets.Client` via interfacce piccole.
- Race detector (`-race`) e coverage in CI.
- Test veloci per default; integra/slow con build tag `//go:build integration`.

## Commit & Pull Request Guidelines

- Conventional Commits (feat, fix, docs, refactor, test, chore, build, ci).
- PR piccole e focalizzate con titolo chiaro e contesto; screenshot/gif per UI.
- Motiva i cambi architetturali e collega ADR rilevanti.
- PR verdi: test, lint, e docs aggiornati quando cambia il comportamento.
- Usa draft PR per feedback precoce; "ready" quando stabile.

## Security & Configuration Tips

- Segreti: mai committare credenziali; usa `.env` in locale e secret manager in CI/CD.
- Google Sheets (OAuth): usa client OAuth e token utente; conserva `token.json` in modo sicuro; non committarlo.
- Config: env-first — `GOOGLE_SPREADSHEET_ID`, `GOOGLE_SHEET_NAME`, `GOOGLE_OAUTH_CLIENT_FILE/JSON`, `GOOGLE_OAUTH_TOKEN_FILE/JSON`, `PORT`, `BASE_URL`.
- Validazione input: lato server per importo, categoria e data (gg/mm).
- CSRF: ok per form same-origin; se apri a JSON/cross-origin, aggiungi protezione CSRF.
- HTTPS: TLS terminato a proxy in prod; cookie `Secure`/`HttpOnly` se usati.
- CORS: disabilitato salvo necessità; se abilitato, whitelist strette.

## Architecture Overview

La documentazione delle decisioni architetturali è disponibile in `docs/adrs`.
Indice ADR: [docs/adrs/README.md](./docs/adrs/README.md)

- Flusso alto livello (backend `sqlite`):
  - L'utente invia una spesa via form HTMX (data precompilata in giorno e mese).
  - Il server valida, salva in SQLite locale (veloce), e pubblica messaggio AMQP.
  - Worker background consuma messaggio, legge spesa da SQLite, sincronizza con Google Sheets.
  - Le categorie sono lette da range configurati nello spreadsheet o hardcoded in SQLite.
- Strati:
  - `core`: logica dominio e validazione.
  - `storage`: repository SQLite con sqlc + go-migrate.
  - `amqp`: messaging asincrono per sincronizzazione.
  - `services`: orchestrazione storage + messaging.
  - `adapters`: pattern adapter per compatibilità interfacce.
  - `worker`: elaborazione background messaggi.
  - `sheets`: adapter Google Sheets (legacy diretto + worker sync).
  - `http`: handlers + templates (pagine e partials HTMX).
- Le ADR catturano scelte chiave (auth, layout dati, templating, dipendenze).

## CI & Release

- CI: fmt, lint, test (`-race`), build. Cache mod Go.
- Docker: build multi-stage; push su `main` e tag.
- Release: tag SemVer; changelog da Conventional Commits (opzionale).
- Pre-commit: format e hygiene prima del commit.
