# ADR-0001: Net Worth domain and storage

## Status
Proposed

## Context
Port the Net Worth section of the Google Sheets dashboard into the app. The sheet
tracks a fixed list of accounts (e.g., Conto BCC, Fondo Fonte, Patreon, Trade
republic E/G, FirstHouse) grouped into three sections (Cash - Liquidity, Rain
day funds, Long Term investment) with a monthly balance per account.

The app must own this data: users insert monthly balances through the UI, and a
later ADR (0003) will sync them to the sheet. EUR only; the USD/EURUSD columns
in the sheet are out of scope.

## Decision

### Domain (internal/core)
- `Account` entity:
  - `ID int64`
  - `Name string` (non-empty, max 80 chars)
  - `Type AccountType` enum: `cash`, `rainy_day`, `long_term`
  - `Active bool` (soft toggle; inactive accounts hidden from UI but balances retained)
- `AccountBalance` entity:
  - `AccountID int64`
  - `Year int` (>= 2000)
  - `Month int` (1-12)
  - `Amount Money` (positive cents; liabilities not modeled in this ADR)
- Validation:
  - `Account.Validate()` rejects empty/long name, unknown type
  - `AccountBalance.Validate()` rejects out-of-range year/month, non-positive amount

### Storage (internal/storage)
- Migration `000015_create_net_worth_tables`:
  ```sql
  CREATE TABLE accounts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL CHECK (type IN ('cash','rainy_day','long_term')),
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
  );
  CREATE TABLE account_balances (
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    year          INTEGER NOT NULL,
    month         INTEGER NOT NULL CHECK (month BETWEEN 1 AND 12),
    amount_cents  INTEGER NOT NULL,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, year, month)
  );
  CREATE INDEX idx_account_balances_period ON account_balances(year, month);
  ```
- sqlc queries:
  - `CreateAccount`, `UpdateAccount`, `ListActiveAccounts`, `ListAllAccounts`, `GetAccountByID`, `GetAccountByName`
  - `UpsertAccountBalance` (`INSERT ... ON CONFLICT DO UPDATE`)
  - `ListBalancesForMonth(year, month)`
  - `ListBalancesForAccount(account_id)`
  - `GetLatestBalancePerAccount` (most recent year/month per account)
  - `GetMonthlyNetWorthTotal(year, month)` (sum of balances)

### Repository methods (internal/storage/repository.go)
- `CreateAccount(ctx, core.Account) (int64, error)`
- `UpdateAccount(ctx, core.Account) error`
- `ListAccounts(ctx, includeInactive bool) ([]core.Account, error)`
- `UpsertBalance(ctx, core.AccountBalance) error`
- `ListBalancesByMonth(ctx, year, month int) ([]core.AccountBalance, error)`
- `ListBalancesByAccount(ctx, accountID int64) ([]core.AccountBalance, error)`
- `LatestBalances(ctx) ([]core.AccountBalance, error)`
- `MonthlyNetWorth(ctx, year, month int) (core.Money, error)`

## Inputs / Outputs

| Operation | Input | Output |
|---|---|---|
| Create account | name, type | account ID |
| Upsert balance | account_id, year, month, amount_cents | none |
| List balances by month | year, month | list of balances |
| Monthly NW | year, month | total cents |

## Edge cases
- Duplicate account name → unique constraint error surfaced as domain error `ErrAccountExists`.
- Upsert balance for non-existent account → FK violation; repo returns `ErrAccountNotFound`.
- Month with no balances → `MonthlyNetWorth` returns 0 (not an error).
- Inactive account → still returned by `ListAccounts(includeInactive=true)`; excluded by default.
- Balance exactly 0 → allowed (account closed for that month).

## Error conditions
- Validation errors return domain sentinels (`ErrEmptyAccountName`, `ErrInvalidAccountType`, `ErrInvalidPeriod`, `ErrInvalidAmount`).
- DB errors wrapped with context; FK and uniqueness mapped to typed errors.

## Test plan (pre-registration)
1. **Acceptance** (`internal/storage/repository_test.go`):
   - Create account → list returns it
   - Upsert balance twice → second overwrites
   - Monthly NW sums all accounts for that month
2. **Unit** (`internal/core/domain_test.go`):
   - Account validation: empty name, long name, bad type
   - AccountBalance validation: bad year/month, zero/negative amount
3. **Integration** (`internal/storage/repository_test.go` w/ in-memory SQLite):
   - FK enforcement on balance insert with bad account_id
   - Unique-name enforcement
   - LatestBalances returns one row per account

All tests must fail (compile or assertion) before implementation begins.

## Out of scope
- HTTP UI (ADR-0002).
- Google Sheets sync (ADR-0003).
- Liabilities (negative balances).
- Multi-currency.
