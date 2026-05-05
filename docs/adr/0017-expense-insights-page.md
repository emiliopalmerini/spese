# ADR-0017: Expense insights page

## Status
Accepted

## Context

`/spese` (the expenses entry page rendered by `handleIndex`) still has the
old shape: inline create form, then a thin `Panoramica Mensile`
(`month_overview.html`) showing total + flat primary-category bars + the
items list. Creation is already reachable from the FAB → "Spesa" → bottom
sheet (`/ui/form/expense`), so the inline form duplicates that affordance.

ADR-0015 / ADR-0016 turned `/recurrent` and `/entrate` into insights
surfaces. This ADR ports the same treatment to `/spese`. The dashboard
(`/`) already shows overall hero/trend/projections; `/spese` focuses on
this-month spending detail.

## Decision

### Page composition

1. Masthead — `Capitolo II` mono kicker + serif `Spese`.
2. Hero stat — current-month total (serif 44px) + suffix `€ / mese`; mono
   caption `{N} voci · Δ {sign}{pct}% vs mese scorso`.
3. Top 5 per impatto — top-5 primary categories with bars.
4. 12-month trend — bar chart of monthly totals for the last 12 months.
5. Categorie — primary→secondary nested accordion (using `<details>`),
   built from the items list (`Expense.Primary` + `Expense.Secondary`).
6. Voci del mese — items list with delete (kept).
7. Inline create form — removed.

### Routes

Unchanged. `/spese` page handler stays. The `/ui/month-overview` endpoint
keeps its URL and now returns the insights tile. The smaller
`/ui/month-{total,categories,expenses}` endpoints remain (still wired for
external callers).

### View-model

```go
type ExpenseInsightsVM struct {
    HasData         bool
    Year, Month     int
    MonthlyTotalFmt string
    ActiveCount     int
    DeltaPct        int
    DeltaSign       string
    DeltaIsZero     bool

    Top5       []ExpenseTopRowVM
    Trend      []IncomeTrendCellVM // reuse existing trend cell shape
    Categories []ExpenseCategoryGroupVM
    Items      []ExpenseItemVM
}
```

`buildExpenseInsights(curr, prev core.MonthOverview, trendCents [12]int64,
trendStartYear, trendStartMonth int, items []sheets.ExpenseWithID)` keeps
the handler thin and unit-testable. Reuses `addMonths` / `trendStart` /
`italianMonthShort` / `formatEuros` / `formatDayPadded`.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Insights tile | none | hero + ≤5 top rows + 12 trend cells + N category groups + N items |

## Edge cases

- Zero items this month → hero `€0,00`, sub-blocks render `.empty-state`;
  trend may still render past months.
- `prev = 0` → `DeltaIsZero = true`.
- Items missing secondary → group under primary with single empty-named child collapsed.
- Trend window crosses year boundary → handled by `addMonths`.

## Test plan

1. Unit (`internal/http/expense_insights_test.go`):
   - Empty input → `HasData=false`, 12 trend cells.
   - Top-5 ordering by current-month total.
   - Nested categories: items with same primary/secondary aggregate; sort children desc.
   - Delta sign/pct.
2. Acceptance (`server_test.go`): GET `/ui/month-overview` after seeding
   two items in distinct categories renders hero + top rows + nested category accordion + items.

## Out of scope

- Year-end projection (dashboard owns).
- Editing expenses inline beyond what already exists.
- Day-of-week / weekday pattern panel.
