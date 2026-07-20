// Package reports renders read-only views derived from local SQLite data.
package reports

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// BalanceRow is one row of v_balance_sheet.
type BalanceRow struct {
	Account     string
	Type        string
	Class       string
	Balance     kernel.Money
	LatestMonth kernel.Date
}

// IncomeRow is one row of v_income_statement.
type IncomeRow struct {
	Month       kernel.Date
	Revenue     kernel.Money
	Expenses    kernel.Money
	NetIncome   kernel.Money
	SavingsRate float64 // 0..1; negative possible
}

// NwRow is one row of v_nw_monthly (just the total column).
type NwRow struct {
	Month    kernel.Date
	NetWorth kernel.Money
}

// InvestmentRow is one row of v_investments.
type InvestmentRow struct {
	Account     string
	CostBasis   kernel.Money
	Value       kernel.Money
	Return      kernel.Money
	ReturnPct   float64
	LatestMonth kernel.Date
}

// BalanceSheet returns the latest canonical balance per account.
func BalanceSheet(ctx context.Context, store *storage.Store, _ bool) ([]BalanceRow, error) {
	rows, err := store.DB().QueryContext(ctx, canonicalSnapshotsSQL+`
		, latest AS (
			SELECT account, effective_month, balance_cents
			FROM (
				SELECT
					account,
					effective_month,
					balance_cents,
					row_number() OVER (
						PARTITION BY account
						ORDER BY effective_month DESC
					) AS rn
				FROM canonical_snapshots
			)
			WHERE rn = 1
		)
		SELECT a.name, a.type, a.class, coalesce(l.balance_cents, 0), coalesce(l.effective_month, '')
		FROM accounts a
		LEFT JOIN latest l ON l.account = a.name
		ORDER BY a.type, a.class, a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("read balance sheet: %w", err)
	}
	defer rows.Close()

	var out []BalanceRow
	for rows.Next() {
		var monthStr string
		row := BalanceRow{}
		var balance int64
		if err := rows.Scan(&row.Account, &row.Type, &row.Class, &balance, &monthStr); err != nil {
			return nil, fmt.Errorf("scan balance sheet: %w", err)
		}
		row.Balance = kernel.Money(balance)
		if monthStr != "" {
			d, err := kernel.ParseDate(monthStr)
			if err != nil {
				return nil, fmt.Errorf("parse balance month: %w", err)
			}
			row.LatestMonth = d.FirstOfMonth()
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// IncomeStatement aggregates income and expense transactions per month.
func IncomeStatement(ctx context.Context, store *storage.Store, _ bool) ([]IncomeRow, error) {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT
			substr(date, 1, 7) AS month,
			sum(CASE WHEN kind = 'Income' THEN amount_cents ELSE 0 END) AS revenue,
			sum(CASE WHEN kind = 'Expense' THEN amount_cents ELSE 0 END) AS expenses
		FROM transactions
		WHERE kind IN ('Income', 'Expense')
		GROUP BY month
		ORDER BY month
	`)
	if err != nil {
		return nil, fmt.Errorf("read income statement: %w", err)
	}
	defer rows.Close()

	var out []IncomeRow
	for rows.Next() {
		var monthStr string
		var revenue, expenses int64
		row := IncomeRow{}
		if err := rows.Scan(&monthStr, &revenue, &expenses); err != nil {
			return nil, fmt.Errorf("scan income statement: %w", err)
		}
		month, err := kernel.ParseDate(monthStr)
		if err != nil {
			return nil, fmt.Errorf("parse income month: %w", err)
		}
		row.Month = month.FirstOfMonth()
		row.Revenue = kernel.Money(revenue)
		row.Expenses = kernel.Money(expenses)
		row.NetIncome = row.Revenue + row.Expenses
		if row.Revenue != 0 {
			row.SavingsRate = float64(row.NetIncome) / float64(row.Revenue)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// NwMonthly returns net worth per month from canonical snapshots.
func NwMonthly(ctx context.Context, store *storage.Store, _ bool) ([]NwRow, error) {
	rows, err := store.DB().QueryContext(ctx, canonicalSnapshotsSQL+`
		, snapshot_bounds AS (
			SELECT min(effective_month) AS first_month, max(effective_month) AS last_month
			FROM canonical_snapshots
		), months(effective_month) AS (
			SELECT first_month
			FROM snapshot_bounds
			WHERE first_month IS NOT NULL
			UNION ALL
			SELECT strftime('%Y-%m', date(effective_month || '-01', '+1 month'))
			FROM months, snapshot_bounds
			WHERE effective_month < last_month
		), monthly_balances AS (
			SELECT
				m.effective_month,
				a.name AS account,
				(
					SELECT cs.balance_cents
					FROM canonical_snapshots cs
					WHERE cs.account = a.name
					  AND cs.effective_month <= m.effective_month
					ORDER BY cs.effective_month DESC
					LIMIT 1
				) AS balance_cents
			FROM months m
			JOIN accounts a
			  ON (a.active_from = '' OR substr(a.active_from, 1, 7) <= m.effective_month)
			 AND (a.active_to = '' OR substr(a.active_to, 1, 7) >= m.effective_month)
			WHERE EXISTS (
				SELECT 1
				FROM canonical_snapshots cs
				WHERE cs.account = a.name
				  AND cs.effective_month <= m.effective_month
			)
		)
		SELECT effective_month, sum(balance_cents)
		FROM monthly_balances
		GROUP BY effective_month
		ORDER BY effective_month
	`)
	if err != nil {
		return nil, fmt.Errorf("read nw monthly: %w", err)
	}
	defer rows.Close()

	var out []NwRow
	for rows.Next() {
		var monthStr string
		var amount int64
		row := NwRow{}
		if err := rows.Scan(&monthStr, &amount); err != nil {
			return nil, fmt.Errorf("scan nw monthly: %w", err)
		}
		month, err := kernel.ParseDate(monthStr)
		if err != nil {
			return nil, fmt.Errorf("parse nw month: %w", err)
		}
		row.Month = month.FirstOfMonth()
		row.NetWorth = kernel.Money(amount)
		out = append(out, row)
	}
	return out, rows.Err()
}

// Investments compares latest investment balances with net recorded transfers.
func Investments(ctx context.Context, store *storage.Store, _ bool) ([]InvestmentRow, error) {
	rows, err := store.DB().QueryContext(ctx, canonicalSnapshotsSQL+`
		, latest AS (
			SELECT account, effective_month, balance_cents
			FROM (
				SELECT
					account,
					effective_month,
					balance_cents,
					row_number() OVER (
						PARTITION BY account
						ORDER BY effective_month DESC
					) AS rn
				FROM canonical_snapshots
			)
			WHERE rn = 1
		),
		cost_basis AS (
			SELECT account, sum(amount_cents) AS cost_cents
			FROM transactions
			WHERE kind = 'Transfer'
			GROUP BY account
		)
		SELECT
			a.name,
			coalesce(c.cost_cents, 0),
			coalesce(l.balance_cents, 0),
			coalesce(l.effective_month, '')
		FROM accounts a
		LEFT JOIN latest l ON l.account = a.name
		LEFT JOIN cost_basis c ON c.account = a.name
		WHERE a.class = 'Investment'
		ORDER BY a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("read investments: %w", err)
	}
	defer rows.Close()

	var out []InvestmentRow
	for rows.Next() {
		var monthStr string
		var cost, value int64
		row := InvestmentRow{}
		if err := rows.Scan(&row.Account, &cost, &value, &monthStr); err != nil {
			return nil, fmt.Errorf("scan investments: %w", err)
		}
		row.CostBasis = kernel.Money(cost)
		row.Value = kernel.Money(value)
		row.Return = row.Value - row.CostBasis
		if row.CostBasis != 0 {
			row.ReturnPct = float64(row.Return) / float64(row.CostBasis)
		}
		if monthStr != "" {
			month, err := kernel.ParseDate(monthStr)
			if err != nil {
				return nil, fmt.Errorf("parse investment month: %w", err)
			}
			row.LatestMonth = month.FirstOfMonth()
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

const canonicalSnapshotsSQL = `
	WITH RECURSIVE canonical_snapshots AS (
		SELECT effective_month, account, balance_cents, note
		FROM (
			SELECT
				b.effective_month,
				sb.account,
				sb.balance_cents,
				sb.note,
				row_number() OVER (
					PARTITION BY b.effective_month, sb.account
					ORDER BY b.captured_at DESC, b.id DESC
				) AS rn
			FROM snapshot_balances sb
			JOIN snapshot_batches b ON b.id = sb.batch_id
		)
		WHERE rn = 1
	)
`
