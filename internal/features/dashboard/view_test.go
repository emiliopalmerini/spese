package dashboard

import (
	"strings"
	"testing"

	"spese/internal/features/reports"
	"spese/internal/features/transactions"
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
			{Account: "Broker", Class: "Investment", Balance: kernel.Money(3300000)},
			{Account: "Carta", Class: "Credit", Balance: kernel.Money(-150000)},
		},
		[]reports.InvestmentRow{
			{Account: "Broker", CostBasis: kernel.Money(3000000), Value: kernel.Money(3300000), Return: kernel.Money(300000), ReturnPct: 0.10, LatestMonth: may},
		},
		[]transactions.Transaction{
			{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-90000), Category: "Casa"},
			{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-50000), Category: "Cibo"},
			{Date: may, Kind: transactions.Income, Amount: kernel.Money(300000), Category: "Stipendio"},
			{Date: may, Kind: transactions.Income, Amount: kernel.Money(50000), Category: "Extra"},
		},
		may,
	)

	assertKPI(t, view.KPIs, "Patrimonio netto", "45.000,00 €")
	assertKPI(t, view.KPIs, "Risparmio mese", "2.100,00 €")
	assertKPI(t, view.KPIs, "Tasso risparmio", "60,0%")
	assertKPI(t, view.KPIs, "Investimenti", "33.000,00 €")
	assertKPIHelp(t, view.KPIs, "Patrimonio netto", "Aggiornato a maggio 2026")
	assertKPIHelp(t, view.KPIs, "Risparmio mese", "Maggio 2026 · entrate meno uscite")
	assertKPIHelp(t, view.KPIs, "Investimenti", "Aggiornato a maggio 2026")
	if view.Period != "2026-05" || view.PreviousPeriodURL != "/?month=2026-04" || view.NextPeriodURL != "/?month=2026-06" {
		t.Fatalf("period navigation = %q, %q, %q", view.Period, view.PreviousPeriodURL, view.NextPeriodURL)
	}

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
	if view.Allocation.Rows[0].Label != "Investimenti" {
		t.Fatalf("first allocation label = %q, want Investimenti", view.Allocation.Rows[0].Label)
	}
	if view.Allocation.TotalFmt != "43.500,00 €" || view.Allocation.GrossFmt != "46.500,00 €" {
		t.Fatalf("allocation totals = %q net, %q gross", view.Allocation.TotalFmt, view.Allocation.GrossFmt)
	}
	if len(view.Investments.Rows) != 1 {
		t.Fatalf("investment rows = %d, want 1", len(view.Investments.Rows))
	}
	if len(view.ExpenseBreakdown.Rows) != 2 {
		t.Fatalf("expense category rows = %d, want 2", len(view.ExpenseBreakdown.Rows))
	}
	if view.ExpenseBreakdown.TotalFmt != "1.400,00 €" {
		t.Fatalf("expense total = %q, want 1.400,00 €", view.ExpenseBreakdown.TotalFmt)
	}
	if len(view.IncomeComposition.Rows) != 2 {
		t.Fatalf("income category rows = %d, want 2", len(view.IncomeComposition.Rows))
	}
	if view.IncomeComposition.TotalFmt != "3.500,00 €" {
		t.Fatalf("income total = %q, want 3.500,00 €", view.IncomeComposition.TotalFmt)
	}
}

func TestBuildViewUsesRequestedPeriodForMonthlySummary(t *testing.T) {
	may := mustDate(t, "2026-05")
	jun := mustDate(t, "2026-06")

	view := buildView(
		[]reports.IncomeRow{
			{Month: may, Revenue: kernel.Money(350000), Expenses: kernel.Money(-140000), NetIncome: kernel.Money(210000), SavingsRate: 0.60},
		},
		nil,
		nil,
		nil,
		[]transactions.Transaction{
			{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-90000), Category: "Casa"},
			{Date: may, Kind: transactions.Income, Amount: kernel.Money(300000), Category: "Stipendio"},
		},
		jun,
	)

	assertKPI(t, view.KPIs, "Risparmio mese", "—")
	assertKPI(t, view.KPIs, "Tasso risparmio", "—")
	if view.ExpenseBreakdown.Period != "2026-06" {
		t.Fatalf("expense period = %q, want 2026-06", view.ExpenseBreakdown.Period)
	}
	if !view.ExpenseBreakdown.Empty {
		t.Fatal("expense breakdown should be empty for the requested month")
	}
	if view.IncomeComposition.Period != "2026-06" {
		t.Fatalf("income period = %q, want 2026-06", view.IncomeComposition.Period)
	}
	if !view.IncomeComposition.Empty {
		t.Fatal("income composition should be empty for the requested month")
	}
}

func TestDashboardPeriodUsesCurrentMonth(t *testing.T) {
	got := dashboardPeriod()
	want := kernel.Today().FirstOfMonth()
	if !got.Equal(want.Time) {
		t.Fatalf("dashboard period = %s, want current month %s", got.Month(), want.Month())
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
	if chart.Months[0].Label != "Mar 25" {
		t.Fatalf("first month label = %q, want Mar 25", chart.Months[0].Label)
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

func TestBuildCashFlowChartFillsMissingMonths(t *testing.T) {
	chart := buildCashFlowChart([]reports.IncomeRow{
		{Month: mustDate(t, "2026-01"), Revenue: 100000},
		{Month: mustDate(t, "2026-03"), Revenue: 120000},
	})

	if len(chart.Months) != 3 {
		t.Fatalf("months = %d, want 3", len(chart.Months))
	}
	if chart.Months[1].Detail != "2026-02" {
		t.Fatalf("middle month = %q, want 2026-02", chart.Months[1].Detail)
	}
	if len(chart.Bars) != 6 || chart.Bars[2].Height != 0 || chart.Bars[3].Height != 0 {
		t.Fatalf("missing month bars = %+v, want two zero bars", chart.Bars[2:4])
	}
}

func TestBuildViewEmptyRowsUseEmptyStates(t *testing.T) {
	view := buildView(nil, nil, nil, nil, nil, mustDate(t, "2026-05"))

	assertKPI(t, view.KPIs, "Patrimonio netto", "—")
	assertKPI(t, view.KPIs, "Risparmio mese", "—")
	assertKPI(t, view.KPIs, "Tasso risparmio", "—")
	assertKPI(t, view.KPIs, "Investimenti", "—")
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
	if !view.ExpenseBreakdown.Empty {
		t.Fatal("expense breakdown should be empty")
	}
	if !view.IncomeComposition.Empty {
		t.Fatal("income composition should be empty")
	}
}

func TestBuildCategoryChartGroupsAndSortsTransactions(t *testing.T) {
	may := mustDate(t, "2026-05")
	chart := buildCategoryChart([]transactions.Transaction{
		{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-2000), Category: "Cibo"},
		{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-9000), Category: "Casa"},
		{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-1000), Category: "Cibo"},
		{Date: may, Kind: transactions.Income, Amount: kernel.Money(500000), Category: "Stipendio"},
		{Date: may, Kind: transactions.Expense, Amount: kernel.Money(-500), Category: ""},
	}, transactions.Expense, may)

	if chart.Empty {
		t.Fatal("category chart unexpectedly empty")
	}
	if chart.TotalFmt != "125,00 €" {
		t.Fatalf("total = %q, want 125,00 €", chart.TotalFmt)
	}
	if got := chart.Rows[0].Label; got != "Casa" {
		t.Fatalf("first row = %q, want Casa", got)
	}
	if got := chart.Rows[0].URL; got != "/transactions?category=Casa&from=2026-05-01&to=2026-05-31" {
		t.Fatalf("first row URL = %q", got)
	}
	if got := chart.Rows[1].Label; got != "Cibo" {
		t.Fatalf("second row = %q, want Cibo", got)
	}
	if got := chart.Rows[2].Label; got != "Senza categoria" {
		t.Fatalf("third row = %q, want Senza categoria", got)
	}
	if len(chart.Segments) != len(chart.Rows) {
		t.Fatalf("segments = %d, rows = %d", len(chart.Segments), len(chart.Rows))
	}
}

func TestBuildWaterfallChartExplainsMonthlySavings(t *testing.T) {
	may := mustDate(t, "2026-05")
	chart := buildWaterfallChart([]transactions.Transaction{
		{Date: may, Kind: transactions.Income, Amount: 300000, Category: "Stipendio"},
		{Date: may, Kind: transactions.Income, Amount: -20000, Category: "Imposte"},
		{Date: may, Kind: transactions.Expense, Amount: -100000, Category: "Casa"},
		{Date: may, Kind: transactions.Expense, Amount: -50000, Category: "Cibo"},
	}, may)

	if chart.Empty {
		t.Fatal("waterfall chart unexpectedly empty")
	}
	if chart.AxisStart < 90 {
		t.Fatalf("axis start = %d, want room for y-axis labels", chart.AxisStart)
	}
	if len(chart.Bars) != 5 {
		t.Fatalf("bars = %d, want income, three deductions and savings", len(chart.Bars))
	}
	if chart.Bars[0].Label != "Entrate" || chart.Bars[0].ValueFmt != "+3.000,00 €" {
		t.Fatalf("income bar = %+v", chart.Bars[0])
	}
	if chart.Bars[1].Label != "Casa" || chart.Bars[1].ValueFmt != "-1.000,00 €" {
		t.Fatalf("first expense bar = %+v", chart.Bars[1])
	}
	if last := chart.Bars[len(chart.Bars)-1]; last.Label != "Risparmio" || last.ValueFmt != "+1.300,00 €" || last.Tone != "positive" {
		t.Fatalf("savings bar = %+v", last)
	}
	if len(chart.Connectors) != 3 {
		t.Fatalf("connectors = %d, want 3", len(chart.Connectors))
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

func TestBuildCashFlowChartDivergesAroundZeroAndPlotsSavings(t *testing.T) {
	chart := buildCashFlowChart([]reports.IncomeRow{
		{Month: mustDate(t, "2026-05"), Revenue: 300000, Expenses: -180000, NetIncome: 120000},
	})

	if len(chart.Bars) != 2 {
		t.Fatalf("bars = %d, want 2", len(chart.Bars))
	}
	if chart.AxisStart < 90 {
		t.Fatalf("axis start = %d, want room for y-axis labels", chart.AxisStart)
	}
	income, expense := chart.Bars[0], chart.Bars[1]
	if income.Y >= chart.Baseline || income.Y+income.Height != chart.Baseline {
		t.Fatalf("income bar = y %d height %d around baseline %d", income.Y, income.Height, chart.Baseline)
	}
	if expense.Y != chart.Baseline || expense.Height == 0 {
		t.Fatalf("expense bar = y %d height %d, want it below baseline %d", expense.Y, expense.Height, chart.Baseline)
	}
	if len(chart.NetPoints) != 1 || chart.NetPoints[0].Y >= chart.Baseline {
		t.Fatalf("net points = %+v, want positive savings above zero", chart.NetPoints)
	}
	if chart.NetPoints[0].ValueFmt != "+1.200,00 €" || chart.NetPoints[0].Tone != "positive" {
		t.Fatalf("net point = %+v", chart.NetPoints[0])
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
	if chart.AxisStart < 90 {
		t.Fatalf("axis start = %d, want room for y-axis labels", chart.AxisStart)
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
	if len(chart.YTicks) != 4 {
		t.Fatalf("y ticks = %d, want 4", len(chart.YTicks))
	}
	if chart.Points[0].Y == 18 || chart.Points[0].Y == chart.Baseline {
		t.Fatalf("first point y = %d, want padded away from chart edges", chart.Points[0].Y)
	}
	if chart.ChangeFmt != "+1.000,00 €" || chart.ChangePctFmt != "+10,0%" {
		t.Fatalf("change = %q (%q), want +1.000,00 € (+10,0%%)", chart.ChangeFmt, chart.ChangePctFmt)
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
	if chart.ReturnPctFmt != "13,8%" {
		t.Fatalf("return pct = %q, want 13,8%%", chart.ReturnPctFmt)
	}
}

func TestBuildInvestmentChartReportsWhenRowsAreTruncated(t *testing.T) {
	rows := make([]reports.InvestmentRow, 6)
	for i := range rows {
		rows[i] = reports.InvestmentRow{Account: string(rune('A' + i)), Value: kernel.Money(1000 + i)}
	}

	chart := buildInvestmentChart(rows)

	if !chart.Truncated || len(chart.Rows) != 5 {
		t.Fatalf("truncated = %t with %d rows, want true with 5 rows", chart.Truncated, len(chart.Rows))
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

func assertKPIHelp(t *testing.T, kpis []KPI, label, want string) {
	t.Helper()
	for _, kpi := range kpis {
		if kpi.Label == label {
			if kpi.Help != want {
				t.Fatalf("%s help = %q, want %q", label, kpi.Help, want)
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
