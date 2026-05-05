# ADR-0019: Networth insights page

## Status
Proposed

## Context

`/networth` (rendered by `handleNetWorthPage` in
`internal/http/handlers_networth.go:164`) still uses the pre-restyle
shape: a full-width inline create-account form, then a thin "Saldi del
mese" section with an Alpine year/month picker driving a flat
`networth_accounts` partial (sectioned account lists with inline
balance-edit forms).

ADR-0015 (`/recurrent`), ADR-0016 (`/entrate`), and ADR-0017 (`/spese`)
already ported every other non-dashboard page to a uniform "insights"
surface (hero stat + caption + strip + 12-month trend + detail blocks).
This ADR is the last hold-out.

Networth differs from the flow pages in two ways that shape the design:

1. Numbers are **point-in-time balances** (sum of `account_balances`
   for a given month), not a flow over the month. The hero displays
   `MonthlyNetWorth(year, month)`.
2. The page is the **only place to edit balances** and the **only
   place to create accounts** (no FAB option). Both affordances must
   be preserved on the restyled page.

Mobile tab bar is also crammed at 5 tabs + centered FAB; we drop
`Patrimonio` from the bottom tab bar and keep it in the desktop topbar.
Mobile users reach `/networth` via the dashboard net-worth tile which
already links to `/networth`.

## Decision

### Page composition

1. Title — `<h1 class="page__title">Patrimonio</h1>`.
2. Year/month picker — keep the existing Alpine block; on `@change`
   it `htmx.ajax`'s `/ui/networth/insights?year=Y&month=M` with
   `outerHTML` swap.
3. Insights tile — single `#networth-insights` container fed by
   `/ui/networth/insights` on `load` and on `networth:updated`.

Inside the tile:

1. **Hero stat** — serif 44px `MonthlyTotalFmt` + `€` suffix. Mono
   caption: `{ActiveCount} conti · Δ {sign}{pct}% vs {PrevMonthShort}
   · {DeltaAbsFmt}` (delta blocks suppressed when prev=0).
2. **Type strip** — three cells (Cash & Liquidità / Rainy day /
   Long term). Each cell: label (mono), account count, total for the
   month.
3. **12-month trend** — bar chart of `MonthlyNetWorth` for the last
   12 months ending at `(Year, Month)`. Reuses `IncomeTrendCellVM` as
   the cell type. `IsCurrent` set on cell index 11.
4. **Conti per tipo** — three `<details class="net-cat__group">`
   accordions in Cash → RainyDay → LongTerm order. Summary shows
   type label + total + bar (width relative to the largest type).
   Children = current-month account rows with the existing inline
   `hx-post="/ui/networth/balances"` edit form **unchanged**. Inactive
   accounts still appear, with `(inattivo)` and no edit form.
5. **Aggiungi conto** — collapsed `<details class="net-add">`
   containing the existing create-account form (name + type + active
   checkbox).

Empty state: when no accounts exist OR no balances exist for the month,
render hero (`€0,00`) + an `.empty-state` block with hint, and skip
strip / trend / accordion blocks. The "Aggiungi conto" details still
renders.

### Routes

- New: `GET /ui/networth/insights?year=Y&month=M` →
  `handleNetworthInsights`.
- Removed (unused after the swap): `GET /ui/networth/month`, the GET
  branch of `/ui/networth/accounts`. POST handlers for accounts and
  balances stay at the same URLs and now `hx-target="#networth-insights"
  hx-swap="outerHTML"`, returning the full insights tile.
- Dashboard route `/ui/dashboard/net-worth` is unchanged.

### Tab bar

- `web/templates/partials/tab_bar.html`: drop the `Patrimonio` tab.
- `web/templates/partials/topbar.html`: keep the `Patrimonio` link.

### View-model

```go
type NetworthInsightsVM struct {
    HasData         bool
    Year, Month     int
    MonthlyTotalFmt string
    ActiveCount     int
    DeltaPct        int
    DeltaSign       string
    DeltaIsZero     bool
    DeltaAbsFmt     string
    PrevMonthShort  string

    Types  []NetworthTypeCellVM
    Trend  []IncomeTrendCellVM // reuse existing 12-cell trend type
    Groups []NetworthGroupVM
}

type NetworthTypeCellVM struct {
    Key       string // "cash" | "rainy_day" | "long_term"
    Label     string
    Count     int
    AmountFmt string
}

type NetworthGroupVM struct {
    Type     core.AccountType
    Title    string
    TotalFmt string
    WidthPct int
    Accounts []netWorthAccountRow // existing struct, includes inline edit form data
}
```

Pure builder:

```go
func buildNetworthInsights(
    currYear, currMonth int,
    accounts []core.Account,
    currBalances, prevBalances map[int64]core.AccountBalance,
    trendCents [12]int64,
    trendStartYear, trendStartMonth int,
) NetworthInsightsVM
```

Reuses helpers: `formatEuros`, `addMonths`, `trendStart`,
`italianMonthShort`, `IncomeTrendCellVM`, `netWorthSectionTitles`.

## Inputs / Outputs

| Op | Input | Output |
|---|---|---|
| Insights tile | year, month | hero + 3 type cells + 12 trend cells + 3 group accordions + create-account `<details>` |

## Edge cases

- No accounts → `HasData=false`, hero `€0,00`, only empty-state +
  create-account `<details>` render.
- Accounts exist but no balances this month → `HasData=false` (same
  treatment); trend may still display past months.
- `prevBalancesTotal == 0` → `DeltaIsZero = true`; caption omits the
  delta segment.
- Type with zero accounts → cell still renders with count 0 and
  `AmountFmt = "—"`; group accordion omitted from `Groups`.
- Inactive accounts → present in their group with `IsInactive` flag;
  no edit form, balance shown as "—" if missing.
- Trend window crosses year boundary → handled by `addMonths`.

## Test plan

1. Unit (`internal/http/networth_insights_test.go`):
   - Empty input (no accounts) → `HasData=false`, 12 trend cells with
     `IsCurrent` on index 11.
   - No balances this month, balances in past months → `HasData=false`,
     trend still populates.
   - Type strip totals match per-type sum; `Count` reflects active +
     inactive accounts of that type.
   - Group bar widths: largest type → 100, others scaled correctly.
   - Delta sign/pct: positive, negative, prev=0 → `DeltaIsZero`.
   - Trend window aligned with `trendStart(year, month)`.
2. Acceptance (`internal/http/server_networth_test.go`): GET
   `/ui/networth/insights?year=Y&month=M` after seeding two accounts
   with balances; assert response contains hero total, type cells,
   trend cell for current month, group accordion summaries, and the
   `Aggiungi conto` `<details>`.

## Out of scope

- Per-account history sparkline.
- Account edit/delete UI (existing PUT endpoint stays unwired in UI).
- Migration of the create-account form to the FAB.
