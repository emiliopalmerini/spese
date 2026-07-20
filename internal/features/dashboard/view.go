package dashboard

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"spese/internal/features/reports"
	"spese/internal/features/transactions"
	"spese/internal/kernel"
)

const (
	cashFlowWidth   = 720
	cashFlowHeight  = 230
	waterfallWidth  = 720
	waterfallHeight = 260
	lineWidth       = 720
	lineHeight      = 220
)

// View is the template payload for the charted dashboard.
type View struct {
	Period            string
	PreviousPeriodURL string
	NextPeriodURL     string
	KPIs              []KPI
	CashFlow          CashFlowChart
	Waterfall         WaterfallChart
	ExpenseBreakdown  CategoryChart
	IncomeComposition CategoryChart
	NetWorth          LineChart
	Allocation        AllocationChart
	Investments       InvestmentChart
}

// KPI is one headline metric in the dashboard summary strip.
type KPI struct {
	Label string
	Value string
	Help  string
	Tone  string
}

// CashFlowChart is a grouped monthly bar chart.
type CashFlowChart struct {
	Width         int
	Height        int
	AxisStart     int
	AxisEnd       int
	Baseline      int
	Empty         bool
	Months        []MonthTick
	Bars          []CashFlowBar
	NetPoints     []LinePoint
	NetPointsAttr string
	YTicks        []AxisTick
	MaxFmt        string
}

// AxisTick is a labelled horizontal guide in an SVG chart.
type AxisTick struct {
	Y        int
	ValueFmt string
}

// CashFlowBar is one SVG rect in the cash-flow chart.
type CashFlowBar struct {
	X        int
	Y        int
	Width    int
	Height   int
	Class    string
	Label    string
	Kind     string
	ValueFmt string
}

// WaterfallChart explains how monthly income becomes savings after expenses.
type WaterfallChart struct {
	Width       int
	Height      int
	AxisStart   int
	AxisEnd     int
	Baseline    int
	Empty       bool
	Period      string
	SavingsFmt  string
	SavingsTone string
	Bars        []WaterfallBar
	Connectors  []WaterfallConnector
	YTicks      []AxisTick
}

// WaterfallBar is one cumulative step or total in a waterfall chart.
type WaterfallBar struct {
	X          int
	Y          int
	Width      int
	Height     int
	LabelX     int
	LabelY     int
	Label      string
	ValueFmt   string
	BalanceFmt string
	Tone       string
}

// WaterfallConnector joins the cumulative balance between adjacent steps.
type WaterfallConnector struct {
	X1 int
	X2 int
	Y  int
}

// MonthTick is a chart x-axis label.
type MonthTick struct {
	X      int
	Y      int
	Label  string
	Detail string
}

// CategoryChart shows a category composition as a stacked bar plus rows.
type CategoryChart struct {
	Empty    bool
	Period   string
	TotalFmt string
	Segments []CategorySegment
	Rows     []CategoryRow
}

// CategorySegment is one slice of a composition bar.
type CategorySegment struct {
	Label      string
	ValueFmt   string
	PercentFmt string
	X          int
	Width      int
	Palette    int
}

// CategoryRow is one category row under a composition chart.
type CategoryRow struct {
	Label      string
	ValueFmt   string
	PercentFmt string
	URL        string
	Width      int
	Palette    int
}

// LineChart is the net-worth trend chart.
type LineChart struct {
	Width        int
	Height       int
	AxisStart    int
	AxisEnd      int
	Baseline     int
	Empty        bool
	Points       []LinePoint
	PointsAttr   string
	Labels       []MonthTick
	YTicks       []AxisTick
	LatestFmt    string
	MinFmt       string
	MaxFmt       string
	ChangeFmt    string
	ChangePctFmt string
}

// LinePoint is one plotted point in a line chart.
type LinePoint struct {
	X        int
	Y        int
	Label    string
	ValueFmt string
	Tone     string
}

// AllocationChart shows current balance-sheet exposure by class or type.
type AllocationChart struct {
	Empty    bool
	TotalFmt string
	GrossFmt string
	Rows     []AllocationRow
}

// AllocationRow is one horizontal allocation bar.
type AllocationRow struct {
	Label      string
	ValueFmt   string
	PercentFmt string
	Width      int
	Tone       string
}

// InvestmentChart summarizes investment account value and return.
type InvestmentChart struct {
	Empty          bool
	HasValue       bool
	Truncated      bool
	LatestMonth    kernel.Date
	TotalValueFmt  string
	TotalReturnFmt string
	ReturnPctFmt   string
	Rows           []InvestmentChartRow
}

// InvestmentChartRow is one investment account comparison.
type InvestmentChartRow struct {
	Account      string
	CostFmt      string
	ValueFmt     string
	ReturnFmt    string
	ReturnPctFmt string
	CostWidth    int
	ValueWidth   int
	ReturnTone   string
}

func buildView(income []reports.IncomeRow, nw []reports.NwRow, balances []reports.BalanceRow, investments []reports.InvestmentRow, txns []transactions.Transaction, period kernel.Date) View {
	latestNW, hasNW := latestNWRow(nw)
	investmentChart := buildInvestmentChart(investments)
	if period.IsZero() {
		period = dashboardPeriod()
	}
	periodIncome, hasPeriodIncome := incomeRowForPeriod(income, period)
	hasSavingsRate := hasPeriodIncome && periodIncome.Revenue != 0
	periodLabel := titleMonthYear(period)

	kpis := []KPI{
		{
			Label: "Patrimonio netto",
			Value: moneyOrEmpty(latestNW.NetWorth, hasNW),
			Help:  updatedAtHelp(latestNW.Month, hasNW),
			Tone:  moneyTone(latestNW.NetWorth),
		},
		{
			Label: "Risparmio mese",
			Value: moneyOrEmpty(periodIncome.NetIncome, hasPeriodIncome),
			Help:  periodLabel + " · entrate meno uscite",
			Tone:  moneyTone(periodIncome.NetIncome),
		},
		{
			Label: "Tasso risparmio",
			Value: pctOrEmpty(periodIncome.SavingsRate, hasSavingsRate),
			Help:  periodLabel + " · quota del reddito",
			Tone:  rateTone(periodIncome.SavingsRate),
		},
		{
			Label: "Investimenti",
			Value: valueOrEmpty(investmentChart.TotalValueFmt, investmentChart.HasValue),
			Help:  updatedAtHelp(investmentChart.LatestMonth, investmentChart.HasValue),
			Tone:  "neutral",
		},
	}

	return View{
		Period:            period.Month(),
		PreviousPeriodURL: periodURL(kernel.Date{Time: period.AddDate(0, -1, 0)}),
		NextPeriodURL:     periodURL(nextMonth(period)),
		KPIs:              kpis,
		CashFlow:          buildCashFlowChart(income),
		Waterfall:         buildWaterfallChart(txns, period),
		ExpenseBreakdown:  buildCategoryChart(txns, transactions.Expense, period),
		IncomeComposition: buildCategoryChart(txns, transactions.Income, period),
		NetWorth:          buildNetWorthChart(nw),
		Allocation:        buildAllocationChart(balances),
		Investments:       investmentChart,
	}
}

func buildWaterfallChart(txns []transactions.Transaction, period kernel.Date) WaterfallChart {
	const (
		left           = 48
		right          = 18
		top            = 18
		bottom         = 62
		labelY         = waterfallHeight - 18
		maxExpenseBars = 5
	)
	type expenseGroup struct {
		label string
		value kernel.Money
	}
	type step struct {
		label string
		value kernel.Money
		start kernel.Money
		end   kernel.Money
		tone  string
	}

	period = period.FirstOfMonth()
	revenue := kernel.Money(0)
	expenseGroups := make(map[string]kernel.Money)
	for _, txn := range txns {
		if !txn.Date.FirstOfMonth().Equal(period.Time) {
			continue
		}
		switch txn.Kind {
		case transactions.Income:
			if txn.Amount > 0 {
				revenue += txn.Amount
			} else if txn.Amount < 0 {
				label := strings.TrimSpace(txn.Category)
				if label == "" {
					label = "Rettifiche entrate"
				}
				expenseGroups[label] += absMoney(txn.Amount)
			}
		case transactions.Expense:
			label := strings.TrimSpace(txn.Category)
			if label == "" {
				label = "Senza categoria"
			}
			expenseGroups[label] += absMoney(txn.Amount)
		}
	}
	chart := WaterfallChart{
		Width:     waterfallWidth,
		Height:    waterfallHeight,
		AxisStart: left,
		AxisEnd:   waterfallWidth - right,
		Empty:     revenue == 0 && len(expenseGroups) == 0,
		Period:    titleMonthYear(period),
	}
	if chart.Empty {
		return chart
	}

	groups := make([]expenseGroup, 0, len(expenseGroups))
	for label, value := range expenseGroups {
		groups = append(groups, expenseGroup{label: label, value: value})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].value == groups[j].value {
			return groups[i].label < groups[j].label
		}
		return groups[i].value > groups[j].value
	})
	if len(groups) > maxExpenseBars {
		other := expenseGroup{label: "Altre spese"}
		for _, group := range groups[maxExpenseBars-1:] {
			other.value += group.value
		}
		groups = append(groups[:maxExpenseBars-1], other)
	}

	steps := []step{{label: "Entrate", value: revenue, start: 0, end: revenue, tone: "income"}}
	running := revenue
	for _, group := range groups {
		end := running - group.value
		steps = append(steps, step{label: group.label, value: -group.value, start: running, end: end, tone: "expense"})
		running = end
	}
	steps = append(steps, step{label: "Risparmio", value: running, start: 0, end: running, tone: moneyTone(running)})
	chart.SavingsFmt = signedMoneyFmt(running)
	chart.SavingsTone = moneyTone(running)

	domainMin, domainMax := kernel.Money(0), kernel.Money(0)
	for _, item := range steps {
		domainMin = minMoney(domainMin, minMoney(item.start, item.end))
		domainMax = maxMoney(domainMax, maxMoney(item.start, item.end))
	}
	domainRange := domainMax - domainMin
	padding := domainRange / 10
	if padding == 0 {
		padding = 1
	}
	domainMin -= padding
	domainMax += padding
	plotBottom := waterfallHeight - bottom
	plotHeight := plotBottom - top
	mapY := func(value kernel.Money) int {
		position := float64(domainMax-value) / float64(domainMax-domainMin)
		return top + roundFloat(position*float64(plotHeight))
	}
	chart.Baseline = mapY(0)
	for i := 0; i < 5; i++ {
		value := domainMax - kernel.Money(roundFloat(float64(domainMax-domainMin)*float64(i)/4))
		chart.YTicks = append(chart.YTicks, AxisTick{Y: top + roundFloat(float64(plotHeight)*float64(i)/4), ValueFmt: moneyFmt(value)})
	}

	plotWidth := waterfallWidth - left - right
	stepWidth := float64(plotWidth) / float64(len(steps))
	barWidth := 54
	if candidate := roundFloat(stepWidth * 0.58); candidate < barWidth {
		barWidth = candidate
	}
	for i, item := range steps {
		center := left + roundFloat((float64(i)+0.5)*stepWidth)
		startY, endY := mapY(item.start), mapY(item.end)
		y := min(startY, endY)
		height := absInt(endY - startY)
		if item.value != 0 && height == 0 {
			height = 1
		}
		chart.Bars = append(chart.Bars, WaterfallBar{
			X:          center - barWidth/2,
			Y:          y,
			Width:      barWidth,
			Height:     height,
			LabelX:     center,
			LabelY:     labelY,
			Label:      item.label,
			ValueFmt:   signedMoneyFmt(item.value),
			BalanceFmt: moneyFmt(item.end),
			Tone:       item.tone,
		})
		if i < len(steps)-2 {
			nextCenter := left + roundFloat((float64(i+1)+0.5)*stepWidth)
			chart.Connectors = append(chart.Connectors, WaterfallConnector{
				X1: center + barWidth/2,
				X2: nextCenter - barWidth/2,
				Y:  endY,
			})
		}
	}
	return chart
}

func buildCashFlowChart(rows []reports.IncomeRow) CashFlowChart {
	const (
		left     = 32
		right    = 18
		top      = 18
		bottom   = 46
		barGap   = 6
		labelY   = cashFlowHeight - 16
		barWidth = 16
	)
	plotBottom := cashFlowHeight - bottom
	baseline := top + (plotBottom-top)/2
	chart := CashFlowChart{
		Width:     cashFlowWidth,
		Height:    cashFlowHeight,
		AxisStart: left,
		AxisEnd:   cashFlowWidth - right,
		Baseline:  baseline,
		Empty:     len(rows) == 0,
	}
	if len(rows) == 0 {
		return chart
	}

	rows = latestIncomeRows(fillIncomeMonths(rows), 12)
	maxValue := kernel.Money(0)
	for _, row := range rows {
		maxValue = maxMoney(maxValue, absMoney(row.Revenue))
		maxValue = maxMoney(maxValue, absMoney(row.Expenses))
		maxValue = maxMoney(maxValue, absMoney(row.NetIncome))
	}
	if maxValue == 0 {
		maxValue = 1
	}
	chart.MaxFmt = moneyFmt(maxValue)
	for i := 0; i < 5; i++ {
		value := maxValue - kernel.Money(roundFloat(float64(maxValue*2)*float64(i)/4))
		y := top + roundFloat(float64(plotBottom-top)*float64(i)/4)
		chart.YTicks = append(chart.YTicks, AxisTick{Y: y, ValueFmt: moneyFmt(value)})
	}

	plotWidth := cashFlowWidth - left - right
	step := float64(plotWidth) / float64(len(rows))
	netPointsAttr := make([]string, 0, len(rows))
	for i, row := range rows {
		center := left + roundFloat((float64(i)+0.5)*step)
		chart.Months = append(chart.Months, MonthTick{
			X:      center,
			Y:      labelY,
			Label:  axisMonthLabel(row.Month, i == 0),
			Detail: row.Month.Month(),
		})
		monthLabel := monthShort(row.Month)
		incomeHeight := scaledHeight(absMoney(row.Revenue), maxValue, baseline-top)
		expenseHeight := scaledHeight(absMoney(row.Expenses), maxValue, plotBottom-baseline)
		chart.Bars = append(chart.Bars,
			CashFlowBar{
				X:        center - barWidth - barGap/2,
				Y:        baseline - incomeHeight,
				Width:    barWidth,
				Height:   incomeHeight,
				Class:    "income",
				Label:    monthLabel,
				Kind:     "Entrate",
				ValueFmt: moneyFmt(row.Revenue),
			},
			CashFlowBar{
				X:        center + barGap/2,
				Y:        baseline,
				Width:    barWidth,
				Height:   expenseHeight,
				Class:    "expense",
				Label:    monthLabel,
				Kind:     "Uscite",
				ValueFmt: moneyFmt(absMoney(row.Expenses)),
			},
		)
		netY := baseline
		if row.NetIncome > 0 {
			netY -= scaledHeight(row.NetIncome, maxValue, baseline-top)
		} else if row.NetIncome < 0 {
			netY += scaledHeight(absMoney(row.NetIncome), maxValue, plotBottom-baseline)
		}
		point := LinePoint{
			X:        center,
			Y:        netY,
			Label:    monthLabel,
			ValueFmt: signedMoneyFmt(row.NetIncome),
			Tone:     moneyTone(row.NetIncome),
		}
		chart.NetPoints = append(chart.NetPoints, point)
		netPointsAttr = append(netPointsAttr, fmt.Sprintf("%d,%d", point.X, point.Y))
	}
	chart.NetPointsAttr = strings.Join(netPointsAttr, " ")
	return chart
}

func buildCategoryChart(txns []transactions.Transaction, kind transactions.Kind, period kernel.Date) CategoryChart {
	type group struct {
		label string
		value kernel.Money
	}

	groups := make(map[string]kernel.Money)
	period = period.FirstOfMonth()
	for _, txn := range txns {
		if txn.Kind != kind {
			continue
		}
		if !txn.Date.FirstOfMonth().Equal(period.Time) {
			continue
		}
		amount := categoryAmount(txn, kind)
		if amount <= 0 {
			continue
		}
		label := strings.TrimSpace(txn.Category)
		if label == "" {
			label = "Senza categoria"
		}
		groups[label] += amount
	}

	chart := CategoryChart{
		Period:   period.Month(),
		TotalFmt: moneyFmt(0),
		Empty:    len(groups) == 0,
	}
	if len(groups) == 0 {
		return chart
	}

	grouped := make([]group, 0, len(groups))
	total := kernel.Money(0)
	for label, value := range groups {
		if value <= 0 {
			continue
		}
		grouped = append(grouped, group{label: label, value: value})
		total += value
	}
	if total <= 0 || len(grouped) == 0 {
		chart.Empty = true
		return chart
	}
	sort.Slice(grouped, func(i, j int) bool {
		if grouped[i].value == grouped[j].value {
			return grouped[i].label < grouped[j].label
		}
		return grouped[i].value > grouped[j].value
	})

	chart.Empty = false
	chart.TotalFmt = moneyFmt(total)
	periodEnd := kernel.Date{Time: period.AddDate(0, 1, -1)}
	x := 0
	for i, group := range grouped {
		pct := float64(group.value) / float64(total)
		width := clampPercent(roundFloat(pct * 100))
		if width == 0 {
			width = 1
		}
		if i == len(grouped)-1 {
			width = 100 - x
		} else if x+width > 100 {
			width = 100 - x
		}
		if width < 0 {
			width = 0
		}
		palette := i%8 + 1
		percentFmt := fmt.Sprintf("%.0f%%", pct*100)
		chart.Segments = append(chart.Segments, CategorySegment{
			Label:      group.label,
			ValueFmt:   moneyFmt(group.value),
			PercentFmt: percentFmt,
			X:          x,
			Width:      width,
			Palette:    palette,
		})
		chart.Rows = append(chart.Rows, CategoryRow{
			Label:      group.label,
			ValueFmt:   moneyFmt(group.value),
			PercentFmt: percentFmt,
			URL: "/transactions?" + url.Values{
				"category": {group.label},
				"from":     {period.ISO()},
				"to":       {periodEnd.ISO()},
			}.Encode(),
			Width:   width,
			Palette: palette,
		})
		x += width
	}
	return chart
}

func categoryAmount(txn transactions.Transaction, kind transactions.Kind) kernel.Money {
	switch kind {
	case transactions.Expense:
		return absMoney(txn.Amount)
	case transactions.Income:
		return txn.Amount
	default:
		return absMoney(txn.Amount)
	}
}

func buildNetWorthChart(rows []reports.NwRow) LineChart {
	const (
		left   = 32
		right  = 18
		top    = 18
		bottom = 44
		labelY = lineHeight - 16
	)
	baseline := lineHeight - bottom
	chart := LineChart{
		Width:     lineWidth,
		Height:    lineHeight,
		AxisStart: left,
		AxisEnd:   lineWidth - right,
		Baseline:  baseline,
		Empty:     len(rows) == 0,
	}
	if len(rows) == 0 {
		return chart
	}

	rows = latestNwRows(rows, 12)
	minValue, maxValue := rows[0].NetWorth, rows[0].NetWorth
	for _, row := range rows[1:] {
		minValue = minMoney(minValue, row.NetWorth)
		maxValue = maxMoney(maxValue, row.NetWorth)
	}
	chart.MinFmt = moneyFmt(minValue)
	chart.MaxFmt = moneyFmt(maxValue)
	chart.LatestFmt = moneyFmt(rows[len(rows)-1].NetWorth)
	change := rows[len(rows)-1].NetWorth - rows[0].NetWorth
	chart.ChangeFmt = signedMoneyFmt(change)
	if rows[0].NetWorth != 0 {
		chart.ChangePctFmt = signedPctFmt(float64(change) / float64(absMoney(rows[0].NetWorth)))
	} else {
		chart.ChangePctFmt = "—"
	}

	plotWidth := lineWidth - left - right
	plotHeight := baseline - top
	domainRange := maxValue - minValue
	padding := domainRange / 10
	if padding == 0 {
		padding = absMoney(maxValue) / 20
		if padding == 0 {
			padding = 1
		}
	}
	domainMin := minValue - padding
	domainMax := maxValue + padding
	if minValue >= 0 && domainMin < 0 {
		domainMin = 0
	}
	for i := 0; i < 4; i++ {
		value := domainMin + kernel.Money(roundFloat(float64(domainMax-domainMin)*float64(i)/3))
		y := baseline - roundFloat(float64(plotHeight)*float64(i)/3)
		chart.YTicks = append(chart.YTicks, AxisTick{Y: y, ValueFmt: moneyFmt(value)})
	}
	pointsAttr := make([]string, 0, len(rows))
	for i, row := range rows {
		x := left + plotWidth/2
		if len(rows) > 1 {
			x = left + roundFloat(float64(i)*float64(plotWidth)/float64(len(rows)-1))
		}
		position := float64(row.NetWorth-domainMin) / float64(domainMax-domainMin)
		y := baseline - roundFloat(position*float64(plotHeight))
		point := LinePoint{
			X:        x,
			Y:        y,
			Label:    monthShort(row.Month),
			ValueFmt: moneyFmt(row.NetWorth),
		}
		chart.Points = append(chart.Points, point)
		pointsAttr = append(pointsAttr, fmt.Sprintf("%d,%d", point.X, point.Y))
		chart.Labels = append(chart.Labels, MonthTick{
			X:      x,
			Y:      labelY,
			Label:  axisMonthLabel(row.Month, i == 0),
			Detail: row.Month.Month(),
		})
	}
	chart.PointsAttr = strings.Join(pointsAttr, " ")
	return chart
}

func buildAllocationChart(rows []reports.BalanceRow) AllocationChart {
	groups := make(map[string]kernel.Money)
	for _, row := range rows {
		label := strings.TrimSpace(row.Class)
		if label == "" {
			label = strings.TrimSpace(row.Type)
		}
		if label == "" {
			label = "Altro"
		}
		groups[label] += row.Balance
	}
	if len(groups) == 0 {
		return AllocationChart{Empty: true}
	}

	type group struct {
		label string
		value kernel.Money
		size  kernel.Money
	}
	grouped := make([]group, 0, len(groups))
	totalSize := kernel.Money(0)
	totalValue := kernel.Money(0)
	for label, value := range groups {
		size := absMoney(value)
		grouped = append(grouped, group{label: label, value: value, size: size})
		totalSize += size
		totalValue += value
	}
	sort.Slice(grouped, func(i, j int) bool {
		if grouped[i].size == grouped[j].size {
			return grouped[i].label < grouped[j].label
		}
		return grouped[i].size > grouped[j].size
	})

	chart := AllocationChart{TotalFmt: moneyFmt(totalValue), GrossFmt: moneyFmt(totalSize)}
	if totalSize == 0 {
		chart.Empty = true
		return chart
	}
	for _, group := range grouped {
		pct := float64(group.size) / float64(totalSize)
		chart.Rows = append(chart.Rows, AllocationRow{
			Label:      allocationLabel(group.label),
			ValueFmt:   moneyFmt(group.value),
			PercentFmt: fmt.Sprintf("%.0f%%", pct*100),
			Width:      clampPercent(roundFloat(pct * 100)),
			Tone:       moneyTone(group.value),
		})
	}
	return chart
}

func buildInvestmentChart(rows []reports.InvestmentRow) InvestmentChart {
	if len(rows) == 0 {
		return InvestmentChart{Empty: true, TotalValueFmt: "—", TotalReturnFmt: "—", ReturnPctFmt: "—"}
	}

	rows = append([]reports.InvestmentRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Value == rows[j].Value {
			return rows[i].Account < rows[j].Account
		}
		return rows[i].Value > rows[j].Value
	})

	totalCost := kernel.Money(0)
	totalValue := kernel.Money(0)
	totalReturn := kernel.Money(0)
	maxBar := kernel.Money(0)
	for _, row := range rows {
		totalCost += row.CostBasis
		totalValue += row.Value
		totalReturn += row.Return
		maxBar = maxMoney(maxBar, absMoney(row.CostBasis))
		maxBar = maxMoney(maxBar, absMoney(row.Value))
	}
	if maxBar == 0 {
		maxBar = 1
	}

	chart := InvestmentChart{
		TotalValueFmt:  moneyFmt(totalValue),
		TotalReturnFmt: moneyFmt(totalReturn),
		ReturnPctFmt:   pctFmt(returnRate(totalReturn, totalCost)),
	}
	for _, row := range rows {
		if row.LatestMonth.IsZero() {
			continue
		}
		chart.HasValue = true
		if chart.LatestMonth.IsZero() || row.LatestMonth.After(chart.LatestMonth.Time) {
			chart.LatestMonth = row.LatestMonth
		}
	}
	limit := len(rows)
	if limit > 5 {
		limit = 5
		chart.Truncated = true
	}
	for _, row := range rows[:limit] {
		chart.Rows = append(chart.Rows, InvestmentChartRow{
			Account:      row.Account,
			CostFmt:      moneyFmt(row.CostBasis),
			ValueFmt:     moneyFmt(row.Value),
			ReturnFmt:    moneyFmt(row.Return),
			ReturnPctFmt: pctFmt(row.ReturnPct),
			CostWidth:    clampPercent(roundFloat(float64(absMoney(row.CostBasis)) / float64(maxBar) * 100)),
			ValueWidth:   clampPercent(roundFloat(float64(absMoney(row.Value)) / float64(maxBar) * 100)),
			ReturnTone:   moneyTone(row.Return),
		})
	}
	return chart
}

func latestIncomeRow(rows []reports.IncomeRow) (reports.IncomeRow, bool) {
	rows = latestIncomeRows(rows, 1)
	if len(rows) == 0 {
		return reports.IncomeRow{}, false
	}
	return rows[0], true
}

func incomeRowForPeriod(rows []reports.IncomeRow, period kernel.Date) (reports.IncomeRow, bool) {
	period = period.FirstOfMonth()
	for _, row := range rows {
		if row.Month.FirstOfMonth().Equal(period.Time) {
			return row, true
		}
	}
	return reports.IncomeRow{}, false
}

func latestNWRow(rows []reports.NwRow) (reports.NwRow, bool) {
	rows = latestNwRows(rows, 1)
	if len(rows) == 0 {
		return reports.NwRow{}, false
	}
	return rows[0], true
}

func dashboardPeriod() kernel.Date {
	return kernel.Today().FirstOfMonth()
}

func nextMonth(d kernel.Date) kernel.Date {
	if d.IsZero() {
		d = kernel.Today()
	}
	from := d.FirstOfMonth()
	return kernel.Date{Time: from.Time.AddDate(0, 1, 0)}
}

func latestIncomeRows(rows []reports.IncomeRow, limit int) []reports.IncomeRow {
	rows = append([]reports.IncomeRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Month.Before(rows[j].Month.Time) })
	return latestRows(rows, limit)
}

func fillIncomeMonths(rows []reports.IncomeRow) []reports.IncomeRow {
	if len(rows) < 2 {
		return rows
	}
	rows = append([]reports.IncomeRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Month.Before(rows[j].Month.Time) })
	byMonth := make(map[string]reports.IncomeRow, len(rows))
	for _, row := range rows {
		byMonth[row.Month.Month()] = row
	}

	first := rows[0].Month.FirstOfMonth()
	last := rows[len(rows)-1].Month.FirstOfMonth()
	continuous := make([]reports.IncomeRow, 0, len(rows))
	for month := first; !month.After(last.Time); month = nextMonth(month) {
		row, ok := byMonth[month.Month()]
		if !ok {
			row.Month = month
		}
		continuous = append(continuous, row)
	}
	return continuous
}

func latestNwRows(rows []reports.NwRow, limit int) []reports.NwRow {
	rows = append([]reports.NwRow(nil), rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Month.Before(rows[j].Month.Time) })
	return latestRows(rows, limit)
}

func latestRows[T any](rows []T, limit int) []T {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[len(rows)-limit:]
}

func scaledHeight(value, maxValue kernel.Money, maxHeight int) int {
	if value <= 0 || maxValue <= 0 {
		return 0
	}
	return roundFloat(float64(value) / float64(maxValue) * float64(maxHeight))
}

func moneyFmt(m kernel.Money) string {
	return m.String() + " €"
}

func moneyOrEmpty(m kernel.Money, ok bool) string {
	if !ok {
		return "—"
	}
	return moneyFmt(m)
}

func pctFmt(v float64) string {
	return strings.Replace(fmt.Sprintf("%.1f%%", v*100), ".", ",", 1)
}

func pctOrEmpty(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return pctFmt(v)
}

func signedMoneyFmt(value kernel.Money) string {
	if value > 0 {
		return "+" + moneyFmt(value)
	}
	return moneyFmt(value)
}

func signedPctFmt(value float64) string {
	if value > 0 {
		return "+" + pctFmt(value)
	}
	return pctFmt(value)
}

func valueOrEmpty(value string, ok bool) string {
	if !ok {
		return "—"
	}
	return value
}

func updatedAtHelp(month kernel.Date, ok bool) string {
	if !ok || month.IsZero() {
		return "Nessun dato disponibile"
	}
	return "Aggiornato a " + monthYear(month)
}

func titleMonthYear(month kernel.Date) string {
	value := monthYear(month)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func monthYear(month kernel.Date) string {
	if month.IsZero() {
		return ""
	}
	months := [...]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
	return fmt.Sprintf("%s %d", months[month.Time.Month()-1], month.Time.Year())
}

func axisMonthLabel(month kernel.Date, includeYear bool) string {
	label := monthShort(month)
	if includeYear || month.Time.Month() == 1 {
		return fmt.Sprintf("%s %02d", label, month.Time.Year()%100)
	}
	return label
}

func allocationLabel(value string) string {
	if label := map[string]string{
		"Cash":       "Liquidità",
		"Investment": "Investimenti",
		"Property":   "Immobili",
		"Tax":        "Imposte",
		"Credit":     "Credito",
		"Other":      "Altro",
		"Asset":      "Attività",
		"Liability":  "Passività",
	}[value]; label != "" {
		return label
	}
	return value
}

func returnRate(ret, cost kernel.Money) float64 {
	if cost == 0 {
		return 0
	}
	return float64(ret) / float64(cost)
}

func moneyTone(m kernel.Money) string {
	switch {
	case m > 0:
		return "positive"
	case m < 0:
		return "negative"
	default:
		return "neutral"
	}
}

func rateTone(v float64) string {
	switch {
	case v > 0:
		return "positive"
	case v < 0:
		return "negative"
	default:
		return "neutral"
	}
}

func absMoney(m kernel.Money) kernel.Money {
	if m < 0 {
		return -m
	}
	return m
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minMoney(a, b kernel.Money) kernel.Money {
	if a < b {
		return a
	}
	return b
}

func maxMoney(a, b kernel.Money) kernel.Money {
	if a > b {
		return a
	}
	return b
}

func clampPercent(v int) int {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	default:
		return v
	}
}

func roundFloat(v float64) int {
	return int(math.Round(v))
}

func monthShort(d kernel.Date) string {
	months := []string{"Gen", "Feb", "Mar", "Apr", "Mag", "Giu", "Lug", "Ago", "Set", "Ott", "Nov", "Dic"}
	if d.IsZero() {
		return ""
	}
	return months[int(d.Time.Month())-1]
}
