# ADR-0018: Homepage as cross-domain overview

## Status
Accepted

## Context

Three insight ADRs are now in place:

- ADR-0015 turned `/recurrent` into a recurrent-insights surface.
- ADR-0016 turned `/entrate` into an income-insights surface.
- ADR-0017 turned `/spese` into an expense-insights surface.

Each insight page owns its per-domain detail **including** the month
items list. The homepage (`/`, dashboard) still re-renders four blocks
that now duplicate insight pages:

- `Per categoria` list with period chips (`/ui/dashboard/categories`).
- `Spese ricorrenti` list (`/ui/dashboard/recurrents`).
- `Entrate per Categoria` accordion (`/ui/dashboard/income-breakdown`).
- `Ultime Transazioni` accordion (`/ui/dashboard/transactions`).

Result: every per-domain block has two homes, the homepage hides the
cross-domain narrative under domain noise, and kept blocks (hero, pills,
grid, net worth, trend, accordions) lack a consistent Quaderno
hierarchy.

This ADR makes the homepage a strict **cross-domain overview**, removes
the duplicated blocks, and refines the kept blocks for signal and
hierarchy. Per-domain lists remain on the insight pages (no separate
list pages).

## Decision

### Page composition

```
Capitolo I · Diario          (masthead, new)
1. Hero          monthly balance, serif 44px, label-mono caption
                 `{Δsign}{pct}% vs mese scorso · {YTD} YTD`
2. Pills         2 cells: Spese mese · Tasso risparmio (no decorative chrome)
3. Stat grid     2×2: media/giorno · Δ settimana · velocità · fissi%
4. Net worth     value + Δ vs last month + 12-pt sparkline
5. Trend         12-mo bar chart (existing)
6. Domain tiles  3 cards: Spese · Entrate · Ricorrenti  (NEW; after trend)
7. Pick Months   accordion (collapsed, existing)
8. Proiezioni    accordion (collapsed, MERGES Cash Flow + Projections)
```

### Removed blocks

| Block | Removed handler / route | Lives at |
|---|---|---|
| Categories list + period chips | `GET /ui/dashboard/categories` | `/spese` (ADR-0017) |
| Recurrents list section | `GET /ui/dashboard/recurrents` | `/recurrent` (ADR-0015) |
| Entrate per Categoria accordion | `GET /ui/dashboard/income-breakdown` | `/entrate` (ADR-0016) |
| Ultime Transazioni accordion | `GET /ui/dashboard/transactions` | per-domain items list inside insight pages |

`handleDashboardRecurrentsWithSummary` is also removed if it has no
remaining caller after the deletion.

### New: domain shortcut tiles

A single new endpoint `GET /ui/dashboard/shortcuts` returns one HTML
fragment with three cards:

- **Spese** — current-month total, items count, Δ vs prev mese, link to `/spese`.
- **Entrate** — current-month total, items count, Δ vs prev mese, link to `/entrate`.
- **Ricorrenti** — monthly total, active count, link to `/recurrent`.

Reuses already-shipped builders:

- `buildExpenseInsights` (`internal/http/expense_insights.go`)
- `buildIncomeInsights` (`internal/http/income_insights.go`)
- `buildRecurrentInsights` (`internal/http/recurrent_insights.go`)

No new domain logic; only a thin handler that calls the three builders
and renders `dashboard_shortcuts.html`.

### Refinements to kept blocks

**Hero + pills.** Hero stays as monthly balance (income − expenses).
Adds `label-mono` caption `{Δsign}{pct}% vs mese scorso · {YTD} YTD`.
Pills reduced to two cells with `label-mono` kicker + serif numeral; no
decorative container.

**Stat grid.** 2×2 with one shared cell template:
`label-mono` kicker + serif numeral + mono caption. Cells unchanged in
content (daily avg, week Δ, velocità, fissi%) since each is a distinct
actionable signal.

**Forecasting accordions.** Cash Flow and Projections merge into one
`Proiezioni` accordion that nests both existing partials. Pick Months
remains separate (distinct purpose).

Net-worth sparkline + stat-partial visual polish are intentionally
deferred to a follow-up ADR; this one stops at concern split + accordion
merge so the change set stays atomic.

### View-models

```go
type DashboardShortcutsVM struct {
    Spese     ShortcutCardVM
    Entrate   ShortcutCardVM
    Ricorrenti ShortcutCardVM
}

type ShortcutCardVM struct {
    Href        string  // "/spese" | "/entrate" | "/recurrent"
    Label       string  // "Spese" | "Entrate" | "Ricorrenti"
    AmountFmt   string  // "€1.234,56"
    Count       int
    DeltaPct    int     // signed; 0 when prev = 0 or N/A
    DeltaSign   string  // "+" | "−" | ""
    DeltaIsZero bool
    HasData     bool
}

```

### Routes

- Add: `GET /ui/dashboard/shortcuts` → `dashboard_shortcuts` partial.
- Remove: `GET /ui/dashboard/categories`, `GET /ui/dashboard/recurrents`,
  `GET /ui/dashboard/income-breakdown`, `GET /ui/dashboard/transactions`.
- Unchanged: `GET /` (page), `GET /ui/dashboard/stat-hero`,
  `/ui/dashboard/stat-pills`, `/ui/dashboard/stat-grid`,
  `/ui/dashboard/net-worth`, `/ui/dashboard/monthly-trend`,
  `/ui/dashboard/pick-months`, `/ui/dashboard/cash-flow`,
  `/ui/dashboard/projections`.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| `GET /` | none | composed HTML page in the new section order with masthead, hero, pills, 2×2 grid, net worth, trend, shortcut tiles, Pick Months, Proiezioni |
| `GET /ui/dashboard/shortcuts` | none | three cards (Spese, Entrate, Ricorrenti) with totals, counts, Δ where available |
| `GET /ui/dashboard/{categories,recurrents,income-breakdown,transactions}` | none | `404 Not Found` (route removed) |

## Edge cases

- Zero current-month expenses → Spese card amount `€0,00`, count 0, no Δ.
- Zero current-month incomes → Entrate card amount `€0,00`, count 0, no Δ.
- Zero active recurrents → Ricorrenti card amount `€0,00`, count 0.
- `prev = 0` for a domain → `DeltaIsZero = true`, no Δ rendered.
- A removed route is hit by an external HTMX trigger → 404; homepage
  swap target absent so the swap is a no-op.

## Error conditions

- DB error in any shortcut builder → that card renders empty state
  (`HasData = false`); other cards still render. Log via slog.
- Template error → 500 + slog (existing pattern).

## Test plan

1. **Unit** (`internal/http/handlers_dashboard_test.go` — new):
   - `buildDashboardShortcuts` empty inputs → three cards with
     `HasData=false`, zero amounts, zero counts.
   - Builder routes Δ correctly: prev>0 → signed pct; prev=0 →
     `DeltaIsZero`.
   - Recurrenti card has no Δ (per spec) regardless of inputs.
2. **Acceptance** (`internal/http/server_test.go`):
   - `GET /ui/dashboard/shortcuts` → response contains the three labels
     and the three insight links (`/spese`, `/entrate`, `/recurrent`).
     Renders even when the SQLite adapter is unavailable.
   - `GET /ui/dashboard/categories|recurrents|income-breakdown|transactions`
     → 404.
   - `GET /` → body contains `Capitolo I`, `Diario`, `stat-hero`,
     `stat-pills`, `stat-grid`, `net-worth-tile`, `monthly-trend`,
     `dashboard-shortcuts`, `pickMonthsAccordion`, `projectionsAccordion`;
     does NOT contain `categories-list`, `recurrents-list`,
     `income-breakdown-content`, `transactionsAccordion`,
     `cashFlowAccordion`, `incomeAccordion`.
3. **Smoke** (`make smoke`) — homepage 200s and renders.

## Out of scope

- Global cross-domain transactions list (insight pages own their lists).
- Tab bar changes.
- Visual redesign of the trend chart itself.
- Net-worth sparkline (deferred to follow-up ADR).
- Stat-partial visual polish (deferred to follow-up ADR).
- Replacing the four behavioral metrics in the stat grid.
