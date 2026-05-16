# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Build and Development Commands

```bash
# Nix (recommended)
nix develop             # Enter development shell with all tools
nix build               # Build the spese binary (result/bin/spese)
nix build .#docker      # Build OCI image

# Make (inside nix develop or with Go installed)
make build              # Build main app (bin/spese)
make run                # Run application locally
make test               # Run tests with race detector

# Code quality
make fmt                # Format code (gofmt -s -w .)
make vet                # Run go vet
make lint               # Run golangci-lint (optional)
```

## Architecture

Spese is a single binary (`cmd/spese`) that serves an HTMX UI and runs a
background recurring-transaction processor. The Google Sheet is the single
source of truth (see ADR-0020); SQLite has been removed.

### Data Flow

```
User → HTTP handler → sheets.Client.AppendRows → Google Sheet
                                  ↑
                  recurring.Processor (daily scan, append fires)

User → HTTP handler → sheets.Client.ReadRange → cache → Google Sheet
```

The sheets client has a 5-minute in-memory read cache, invalidated by tab
on every write.

### Vertical-Slice Layout

Each feature owns its handler, business logic, sheet I/O, and types under
`internal/features/<feature>/`. Shared low-level concerns live in
`internal/sheets/` (Sheets API client) and `internal/kernel/` (Money, Date
value types).

```
cmd/spese/                main entry: config, mounts slices, starts processor
internal/
  kernel/                 Money + Date value types
  sheets/                 Google Sheets client + cell-value helpers
  render/                 HTML template loader + funcs
  config/                 env-driven Config
  features/
    accounts/             chart of accounts (CRUD-light, append-only)
    transactions/         general journal (Income / Expense / Transfer rows)
    transfers/            two-sided transfer form (writes 2 Transfer rows)
    snapshots/            month-end balance entry
    recurring/            recurring config + day-of-month processor
    reports/              read-only views from v_balance_sheet, v_income_statement,
                          v_nw_monthly, v_investments
    dashboard/            home page (reads `dashboard` tab from sheet)
web/
  templates/              shared per-feature templates (layouts/, dashboard/, etc.)
  static/css/             single base.css
```

### Sheet Schema

Source tabs the app writes to:
- `accounts`     — account, type, class, currency, active_from, active_to, note
- `transactions` — date, kind, account, amount_eur (signed), category,
                   subcategory, payee, note, id
- `snapshots`    — month, account, balance_eur (liabilities negative), note
- `recurring`    — label, kind, account, amount_eur, category, subcategory,
                   payee, day_of_month, active, note
- `fx`           — month, EURUSD

View tabs the app reads:
- `v_balance_sheet`     — latest balance per account
- `v_income_statement`  — monthly revenue, expenses, savings rate
- `v_nw_monthly`        — NW total over time + breakdown by class
- `v_investments`       — per investment account: basis, value, return
- `dashboard`           — label/value KPIs

The view tabs are computed by formulas inside the spreadsheet
(see `/tmp/nw_v2.gs` Apps Script). The Go app does no aggregation.

### Writes

All sheet writes are append-only. Edit and delete happen directly in the
spreadsheet UI. Transfers write two `kind=Transfer` rows in one append call.

### Recurring Processor

Scans the `recurring` tab on `RECURRING_PROCESSOR_INTERVAL` (default 6h).
For each active row whose `day_of_month` has passed today, it appends a
transaction tagged with `[recurring:<label>]` in the note, unless the same
tag already exists in the current month (idempotency).

## Configuration

Copy `.env.example` to `.env`. Required:
- `GOOGLE_SPREADSHEET_ID` — v2 sheet ID
- `GOOGLE_SERVICE_ACCOUNT_FILE` — path to service-account JSON

Optional:
- `SPESE_PORT` — default 8080
- `RECURRING_PROCESSOR_INTERVAL` — default 6h

## NixOS Deployment

```nix
{
  imports = [ inputs.spese.nixosModules.default ];

  services.spese = {
    enable = true;
    port = 8080;
    googleSpreadsheetId = "1BgDhwk-rQOoArP2LS4eNwXO8FLHCqK8shxh0q292Aqw";
    googleServiceAccountFile = "/run/secrets/google-sa.json";
    recurringProcessorInterval = "6h";
  };
}
```
