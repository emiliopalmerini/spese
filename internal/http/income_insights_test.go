package http

import (
	"testing"

	"spese/internal/core"
	"spese/internal/storage"
)

func mkIncome(id string, day, month, year int, cents int64, desc, cat string) storage.IncomeWithID {
	return storage.IncomeWithID{
		ID: id,
		Income: core.Income{
			Date:        core.NewDate(year, month, day),
			Description: desc,
			Amount:      core.Money{Cents: cents},
			Category:    cat,
		},
	}
}

func TestBuildIncomeInsights_Empty(t *testing.T) {
	curr := core.IncomeMonthOverview{Year: 2026, Month: 5}
	prev := core.IncomeMonthOverview{Year: 2026, Month: 4}
	var trend [12]int64
	startY, startM := trendStart(2026, 5)
	vm := buildIncomeInsights(curr, prev, trend, startY, startM, 0, nil, map[string]bool{})

	if vm.HasData {
		t.Fatalf("want HasData=false")
	}
	if len(vm.Sources) != 3 {
		t.Fatalf("want 3 source cells, got %d", len(vm.Sources))
	}
	for _, s := range vm.Sources {
		if s.Count != 0 || s.AmountFmt != "—" {
			t.Fatalf("want zero/em-dash source, got %+v", s)
		}
	}
	if len(vm.Trend) != 12 {
		t.Fatalf("want 12 trend cells, got %d", len(vm.Trend))
	}
	if !vm.DeltaIsZero {
		t.Fatalf("want DeltaIsZero when prev=0")
	}
}

func TestBuildIncomeInsights_SourceSegmentation(t *testing.T) {
	items := []storage.IncomeWithID{
		mkIncome("1", 1, 5, 2026, 200000, "Stipendio aprile", "Stipendio E"),
		mkIncome("2", 5, 5, 2026, 100000, "Fattura cliente A", "Freelance E"),
		mkIncome("3", 7, 5, 2026, 5000, "Rimborso", "Rimborsi"),
	}
	curr := core.IncomeMonthOverview{
		Year: 2026, Month: 5,
		Total: core.Money{Cents: 305000},
		ByCategory: []core.CategoryAmount{
			{Name: "Stipendio E", Amount: core.Money{Cents: 200000}},
			{Name: "Freelance E", Amount: core.Money{Cents: 100000}},
			{Name: "Rimborsi", Amount: core.Money{Cents: 5000}},
		},
	}
	prev := core.IncomeMonthOverview{Year: 2026, Month: 4, Total: core.Money{Cents: 305000}}
	freelance := map[string]bool{"Freelance E": true}
	var trend [12]int64
	startY, startM := trendStart(2026, 5)
	vm := buildIncomeInsights(curr, prev, trend, startY, startM, 305000, items, freelance)

	if !vm.HasData {
		t.Fatalf("want HasData=true")
	}
	get := map[string]IncomeSourceVM{}
	for _, s := range vm.Sources {
		get[s.Key] = s
	}
	if get[incomeSourceStipendio].Count != 1 {
		t.Fatalf("want 1 stipendio, got %d", get[incomeSourceStipendio].Count)
	}
	if get[incomeSourceFreelance].Count != 1 {
		t.Fatalf("want 1 freelance, got %d", get[incomeSourceFreelance].Count)
	}
	if get[incomeSourceAltro].Count != 1 {
		t.Fatalf("want 1 altro, got %d", get[incomeSourceAltro].Count)
	}
}

func TestBuildIncomeInsights_TopOrderingAndDelta(t *testing.T) {
	curr := core.IncomeMonthOverview{
		Year: 2026, Month: 5,
		Total: core.Money{Cents: 600000},
		ByCategory: []core.CategoryAmount{
			{Name: "C", Amount: core.Money{Cents: 100000}},
			{Name: "A", Amount: core.Money{Cents: 300000}},
			{Name: "B", Amount: core.Money{Cents: 200000}},
		},
	}
	prev := core.IncomeMonthOverview{Year: 2026, Month: 4, Total: core.Money{Cents: 500000}}
	var trend [12]int64
	startY, startM := trendStart(2026, 5)
	vm := buildIncomeInsights(curr, prev, trend, startY, startM, 0, nil, nil)

	if len(vm.Top5) != 3 {
		t.Fatalf("want 3 top rows, got %d", len(vm.Top5))
	}
	if vm.Top5[0].Name != "A" || vm.Top5[1].Name != "B" || vm.Top5[2].Name != "C" {
		t.Fatalf("ordering wrong: %+v", vm.Top5)
	}
	if vm.Top5[0].WidthPct != 100 {
		t.Fatalf("top width want 100 got %d", vm.Top5[0].WidthPct)
	}

	// Delta: (600000 - 500000) / 500000 * 100 = 20
	if vm.DeltaSign != "+" || vm.DeltaPct != 20 {
		t.Fatalf("delta want +20%%, got %s%d", vm.DeltaSign, vm.DeltaPct)
	}
}

func TestBuildIncomeInsights_TrendOrdering(t *testing.T) {
	curr := core.IncomeMonthOverview{Year: 2026, Month: 5, Total: core.Money{Cents: 1000}}
	prev := core.IncomeMonthOverview{Year: 2026, Month: 4}
	var trend [12]int64
	for i := range trend {
		trend[i] = int64(i + 1)
	}
	startY, startM := trendStart(2026, 5)
	if startY != 2025 || startM != 6 {
		t.Fatalf("trendStart for May 2026: want (2025, 6), got (%d, %d)", startY, startM)
	}
	vm := buildIncomeInsights(curr, prev, trend, startY, startM, 0, nil, nil)
	if len(vm.Trend) != 12 {
		t.Fatalf("want 12 cells, got %d", len(vm.Trend))
	}
	if vm.Trend[0].MonthShort != "Giu" || vm.Trend[0].Year != 2025 {
		t.Fatalf("first cell want Giu 2025, got %s %d", vm.Trend[0].MonthShort, vm.Trend[0].Year)
	}
	if vm.Trend[11].MonthShort != "Mag" || vm.Trend[11].Year != 2026 {
		t.Fatalf("last cell want Mag 2026, got %s %d", vm.Trend[11].MonthShort, vm.Trend[11].Year)
	}
	if !vm.Trend[11].IsCurrent {
		t.Fatalf("last cell should be current")
	}
}
