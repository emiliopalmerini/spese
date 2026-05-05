package http

import (
	"sort"
	"strings"

	"spese/internal/core"
	"spese/internal/storage"
)

type IncomeSourceVM struct {
	Key       string
	Label     string
	Count     int
	AmountFmt string
}

type IncomeTopRowVM struct {
	Name      string
	AmountFmt string
	WidthPct  int
}

type IncomeTrendCellVM struct {
	MonthShort string // "Mag"
	Year       int
	Cents      int64
	HeightPct  int  // 0..100 relative to max in window
	IsCurrent  bool // highlight current month
}

type IncomeItemVM struct {
	ID       string
	Day      int
	DateMono string // "07/05"
	Desc     string
	Cat      string
	AmtFmt   string
}

type IncomeInsightsVM struct {
	HasData         bool
	Year            int
	Month           int
	MonthlyTotalFmt string
	YTDTotalFmt     string
	ActiveCount     int
	DeltaPct        int
	DeltaSign       string
	DeltaIsZero     bool

	Sources []IncomeSourceVM
	Top5    []IncomeTopRowVM
	Trend   []IncomeTrendCellVM
	Items   []IncomeItemVM
}

const (
	incomeSourceStipendio = "stipendio"
	incomeSourceFreelance = "freelance"
	incomeSourceAltro     = "altro"
)

func classifyIncomeSource(category string, freelance map[string]bool) string {
	if freelance[category] {
		return incomeSourceFreelance
	}
	if strings.Contains(strings.ToLower(category), "stipendio") {
		return incomeSourceStipendio
	}
	return incomeSourceAltro
}

// buildIncomeInsights builds the /entrate insights view-model.
//
//   - curr: current-month overview (year, month, total, by category).
//   - prev: previous-month overview for the delta.
//   - trendCents: 12 cells, index 0 = 11 months ago, index 11 = current month.
//   - trendStartYear/trendStartMonth: year/month for trendCents[0].
//   - ytdCents: cumulative income from January up to and including current month.
//   - items: current-month items.
//   - freelance: set of category names treated as freelance.
func buildIncomeInsights(
	curr core.IncomeMonthOverview,
	prev core.IncomeMonthOverview,
	trendCents [12]int64,
	trendStartYear, trendStartMonth int,
	ytdCents int64,
	items []storage.IncomeWithID,
	freelance map[string]bool,
) IncomeInsightsVM {
	vm := IncomeInsightsVM{
		Year:            curr.Year,
		Month:           curr.Month,
		MonthlyTotalFmt: formatEuros(curr.Total.Cents),
		YTDTotalFmt:     formatEuros(ytdCents),
		ActiveCount:     len(items),
	}

	// Delta vs previous month.
	if prev.Total.Cents == 0 {
		vm.DeltaIsZero = true
	} else {
		diff := curr.Total.Cents - prev.Total.Cents
		pct := int((diff * 100) / prev.Total.Cents)
		if pct < 0 {
			vm.DeltaSign = "−"
			vm.DeltaPct = -pct
		} else if pct > 0 {
			vm.DeltaSign = "+"
			vm.DeltaPct = pct
		} else {
			vm.DeltaIsZero = true
		}
	}

	if curr.Total.Cents > 0 || len(items) > 0 {
		vm.HasData = true
	}

	// Source split.
	srcCount := map[string]int{}
	srcAmount := map[string]int64{}
	for _, it := range items {
		k := classifyIncomeSource(it.Income.Category, freelance)
		srcCount[k]++
		srcAmount[k] += it.Income.Amount.Cents
	}
	for _, k := range []string{incomeSourceStipendio, incomeSourceFreelance, incomeSourceAltro} {
		label := map[string]string{
			incomeSourceStipendio: "Stipendio",
			incomeSourceFreelance: "Freelance",
			incomeSourceAltro:     "Altro",
		}[k]
		amt := "—"
		if srcCount[k] > 0 {
			amt = formatEuros(srcAmount[k])
		}
		vm.Sources = append(vm.Sources, IncomeSourceVM{
			Key:       k,
			Label:     label,
			Count:     srcCount[k],
			AmountFmt: amt,
		})
	}

	// Top 5 categories by amount.
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
		vm.Top5 = append(vm.Top5, IncomeTopRowVM{
			Name:      rows[i].name,
			AmountFmt: formatEuros(rows[i].amount),
			WidthPct:  w,
		})
	}

	// Trend window.
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
		month++
		if month > 12 {
			month = 1
			year++
		}
	}

	// Items.
	for _, it := range items {
		day := it.Income.Date.Day()
		mm := int(it.Income.Date.Time.Month())
		vm.Items = append(vm.Items, IncomeItemVM{
			ID:       it.ID,
			Day:      day,
			DateMono: padTwo(day) + "/" + padTwo(mm),
			Desc:     it.Income.Description,
			Cat:      it.Income.Category,
			AmtFmt:   formatEuros(it.Income.Amount.Cents),
		})
	}

	return vm
}

func padTwo(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return formatDayPadded(n)
}

// addMonths returns (year, month) shifted by `delta` months from base.
func addMonths(year, month, delta int) (int, int) {
	month += delta
	for month <= 0 {
		month += 12
		year--
	}
	for month > 12 {
		month -= 12
		year++
	}
	return year, month
}

// trendStart returns the (year, month) for index 0 of a 12-cell trend window
// ending at (currYear, currMonth).
func trendStart(currYear, currMonth int) (int, int) {
	return addMonths(currYear, currMonth, -11)
}
