package http

import (
	"testing"
	"time"

	"spese/internal/core"
)

func TestBuildDashboardShortcutsEmpty(t *testing.T) {
	vm := buildDashboardShortcuts(
		core.MonthOverview{}, core.MonthOverview{}, 0,
		core.IncomeMonthOverview{}, core.IncomeMonthOverview{}, 0,
		nil,
		behavioralMetrics{},
		time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
	)

	if vm.Spese.HasData || vm.Entrate.HasData || vm.Ricorrenti.HasData {
		t.Fatalf("empty inputs: all cards should have HasData=false, got Spese=%v Entrate=%v Ricorrenti=%v",
			vm.Spese.HasData, vm.Entrate.HasData, vm.Ricorrenti.HasData)
	}
	if vm.Spese.Count != 0 || vm.Entrate.Count != 0 || vm.Ricorrenti.Count != 0 {
		t.Fatalf("empty inputs: counts should be 0; got %+v", vm)
	}
	if vm.Spese.AmountFmt != "€0,00" || vm.Entrate.AmountFmt != "€0,00" || vm.Ricorrenti.AmountFmt != "€0,00" {
		t.Fatalf("empty inputs: amounts should be €0,00; got %+v", vm)
	}
	if vm.Spese.Href != "/spese" || vm.Entrate.Href != "/entrate" || vm.Ricorrenti.Href != "/recurrent" {
		t.Fatalf("hrefs wrong: %+v", vm)
	}
}

func TestBuildDashboardShortcutsDeltaSpese(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		curr     int64
		prev     int64
		wantSign string
		wantPct  int
		wantZero bool
	}{
		{"prev_zero", 5000, 0, "", 0, true},
		{"increase", 12000, 10000, "+", 20, false},
		{"decrease", 8000, 10000, "−", 20, false},
		{"flat", 10000, 10000, "", 0, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vm := buildDashboardShortcuts(
				core.MonthOverview{Total: core.Money{Cents: c.curr}},
				core.MonthOverview{Total: core.Money{Cents: c.prev}},
				1,
				core.IncomeMonthOverview{}, core.IncomeMonthOverview{}, 0,
				nil,
				behavioralMetrics{},
				now,
			)
			if vm.Spese.DeltaIsZero != c.wantZero {
				t.Fatalf("%s: DeltaIsZero=%v want %v", c.name, vm.Spese.DeltaIsZero, c.wantZero)
			}
			if !c.wantZero {
				if vm.Spese.DeltaSign != c.wantSign || vm.Spese.DeltaPct != c.wantPct {
					t.Fatalf("%s: sign=%q pct=%d want %q %d", c.name, vm.Spese.DeltaSign, vm.Spese.DeltaPct, c.wantSign, c.wantPct)
				}
			}
		})
	}
}

func TestBuildDashboardShortcutsDeltaEntrate(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	vm := buildDashboardShortcuts(
		core.MonthOverview{}, core.MonthOverview{}, 0,
		core.IncomeMonthOverview{Total: core.Money{Cents: 15000}},
		core.IncomeMonthOverview{Total: core.Money{Cents: 10000}},
		1,
		nil, behavioralMetrics{}, now,
	)
	if vm.Entrate.DeltaIsZero {
		t.Fatalf("Entrate Δ should be set, got %+v", vm.Entrate)
	}
	if vm.Entrate.DeltaSign != "+" || vm.Entrate.DeltaPct != 50 {
		t.Fatalf("Entrate Δ wrong: sign=%q pct=%d", vm.Entrate.DeltaSign, vm.Entrate.DeltaPct)
	}
}

func TestBuildDashboardShortcutsRicorrentiNoDelta(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	recs := []core.RecurrentExpenses{
		{Description: "Netflix", Every: core.Monthly, Amount: core.Money{Cents: 1000}, StartDate: core.NewDate(2025, 1, 1)},
		{Description: "Gym", Every: core.Monthly, Amount: core.Money{Cents: 4000}, StartDate: core.NewDate(2025, 1, 1)},
	}
	vm := buildDashboardShortcuts(
		core.MonthOverview{}, core.MonthOverview{}, 0,
		core.IncomeMonthOverview{}, core.IncomeMonthOverview{}, 0,
		recs, behavioralMetrics{}, now,
	)

	if !vm.Ricorrenti.HasData {
		t.Fatalf("Ricorrenti should HaveData with active recurrents, got %+v", vm.Ricorrenti)
	}
	if vm.Ricorrenti.Count != 2 {
		t.Fatalf("Ricorrenti count = %d want 2", vm.Ricorrenti.Count)
	}
	if vm.Ricorrenti.AmountFmt != "€50,00" {
		t.Fatalf("Ricorrenti monthly = %q want €50,00", vm.Ricorrenti.AmountFmt)
	}
	if !vm.Ricorrenti.DeltaIsZero {
		t.Fatalf("Ricorrenti card must never render Δ; DeltaIsZero=false")
	}
	if vm.Ricorrenti.DeltaSign != "" || vm.Ricorrenti.DeltaPct != 0 {
		t.Fatalf("Ricorrenti Δ must be empty, got sign=%q pct=%d", vm.Ricorrenti.DeltaSign, vm.Ricorrenti.DeltaPct)
	}
}

func TestBuildDashboardShortcutsHasDataFlags(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	vm := buildDashboardShortcuts(
		core.MonthOverview{Total: core.Money{Cents: 100}}, core.MonthOverview{}, 1,
		core.IncomeMonthOverview{}, core.IncomeMonthOverview{}, 0,
		nil, behavioralMetrics{}, now,
	)
	if !vm.Spese.HasData {
		t.Fatalf("Spese HasData should be true with non-zero total")
	}
	if vm.Entrate.HasData {
		t.Fatalf("Entrate HasData should be false with zero data")
	}
}
