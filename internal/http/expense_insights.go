package http

import (
	"sort"

	"spese/internal/core"
	"spese/internal/sheets"
)

type ExpenseTopRowVM struct {
	Name      string
	AmountFmt string
	WidthPct  int
}

type ExpenseSecondaryRowVM struct {
	Name      string
	AmountFmt string
	WidthPct  int
}

type ExpenseCategoryGroupVM struct {
	Name      string
	AmountFmt string
	WidthPct  int
	Children  []ExpenseSecondaryRowVM
}

type ExpenseItemVM struct {
	ID       string
	DateMono string
	Desc     string
	Cat      string
	Sub      string
	AmtFmt   string
}

type ExpenseInsightsVM struct {
	HasData         bool
	Year, Month     int
	MonthlyTotalFmt string
	ActiveCount     int
	DeltaPct        int
	DeltaSign       string
	DeltaIsZero     bool

	Top5       []ExpenseTopRowVM
	Trend      []IncomeTrendCellVM
	Categories []ExpenseCategoryGroupVM
	Items      []ExpenseItemVM
}

// buildExpenseInsights builds the /spese insights view-model.
//
//   - curr: current-month overview (year, month, total, by primary category).
//   - prev: previous-month overview for the delta.
//   - trendCents: 12 cells, index 0 = 11 months ago, index 11 = current month.
//   - trendStartYear/trendStartMonth: year/month for trendCents[0].
//   - items: current-month items with IDs.
func buildExpenseInsights(
	curr core.MonthOverview,
	prev core.MonthOverview,
	trendCents [12]int64,
	trendStartYear, trendStartMonth int,
	items []sheets.ExpenseWithID,
) ExpenseInsightsVM {
	vm := ExpenseInsightsVM{
		Year:            curr.Year,
		Month:           curr.Month,
		MonthlyTotalFmt: formatEuros(curr.Total.Cents),
		ActiveCount:     len(items),
	}

	if prev.Total.Cents == 0 {
		vm.DeltaIsZero = true
	} else {
		diff := curr.Total.Cents - prev.Total.Cents
		pct := int((diff * 100) / prev.Total.Cents)
		switch {
		case pct < 0:
			vm.DeltaSign = "−"
			vm.DeltaPct = -pct
		case pct > 0:
			vm.DeltaSign = "+"
			vm.DeltaPct = pct
		default:
			vm.DeltaIsZero = true
		}
	}

	if curr.Total.Cents > 0 || len(items) > 0 {
		vm.HasData = true
	}

	// Top 5 by primary category (use ByCategory from overview).
	type catRow struct {
		name   string
		amount int64
	}
	rows := make([]catRow, 0, len(curr.ByCategory))
	for _, c := range curr.ByCategory {
		rows = append(rows, catRow{name: c.Name, amount: c.Amount.Cents})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].amount > rows[j].amount })
	limit := len(rows)
	if limit > 5 {
		limit = 5
	}
	var topMax int64
	if limit > 0 {
		topMax = rows[0].amount
	}
	for i := 0; i < limit; i++ {
		w := 0
		if topMax > 0 {
			w = int((rows[i].amount * 100) / topMax)
		}
		vm.Top5 = append(vm.Top5, ExpenseTopRowVM{
			Name:      rows[i].name,
			AmountFmt: formatEuros(rows[i].amount),
			WidthPct:  w,
		})
	}

	// Trend window — reuse IncomeTrendCellVM for visual parity.
	var trendMax int64
	for _, c := range trendCents {
		if c > trendMax {
			trendMax = c
		}
	}
	year, month := trendStartYear, trendStartMonth
	for i := 0; i < 12; i++ {
		h := 0
		if trendMax > 0 {
			h = int((trendCents[i] * 100) / trendMax)
		}
		vm.Trend = append(vm.Trend, IncomeTrendCellVM{
			MonthShort: italianMonthShort(month),
			Year:       year,
			Cents:      trendCents[i],
			HeightPct:  h,
			IsCurrent:  i == 11,
		})
		year, month = addMonths(year, month, 1)
	}

	// Nested primary→secondary from items.
	type secKey struct{ p, s string }
	primTotal := map[string]int64{}
	secTotal := map[secKey]int64{}
	primSecs := map[string]map[string]struct{}{}
	for _, e := range items {
		primTotal[e.Expense.Primary] += e.Expense.Amount.Cents
		secTotal[secKey{e.Expense.Primary, e.Expense.Secondary}] += e.Expense.Amount.Cents
		if _, ok := primSecs[e.Expense.Primary]; !ok {
			primSecs[e.Expense.Primary] = map[string]struct{}{}
		}
		primSecs[e.Expense.Primary][e.Expense.Secondary] = struct{}{}
	}
	type primaryRow struct {
		name   string
		amount int64
	}
	prims := make([]primaryRow, 0, len(primTotal))
	for n, a := range primTotal {
		prims = append(prims, primaryRow{name: n, amount: a})
	}
	sort.SliceStable(prims, func(i, j int) bool { return prims[i].amount > prims[j].amount })
	var topPrim int64
	if len(prims) > 0 {
		topPrim = prims[0].amount
	}
	for _, p := range prims {
		w := 0
		if topPrim > 0 {
			w = int((p.amount * 100) / topPrim)
		}
		grp := ExpenseCategoryGroupVM{
			Name:      p.name,
			AmountFmt: formatEuros(p.amount),
			WidthPct:  w,
		}
		type subRow struct {
			name   string
			amount int64
		}
		subs := primSecs[p.name]
		ss := make([]subRow, 0, len(subs))
		for s := range subs {
			ss = append(ss, subRow{name: s, amount: secTotal[secKey{p.name, s}]})
		}
		sort.SliceStable(ss, func(i, j int) bool { return ss[i].amount > ss[j].amount })
		for _, s := range ss {
			sw := 0
			if p.amount > 0 {
				sw = int((s.amount * 100) / p.amount)
			}
			grp.Children = append(grp.Children, ExpenseSecondaryRowVM{
				Name:      s.name,
				AmountFmt: formatEuros(s.amount),
				WidthPct:  sw,
			})
		}
		vm.Categories = append(vm.Categories, grp)
	}

	// Items.
	for _, e := range items {
		day := e.Expense.Date.Day()
		mm := int(e.Expense.Date.Time.Month())
		vm.Items = append(vm.Items, ExpenseItemVM{
			ID:       e.ID,
			DateMono: padTwo(day) + "/" + padTwo(mm),
			Desc:     e.Expense.Description,
			Cat:      e.Expense.Primary,
			Sub:      e.Expense.Secondary,
			AmtFmt:   formatEuros(e.Expense.Amount.Cents),
		})
	}

	return vm
}
