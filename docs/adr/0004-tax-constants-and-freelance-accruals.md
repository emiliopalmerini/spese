# ADR-0004: Tax constants and freelance taxation accruals

## Status
Accepted

## Context
The sheet's Cash Flow section shows two negative rows tied to freelance income:
`Imposta sostitutiva` and `INPS`. Both are computed as a percentage of the
underlying freelance gross. The user wants the app to compute these from
constants × freelance income (option (c)) rather than entering them by hand.

Freelance income is identified by category. The current `incomes` table already
stores categories such as `GFreelance`, `EFreelance`, `2DP+`, etc.

## Decision

### Tax rates store
- Migration `000017_create_tax_rates`:
  ```sql
  CREATE TABLE tax_rates (
    code            TEXT PRIMARY KEY,
    label           TEXT NOT NULL,
    rate_basis_pts  INTEGER NOT NULL,             -- basis points (e.g., 500 = 5.00%)
    valid_from      DATE NOT NULL,
    valid_to        DATE,                          -- nullable, exclusive upper bound
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
  );
  CREATE INDEX idx_tax_rates_code_period ON tax_rates(code, valid_from);
  ```
- Seed rows on first migration:
  - `imposta_sostitutiva`: 500 bps (5%) (placeholder; user can change via UI later)
  - `inps`: 2613 bps (26.13%) (placeholder)
- Lookup rule: pick row where `code = ?` and the income date falls in
  `[valid_from, valid_to)`; pick the most recent if `valid_to` is null.

### Freelance categories
- New table `freelance_income_categories`:
  ```sql
  CREATE TABLE freelance_income_categories (
    category TEXT PRIMARY KEY,
    active   INTEGER NOT NULL DEFAULT 1
  );
  ```
- Seed: `GFreelance`, `EFreelance`, `2DP+`, `2DP`. (Editable later via admin
  page; out of scope here.)
- An income's category being in this table marks it as freelance for tax
  purposes.

### Tax accruals
- Migration `000018_create_tax_accruals`:
  ```sql
  CREATE TABLE tax_accruals (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    income_id       INTEGER NOT NULL REFERENCES incomes(id) ON DELETE CASCADE,
    tax_code        TEXT NOT NULL REFERENCES tax_rates(code),
    rate_basis_pts  INTEGER NOT NULL,             -- snapshot at compute time
    amount_cents    INTEGER NOT NULL,             -- positive; rendered as negative
    date            DATE NOT NULL,                 -- mirrors income.date
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (income_id, tax_code)
  );
  CREATE INDEX idx_tax_accruals_date ON tax_accruals(date);
  ```

### Service hook (internal/services)
- New `TaxAccrualService`:
  - `OnIncomeCreated(ctx, income)`:
    - If income.Category not in `freelance_income_categories`, return.
    - For each active tax_code (`imposta_sostitutiva`, `inps`):
      - Resolve rate at income.Date.
      - Compute `amount = round(income.AmountCents * rate_bps / 10000)`.
      - Insert into `tax_accruals` (idempotent via UNIQUE).
- Wire into the existing income creation path (HTTP handler / income service).

### Display (internal/http/handlers_dashboard.go)
- `GET /ui/dashboard/cash-flow` partial (extended in ADR-0006) shows accrued
  taxes per month as negative rows under a `Tasse statali` header.

### Sync (sheets)
- Out of scope here. ADR-0006 plans the cash-flow partial; sync of accruals to
  the sheet's Cash Flow section is deferred.

## Inputs / Outputs

| Operation | Input | Output |
|---|---|---|
| Resolve rate | code, date | basis points |
| Compute accrual | income | rows in `tax_accruals` |
| Sum accruals for month | year, month, code | total cents |

## Edge cases
- Income deleted → accruals cascade-deleted.
- Income amount edited → accruals are NOT auto-recomputed (income edits are
  rare; we re-emit on a future ADR if needed).
- Rate not configured for a code → skip that code, log warn.
- Income outside any rate window → skip with warn.
- Non-freelance category → no accrual.

## Error conditions
- Compute errors logged, do not block income creation.
- Validation: `rate_basis_pts >= 0`, `valid_to > valid_from` if set.

## Test plan
1. **Acceptance** (`internal/services/tax_accrual_test.go`):
   - Create freelance income → two accrual rows with correct amounts.
   - Create non-freelance income → zero accruals.
   - Idempotency: calling twice produces no duplicates.
2. **Unit**:
   - Rate resolver: pick by date window, pick latest if open-ended.
   - Rounding: half-even on cents.
3. **Integration**:
   - In-memory SQLite end-to-end: insert income via service → verify accruals.

## Out of scope
- UI for editing tax rates.
- Sheets sync of accruals (will reuse ADR-0006 pipeline).
- Recompute on income edit/delete (delete is handled by FK cascade).
- Multi-currency.
