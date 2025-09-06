# Spese (Go + HTMX)

Semplice app per registrare spese su un Google Spreadsheet.
- Data automatica (giorno e mese) precompilata nel form.
- Inserimento descrizione e valore spesa.
- Categorie e sottocategorie lette dallo Spreadsheet.

Stack: Go, HTMX, SQLite, RabbitMQ, Google Sheets API, Docker (multistage), Docker Compose, Makefile, pre-commit.

## Requisiti

- Go 1.22+
- Docker + Docker Compose (per container)
- Accesso a Google Sheets API via OAuth (client + token)

## Esecuzione locale

1) Configura le variabili d'ambiente (vedi sotto).
   - Copia l'esempio: `cp .env.example .env`
   - Modifica `.env` con i tuoi valori. Docker Compose carica `.env` e lo inietta nei container.

Esempio di `.env` (nomi base senza anno; l'app prefigge automaticamente l'anno corrente):

```bash
GOOGLE_SPREADSHEET_ID=...
GOOGLE_SHEET_NAME=Expenses
GOOGLE_CATEGORIES_SHEET_NAME=Dashboard
GOOGLE_SUBCATEGORIES_SHEET_NAME=Dashboard
# Nome base del foglio dashboard usato per il riepilogo mensile
# L'app costruisce "<anno> <nome>", es: "2025 Dashboard"
DASHBOARD_SHEET_NAME=Dashboard
# (fallback legacy) Pattern con %d: es. "%d Dashboard"
# DASHBOARD_SHEET_PREFIX="%d Dashboard"

DATA_BACKEND=memory # usa 'sheets' per integrare Google Sheets direttamente o 'sqlite' per storage locale + sync asincrono
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
- `GOOGLE_SHEET_NAME`: base name del foglio spese (senza anno), default `Expenses` → risolto a `"<anno> Expenses"`
- `GOOGLE_CATEGORIES_SHEET_NAME`: base name foglio categorie, default `Dashboard` → `"<anno> Dashboard"`
- `GOOGLE_SUBCATEGORIES_SHEET_NAME`: base name foglio sottocategorie, default `Dashboard` → `"<anno> Dashboard"`
- `DATA_BACKEND`: `memory` (default), `sheets`, o `sqlite`
- `DASHBOARD_SHEET_NAME`: base name del foglio dashboard annuale da cui leggere i totali (preferito). Risultato: `"<anno> <nome>"`.
- `DASHBOARD_SHEET_PREFIX`: (legacy) pattern o prefisso del foglio dashboard annuale (es. `%d Dashboard`). Usato solo se `DASHBOARD_SHEET_NAME` non è impostato.

SQLite + AMQP (backend `sqlite`):
- `SQLITE_DB_PATH`: percorso database SQLite (default: `./data/spese.db`)
- `AMQP_URL`: connessione RabbitMQ (default: `amqp://guest:guest@localhost:5672/`)
- `AMQP_EXCHANGE`: nome exchange AMQP (default: `spese`)
- `AMQP_QUEUE`: nome coda AMQP (default: `sync_expenses`)
- `SYNC_BATCH_SIZE`: dimensione batch worker (default: `10`)
- `SYNC_INTERVAL`: intervallo sync periodico (default: `30s`)

OAuth:
- `GOOGLE_OAUTH_CLIENT_JSON` oppure `GOOGLE_OAUTH_CLIENT_FILE`: credenziali client OAuth (JSON)
- `GOOGLE_OAUTH_TOKEN_JSON` oppure `GOOGLE_OAUTH_TOKEN_FILE`: token utente generato via consenso

## Comandi Makefile utili

- `make setup`: setup strumenti dev (pre-commit, linters)
- `make tidy`: gestisce mod Go
- `make build`: compila binario principale
- `make build-worker`: compila worker di sincronizzazione
- `make build-all`: compila entrambi i servizi
- `make run`: esegue app principale in locale
- `make run-worker`: esegue worker in locale
- `make sqlc-generate`: rigenera codice sqlc dopo modifiche schema
- `make test`: unit test con race/coverage
- `make lint`: lints e vet
- `make fmt`: formatta codice
- `make docker-build`: build immagine Docker
- `make docker-up`: avvia stack con Compose
- `make docker-logs`: segue i log

## Architettura SQLite + AMQP (Raccomandato)

Per migliorare le performance e la scalabilità, l'app supporta un'architettura asincrona:

1. **App principale** (`cmd/spese`): salva le spese in SQLite locale (veloce)
2. **Messaggio AMQP**: pubblica ID spesa + versione (leggero, ~100 byte)
3. **Worker background** (`cmd/spese-worker`): consuma messaggi, legge spesa completa da SQLite, sincronizza con Google Sheets
4. **Fallback graceful**: funziona anche senza RabbitMQ (solo SQLite locale)

Vantaggi:
- **Performance**: risposte HTTP immediate (solo SQLite)
- **Affidabilità**: messaggi persistenti, retry automatici
- **Scalabilità**: worker indipendenti, elaborazione batch
- **Resilienza**: continua a funzionare anche se Google Sheets non è disponibile

Per usare questa modalità:
```bash
# Avvia RabbitMQ
docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management

# Avvia app principale con SQLite
DATA_BACKEND=sqlite AMQP_URL=amqp://localhost:5672 make run

# In un altro terminale, avvia il worker
DATA_BACKEND=sqlite AMQP_URL=amqp://localhost:5672 make run-worker
```

O più semplicemente con Docker Compose (include RabbitMQ, app e worker):
```bash
make docker-up
```

## Docker

- Dockerfile multistage per immagini piccole (builder + runner distroless/alpine).
- `docker compose up -d` per esecuzione locale; la configurazione legge `.env` e lo inietta nei container (`env_file`).
- Include RabbitMQ con management UI su http://localhost:15672 (admin/admin).

## Setup Google Sheets (rapido)

1) Crea il documento e i fogli:
- Foglio spese (es. `2025 Expenses`) con intestazioni in riga 1:
  - A: Month, B: Day, C: Expense, D: Amount, E: Currency, F: EUR, G: Primary, H: Secondary
- Foglio categorie (es. `2025 Dashboard` colonna `A2:A65`) 
- Foglio sottocategorie (es. `2025 Dashboard` colonna `B2:B65`)

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

Troubleshooting OAuth (Docker):
- Place your OAuth client file at `./configs/client.json` or set `GOOGLE_OAUTH_CLIENT_FILE` to a path inside the container and bind-mount it.
- The compose profile `oauth-init` binds `./configs/client.json` to `${GOOGLE_OAUTH_CLIENT_FILE:-/client.json}` so the default works out-of-the-box.
- If the token lacks `refresh_token`, revoke previous consent in your Google Account and rerun `make oauth-init-docker` (the flow forces `prompt=consent`).

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
