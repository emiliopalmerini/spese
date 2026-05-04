# ADR-0002: Net Worth HTTP UI

## Status
Proposed

## Context
ADR-0001 introduces accounts and monthly balances. This ADR adds the HTTP/HTMX
surface so the user can manage accounts and enter monthly balances, plus a
dashboard tile showing total Net Worth and month-over-month change.

## Decision

### Routes
- `GET /networth` — full page: list accounts grouped by type with current-month balance and an inline editor.
- `GET /ui/networth/accounts` — partial: accounts table.
- `POST /ui/networth/accounts` — create account (name, type, active).
- `PUT /ui/networth/accounts/{id}` — rename / toggle active / change type.
- `POST /ui/networth/balances` — upsert balance (account_id, year, month, amount).
- `GET /ui/networth/month?year=YYYY&month=MM` — partial: balances for a given month.
- `GET /ui/dashboard/net-worth` — dashboard partial: total NW + MoM delta + delta %.

### Templates
- `web/templates/pages/networth.html` (`networth_page`, `networth_content`)
- `web/templates/partials/networth_accounts.html`
- `web/templates/partials/networth_month.html`
- `web/templates/partials/networth_form.html` (account form, bottom sheet)
- `web/templates/partials/networth_balance_form.html` (balance form, bottom sheet)
- `web/templates/partials/dashboard_net_worth.html`

### Dashboard integration
- New section in `dashboard_content` (after stat-grid):
  ```html
  <section class="page__section">
    <div id="net-worth-tile"
         hx-get="/ui/dashboard/net-worth"
         hx-trigger="load, dashboard:refresh from:body, networth:updated from:body"
         hx-swap="innerHTML">
      <div class="skeleton" style="height: 80px;"></div>
    </div>
  </section>
  ```
- Tile contents: NW total (current month), MoM change in EUR + %, sparkline TBD (out of scope).

### Topbar nav
- Add link to `/networth` from existing topbar.

### Handlers (internal/http/handlers_networth.go)
- `handleNetWorthPage`
- `handleNetWorthAccountsList`
- `handleNetWorthAccountCreate`
- `handleNetWorthAccountUpdate`
- `handleNetWorthBalanceUpsert` (emits `networth:updated` HX-Trigger)
- `handleNetWorthMonth`
- `handleDashboardNetWorth`

## Inputs / Outputs

| Route | Method | Form / Query | Response |
|---|---|---|---|
| `/networth` | GET | — | full HTML page |
| `/ui/networth/accounts` | POST | `name`, `type`, `active` | `networth_accounts` partial |
| `/ui/networth/accounts/{id}` | PUT | `name`, `type`, `active` | `networth_accounts` partial |
| `/ui/networth/balances` | POST | `account_id`, `year`, `month`, `amount` | `networth_month` partial |
| `/ui/dashboard/net-worth` | GET | — | `dashboard_net_worth` partial |

Amount input accepts decimals (e.g., `1234.56`) parsed via existing money helper.

## Edge cases
- Submit balance for inactive account → reject with 400 + flash.
- Submit duplicate account name → 409 + form error message.
- Month with no balances → tile shows `—` and 0% delta.
- Previous month with no data → MoM delta hidden (no div-by-zero).
- Year/month out of range → 400.

## Error conditions
- Validation: 400 + inline form error partial.
- Not found: 404.
- Conflict (unique name): 409.
- Storage error: 500 + slog error.

## Test plan
1. **Acceptance** (`internal/http/server_test.go`):
   - GET `/networth` returns 200 with content
   - POST account → GET accounts contains it
   - POST balance → GET month returns the value
   - GET dashboard tile returns total + delta after 2 monthly entries
2. **Unit**:
   - Form parsing: amount, year/month bounds
   - Delta calculation (zero-prev-month case)
3. **Integration**:
   - HX-Trigger `networth:updated` emitted on balance upsert

## Out of scope
- Bulk import.
- Charts (sparkline).
- Sheet sync (ADR-0003).
- Net worth history page.
