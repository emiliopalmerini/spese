// Package reports renders read-only views derived from the v_* tabs that
// the spreadsheet builds via QUERY formulas. The Go code does no aggregation
// itself: it parses rows and hands them to templates.
package reports

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/sheets"
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

// BalanceSheet reads v_balance_sheet.
func BalanceSheet(ctx context.Context, client *sheets.Client, force bool) ([]BalanceRow, error) {
	_, rows, err := client.ReadTable(ctx, "v_balance_sheet", force)
	if err != nil {
		return nil, fmt.Errorf("read v_balance_sheet: %w", err)
	}
	out := make([]BalanceRow, 0, len(rows))
	for _, r := range rows {
		if len(r) == 0 || sheets.CellString(r[0]) == "" {
			continue
		}
		row := BalanceRow{
			Account: sheets.CellString(at(r, 0)),
			Type:    sheets.CellString(at(r, 1)),
			Class:   sheets.CellString(at(r, 2)),
		}
		if m, ok := sheets.CellMoney(at(r, 3)); ok {
			row.Balance = m
		}
		if d, ok := sheets.CellDate(at(r, 4)); ok {
			row.LatestMonth = d
		}
		out = append(out, row)
	}
	return out, nil
}

// IncomeStatement reads v_income_statement.
func IncomeStatement(ctx context.Context, client *sheets.Client, force bool) ([]IncomeRow, error) {
	_, rows, err := client.ReadTable(ctx, "v_income_statement", force)
	if err != nil {
		return nil, fmt.Errorf("read v_income_statement: %w", err)
	}
	out := make([]IncomeRow, 0, len(rows))
	for _, r := range rows {
		if len(r) == 0 {
			continue
		}
		row := IncomeRow{}
		if d, ok := sheets.CellDate(at(r, 0)); ok {
			row.Month = d
		} else {
			continue
		}
		if m, ok := sheets.CellMoney(at(r, 1)); ok {
			row.Revenue = m
		}
		if m, ok := sheets.CellMoney(at(r, 2)); ok {
			row.Expenses = m
		}
		if m, ok := sheets.CellMoney(at(r, 3)); ok {
			row.NetIncome = m
		}
		if f, ok := sheets.CellFloat(at(r, 4)); ok {
			row.SavingsRate = f
		}
		out = append(out, row)
	}
	return out, nil
}

// NwMonthly reads v_nw_monthly (just the month + net_worth columns).
func NwMonthly(ctx context.Context, client *sheets.Client, force bool) ([]NwRow, error) {
	_, rows, err := client.ReadTable(ctx, "v_nw_monthly", force)
	if err != nil {
		return nil, fmt.Errorf("read v_nw_monthly: %w", err)
	}
	out := make([]NwRow, 0, len(rows))
	for _, r := range rows {
		if len(r) == 0 {
			continue
		}
		row := NwRow{}
		if d, ok := sheets.CellDate(at(r, 0)); ok {
			row.Month = d
		} else {
			continue
		}
		if m, ok := sheets.CellMoney(at(r, 1)); ok {
			row.NetWorth = m
		}
		out = append(out, row)
	}
	return out, nil
}

// Investments reads v_investments.
func Investments(ctx context.Context, client *sheets.Client, force bool) ([]InvestmentRow, error) {
	_, rows, err := client.ReadTable(ctx, "v_investments", force)
	if err != nil {
		return nil, fmt.Errorf("read v_investments: %w", err)
	}
	out := make([]InvestmentRow, 0, len(rows))
	for _, r := range rows {
		if len(r) == 0 || sheets.CellString(r[0]) == "" {
			continue
		}
		row := InvestmentRow{Account: sheets.CellString(at(r, 0))}
		if m, ok := sheets.CellMoney(at(r, 1)); ok {
			row.CostBasis = m
		}
		if m, ok := sheets.CellMoney(at(r, 2)); ok {
			row.Value = m
		}
		if m, ok := sheets.CellMoney(at(r, 3)); ok {
			row.Return = m
		}
		if f, ok := sheets.CellFloat(at(r, 4)); ok {
			row.ReturnPct = f
		}
		if d, ok := sheets.CellDate(at(r, 5)); ok {
			row.LatestMonth = d
		}
		out = append(out, row)
	}
	return out, nil
}

func at(r []any, i int) any {
	if i >= len(r) {
		return nil
	}
	return r[i]
}
