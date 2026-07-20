package reports

import (
	"sort"

	"spese/internal/kernel"
)

// BalanceViewRow adds data freshness to one balance-sheet row.
type BalanceViewRow struct {
	BalanceRow
	Freshness string
}

// BalanceSheetView is the payload for the balance-sheet report.
type BalanceSheetView struct {
	Rows        []BalanceViewRow
	Assets      kernel.Money
	Liabilities kernel.Money
	NetWorth    kernel.Money
	LatestMonth kernel.Date
}

func buildBalanceSheetView(rows []BalanceRow) BalanceSheetView {
	view := BalanceSheetView{}
	for _, row := range rows {
		switch row.Type {
		case "Asset":
			view.Assets += row.Balance
		case "Liability":
			view.Liabilities += row.Balance
		}
		if !row.LatestMonth.IsZero() && (view.LatestMonth.IsZero() || row.LatestMonth.After(view.LatestMonth.Time)) {
			view.LatestMonth = row.LatestMonth
		}
	}
	view.NetWorth = view.Assets + view.Liabilities
	for _, row := range rows {
		freshness := ""
		switch {
		case row.LatestMonth.IsZero():
			freshness = "Mai aggiornato"
		case !view.LatestMonth.IsZero() && row.LatestMonth.Before(view.LatestMonth.Time):
			freshness = "Obsoleto"
		}
		view.Rows = append(view.Rows, BalanceViewRow{BalanceRow: row, Freshness: freshness})
	}
	return view
}

// IncomeStatementView is the payload for the income-statement report.
type IncomeStatementView struct {
	Rows           []IncomeRow
	Revenue        kernel.Money
	Expenses       kernel.Money
	NetIncome      kernel.Money
	SavingsRate    float64
	HasSavingsRate bool
}

func buildIncomeStatementView(rows []IncomeRow) IncomeStatementView {
	view := IncomeStatementView{Rows: append([]IncomeRow(nil), rows...)}
	for _, row := range rows {
		view.Revenue += row.Revenue
		view.Expenses += row.Expenses
		view.NetIncome += row.NetIncome
	}
	if view.Revenue != 0 {
		view.SavingsRate = float64(view.NetIncome) / float64(view.Revenue)
		view.HasSavingsRate = true
	}
	sort.Slice(view.Rows, func(i, j int) bool { return view.Rows[i].Month.After(view.Rows[j].Month.Time) })
	return view
}

// NwTimelineView is the payload for the net-worth timeline report.
type NwTimelineView struct {
	Rows         []NwRow
	Latest       kernel.Money
	Change       kernel.Money
	ChangePct    float64
	HasChangePct bool
}

func buildNwTimelineView(rows []NwRow) NwTimelineView {
	view := NwTimelineView{Rows: append([]NwRow(nil), rows...)}
	sort.Slice(view.Rows, func(i, j int) bool { return view.Rows[i].Month.After(view.Rows[j].Month.Time) })
	if len(view.Rows) == 0 {
		return view
	}
	view.Latest = view.Rows[0].NetWorth
	oldest := view.Rows[len(view.Rows)-1].NetWorth
	view.Change = view.Latest - oldest
	if oldest != 0 {
		view.ChangePct = float64(view.Change) / float64(absReportMoney(oldest))
		view.HasChangePct = true
	}
	return view
}

// InvestmentsView is the payload for the investments report.
type InvestmentsView struct {
	Rows       []InvestmentRow
	NetFlows   kernel.Money
	Value      kernel.Money
	Difference kernel.Money
}

func buildInvestmentsView(rows []InvestmentRow) InvestmentsView {
	view := InvestmentsView{Rows: rows}
	for _, row := range rows {
		view.NetFlows += row.CostBasis
		view.Value += row.Value
		view.Difference += row.Return
	}
	return view
}

func absReportMoney(value kernel.Money) kernel.Money {
	if value < 0 {
		return -value
	}
	return value
}
