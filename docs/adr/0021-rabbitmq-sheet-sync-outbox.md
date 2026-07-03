# ADR-0021: RabbitMQ sheet-sync outbox

## Status

Accepted.

## Context

SQLite is the local source of truth and Google Sheets is a derived mirror. App
writes must remain fast and available even when RabbitMQ or Google Sheets are
temporarily unavailable, but sheet-sync work still needs durable delivery.

## Decision

1. Feature writes store domain rows and a `sheet_sync_outbox` row in the same
   SQLite transaction.
2. The web process starts a background relay that publishes pending outbox
   rows to RabbitMQ with persistent messages and publisher confirms.
3. A separate `spese-worker` process consumes RabbitMQ messages with manual
   acknowledgements and rebuilds all source tabs from SQLite.
4. Failed worker exports are retried through a retry queue and eventually sent
   to a dead-letter queue.
5. Every sync event is treated as a full rebuild request. Duplicate messages
   are acceptable because the export is idempotent.

## Consequences

Positive:
- HTTP writes depend only on SQLite.
- RabbitMQ publishing can be retried after commit without losing sync work.
- The worker can be deployed, restarted, and scaled separately from the web
  process.

Negative:
- A working deployment now runs two app processes plus RabbitMQ.
- The web process needs RabbitMQ credentials when mirroring is enabled.
- Existing databases may retain obsolete internal tables from older queue
  implementations; cleanup is intentionally manual.
