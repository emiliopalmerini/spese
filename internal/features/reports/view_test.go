package reports

import (
	"testing"

	"spese/internal/kernel"
)

func TestBuildBalanceSheetViewSummarizesAndMarksFreshness(t *testing.T) {
	jan := reportDate(t, "2026-01")
	feb := reportDate(t, "2026-02")
	view := buildBalanceSheetView([]BalanceRow{
		{Account: "Bank", Type: "Asset", Balance: 100000, LatestMonth: jan},
		{Account: "Card", Type: "Liability", Balance: -20000, LatestMonth: feb},
		{Account: "Cash", Type: "Asset"},
	})

	if view.Assets != 100000 || view.Liabilities != -20000 || view.NetWorth != 80000 {
		t.Fatalf("totals = assets %s, liabilities %s, net %s", view.Assets, view.Liabilities, view.NetWorth)
	}
	if view.LatestMonth.Month() != "2026-02" {
		t.Fatalf("latest month = %q, want 2026-02", view.LatestMonth.Month())
	}
	if view.Rows[0].Freshness != "Obsoleto" || view.Rows[2].Freshness != "Mai aggiornato" {
		t.Fatalf("freshness = %q, %q, want Obsoleto and Mai aggiornato", view.Rows[0].Freshness, view.Rows[2].Freshness)
	}
}

func TestBuildIncomeStatementViewCalculatesPeriodTotals(t *testing.T) {
	view := buildIncomeStatementView([]IncomeRow{
		{Month: reportDate(t, "2026-01"), Revenue: 100000, Expenses: -60000, NetIncome: 40000},
		{Month: reportDate(t, "2026-02"), Revenue: 200000, Expenses: -50000, NetIncome: 150000},
	})

	if view.Revenue != 300000 || view.Expenses != -110000 || view.NetIncome != 190000 {
		t.Fatalf("totals = revenue %s, expenses %s, net %s", view.Revenue, view.Expenses, view.NetIncome)
	}
	if view.SavingsRate != float64(190000)/float64(300000) || !view.HasSavingsRate {
		t.Fatalf("savings rate = %f (%t)", view.SavingsRate, view.HasSavingsRate)
	}
	if view.Rows[0].Month.Month() != "2026-02" {
		t.Fatalf("first row = %q, want newest month", view.Rows[0].Month.Month())
	}
}

func TestBuildTimelineViewShowsLatestChange(t *testing.T) {
	view := buildNwTimelineView([]NwRow{
		{Month: reportDate(t, "2026-01"), NetWorth: 100000},
		{Month: reportDate(t, "2026-02"), NetWorth: 120000},
	})

	if view.Latest != 120000 || view.Change != 20000 || view.ChangePct != 0.2 || !view.HasChangePct {
		t.Fatalf("timeline summary = latest %s, change %s, pct %f", view.Latest, view.Change, view.ChangePct)
	}
	if view.Rows[0].Month.Month() != "2026-02" {
		t.Fatalf("first row = %q, want newest month", view.Rows[0].Month.Month())
	}
}

func reportDate(t *testing.T, value string) kernel.Date {
	t.Helper()
	date, err := kernel.ParseDate(value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}
