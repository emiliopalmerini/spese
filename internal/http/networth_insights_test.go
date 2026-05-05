package http

import (
	"testing"

	"spese/internal/core"
)

func mkAcct(id int64, name string, t core.AccountType, active bool) core.Account {
	return core.Account{ID: id, Name: name, Type: t, Active: active}
}

func mkBal(id int64, year, month int, cents int64) core.AccountBalance {
	return core.AccountBalance{
		AccountID: id,
		Year:      year,
		Month:     month,
		Amount:    core.Money{Cents: cents},
	}
}

func TestBuildNetworthInsights_EmptyNoAccounts(t *testing.T) {
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildNetworthInsights(2026, 5, nil, nil, nil, trend, sy, sm)

	if vm.HasData {
		t.Fatalf("want HasData=false")
	}
	if len(vm.Types) != 3 {
		t.Fatalf("want 3 type cells, got %d", len(vm.Types))
	}
	for _, c := range vm.Types {
		if c.Count != 0 || c.AmountFmt != "—" {
			t.Fatalf("want zero/em-dash type cell, got %+v", c)
		}
	}
	if len(vm.Trend) != 12 {
		t.Fatalf("want 12 trend cells, got %d", len(vm.Trend))
	}
	if !vm.Trend[11].IsCurrent {
		t.Fatalf("last cell should be IsCurrent")
	}
	if !vm.DeltaIsZero {
		t.Fatalf("want DeltaIsZero when no prev balances")
	}
	if len(vm.Groups) != 0 {
		t.Fatalf("want no group accordions when no accounts, got %d", len(vm.Groups))
	}
}

func TestBuildNetworthInsights_AccountsButNoBalancesThisMonth(t *testing.T) {
	accounts := []core.Account{
		mkAcct(1, "Conto BPM", core.AccountCash, true),
		mkAcct(2, "Buoni postali", core.AccountLongTerm, true),
	}
	var trend [12]int64
	trend[0] = 100000 // some history
	sy, sm := trendStart(2026, 5)
	vm := buildNetworthInsights(2026, 5, accounts, nil, nil, trend, sy, sm)

	if vm.HasData {
		t.Fatalf("want HasData=false when no current-month balances")
	}
	if vm.MonthlyTotalFmt != formatEuros(0) {
		t.Fatalf("hero want €0,00, got %q", vm.MonthlyTotalFmt)
	}
	if vm.Trend[0].Cents != 100000 {
		t.Fatalf("trend should still expose past data, got %d", vm.Trend[0].Cents)
	}
}

func TestBuildNetworthInsights_TypeStripAndGroups(t *testing.T) {
	accounts := []core.Account{
		mkAcct(1, "Conto BPM", core.AccountCash, true),
		mkAcct(2, "Cassa", core.AccountCash, true),
		mkAcct(3, "Buffer", core.AccountRainyDay, true),
		mkAcct(4, "ETF", core.AccountLongTerm, true),
		mkAcct(5, "Vecchio conto", core.AccountCash, false), // inactive
	}
	curr := map[int64]core.AccountBalance{
		1: mkBal(1, 2026, 5, 200000),  // Cash
		2: mkBal(2, 2026, 5, 50000),   // Cash
		3: mkBal(3, 2026, 5, 800000),  // RainyDay
		4: mkBal(4, 2026, 5, 1500000), // LongTerm
	}
	prev := map[int64]core.AccountBalance{
		1: mkBal(1, 2026, 4, 100000),
		2: mkBal(2, 2026, 4, 50000),
		3: mkBal(3, 2026, 4, 800000),
		4: mkBal(4, 2026, 4, 1300000),
	}
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildNetworthInsights(2026, 5, accounts, curr, prev, trend, sy, sm)

	if !vm.HasData {
		t.Fatalf("want HasData=true")
	}
	// Hero total: 200000 + 50000 + 800000 + 1500000 = 2,550,000 cents.
	if vm.MonthlyTotalFmt != formatEuros(2550000) {
		t.Fatalf("hero want €25.500,00, got %q", vm.MonthlyTotalFmt)
	}
	if vm.ActiveCount != 4 {
		t.Fatalf("ActiveCount want 4 (accounts with current balance), got %d", vm.ActiveCount)
	}

	// Type strip: order Cash, RainyDay, LongTerm.
	if vm.Types[0].Key != string(core.AccountCash) || vm.Types[0].Count != 2 || vm.Types[0].AmountFmt != formatEuros(250000) {
		t.Fatalf("Cash cell wrong: %+v", vm.Types[0])
	}
	if vm.Types[1].Key != string(core.AccountRainyDay) || vm.Types[1].Count != 1 || vm.Types[1].AmountFmt != formatEuros(800000) {
		t.Fatalf("RainyDay cell wrong: %+v", vm.Types[1])
	}
	if vm.Types[2].Key != string(core.AccountLongTerm) || vm.Types[2].Count != 1 || vm.Types[2].AmountFmt != formatEuros(1500000) {
		t.Fatalf("LongTerm cell wrong: %+v", vm.Types[2])
	}

	// Groups: 3 (one per type), in same order.
	if len(vm.Groups) != 3 {
		t.Fatalf("want 3 groups, got %d", len(vm.Groups))
	}
	if vm.Groups[0].Type != core.AccountCash {
		t.Fatalf("group[0] type want cash, got %s", vm.Groups[0].Type)
	}
	// Group totals match cells.
	if vm.Groups[0].TotalFmt != formatEuros(250000) {
		t.Fatalf("Cash group total wrong: %q", vm.Groups[0].TotalFmt)
	}
	// Bar widths: largest = LongTerm (1,500,000) → 100; Cash 250,000 → 16; RainyDay 800,000 → 53.
	if vm.Groups[2].WidthPct != 100 {
		t.Fatalf("LongTerm bar should be 100, got %d", vm.Groups[2].WidthPct)
	}
	if vm.Groups[0].WidthPct != int(250000*100/1500000) {
		t.Fatalf("Cash bar wrong: %d", vm.Groups[0].WidthPct)
	}
	if vm.Groups[1].WidthPct != int(800000*100/1500000) {
		t.Fatalf("RainyDay bar wrong: %d", vm.Groups[1].WidthPct)
	}

	// Cash group should contain 3 accounts (2 active + 1 inactive), inactive last.
	if len(vm.Groups[0].Accounts) != 3 {
		t.Fatalf("Cash group want 3 rows (incl inactive), got %d", len(vm.Groups[0].Accounts))
	}
	var sawInactive bool
	for _, r := range vm.Groups[0].Accounts {
		if r.IsInactive {
			sawInactive = true
		}
	}
	if !sawInactive {
		t.Fatalf("inactive account should appear in its type group")
	}
}

func TestBuildNetworthInsights_DeltaPositive(t *testing.T) {
	accounts := []core.Account{mkAcct(1, "A", core.AccountCash, true)}
	curr := map[int64]core.AccountBalance{1: mkBal(1, 2026, 5, 600000)}
	prev := map[int64]core.AccountBalance{1: mkBal(1, 2026, 4, 500000)}
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildNetworthInsights(2026, 5, accounts, curr, prev, trend, sy, sm)

	// (600000 - 500000) / 500000 * 100 = 20.
	if vm.DeltaSign != "+" || vm.DeltaPct != 20 {
		t.Fatalf("delta want +20%%, got %s%d", vm.DeltaSign, vm.DeltaPct)
	}
	if vm.DeltaIsZero {
		t.Fatalf("DeltaIsZero should be false")
	}
	if vm.DeltaAbsFmt != formatEuros(100000) {
		t.Fatalf("DeltaAbsFmt want €1.000,00, got %q", vm.DeltaAbsFmt)
	}
	if vm.PrevMonthShort != italianMonthShort(4) {
		t.Fatalf("PrevMonthShort want Apr, got %q", vm.PrevMonthShort)
	}
}

func TestBuildNetworthInsights_DeltaNegative(t *testing.T) {
	accounts := []core.Account{mkAcct(1, "A", core.AccountCash, true)}
	curr := map[int64]core.AccountBalance{1: mkBal(1, 2026, 5, 400000)}
	prev := map[int64]core.AccountBalance{1: mkBal(1, 2026, 4, 500000)}
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildNetworthInsights(2026, 5, accounts, curr, prev, trend, sy, sm)

	// (400000 - 500000) / 500000 * 100 = -20.
	if vm.DeltaSign != "−" || vm.DeltaPct != 20 {
		t.Fatalf("delta want −20%%, got %s%d", vm.DeltaSign, vm.DeltaPct)
	}
	if vm.DeltaAbsFmt != formatEuros(100000) {
		t.Fatalf("DeltaAbsFmt want €1.000,00 (abs), got %q", vm.DeltaAbsFmt)
	}
}

func TestBuildNetworthInsights_TrendWindow(t *testing.T) {
	var trend [12]int64
	for i := range trend {
		trend[i] = int64((i + 1) * 1000)
	}
	sy, sm := trendStart(2026, 5)
	if sy != 2025 || sm != 6 {
		t.Fatalf("trendStart May 2026: want (2025, 6), got (%d, %d)", sy, sm)
	}
	vm := buildNetworthInsights(2026, 5, nil, nil, nil, trend, sy, sm)

	if vm.Trend[0].MonthShort != italianMonthShort(6) || vm.Trend[0].Year != 2025 {
		t.Fatalf("first cell want Giu 2025, got %s %d", vm.Trend[0].MonthShort, vm.Trend[0].Year)
	}
	if vm.Trend[11].MonthShort != italianMonthShort(5) || vm.Trend[11].Year != 2026 {
		t.Fatalf("last cell want Mag 2026, got %s %d", vm.Trend[11].MonthShort, vm.Trend[11].Year)
	}
	if !vm.Trend[11].IsCurrent {
		t.Fatalf("last trend cell should be current")
	}
	if vm.Trend[11].HeightPct != 100 {
		t.Fatalf("largest cell should be 100%% height, got %d", vm.Trend[11].HeightPct)
	}
}
