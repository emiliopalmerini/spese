package dashboard

import (
	"strings"
	"testing"

	"spese/internal/features/reports"
	"spese/internal/kernel"
)

func TestBuildViewSummarizesLatestReportRows(t *testing.T) {
	apr := mustDate(t, "2026-04")
	may := mustDate(t, "2026-05")

	view := buildView(
		[]reports.IncomeRow{
			{Month: apr, Revenue: kernel.Money(300000), Expenses: kernel.Money(-120000), NetIncome: kernel.Money(180000), SavingsRate: 0.60},
			{Month: may, Revenue: kernel.Money(350000), Expenses: kernel.Money(-140000), NetIncome: kernel.Money(210000), SavingsRate: 0.60},
		},
		[]reports.NwRow{
			{Month: apr, NetWorth: kernel.Money(4200000)},
			{Month: may, NetWorth: kernel.Money(4500000)},
		},
		[]reports.BalanceRow{
			{Account: "Conto", Class: "Cash", Balance: kernel.Money(1200000)},
			{Account: "Broker", Class: "Investments", Balance: kernel.Money(3300000)},
			{Account: "Carta", Class: "Credit", Balance: kernel.Money(-150000)},
		},
		[]reports.InvestmentRow{
			{Account: "Broker", CostBasis: kernel.Money(3000000), Value: kernel.Money(3300000), Return: kernel.Money(300000), ReturnPct: 0.10},
		},
		[]Item{{Label: "Nota", Value: "Manuale"}},
	)

	assertKPI(t, view.KPIs, "Patrimonio netto", "45.000,00 €")
	assertKPI(t, view.KPIs, "Risparmio mese", "2.100,00 €")
	assertKPI(t, view.KPIs, "Tasso risparmio", "60.0%")
	assertKPI(t, view.KPIs, "Investimenti", "33.000,00 €")

	if len(view.CashFlow.Bars) != 4 {
		t.Fatalf("cashflow bars = %d, want 4", len(view.CashFlow.Bars))
	}
	if view.CashFlow.Empty {
		t.Fatal("cashflow chart unexpectedly empty")
	}
	if len(view.NetWorth.Points) != 2 {
		t.Fatalf("net worth points = %d, want 2", len(view.NetWorth.Points))
	}
	if len(view.Allocation.Rows) != 3 {
		t.Fatalf("allocation rows = %d, want 3", len(view.Allocation.Rows))
	}
	if len(view.Investments.Rows) != 1 {
		t.Fatalf("investment rows = %d, want 1", len(view.Investments.Rows))
	}
	if len(view.Items) != 1 {
		t.Fatalf("manual dashboard items = %d, want 1", len(view.Items))
	}
}

func TestBuildCashFlowChartKeepsLatestTwelveMonths(t *testing.T) {
	rows := make([]reports.IncomeRow, 0, 14)
	for month := 1; month <= 14; month++ {
		date := mustDate(t, "2025-01").AddDate(0, month-1, 0)
		rows = append(rows, reports.IncomeRow{
			Month:     kernel.Date{Time: date},
			Revenue:   kernel.Money(int64(month) * 10000),
			Expenses:  kernel.Money(int64(-month) * 5000),
			NetIncome: kernel.Money(int64(month) * 5000),
		})
	}

	chart := buildCashFlowChart(rows)

	if len(chart.Months) != 12 {
		t.Fatalf("months = %d, want 12", len(chart.Months))
	}
	if chart.Months[0].Label != "Mar" {
		t.Fatalf("first month label = %q, want Mar", chart.Months[0].Label)
	}
	if chart.Months[11].Label != "Feb" {
		t.Fatalf("last month label = %q, want Feb", chart.Months[11].Label)
	}
	if len(chart.Bars) != 24 {
		t.Fatalf("bars = %d, want 24", len(chart.Bars))
	}
	for _, bar := range chart.Bars {
		if bar.Height < 0 {
			t.Fatalf("bar height must not be negative: %+v", bar)
		}
	}
}

func TestBuildViewEmptyRowsUseEmptyStates(t *testing.T) {
	view := buildView(nil, nil, nil, nil, nil)

	assertKPI(t, view.KPIs, "Patrimonio netto", "0,00 €")
	assertKPI(t, view.KPIs, "Risparmio mese", "0,00 €")
	assertKPI(t, view.KPIs, "Tasso risparmio", "0.0%")
	assertKPI(t, view.KPIs, "Investimenti", "0,00 €")
	if !view.CashFlow.Empty {
		t.Fatal("cashflow chart should be empty")
	}
	if !view.NetWorth.Empty {
		t.Fatal("net worth chart should be empty")
	}
	if !view.Allocation.Empty {
		t.Fatal("allocation chart should be empty")
	}
	if !view.Investments.Empty {
		t.Fatal("investment chart should be empty")
	}
}

func TestBuildCashFlowChartAllZeroValuesDoesNotDivideByZero(t *testing.T) {
	chart := buildCashFlowChart([]reports.IncomeRow{
		{Month: mustDate(t, "2026-05")},
	})

	if chart.Empty {
		t.Fatal("cashflow chart with a row should not be empty")
	}
	if len(chart.Bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(chart.Bars))
	}
	for _, bar := range chart.Bars {
		if bar.Height != 0 {
			t.Fatalf("zero-value bar height = %d, want 0", bar.Height)
		}
		if bar.Y != chart.Baseline {
			t.Fatalf("zero-value bar y = %d, want baseline %d", bar.Y, chart.Baseline)
		}
	}
}

func TestBuildNetWorthChartProducesPolylinePoints(t *testing.T) {
	rows := []reports.NwRow{
		{Month: mustDate(t, "2026-01"), NetWorth: kernel.Money(1000000)},
		{Month: mustDate(t, "2026-02"), NetWorth: kernel.Money(1200000)},
		{Month: mustDate(t, "2026-03"), NetWorth: kernel.Money(1100000)},
	}

	chart := buildNetWorthChart(rows)

	if chart.Empty {
		t.Fatal("net worth chart unexpectedly empty")
	}
	if got := strings.Count(chart.PointsAttr, " "); got != 2 {
		t.Fatalf("point separators = %d, want 2 in %q", got, chart.PointsAttr)
	}
	if chart.MinFmt != "10.000,00 €" {
		t.Fatalf("min = %q, want 10.000,00 €", chart.MinFmt)
	}
	if chart.MaxFmt != "12.000,00 €" {
		t.Fatalf("max = %q, want 12.000,00 €", chart.MaxFmt)
	}
}

func TestBuildNetWorthChartSinglePointIsCentered(t *testing.T) {
	chart := buildNetWorthChart([]reports.NwRow{
		{Month: mustDate(t, "2026-05"), NetWorth: kernel.Money(1000000)},
	})

	if len(chart.Points) != 1 {
		t.Fatalf("points = %d, want 1", len(chart.Points))
	}
	if chart.Points[0].X != (chart.AxisStart+chart.AxisEnd)/2 {
		t.Fatalf("single point x = %d, want center between %d and %d", chart.Points[0].X, chart.AxisStart, chart.AxisEnd)
	}
}

func TestBuildInvestmentChartUsesWeightedTotalReturn(t *testing.T) {
	chart := buildInvestmentChart([]reports.InvestmentRow{
		{Account: "A", CostBasis: kernel.Money(100000), Value: kernel.Money(125000), Return: kernel.Money(25000), ReturnPct: 0.25},
		{Account: "B", CostBasis: kernel.Money(300000), Value: kernel.Money(330000), Return: kernel.Money(30000), ReturnPct: 0.10},
	})

	if chart.TotalValueFmt != "4.550,00 €" {
		t.Fatalf("total value = %q, want 4.550,00 €", chart.TotalValueFmt)
	}
	if chart.TotalReturnFmt != "550,00 €" {
		t.Fatalf("total return = %q, want 550,00 €", chart.TotalReturnFmt)
	}
	if chart.ReturnPctFmt != "13.8%" {
		t.Fatalf("return pct = %q, want 13.8%%", chart.ReturnPctFmt)
	}
}

func assertKPI(t *testing.T, kpis []KPI, label, want string) {
	t.Helper()
	for _, kpi := range kpis {
		if kpi.Label == label {
			if kpi.Value != want {
				t.Fatalf("%s = %q, want %q", label, kpi.Value, want)
			}
			return
		}
	}
	t.Fatalf("missing KPI %q in %+v", label, kpis)
}

func mustDate(t *testing.T, value string) kernel.Date {
	t.Helper()
	d, err := kernel.ParseDate(value)
	if err != nil {
		t.Fatalf("parse date %q: %v", value, err)
	}
	return d
}
