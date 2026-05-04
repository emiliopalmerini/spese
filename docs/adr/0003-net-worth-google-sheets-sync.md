# ADR-0003: Net Worth Google Sheets sync

## Status
Proposed

## Context
ADR-0001 owns the data; ADR-0002 lets the user enter it. This ADR adds a one-way
sync from app to the existing dashboard sheet (the same spreadsheet currently
written to for expenses/incomes).

The sheet has a fixed Net Worth layout:

```
| Asset/Liability | Cur | YYYY | Jan | Feb | Mar | ... | Dec | YYYY+1 |
| Net Worth |
| Cash - Liquidity |
|   <account rows...> |
| (blank) |
| Rain day funds |
|   <account rows...> |
| (blank) |
| Long Term investment |
|   <account rows...> |
```

Account rows live in fixed positions inside their section; columns map to
months. The app must update the cell at row(account) × col(month) when a
balance is upserted.

## Decision

### Port (internal/sheets/ports.go)
```go
NetWorthWriter interface {
    UpsertBalance(ctx context.Context, accountName string, accountType core.AccountType,
                   year, month int, amount core.Money) (rowRef string, err error)
}
```

### Sync queue
- Migration `000016_create_nw_sync_queue`:
  ```sql
  CREATE TABLE nw_sync_queue (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    year          INTEGER NOT NULL,
    month         INTEGER NOT NULL,
    amount_cents  INTEGER NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','synced','error')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    synced_at     TIMESTAMP
  );
  CREATE INDEX idx_nw_sync_status ON nw_sync_queue(status);
  ```
- On `UpsertBalance` repo call, also enqueue a sync row in the same transaction
  (insert-or-update by `(account_id, year, month)` keeping latest amount,
  resetting status to `pending`).

### Processor (internal/services/nw_sync_processor.go)
- Mirrors the existing `SyncQueueProcessor`/`IncomeSyncProcessor` pattern.
- Tick interval: reuse `SYNC_INTERVAL` env (default 30s).
- For each pending row:
  1. Resolve account name + type from `accounts`.
  2. Call `NetWorthWriter.UpsertBalance`.
  3. Mark `synced` on success; on error, increment attempts, store `last_error`,
     set `error` after N attempts (use existing constant).

### Google adapter (internal/sheets/google/google_networth.go)
- Read sheet layout once (cached): scan column A starting at the Net Worth header
  for section labels; for each section, the rows that follow until a blank row
  are accounts.
- If account name is missing in its section, append a new row inside that section
  before the trailing blank.
- Locate target column from header row: match `Jan`/`Feb`/... for the requested
  month and the requested year header (`YYYY`).
- Write amount in EUR (no formatting beyond the existing number formatting in the
  template sheet).

### Wiring (cmd/spese/main.go)
- Construct `NetWorthSyncProcessor` with `NetWorthWriter` from the Google adapter
  and start it alongside the existing processors.

## Inputs / Outputs

| Operation | Input | Output |
|---|---|---|
| Enqueue sync | account_id, year, month, amount | queue row |
| Process tick | — | per-row HTTP write to Sheets |

## Edge cases
- New account — adapter creates a row in its section.
- Section not present in sheet — return error; nothing auto-created (avoid corrupting layout).
- Year not in header — return error; user must pre-extend the sheet.
- Multiple pending rows for same (account, year, month) — collapsed by upsert in repo, only one queue entry remains.
- Sheet API quota errors — handled by existing retry mechanics.

## Error conditions
- Google API errors propagate to processor, increment attempts, written to `last_error`.
- After max attempts, status `error`; row inspectable via existing admin tooling (none yet — out of scope).

## Test plan
1. **Acceptance** (`internal/services/nw_sync_processor_test.go`):
   - Pending row → processor calls writer once → status `synced`.
   - Writer error → status remains `pending`, attempts increments.
   - Max attempts exceeded → status `error`.
2. **Unit** (`internal/sheets/google/google_networth_test.go`):
   - Section/row resolution from a fixture sheet snapshot.
   - Column resolution for given year/month.
3. **Integration**:
   - End-to-end: balance upsert via repo enqueues, processor flushes to a fake Sheets server.

## Out of scope
- Bidirectional sync.
- Liabilities.
- Sheet auto-extension across years.
- Reconciliation report.
