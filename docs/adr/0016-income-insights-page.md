# ADR-0016: Income insights page

## Status
Accepted

## Context

`/entrate` mirrors the pre-ADR-0015 `/recurrent` page: it leads with an
inline create form, then renders a thin `Panoramica Mensile Entrate`
(monthly total + flat category bars + per-entry list with delete). The FAB
already offers an "Entrata" action that opens the same create form in a
bottom sheet (`/ui/form/income`), so the inline form is redundant and
pushes the data below the fold.

This ADR turns `/entrate` into an insights surface, analogous to ADR-0015
for recurrents.

## Decision

### Page composition

1. Masthead — `Capitolo III` mono kicker + serif `Entrate`.
2. Hero stat — current-month total (serif 44px) + suffix `€ / mese`; mono
   caption `{N} entrate · {YTD} YTD · Δ {sign}{pct}% vs mese scorso`.
3. Source split — three cells: **Stipendio** (employment), **Freelance**,
   **Altro**. Cells show count + monthly amount. Segmentation:
   - Freelance: category in `ListFreelanceIncomeCategories()`.
   - Stipendio: category contains `Stipendio` (case-insensitive).
   - Altro: everything else.
4. Top contributors — top-5 categories this month with bars (already
   computed by `ReadIncomeMonthOverview().ByCategory`, just add ordering +
   bar).
5. 12-month trend — small bar chart of monthly totals for the last 12
   months ending at the current month, fed by repeated calls to
   `ReadIncomeMonthOverview()` (one per month, year-aware so December of
   prior year is included).
6. Voci del mese — existing items list (kept; unchanged delete affordance).
7. Inline create form: removed. Creation goes through the existing FAB.

### Routes

Unchanged. `/entrate` page handler stays. The `/ui/income-month-overview`
endpoint keeps its URL and now returns the insights tile. The smaller
`/ui/income-month-{total,categories,incomes}` endpoints remain (unused by
`/entrate` after this ADR; still wired for the dashboard tile and any
external caller).

### View-model

```go
type IncomeInsightsVM struct {
    HasData         bool
    MonthlyTotalFmt string
    YTDTotalFmt     string
    ActiveCount     int
    DeltaPct        int    // signed % vs previous month; 0 when prev = 0
    DeltaSign       string // "+", "−", ""
    DeltaIsZero     bool

    Sources    []IncomeSourceVM   // 3 cells: Stipendio, Freelance, Altro
    Top5       []IncomeTopRowVM   // up to 5 categories by current-month amount
    Trend      []IncomeTrendCellVM // 12 cells, oldest → newest
    Items      []IncomeItemVM     // current-month items list, ordered by day asc
    Year, Month int
}
```

Pure helper `buildIncomeInsights(curr, prev core.IncomeMonthOverview,
trendCents [12]int64, items []storage.IncomeWithID, freelance map[string]bool, now time.Time)` keeps the
handler thin and unit-testable.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Insights tile | none | hero + 3 source cells + ≤5 top rows + 12 trend cells + N items |

## Edge cases

- Zero entries this month → hero `€0,00`, source/top/trend empty states; trend can still render past months if any.
- `prev = 0` → `DeltaIsZero = true`, no Δ rendered.
- Categories not yet in freelance set → fall through to Stipendio/Altro segmentation.
- Trend window crosses year boundary → handled by computing year/month per offset.

## Error conditions

- DB error on overview → 500 + slog (existing).
- DB error on freelance list → fall back to empty set (segmentation degrades gracefully).
- DB error on trend month → that cell renders 0; do not abort the page.

## Test plan

1. Unit (`internal/http/income_insights_test.go`):
   - `buildIncomeInsights` empty input → `HasData=false`, three zero source cells, twelve trend cells.
   - Source segmentation: freelance category routed to Freelance; "Stipendio E" routed to Stipendio; misc routed to Altro.
   - Top-5 ordering by amount desc, capped at 5.
   - Delta: prev=0 → DeltaIsZero; prev>0 → signed pct.
   - Trend ordering: cell 11 = current month; cell 0 = 11 months ago.
2. Acceptance (`server_test.go`): GET `/ui/income-month-overview` after
   seeding two stipendio + one freelance + one altro income → response
   includes hero total, three source cells, three top rows, current-month
   items.

## Out of scope

- Tax accruals visualization (covered by the dashboard cash-flow panel).
- Year-end projection (future ADR; needs run-rate logic).
- Editing income from the list (only delete supported today).
