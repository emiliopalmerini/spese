package http

import (
	"testing"

	"spese/internal/core"
	"spese/internal/sheets"
)

func mkExp(id string, day, month, year int, cents int64, desc, p, s string) sheets.ExpenseWithID {
	return sheets.ExpenseWithID{
		ID: id,
		Expense: core.Expense{
			Date:        core.NewDate(year, month, day),
			Description: desc,
			Amount:      core.Money{Cents: cents},
			Primary:     p,
			Secondary:   s,
		},
	}
}

func TestBuildExpenseInsights_Empty(t *testing.T) {
	curr := core.MonthOverview{Year: 2026, Month: 5}
	prev := core.MonthOverview{Year: 2026, Month: 4}
	var trend [12]int64
	startY, startM := trendStart(2026, 5)
	vm := buildExpenseInsights(curr, prev, trend, startY, startM, nil)

	if vm.HasData {
		t.Fatalf("want HasData=false")
	}
	if !vm.DeltaIsZero {
		t.Fatalf("want DeltaIsZero when prev=0")
	}
	if len(vm.Trend) != 12 {
		t.Fatalf("want 12 trend cells, got %d", len(vm.Trend))
	}
	if len(vm.Top5) != 0 || len(vm.Categories) != 0 || len(vm.Items) != 0 {
		t.Fatalf("want all blocks empty")
	}
}

func TestBuildExpenseInsights_TopAndDelta(t *testing.T) {
	curr := core.MonthOverview{
		Year: 2026, Month: 5,
		Total: core.Money{Cents: 60000},
		ByCategory: []core.CategoryAmount{
			{Name: "C", Amount: core.Money{Cents: 10000}},
			{Name: "A", Amount: core.Money{Cents: 30000}},
			{Name: "B", Amount: core.Money{Cents: 20000}},
		},
	}
	prev := core.MonthOverview{Year: 2026, Month: 4, Total: core.Money{Cents: 50000}}
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildExpenseInsights(curr, prev, trend, sy, sm, nil)

	if len(vm.Top5) != 3 {
		t.Fatalf("want 3 top, got %d", len(vm.Top5))
	}
	if vm.Top5[0].Name != "A" || vm.Top5[1].Name != "B" || vm.Top5[2].Name != "C" {
		t.Fatalf("ordering wrong: %+v", vm.Top5)
	}
	if vm.Top5[0].WidthPct != 100 {
		t.Fatalf("top width want 100, got %d", vm.Top5[0].WidthPct)
	}
	// Δ = (60000-50000)/50000 = 20%
	if vm.DeltaSign != "+" || vm.DeltaPct != 20 {
		t.Fatalf("delta want +20%%, got %s%d", vm.DeltaSign, vm.DeltaPct)
	}
}

func TestBuildExpenseInsights_NestedCategories(t *testing.T) {
	items := []sheets.ExpenseWithID{
		mkExp("1", 1, 5, 2026, 5000, "Spesa", "Casa", "Affitto"),
		mkExp("2", 5, 5, 2026, 2000, "Bolletta", "Casa", "Utenze"),
		mkExp("3", 7, 5, 2026, 1500, "Bolletta gas", "Casa", "Utenze"),
		mkExp("4", 8, 5, 2026, 1000, "Pranzo", "Cibo", "Ristoranti"),
	}
	curr := core.MonthOverview{Year: 2026, Month: 5, Total: core.Money{Cents: 9500}, ByCategory: []core.CategoryAmount{
		{Name: "Casa", Amount: core.Money{Cents: 8500}},
		{Name: "Cibo", Amount: core.Money{Cents: 1000}},
	}}
	prev := core.MonthOverview{Year: 2026, Month: 4}
	var trend [12]int64
	sy, sm := trendStart(2026, 5)
	vm := buildExpenseInsights(curr, prev, trend, sy, sm, items)

	if !vm.HasData {
		t.Fatalf("want HasData=true")
	}
	if len(vm.Categories) != 2 {
		t.Fatalf("want 2 groups, got %d", len(vm.Categories))
	}
	if vm.Categories[0].Name != "Casa" {
		t.Fatalf("want Casa first, got %s", vm.Categories[0].Name)
	}
	if len(vm.Categories[0].Children) != 2 {
		t.Fatalf("want 2 secondaries under Casa, got %d", len(vm.Categories[0].Children))
	}
	// Affitto 5000 > Utenze 3500
	if vm.Categories[0].Children[0].Name != "Affitto" {
		t.Fatalf("want Affitto child first, got %s", vm.Categories[0].Children[0].Name)
	}
	if len(vm.Items) != 4 {
		t.Fatalf("want 4 items, got %d", len(vm.Items))
	}
}
