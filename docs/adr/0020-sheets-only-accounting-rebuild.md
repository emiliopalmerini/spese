# ADR-0020: Sheets-only accounting rebuild

## Status

Accepted (supersedes ADR-0007).

## Context

The original spese app stored expense and income rows in SQLite and pushed them
to a Google Sheet asynchronously. ADR-0007 declared SQLite the source of truth
and the sheet a derived backup. Over time, the sheet grew into the actual
analytical surface: net-worth grids, FIRE projections, pivot tables. The app
became a glorified data-entry form for that sheet.

Meanwhile the schema in the sheet had drifted into something hard to query:
one tab per year for expenses and income, free-form NW grids, ad-hoc category
columns. Reports were patched in place rather than rebuilt from first
principles.

A new spreadsheet ("GE Net Worth v2") was built from scratch in long format
with an accounting model: a single `transactions` general journal, an
`accounts` chart of accounts, and month-end `snapshots`. All reports (balance
sheet, income statement, NW timeline, investments) are derived view tabs
inside the sheet.

## Decision

1. The Google Sheet is the single source of truth. SQLite, the sync queue,
   and all backend selection code are removed.
2. The app's job is data entry into source tabs (`transactions`,
   `snapshots`, `accounts`, `recurring`) and rendering reports by reading
   view tabs (`v_*`, `dashboard`). It does not compute aggregates locally.
3. The codebase is reorganised by vertical slice. Each feature lives under
   `internal/features/<feature>/` and owns its handler, business logic, and
   sheet I/O. Shared low-level concerns are `internal/sheets/` (Sheets API
   client) and `internal/kernel/` (small value types: Money, Date).
4. Writes are append-only. Edit and delete happen directly in the sheet.
5. The recurring-expense processor stays, but fires on a configured
   `day_of_month` and appends transactions of `kind=Expense`.
6. The UI is rebuilt with HTMX + Go templates and matches the polish level
   of the previous spese UI (italianised labels, same visual language).

## Consequences

Positive:
- Single source of truth eliminates sync drift and the queue subsystem.
- Reports are correct by construction; we render what the sheet says.
- Vertical slices keep each feature self-contained and easy to extend.

Negative:
- The app is now dependent on Google Sheets API availability and rate limits.
  We mitigate with an in-memory read cache validated by Sheets ETags on every
  read, invalidated on write.
- Edit and delete require opening the sheet. Acceptable trade-off for
  append-only simplicity in the app.

## Reversal

ADR-0007 is reversed. SQLite is no longer the source of truth; the sheet is.
