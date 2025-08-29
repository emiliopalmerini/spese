# ADR 0002 — DDD e Architettura Esagonale

Status: Accepted

## Contesto

Vogliamo mantenere il dominio indipendente da dettagli infrastrutturali (Google Sheets, HTTP), facilitare testabilità e futura sostituibilità degli adapter (es. passaggio a DB). La dimensione del progetto resta piccola ma con necessità di limiti chiari.

## Decisione

- DDD leggero:
  - Entità principali: Expense, Category, Subcategory.
  - Value Objects: Amount (cent), DateParts (day, month), CategoryPath (category/subcategory).
  - Regole di validazione nel dominio (non nei controller).
- Ports & Adapters (Hexagonal):
  - Ports (interfacce) esposte dal dominio/applicazione, implementate dagli adapter.
  - Adapter in ingresso: HTTP (handlers + templates HTMX).
  - Adapter in uscita: Google Sheets client/repository.
  - Il dominio non importa pacchetti Google; dipende solo da interfacce.
- Struttura pacchetti:
  - `internal/core/`: modelli e servizi di dominio, ports.
  - `internal/sheets/`: adapter verso Google Sheets (implementazione ports).
  - `internal/http/`: router, middleware, handlers (adapter in ingresso).
  - `cmd/spese/`: composition root (wire di dipendenze, config, logging).
- Gestione errori: error wrapping con `%w`, errori di dominio espliciti (es. ErrInvalidAmount).
- DI semplice: costruttori che ricevono le interfacce necessarie (ports) e config.

## Conseguenze

- Pro:
  - Test unitari facili (mock degli adapter via ports), coupling ridotto.
  - Sostituibilità del backend dati (Sheets → DB) senza toccare il dominio.
  - Codice HTTP più snello: traduce I/O e delega al dominio.
- Contro:
  - Leggera complessità iniziale (boilerplate interfacce/adapter).
  - Rischio di over-engineering se non mantenuta disciplina YAGNI.

