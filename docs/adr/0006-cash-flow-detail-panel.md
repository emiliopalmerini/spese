# ADR-0006: Cash flow detail panel

## Status
Proposed

## Context
The sheet's Cash Flow section breaks income down by source (`EFreelance`,
`GFreelance`, `2DP+`, `2DP`, `ESalary`, `GSalary`, `Gifts n reimbursements`,
`Gold`) and adds two negative rows for state taxes (`Imposta sostitutiva`,
`INPS`) computed in ADR-0004.

This ADR adds a read-only dashboard panel that mirrors that view: rows are
income categories + tax accruals; columns are months of the current year;
total per row + grand total.

No new storage.

## Decision

### Aggregation (internal/storage)
- New methods:
  - `MonthlyIncomeByCategory(ctx, year int) (map[string][12]int64, error)` — keyed by income category.
  - `MonthlyTaxAccrualsByCode(ctx, year int) (map[string][12]int64, error)` — keyed by `tax_code` (from ADR-0004).
- Both fill all 12 months (zeros where missing).

### Service (internal/services/cash_flow_service.go)
- Combines incomes and accruals into one shape:
  ```go
  type CashFlowRow struct {
      Label    string   // category name or tax label
      Group    string   // "income" | "tax"
      Section  string   // e.g., "Sole Proprietorship", "Employment", "Other", "Tasse statali"
      Months   [12]int64 // cents (negative for tax)
      Total    int64
  }
  ```
- Section assignment uses a small static map by category name (configurable in
  a follow-up ADR if it grows).

### Handler (internal/http/handlers_dashboard.go)
- `GET /ui/dashboard/cash-flow?year=YYYY` (default current year).
- Renders `cash_flow_panel` partial.

### Template (web/templates/partials/cash_flow_panel.html)
- Grouped table with section headers.
- Negative values rendered with a leading `-` and the existing
  `stat-pill__value--negative` style (or equivalent).

### Dashboard integration
- New collapsed accordion below `Pick Months`:
  ```html
  <section class="page__section">
    <div class="accordion" id="cashFlowAccordion">
      <button class="accordion__trigger" type="button">
        <h3 class="accordion__title">Cash Flow</h3>
        <svg class="accordion__icon" viewBox="0 0 24 24"><polyline points="6 9 12 15 18 9"/></svg>
      </button>
      <div class="accordion__content">
        <div class="accordion__body" id="cash-flow-content"
             hx-get="/ui/dashboard/cash-flow"
             hx-trigger="load, dashboard:refresh from:body"
             hx-swap="innerHTML">
          <div class="skeleton" style="height: 220px;"></div>
        </div>
      </div>
    </div>
  </section>
  ```

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Cash flow for year | year | grouped rows × 12 months + totals |

## Edge cases
- Year with no income → placeholder rendered.
- Income category not in the section map → bucketed under `Other`.
- Tax accrual without a matching income (orphan) → still rendered under
  `Tasse statali` with the accrual's date.

## Error conditions
- DB error → 500 + slog.
- Invalid `year` → 400.

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - Seed incomes in two categories + a freelance income (auto-creates accruals)
     → GET `/ui/dashboard/cash-flow` returns both income rows and two tax rows
     with negative totals.
   - Empty year → placeholder.
2. **Unit**:
   - Section mapping: known + unknown categories.
   - Twelve-month padding.
3. **Integration**:
   - End-to-end with in-memory SQLite + ADR-0004 service to verify combined view.

## Out of scope
- Editable section mapping UI.
- Multi-year comparison.
- Sheet sync of accruals back to the Cash Flow section (deferred; can use the
  same enqueue pattern as ADR-0003 in a future ADR).
- Net cash-flow row (total income − total taxes − total expenses) — keep panel
  scoped to source-level breakdown.
