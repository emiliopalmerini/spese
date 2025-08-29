# ADR 0001 — Linguaggi e Stack

Status: Accepted

## Contesto

Costruiamo una piccola applicazione per registrare spese su un Google Spreadsheet, con UI semplice e reattiva senza complessità SPA. I vincoli principali: avvio rapido, footprint ridotto, facilità di deploy, team con forte competenza in Go, minima dipendenza da JS, integrazione Google Sheets.

## Decisione

- Linguaggio backend: Go (>= 1.22).
- Rendering: HTML server-side con `html/template`; interazioni incrementali via HTMX (hx-get/post/swap).
- Frontend JS: nessun framework; solo HTMX e JS minimo ove strettamente necessario.
- Styling: CSS leggero (senza framework obbligatorio; eventuale utility CSS minimale opzionale).
- Persistenza: Google Sheets come System of Record (append riga per spese, lettura categorie/sottocategorie).
- Autenticazione verso Google: Service Account con condivisione mirata dello spreadsheet.
- Packaging: Dockerfile multistage (builder + runner snello) e Docker Compose per locale.
- Automazione: Makefile per dev/build/test; pre-commit per formattazione e lint.
- CI: fmt, vet/lint, test con `-race` e build immagine.

## Conseguenze

- Pro:
  - Semplicità operativa, immagine piccola, avvio veloce.
  - Poco JS, UI reattiva grazie a HTMX con costi contenuti.
  - Portabilità: deploy container-first ovunque.
- Contro:
  - Google Sheets non è una base dati transazionale; limiti di concorrenza e rate limit.
  - Query/ricerche avanzate limitate; costi di trasformazione lato app.
  - Mancanza di framework frontend può richiedere più disciplina sui template.

