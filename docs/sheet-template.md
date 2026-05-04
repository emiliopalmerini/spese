# Sheet template (ADR-0007)

This document is the source of truth for the layout the app writes against.
Anything that touches the sheet (raw tabs, dashboard formulas, year rollover)
should agree with what is below. If you change the live sheet, change this
file too.

The app keeps SQLite as the source of truth and uses the sheet as a derived
backup + dashboard view. Read ADR-0007 for the why; this file is the how.

## Spreadsheet

- Title: `GE Net Worth`
- Tabs (per year `YYYY`):
  - `YYYY` — net worth + cash flow + FIRE projections
  - `YYYY Dashboard` — pivot views
  - `YYYY Expenses` — raw expense rows (app-managed)
  - `YYYY Incomes` — raw income rows (app-managed)
- History tabs (cross-year): unchanged, read-only.

## Raw tabs (app-managed)

The adapter pins these layouts; mismatched headers raise
`ErrSheetLayoutMismatch` and stop sync until corrected.

### `YYYY Expenses`

| col | header     | source            |
|-----|------------|-------------------|
| A   | m          | expense month     |
| B   | d          | expense day       |
| C   | expense    | description       |
| D   | amount     | amount in EUR     |
| E   | curr       | manual / formula  |
| F   | EUR        | manual / formula  |
| G   | primary    | primary category  |
| H   | secondary  | secondary category|
| I   | note       | manual            |
| J   | id         | SQLite expense.id |

### `YYYY Incomes`

| col | header   | source            |
|-----|----------|-------------------|
| A   | m        | income month      |
| B   | d        | income day        |
| C   | income   | description       |
| D   | amount   | amount in EUR     |
| E   | curr     | manual / formula  |
| F   | EUR      | manual / formula  |
| G   | primary  | income category   |
| H   | note     | manual            |
| I   | id       | SQLite income.id  |

The `id` column is what makes the upsert idempotent. Do not rename it. Do
not move it. Extra columns to the right of `id` are fine; the adapter
ignores them.

## Dashboard formulas

The dashboard tab has rows of `Primary` / `Secondary` and twelve month
columns (Jan..Dec) plus `Average` / `Total`. Replace any hard-pasted values
with this formula so new expense rows in `YYYY Expenses` propagate
automatically:

```
=SUMIFS('YYYY Expenses'!F:F,
        'YYYY Expenses'!G:G, $A<row>,
        'YYYY Expenses'!H:H, $B<row>,
        'YYYY Expenses'!A:A, COLUMN()-2)
```

- `F` is the EUR-converted amount column.
- `G` / `H` are primary / secondary category.
- `A` is the month column (1..12). `COLUMN()-2` matches the dashboard's
  Jan column (col C → 3 → 1, etc.); adjust the `-2` if your layout shifts.
- `Average` and `Total` cells use plain `AVERAGE` / `SUM` over the twelve
  month cells in the row.

For the Cash Flow block in the `YYYY` tab, point each income row at
`'YYYY Incomes'`:

```
=SUMIFS('YYYY Incomes'!F:F,
        'YYYY Incomes'!G:G, $A<row>,
        'YYYY Incomes'!A:A, <month-col>)
```

State-tax rows (`Imposta sostitutiva`, `INPS`) keep the constants logic from
ADR-0004 (rate × freelance income). The FIRE block is independent; it reads
totals from the cash flow rows above it.

## Year rollover (manual)

Once a year (early January is fine), copy the previous year's tabs to the
new year. Allow ~5 minutes.

1. Right-click `YYYY` → **Duplicate**. Rename copy to `YYYY+1`.
2. Right-click `YYYY Dashboard` → **Duplicate**. Rename copy to
   `YYYY+1 Dashboard`.
3. Right-click `YYYY Expenses` → **Duplicate**. Rename copy to
   `YYYY+1 Expenses`. Delete all data rows; keep the header.
4. Right-click `YYYY Incomes` → **Duplicate**. Rename copy to
   `YYYY+1 Incomes`. Delete all data rows; keep the header.
5. In `YYYY+1` and `YYYY+1 Dashboard`, **Edit → Find and replace** scoped
   to those tabs:
   - Find: `'YYYY Expenses'`  Replace: `'YYYY+1 Expenses'`
   - Find: `'YYYY Incomes'`   Replace: `'YYYY+1 Incomes'`
   - Tick **Also search within formulas**.
6. On the `YYYY+1` tab, extend the Net Worth header: the rightmost columns
   end with `YYYY+1` after `Dec`; add a new `YYYY+2` cell to the right so
   future-year writes have a destination column. (Skip if the template
   already has it.)

After the rollover, run `spese export-sheet --year YYYY+1 --dry-run` to
sanity-check the new tabs are reachable, then drop `--dry-run`.

## Reproducing a fresh sheet

If you ever need to start from scratch:

1. Create a blank spreadsheet titled `GE Net Worth`.
2. Add the tabs listed above for the current year.
3. Add the raw-tab headers from the tables above (in row 1, exact spelling).
4. Add the Net Worth section labels in column A on the `YYYY` tab in this
   order, separated by blank rows: `Cash - Liquidity`, `Rain day funds`,
   `Long Term investment`. Below each label, list the account names you
   want tracked.
5. Add the dashboard formula above to each `YYYY Dashboard` cell.
6. `spese export-sheet --year YYYY` to populate.
