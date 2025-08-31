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

1) Configura le variabili d'ambiente (vedi sotto).
   - Copia l'esempio: `cp .env.example .env`
   - Modifica `.env` con i tuoi valori. Docker Compose carica `.env` e lo inietta nei container.

Esempio di `.env`:

```bash
GOOGLE_SPREADSHEET_ID=...
GOOGLE_SHEET_NAME=2025 Expenses
GOOGLE_CATEGORIES_SHEET_NAME=2025 Dashboard
GOOGLE_SUBCATEGORIES_SHEET_NAME=2025 Dashboard
# Prefisso/pattern foglio dashboard annuale usato per il riepilogo mensile
# Usa %d per l'anno, es: "%d Dashboard" -> "2025 Dashboard"
DASHBOARD_SHEET_PREFIX="%d Dashboard"

DATA_BACKEND=memory # usa 'sheets' per integrare Google Sheets
PORT=8080
# OAuth
# GOOGLE_OAUTH_CLIENT_FILE=/percorso/client.json
# GOOGLE_OAUTH_TOKEN_FILE=token.json
```

2) Avvia l'app:

- `make run` per sviluppo locale (con graceful shutdown)
- `make docker-up` per esecuzione via Docker Compose

App disponibile su `http://localhost:8080` (variabile `PORT`).

Con `DATA_BACKEND=memory`, l'app carica dati di sviluppo da `./data`:
- `data/seed_categories.txt` e `data/seed_subcategories.txt`
- opzionale `data/seed_expenses.csv` (Month,Day,Description,Amount,Primary,Secondary)
Questi dati sono usati anche per popolare la panoramica mensile sotto il form.

**Sicurezza e Performance:**
- Rate limiting: 60 richieste per minuto per IP
- Timeout: 10s read/write, 60s idle
- Headers di sicurezza: CSP, XSS protection, CSRF mitigation
- Input sanitization e validazione completa

## Variabili d'ambiente supportate

Vedi `.env.example` per i default. Principali:
- `PORT`: porta HTTP (default: 8080)
- `BASE_URL`: base URL pubblico (per link assoluti)
- `GOOGLE_SPREADSHEET_ID`: ID del documento Google Sheets
- `GOOGLE_SHEET_NAME`: foglio (tab) spese, default `Spese`
- `GOOGLE_CATEGORIES_SHEET_NAME`: foglio categorie, default `Categories`
- `GOOGLE_SUBCATEGORIES_SHEET_NAME`: foglio sottocategorie, default `Subcategories`
- `DATA_BACKEND`: `memory` (default) o `sheets`
- `DASHBOARD_SHEET_PREFIX`: pattern del foglio dashboard annuale da cui leggere i totali mensili (default: `%d Dashboard`, es. `2025 Dashboard`). Usato per mostrare la panoramica del mese sotto il form.

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
- `docker compose up -d` per esecuzione locale; la configurazione legge `.env` e lo inietta nei container (`env_file`).

## Setup Google Sheets (rapido)

1) Crea il documento e i fogli:
- Foglio spese (default `2025 Expenses`) con intestazioni in riga 1:
  - A: Month, B: Day, C: Expense, D: Amount, E: Currency, F: EUR, G: Primary, H: Secondary
- Foglio categorie (default `2025 Dashboard A2:A65`) 
- Foglio sottocategorie (default `2025 Dashboard B2:B65`)

2) OAuth user consent:
- Crea un OAuth Client (Tipo: App per desktop o Web con redirect `http://localhost:8085/callback`).
- Esporta `GOOGLE_OAUTH_CLIENT_FILE=/percorso/client.json` (o `GOOGLE_OAUTH_CLIENT_JSON`).
- **Opzione A** - Locale: Esegui `make oauth-init` e completa il consenso nel browser
- **Opzione B** - Docker: Esegui `make oauth-init-docker` per containerized OAuth flow
- Genera `token.json` (configura `GOOGLE_OAUTH_TOKEN_FILE` se vuoi un path diverso).
- Avvia l'app con `DATA_BACKEND=sheets` e le variabili OAuth impostate.

**Sicurezza OAuth:**
- Token salvati con permessi 0600 (solo proprietario)
- Auto-refresh dei token scaduti
- Nessun Service Account key committato nel repo

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
