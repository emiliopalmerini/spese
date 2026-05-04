# ADR-0005: Pick Months pivot view

## Status
Proposed

## Context
The sheet has a `Pick Months` table that pivots primary expense categories
(columns) by month (rows) for the current year, plus a row total. It's a
display-only summary that complements the existing category-by-period chart in
the dashboard. Pure derivation from `expenses`; no new storage.

## Decision

### Aggregation (internal/storage)
- New repository method `MonthlyExpensesByPrimary(ctx, year int) ([]MonthlyPrimaryRow, error)`:
  - Returns one row per month (1..12) including months with zero spend.
  - Each row has: `Month int`, `ByPrimary map[string]int64` (cents), `Total int64`.
- sqlc query: `GROUP BY strftime('%m', date), primary_category` then assembled in Go.
- Excludes `Lavoro` (matches sheet's "Pick Months" exclusion of work expenses
  from monthly spending pivot — see sheet rows 214–223 where `Lavoro` is
  absent from the pivot total even though it appears in the per-category list).

### Handler (internal/http/handlers_dashboard.go)
- `GET /ui/dashboard/pick-months?year=YYYY` (defaults to current year).
- Returns `pick_months` partial.

### Template (web/templates/partials/pick_months.html)
- HTML table:
  - Columns: Mese, then one column per active primary category, then Totale.
  - Rows: Jan…Dec; values formatted via existing `formatEuros`.
  - Empty state when year has no expenses: a small "Nessun dato" placeholder.

### Dashboard integration
- New collapsed accordion section after the existing categories block:
  ```html
  <section class="page__section">
    <div class="accordion" id="pickMonthsAccordion">
      <button class="accordion__trigger" type="button">
        <h3 class="accordion__title">Pick Months</h3>
        <svg class="accordion__icon" viewBox="0 0 24 24"><polyline points="6 9 12 15 18 9"/></svg>
      </button>
      <div class="accordion__content">
        <div class="accordion__body" id="pick-months-content"
             hx-get="/ui/dashboard/pick-months"
             hx-trigger="load, dashboard:refresh from:body"
             hx-swap="innerHTML">
          <div class="skeleton" style="height: 200px;"></div>
        </div>
      </div>
    </div>
  </section>
  ```

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Pivot for year | year | 12 rows × N primary categories + total |

## Edge cases
- Year with no data → render placeholder, not an empty table.
- Primary category empty/null → bucketed under "Senza categoria".
- Primary category appears mid-year only → still gets a column with zeros for other months.
- Future months → still listed (zero values) so the table is always 12 rows.

## Error conditions
- DB error → 500 + slog.
- Invalid `year` query param → 400.

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - Insert expenses across two months → GET `/ui/dashboard/pick-months` includes both.
   - Empty year → placeholder rendered.
2. **Unit**:
   - Pivot assembly: months with no data still present.
   - `Lavoro` exclusion verified by fixture.
3. **Integration**:
   - sqlc query returns expected rows on in-memory SQLite.

## Out of scope
- Multi-year view.
- CSV export.
- Drill-down into secondary categories.
