# ADR-0015: Recurrent insights page

## Status
Accepted

## Context

`/recurrent` today is an input surface: the inline create form sits at the
top, a single monthly total + flat primary-category breakdown
(`Panoramica Mensile`) sits in the middle, and the per-entry list (ADR-0012)
sits at the bottom. Creation is rare in practice and is already reachable
from the FAB → "Ricorrente" → bottom sheet (`/ui/form/recurring`); the inline
form duplicates that affordance and pushes the actual data below the fold.

This ADR turns `/recurrent` into an **insights** surface while keeping the
list intact for editing / deleting individual entries.

## Decision

### Page composition

1. Masthead (`Capitolo IV` mono kicker + serif `Spese ricorrenti`).
2. Hero stat: serif 44px monthly total + suffix `€ / mese`; mono caption
   `{N} attive · {annual} € / anno · {daily} € / giorno`.
3. Frequency strip: four cells (Giornaliera / Settimanale / Mensile /
   Annuale) with `count` + monthly contribution (`daily ×30, weekly ×4,
   yearly ÷12`).
4. Top 5 per impatto: rows with name + freq + amount + horizontal bar
   (width = monthly / topMonthly).
5. Prossimi 30 giorni: chronological next-occurrence list.
6. Categorie: nested **primary → secondary** as `<details>` accordions, no JS.
7. Tutte le voci: existing `recurrent-list` partial (unchanged).
8. Inline create form: removed. Creation goes through the existing FAB.

### Routes

Unchanged. Page handler remains `/recurrent`; insights endpoint reuses
`/ui/recurrent-monthly-overview` so the existing HTMX triggers
(`recurrent:created/updated/deleted`) continue to refresh the block.

### View-model

```go
type RecurrentInsightsVM struct {
    MonthlyTotalFmt string  // "€1234,56"
    AnnualTotalFmt  string
    DailyAvgFmt     string
    ActiveCount     int

    Frequencies []FrequencyCellVM
    Top5        []TopRowVM
    Upcoming    []UpcomingRowVM
    Categories  []CategoryGroupVM
}
```

`nextOccurrence(re, today)` rules:
- Daily → today (or `EndDate` if past today).
- Weekly → today + (7 - delta) % 7 from `StartDate.Weekday()`.
- Monthly → next month-day matching `StartDate.Day`, capped to month length.
- Yearly → next anniversary of `StartDate.Month/Day` ≥ today; Feb 29 falls
  back to Feb 28 in non-leap years.
- `EndDate` past today → no occurrence (skip from upcoming, totals, top, categories).

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Insights tile | none | hero VM + 4 frequency cells + ≤5 top rows + ≤N upcoming rows + N category groups |

## Edge cases

- Zero recurrents → hero `€0,00`, all sub-blocks render `.empty-state`.
- Frequency absent → cell renders `count = 0`, amount `—`.
- `EndDate` past today → entry excluded from every aggregate.
- `StartDate.Day = 31` on 30-day month → cap to last day.
- Feb 29 yearly on non-leap year → Feb 28.

## Error conditions

- DB error → existing 500 + slog (unchanged).

## Test plan

1. Unit (`internal/http/recurrent_insights_test.go`):
   - `nextOccurrence` table tests: daily, weekly across week-boundary, monthly
     day-31 cap, yearly leap-year fallback, expired `EndDate`.
   - `buildInsights`: top-5 ordering, category nesting, frequency conversion,
     upcoming-30-day window.
2. Acceptance (`internal/http/server_test.go`): GET `/recurrent` + GET
   `/ui/recurrent-monthly-overview` after seeding three monthly + one annual
   include hero total, four frequency cells, expected top-row, and an
   upcoming row.

## Out of scope

- Inline form path (creation lives in FAB sheet).
- Pause / resume / archive (future ADR).
- Sparklines (future "history" panel).
