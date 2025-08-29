# Spese (Go + HTMX)

Semplice app per registrare spese su un Google Spreadsheet.
- Data automatica (giorno e mese) precompilata nel form.
- Inserimento descrizione e valore spesa.
- Categorie e sottocategorie lette dallo Spreadsheet.

Stack: Go, HTMX, Google Sheets API, Docker (multistage), Docker Compose, Makefile, pre-commit.

## Requisiti

- Go 1.22+
- Docker + Docker Compose (per container)
- Accesso a Google Sheets API via OAuth (client + token)

## Esecuzione locale

1) Configura le variabili d’ambiente (vedi sotto). Esempio con `.env`:

```
GOOGLE_SPREADSHEET_ID=...
GOOGLE_SHEET_NAME=Spese
GOOGLE_CATEGORIES_SHEET_NAME=Categories
GOOGLE_SUBCATEGORIES_SHEET_NAME=Subcategories
DATA_BACKEND=memory # usa 'sheets' per integrare Google Sheets
PORT=8080
# OAuth
# GOOGLE_OAUTH_CLIENT_FILE=/percorso/client.json
# GOOGLE_OAUTH_TOKEN_FILE=token.json
```

2) Avvia l’app:

- `make run` per sviluppo locale
- `make docker-up` per esecuzione via Docker Compose

App disponibile su `http://localhost:8080` (variabile `PORT`).

## Variabili d'ambiente supportate

Vedi `.env.example` per i default. Principali:
- `PORT`: porta HTTP (default: 8080)
- `BASE_URL`: base URL pubblico (per link assoluti)
- `GOOGLE_SPREADSHEET_ID`: ID del documento Google Sheets
- `GOOGLE_SHEET_NAME`: foglio (tab) spese, default `Spese`
- `GOOGLE_CATEGORIES_SHEET_NAME`: foglio categorie, default `Categories`
- `GOOGLE_SUBCATEGORIES_SHEET_NAME`: foglio sottocategorie, default `Subcategories`
- `DATA_BACKEND`: `memory` (default) o `sheets`

OAuth:
- `GOOGLE_OAUTH_CLIENT_JSON` oppure `GOOGLE_OAUTH_CLIENT_FILE`: credenziali client OAuth (JSON)
- `GOOGLE_OAUTH_TOKEN_JSON` oppure `GOOGLE_OAUTH_TOKEN_FILE`: token utente generato via consenso

## Comandi Makefile utili

- `make setup`: setup strumenti dev (pre-commit, linters)
- `make tidy`: gestisce mod Go
- `make build`: compila binario
- `make run`: esegue in locale
- `make test`: unit test con race/coverage
- `make lint`: lints e vet
- `make fmt`: formatta codice
- `make docker-build`: build immagine Docker
- `make docker-up`: avvia stack con Compose
- `make docker-logs`: segue i log

## Docker

- Dockerfile multistage per immagini piccole (builder + runner distroless/alpine).
- `docker compose up -d` per esecuzione locale; la configurazione legge `.env`.

## Setup Google Sheets (rapido)

1) Crea il documento e i fogli:
- Foglio spese (default `Spese`) con intestazioni in riga 1:
  - A: Day, B: Month, C: Description, D: Amount, E: Category, F: Subcategory
- Foglio categorie (default `Categories`) con intestazione riga 1: `Category`, poi una categoria per riga nella colonna A.
- Foglio sottocategorie (default `Subcategories`) con intestazione riga 1: `Subcategory`, poi una sottocategoria per riga nella colonna A.

2) OAuth user consent:
- Crea un OAuth Client (Tipo: App per desktop o Web con redirect `http://localhost:8085/callback`).
- Esporta `GOOGLE_OAUTH_CLIENT_FILE=/percorso/client.json` (o `GOOGLE_OAUTH_CLIENT_JSON`).
- Esegui `make oauth-init` e completa il consenso nel browser; genera `token.json` (configura `GOOGLE_OAUTH_TOKEN_FILE` se vuoi un path diverso).
- Avvia l'app con `DATA_BACKEND=sheets` e le variabili OAuth impostate.

## Health & Readiness

- `GET /healthz`: quick health (sempre 200 se processo vivo)
- `GET /readyz`: readiness (opzionale: include check Google API/rate limits)

## Deploy

- Container-first: build e push immagine su registry; run su container runtime (Fly.io, Render, k8s, ECS, ecc.).
- Variabili d’ambiente fornite dal platform secret manager.

## Commit message template (Conventional Commits)

Usiamo lo standard Conventional Commits per messaggi chiari e automatizzabili.

Formato base:

```
<type>(<scope>)<!>: <subject>

<body>

<footer>
```

- type: tipo di cambiamento. Esempi comuni: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`.
- scope: (opzionale) area toccata, es. `templates`, `encounters`, `router`.
- !: (opzionale) indica breaking change.
- subject: (obbligatorio) riassunto al presente, in minuscolo, senza punto finale.
- body: (opzionale) contesto/motivazione, dettagli tecnici se utili.
- footer: (opzionale) riferimenti a issue/PR o `BREAKING CHANGE:` con spiegazione.

Esempi:

```
fix(templates): allinea route HTMX a /encounters/*

docs(readme): aggiungi template per Conventional Commits
```

Note:
- Imperativo presente nel subject (es. "aggiungi", "correggi").
- Mantieni il subject entro ~72 caratteri quando possibile.
- Un commit dovrebbe fare una cosa sola e bene.

## Pre-commit hook

Pre-commit per mantenere qualità e coerenza:
- gofmt/goimports
- golangci-lint (se configurato)
- yamllint/hadolint (per YAML/Dockerfile)
- prettier (solo per static/ o templates opzionalmente)

Installa ed attiva:
- `pipx install pre-commit` (o `pip install pre-commit`)
- `pre-commit install`

## ADR (Architectural Decision Records)

La documentazione delle decisioni architetturali è disponibile in `docs/adrs`.
Indice ADR: [docs/adrs/README.md](./docs/adrs/README.md)
