# ADR-0007: Sheet-as-backup model

## Status
Accepted

## Context
The Google Sheet (`GE Net Worth`, ID `1CJf-cbHqTKOJRUi0unnwycLvnh4Smo13tzXzNUdimgc`)
currently mixes three roles:

1. **Raw store** — `YYYY Expenses`, `YYYY Incomes` tabs (flat append by app).
2. **Aggregation** — `YYYY Dashboard` pivots and `YYYY` Cash Flow / FIRE blocks.
3. **NW ledger** — Net Worth section in the `YYYY` tab (per-account × per-month
   cells written by ADR-0003).

ADR-0001..0006 made SQLite the de facto source of truth, but the sheet is still
edited each month for two reasons:

- Dashboard cells contain manually-entered or hard-pasted values rather than
  formulas, so new rows in `YYYY Expenses` do not propagate.
- Yearly rollover (new tabs, NW header column, formula re-pointing) is manual.

Goal: keep the sheet useful as (a) a human-readable backup and (b) the existing
dashboard surface, **without** monthly hand-editing. SQLite remains primary;
the sheet becomes a derived mirror.

## Decision

### Roles, fixed
- **SQLite**: primary, read/write.
- **Sheet raw tabs** (`YYYY Expenses`, `YYYY Incomes`, NW block of `YYYY`):
  app-managed mirror. Idempotent upsert by stable id.
- **Sheet derived cells** (`YYYY Dashboard`, Cash Flow, FIRE, Pick Months,
  Pick Expenses, history tabs): pure spreadsheet formulas. App never writes
  these.
- `DATA_BACKEND=sheets` mode is removed; `sqlite` is the only supported backend.

### Stable row id on raw tabs
- New **trailing** column `id` on `YYYY Expenses` and `YYYY Incomes`
  (after `note`). Trailing avoids shifting existing columns or invalidating
  dashboard formulas that reference column letters.
- Value: SQLite primary key (`expenses.id`, `incomes.id`) as decimal string.
- `core.Expense` and `core.Income` gain an `ID int64` field (matches the
  existing `RecurrentExpenses.ID` pattern). Local-insert flow leaves it zero;
  sync flow populates it from SQLite before handing off to the remote writer.
- New ports for remote (sheet) sync, distinct from the local-insert
  `ExpenseWriter`/`IncomeWriter` used by HTTP handlers:
  ```go
  RemoteExpenseWriter interface {
      UpsertExpense(ctx context.Context, e core.Expense) (rowRef string, err error)
  }
  RemoteIncomeWriter interface {
      UpsertIncome(ctx context.Context, i core.Income) (rowRef string, err error)
  }
  ```
- Behavior: locate sheet row whose `id` column equals `e.ID`; update in place
  if found, append otherwise. Require `e.ID > 0`; zero is `ErrMissingID`.
- The Google adapter implements the new ports. The sync processors
  (`SyncQueueProcessor`, `IncomeSyncProcessor`) switch to the new ports and
  pass the SQLite id through.
- The old Google adapter `Append`/`AppendIncome` methods are removed (no
  backwards-compat shim).

### Reconciliation CLI
- New subcommand `spese export-sheet --year YYYY [--dry-run]`.
- Walks SQLite for the year and upserts every expense/income/NW balance to the
  sheet through the existing adapters.
- Idempotent: re-running yields the same sheet state.
- Logs counts (added, updated, unchanged) and exits non-zero on any adapter
  error.

### Dashboard formulas (one-time sheet edit, documented)
For each row in `YYYY Dashboard` the value cell uses:
```
=SUMIFS('YYYY Expenses'!F:F,
        'YYYY Expenses'!G:G, $A<row>,
        'YYYY Expenses'!H:H, $B<row>,
        'YYYY Expenses'!A:A, COLUMN()-2)
```
Cash Flow rows in `YYYY` use `SUMIFS` against `YYYY Incomes` keyed by category;
tax rows reference Cash Flow constants (already formulas per ADR-0004).
Pick Months / Pick Expenses pivots reference the `YYYY Dashboard` totals.

The exact formula set lives in `docs/sheet-template.md` (new), not in code, so
a fresh sheet can be reproduced by hand or by the rollover CLI.

### Year rollover (manual, documented)
- A `spese roll-year YYYY+1` CLI was considered and deferred. Year rollover
  is a once-per-year, ~5-minute task in the Sheets UI; codifying it adds
  meaningful sheet-API surface area (duplicateSheet, rename, formula
  rewrite, NW header extension) for limited recurring value.
- The procedure lives in `docs/sheet-template.md`: duplicate the previous
  year's tabs, rename them, retarget formulas. The same document defines the
  expected raw-tab headers and the dashboard SUMIFS template, so a fresh
  sheet can be reproduced by hand without grepping the codebase.
- Reopen this section if the manual step turns out to be a recurring source
  of mistakes.

### Layout pinning
- New file `internal/sheets/google/layout.go` exposing constants for:
  - NW section labels and account-row offsets.
  - Raw-tab column order (`m, d, expense|income, amount, curr, EUR,
    primary, secondary?, note, id`).
- Adapter validates layout on first call per process; returns
  `ErrSheetLayoutMismatch` if any expected header is absent. Processor logs
  and pauses (status `error`) instead of writing into a shifted layout.

### Out of scope (this ADR)
- Snapshot / nightly full-dump tab.
- Bidirectional sync (sheet edits flowing back to SQLite).
- Liabilities in NW.
- UI for triggering `export-sheet` or `roll-year`.

These are tracked as follow-up ADRs.

## Inputs / Outputs

| Operation | Input | Output |
|---|---|---|
| Upsert expense row | `core.Expense` (with id) | sheet row at id, created or updated |
| Upsert income row | `core.Income` (with id) | sheet row at id, created or updated |
| `export-sheet --year Y` | year | counts (added/updated/unchanged) per tab |
| Layout check | — | nil or `ErrSheetLayoutMismatch` |

## Edge cases
- Existing sheet rows lack the `id` column → first run of `export-sheet`
  appends an `id` header at the next free column and fills ids for rows whose
  `(date, amount, primary, secondary, note)` match exactly one SQLite row;
  ambiguous matches are left blank and logged for manual reconciliation.
- Sheet row exists with id not present in SQLite → left untouched (treated as
  user note); logged at INFO. Not deleted automatically.
- SQLite row exists, id missing in sheet → appended.
- Concurrent edits during `export-sheet` → last-write-wins from app side; user
  warned in CLI banner.
- Layout mismatch (renamed section, removed account row) → adapter returns
  `ErrSheetLayoutMismatch`, processor stops syncing for that account; no
  partial writes.
- Quota / 5xx from Sheets API → existing retry/backoff in adapter applies;
  CLI exits non-zero after exhaustion.

## Error conditions
- Upsert by id finds duplicate ids → return `ErrDuplicateRowID`; no write.
- `export-sheet` on a year with no `YYYY Expenses` tab → caller-visible
  Sheets API error; user must roll the year manually first
  (see `docs/sheet-template.md`).
- Sheets auth error → propagated, no retry, exit non-zero.

## Test plan

1. **Acceptance**
   - `internal/sheets/google/google_test.go`:
     - Upsert with new id → row appended with id in column A.
     - Upsert with existing id → same row updated, no append.
     - Upsert with duplicate ids in sheet → `ErrDuplicateRowID`.
   - `internal/services/export_sheet_test.go`:
     - SQLite seeded with N expenses + M incomes + K NW balances → CLI produces
       a sheet with N+M+K rows; second run reports 0 added, 0 updated.
2. **Unit**
   - `internal/sheets/google/layout_test.go`: header validation + missing
     headers produce `ErrSheetLayoutMismatch`.
   - Id-backfill matcher: exact / ambiguous / no-match cases.

3. **Integration**
   - End-to-end against fake Sheets server: seed SQLite, run `export-sheet`,
     assert tab contents; mutate one row in SQLite, re-run, assert single
     update.
   - Layout drift simulation: rename a section row in the fixture → adapter
     surfaces `ErrSheetLayoutMismatch`; existing processors set queue rows to
     `error`.

## Migration plan
1. Land port renames (`Append*` → `Upsert*`) and id column adapter changes;
   keep behavior append-only until step 3.
2. Ship `export-sheet --dry-run`; verify diff against current sheet by hand.
3. Run `export-sheet` for `2024`, `2025`, `2026` to backfill ids.
4. Replace dashboard cells with formulas (manual, per `docs/sheet-template.md`).
5. Remove `DATA_BACKEND=sheets` code path.
