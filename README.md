# Spese v2

Spese è un’applicazione single-user per movimenti, saldi, riconciliazioni e analisi finanziarie. SQLite è l’unica fonte di verità; Google Sheets è un mirror/export asincrono derivato tramite outbox e RabbitMQ.

Il runtime HTTP è un singolo binario Go che incorpora la SPA React e serve UI, API e dettatura dalla stessa origine.

## Stack

- Go 1.25, SQLite e migrazioni numerate transazionali
- React 19, TypeScript strict, Vite 8 e Tailwind CSS 4
- componenti shadcn/ui personalizzati, React Router, TanStack Query, React Hook Form e Zod
- Recharts attraverso il wrapper chart shadcn
- RabbitMQ e transactional outbox per il mirror Google Sheets
- ElevenLabs e OpenCode opzionali per la dettatura
- Node.js 24 LTS solo per sviluppo e build, mai nel runtime

## Avvio locale

```bash
nix develop
make frontend-install
make build
make run-demo
```

Aprire `http://127.0.0.1:8080`. `make run-demo` rigenera esclusivamente `tmp/spese-demo.db`; il seeder rifiuta percorsi che non contengono `demo`.

Per lavorare con HMR:

```bash
make run-demo
npm --prefix frontend run dev
```

Vite inoltra `/api` a `127.0.0.1:8080` e riscrive l’Origin verso la stessa origine backend. Il deployment non richiede né espone un server Vite.

## Comandi

```text
make frontend-install  npm ci dal lockfile
make frontend-build    typecheck e build Vite in web/dist
make build             SPA + spese + worker + tool migrazione
make test              race test Go + typecheck/test frontend
make lint              go vet, golangci-lint se presente, ESLint
make test-e2e          flussi Playwright desktop e mobile
make run               server locale
make run-demo          dati dimostrativi e server senza servizi esterni
make run-local         server con outbox/RabbitMQ e mirror JSON locale
make run-worker-local  worker del mirror JSON locale
make smoke-local       verifica HTTP/outbox/worker locale
make nix-build         artefatti Nix
```

Build riproducibile da checkout pulita:

```bash
npm --prefix frontend ci
npm --prefix frontend run build
go test -race ./...
go build -o bin/spese ./cmd/spese
```

## Runtime

Il server espone:

- `/`, `/movimenti`, `/analisi`, `/ricorrenti`, `/conti`, `/categorie`, `/impostazioni`: SPA
- `/api/v1/*`: API JSON versionata
- `/api/v1/openapi.yaml`: contratto OpenAPI 3.1
- `/api/v1/dictation/*`: fallback audio, conferma batch e WebSocket quando abilitati
- `/healthz`: liveness del processo
- `/readyz`: readiness SQLite
- `/assets/*`: asset Vite hashati con cache immutabile

Il documento HTML non è memorizzabile in cache. Route API, health e WebSocket non ricevono mai il fallback SPA.

## Contabilità

Il ledger usa un movimento logico con posting firmati:

- attività positive e passività negative;
- spesa: posting negativo sul conto;
- entrata e rimborso: posting positivo;
- trasferimento: due posting su conti diversi con somma zero e nessuna categoria;
- draft e void non incidono sui saldi;
- le allocazioni coprono integralmente spese, entrate e rimborsi;
- un rimborso riduce la spesa della categoria originale;
- gli importi sono sempre centesimi `int64` nel backend.

Il saldo a una data parte dall’ultima riconciliazione con `closed_through <= asOf`, oppure dal saldo iniziale, e applica i posting successivi. Un movimento retrodatato prima di un anchor cambia le analisi storiche ma non il saldo successivo all’anchor.

## Migrazione legacy

`spese-migrate` opera solo sul file indicato e non contatta servizi esterni. Il default è un dry-run su copia SQLite isolata:

```bash
go run ./cmd/spese-migrate -db /copia/spese.db
```

Il report JSON include versioni schema, tabelle legacy preservate, record convertiti, totali mensili e verifica dei trasferimenti zero-sum. Per applicare:

```bash
go run ./cmd/spese-migrate -db /copia/spese.db -dry-run=false
```

Prima dell’applicazione viene creato un backup consistente con `VACUUM INTO`. Anche l’avvio applicativo crea automaticamente un backup `*.pre-v2-*.bak` prima di trasformare un database non versionato. Le tabelle originali restano `legacy_*` durante la finestra di rollback.

Ripristino testato:

```bash
go run ./cmd/spese-migrate -db /copia/spese.db -restore /copia/spese.db.backup-TIMESTAMP
```

Il comando preserva a sua volta il file corrente come `*.before-restore-*`. Fermare web e worker prima di migrare o ripristinare. Non eseguire questi comandi direttamente sul file di produzione senza copia e finestra operativa.

La conversione:

- assegna ID UUID deterministici a conti, categorie e record legacy;
- deduplica categorie e sottocategorie senza distinzione maiuscole/minuscole;
- converte entrate, spese e adjustment mantenendone la natura;
- accoppia trasferimenti solo con data, importo opposto, conti distinti e nota coincidenti;
- interrompe l’intera transazione in presenza di match mancanti o ambigui;
- converte gli snapshot `YYYY-MM`, documentati come saldi di fine mese, nell’ultimo giorno reale del mese;
- mantiene i dati sorgente nelle tabelle `legacy_*`.

## Ricorrenze

Il catch-up parte all’avvio e viene rieseguito ogni ora. `UNIQUE(rule_id, scheduled_for)` e la transazione che include occurrence, movimento, posting, allocazioni e avanzamento della regola impediscono duplicati anche con retry concorrenti. I giorni 29–31 vengono limitati all’ultimo giorno valido.

Gli importi fissi usano `auto_post`; quelli variabili sono bozze `needs_confirmation` e restano stimati nel forecast. Le modifiche alle regole non riscrivono occurrence o movimenti passati.

## Mirror Sheets

Ogni mutation locale registra un evento nella stessa transazione SQLite. Il relay pubblica su RabbitMQ con conferma; `spese-worker` ricostruisce tab derivate (`accounts`, `categories`, `movements`, `postings`, `allocations`, `reconciliations`, `recurring_rules`). Le celle vengono scritte come `RAW` per evitare formula injection.

Il mirror può essere `none`, `local` o `google`:

```bash
SPESE_SHEET_MIRROR_BACKEND=local
SPESE_LOCAL_SHEET_PATH=tmp/local-sheet.json
SPESE_RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

Per Google servono anche `GOOGLE_SPREADSHEET_ID` e `GOOGLE_SERVICE_ACCOUNT_FILE`. Il worker deve condividere lo stesso file SQLite persistente del web process.

## Configurazione

Variabili principali:

```text
SPESE_HOST=127.0.0.1
SPESE_PORT=8080
SPESE_DB_PATH=spese.db
SPESE_TIMEZONE=Europe/Rome
SPESE_SHEET_MIRROR_BACKEND=auto|none|local|google
SPESE_RABBITMQ_URL=...
SPESE_RABBITMQ_QUEUE=spese.sheet-sync
SPESE_LOCAL_SHEET_PATH=tmp/local-sheet.json
```

Dettatura opzionale:

```text
SPESE_DICTATION_ENABLED=true
ELEVENLABS_API_KEY=...
SPESE_OPENCODE_URL=http://127.0.0.1:4096
SPESE_OPENCODE_USERNAME=opencode
SPESE_OPENCODE_PASSWORD=...
SPESE_OPENCODE_PROVIDER=...
SPESE_OPENCODE_MODEL=...
SPESE_OPENCODE_AGENT=dictation
```

Le chiavi restano nel server. I test usano adapter e route mockate, non effettuano chiamate ElevenLabs/OpenCode reali.

## Sicurezza

Spese conserva intenzionalmente il modello single-user e non implementa login, multi-tenancy o ruoli. Il default ascolta solo su `127.0.0.1`. Per accesso remoto usare TLS e autenticazione nel reverse proxy; non pubblicare direttamente la porta applicativa.

Il server applica Origin check e header CSRF same-origin alle mutation, Origin check ai WebSocket, request ID, body limit, validazione audio, timeout HTTP, CSP, frame denial, MIME sniffing denial e foreign key SQLite su ogni connessione usata. I log non includono body, audio o credenziali.

## Docker

```bash
docker compose up --build
```

Compose usa il mirror JSON locale per default, porta 8080 e un volume condiviso `/data`. Per Google aggiungere credenziali e bind mount tramite un override Compose, senza commetterle.

## ADR

Le decisioni storiche sono in `docs/adr`. ADR-0022 descrive il big-bang v2 e supersede le parti incompatibili degli ADR precedenti; ADR-0021 resta valido per outbox, RabbitMQ e mirror derivato.
