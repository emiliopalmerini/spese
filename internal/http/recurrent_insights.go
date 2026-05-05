package http

import (
	"sort"
	"time"

	"spese/internal/core"
)

type FrequencyCellVM struct {
	Key        string
	Label      string
	Count      int
	MonthlyFmt string
}

type TopRowVM struct {
	ID         int64
	Name       string
	FreqLabel  string
	MonthlyFmt string
	WidthPct   int
}

type UpcomingRowVM struct {
	ID        int64
	DateShort string
	DayPadded string
	Name      string
	AmountFmt string
}

type SecondaryRowVM struct {
	Name       string
	MonthlyFmt string
	WidthPct   int
}

type CategoryGroupVM struct {
	Name       string
	MonthlyFmt string
	WidthPct   int
	Children   []SecondaryRowVM
}

type RecurrentInsightsVM struct {
	HasData         bool
	MonthlyTotalFmt string
	AnnualTotalFmt  string
	DailyAvgFmt     string
	ActiveCount     int
	Frequencies     []FrequencyCellVM
	Top5            []TopRowVM
	Upcoming        []UpcomingRowVM
	Categories      []CategoryGroupVM
}

var frequencyOrder = []core.RepetitionTypes{core.Daily, core.Weekly, core.Monthly, core.Yearly}

var frequencyLabel = map[core.RepetitionTypes]string{
	core.Daily:   "Giornaliera",
	core.Weekly:  "Settimanale",
	core.Monthly: "Mensile",
	core.Yearly:  "Annuale",
}

func monthlyEquivalentCents(re core.RecurrentExpenses) int64 {
	switch re.Every {
	case core.Daily:
		return re.Amount.Cents * 30
	case core.Weekly:
		return re.Amount.Cents * 4
	case core.Monthly:
		return re.Amount.Cents
	case core.Yearly:
		return re.Amount.Cents / 12
	}
	return 0
}

func lastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// nextOccurrence returns the next due date ≥ today honoring EndDate.
// Returns ok=false when the recurrence has ended or has no future date in
// the supported window.
func nextOccurrence(re core.RecurrentExpenses, today time.Time) (time.Time, bool) {
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	if !re.EndDate.IsZero() {
		end := time.Date(re.EndDate.Year(), re.EndDate.Time.Month(), re.EndDate.Day(), 0, 0, 0, 0, time.UTC)
		if end.Before(today) {
			return time.Time{}, false
		}
	}

	start := time.Date(re.StartDate.Year(), re.StartDate.Time.Month(), re.StartDate.Day(), 0, 0, 0, 0, time.UTC)
	anchor := start
	if start.Before(today) {
		anchor = today
	}

	var next time.Time
	switch re.Every {
	case core.Daily:
		next = anchor
	case core.Weekly:
		delta := (int(start.Weekday()) - int(anchor.Weekday()) + 7) % 7
		next = anchor.AddDate(0, 0, delta)
	case core.Monthly:
		year, month := anchor.Year(), anchor.Month()
		day := start.Day()
		capped := day
		if d := lastDayOfMonth(year, month); capped > d {
			capped = d
		}
		candidate := time.Date(year, month, capped, 0, 0, 0, 0, time.UTC)
		if candidate.Before(anchor) {
			month++
			if month > 12 {
				month = 1
				year++
			}
			capped = day
			if d := lastDayOfMonth(year, month); capped > d {
				capped = d
			}
			candidate = time.Date(year, month, capped, 0, 0, 0, 0, time.UTC)
		}
		next = candidate
	case core.Yearly:
		year := anchor.Year()
		month := start.Month()
		day := start.Day()
		capped := day
		if d := lastDayOfMonth(year, month); capped > d {
			capped = d
		}
		candidate := time.Date(year, month, capped, 0, 0, 0, 0, time.UTC)
		if candidate.Before(anchor) {
			year++
			capped = day
			if d := lastDayOfMonth(year, month); capped > d {
				capped = d
			}
			candidate = time.Date(year, month, capped, 0, 0, 0, 0, time.UTC)
		}
		next = candidate
	default:
		return time.Time{}, false
	}

	if !re.EndDate.IsZero() {
		end := time.Date(re.EndDate.Year(), re.EndDate.Time.Month(), re.EndDate.Day(), 0, 0, 0, 0, time.UTC)
		if next.After(end) {
			return time.Time{}, false
		}
	}

	return next, true
}

func formatDayPadded(d int) string {
	if d < 10 {
		return "0" + string(rune('0'+d))
	}
	s := []rune{}
	x := d
	for x > 0 {
		s = append([]rune{rune('0' + x%10)}, s...)
		x /= 10
	}
	return string(s)
}

func formatItalianShort(t time.Time) string {
	return formatDayPadded(t.Day()) + " " + italianMonthShort(int(t.Month()))
}

// buildInsights produces the view-model for the /recurrent insights tile.
func buildInsights(expenses []core.RecurrentExpenses, now time.Time) RecurrentInsightsVM {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	active := make([]core.RecurrentExpenses, 0, len(expenses))
	for _, re := range expenses {
		if !re.EndDate.IsZero() {
			end := time.Date(re.EndDate.Year(), re.EndDate.Time.Month(), re.EndDate.Day(), 0, 0, 0, 0, time.UTC)
			if end.Before(today) {
				continue
			}
		}
		active = append(active, re)
	}

	vm := RecurrentInsightsVM{
		ActiveCount: len(active),
		Frequencies: make([]FrequencyCellVM, 0, 4),
	}

	if len(active) == 0 {
		vm.MonthlyTotalFmt = formatEuros(0)
		vm.AnnualTotalFmt = formatEuros(0)
		vm.DailyAvgFmt = formatEuros(0)
		for _, k := range frequencyOrder {
			vm.Frequencies = append(vm.Frequencies, FrequencyCellVM{
				Key: string(k), Label: frequencyLabel[k], Count: 0, MonthlyFmt: "—",
			})
		}
		return vm
	}

	vm.HasData = true

	// Aggregates.
	var totalMonthly int64
	freqCount := map[core.RepetitionTypes]int{}
	freqMonthly := map[core.RepetitionTypes]int64{}
	primaryMonthly := map[string]int64{}
	type subKey struct{ p, s string }
	subMonthly := map[subKey]int64{}
	primaryToSubs := map[string]map[string]struct{}{}

	type rankRow struct {
		re      core.RecurrentExpenses
		monthly int64
	}
	rows := make([]rankRow, 0, len(active))

	for _, re := range active {
		m := monthlyEquivalentCents(re)
		totalMonthly += m
		freqCount[re.Every]++
		freqMonthly[re.Every] += m
		primaryMonthly[re.Primary] += m
		subMonthly[subKey{re.Primary, re.Secondary}] += m
		if _, ok := primaryToSubs[re.Primary]; !ok {
			primaryToSubs[re.Primary] = map[string]struct{}{}
		}
		primaryToSubs[re.Primary][re.Secondary] = struct{}{}
		rows = append(rows, rankRow{re: re, monthly: m})
	}

	annual := totalMonthly * 12
	dailyAvg := annual / 365

	vm.MonthlyTotalFmt = formatEuros(totalMonthly)
	vm.AnnualTotalFmt = formatEuros(annual)
	vm.DailyAvgFmt = formatEuros(dailyAvg)

	for _, k := range frequencyOrder {
		amt := "—"
		if c := freqCount[k]; c > 0 {
			amt = formatEuros(freqMonthly[k])
		}
		vm.Frequencies = append(vm.Frequencies, FrequencyCellVM{
			Key:        string(k),
			Label:      frequencyLabel[k],
			Count:      freqCount[k],
			MonthlyFmt: amt,
		})
	}

	// Top 5 by monthly impact.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].monthly > rows[j].monthly })
	topMonthly := rows[0].monthly
	limit := len(rows)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		r := rows[i]
		w := 0
		if topMonthly > 0 {
			w = int((r.monthly * 100) / topMonthly)
		}
		vm.Top5 = append(vm.Top5, TopRowVM{
			ID:         r.re.ID,
			Name:       r.re.Description,
			FreqLabel:  frequencyLabel[r.re.Every],
			MonthlyFmt: formatEuros(r.monthly),
			WidthPct:   w,
		})
	}

	// Upcoming next 30 days.
	type upRow struct {
		when time.Time
		re   core.RecurrentExpenses
	}
	var ups []upRow
	cutoff := today.AddDate(0, 0, 30)
	for _, re := range active {
		when, ok := nextOccurrence(re, today)
		if !ok {
			continue
		}
		if when.After(cutoff) {
			continue
		}
		ups = append(ups, upRow{when: when, re: re})
	}
	sort.SliceStable(ups, func(i, j int) bool { return ups[i].when.Before(ups[j].when) })
	for _, u := range ups {
		vm.Upcoming = append(vm.Upcoming, UpcomingRowVM{
			ID:        u.re.ID,
			DateShort: formatItalianShort(u.when),
			DayPadded: formatDayPadded(u.when.Day()),
			Name:      u.re.Description,
			AmountFmt: formatEuros(u.re.Amount.Cents),
		})
	}

	// Nested categories.
	type primaryRow struct {
		name    string
		monthly int64
	}
	prims := make([]primaryRow, 0, len(primaryMonthly))
	for k, v := range primaryMonthly {
		prims = append(prims, primaryRow{name: k, monthly: v})
	}
	sort.SliceStable(prims, func(i, j int) bool { return prims[i].monthly > prims[j].monthly })
	topCat := int64(0)
	if len(prims) > 0 {
		topCat = prims[0].monthly
	}
	for _, p := range prims {
		w := 0
		if topCat > 0 {
			w = int((p.monthly * 100) / topCat)
		}
		grp := CategoryGroupVM{Name: p.name, MonthlyFmt: formatEuros(p.monthly), WidthPct: w}
		// Children.
		subs := primaryToSubs[p.name]
		type subRow struct {
			name    string
			monthly int64
		}
		ss := make([]subRow, 0, len(subs))
		for s := range subs {
			ss = append(ss, subRow{name: s, monthly: subMonthly[subKey{p.name, s}]})
		}
		sort.SliceStable(ss, func(i, j int) bool { return ss[i].monthly > ss[j].monthly })
		for _, s := range ss {
			sw := 0
			if p.monthly > 0 {
				sw = int((s.monthly * 100) / p.monthly)
			}
			grp.Children = append(grp.Children, SecondaryRowVM{
				Name:       s.name,
				MonthlyFmt: formatEuros(s.monthly),
				WidthPct:   sw,
			})
		}
		vm.Categories = append(vm.Categories, grp)
	}

	return vm
}
