package http

import (
	"fmt"
	"sort"

	"spese/internal/core"
)

type NetworthTypeCellVM struct {
	Key       string
	Label     string
	Count     int
	AmountFmt string
}

type NetworthGroupVM struct {
	Type     core.AccountType
	Title    string
	TotalFmt string
	WidthPct int
	Accounts []netWorthAccountRow
}

type NetworthInsightsVM struct {
	HasData         bool
	Year            int
	Month           int
	MonthlyTotalFmt string
	ActiveCount     int
	DeltaPct        int
	DeltaSign       string
	DeltaIsZero     bool
	DeltaAbsFmt     string
	PrevMonthShort  string

	Types  []NetworthTypeCellVM
	Trend  []IncomeTrendCellVM
	Groups []NetworthGroupVM
}

var networthTypeOrder = []core.AccountType{
	core.AccountCash,
	core.AccountRainyDay,
	core.AccountLongTerm,
}

// buildNetworthInsights produces the view-model for /ui/networth/insights.
//
//   - currYear, currMonth: month being displayed.
//   - accounts: full list (active and inactive).
//   - currBalances, prevBalances: balance per account ID for current and
//     previous month (only accounts present in the map contribute).
//   - trendCents: 12 cells, index 0 = 11 months ago, index 11 = current.
//   - trendStartYear, trendStartMonth: (year, month) for trendCents[0].
func buildNetworthInsights(
	currYear, currMonth int,
	accounts []core.Account,
	currBalances, prevBalances map[int64]core.AccountBalance,
	trendCents [12]int64,
	trendStartYear, trendStartMonth int,
) NetworthInsightsVM {
	vm := NetworthInsightsVM{Year: currYear, Month: currMonth}

	typeCount := map[core.AccountType]int{}
	typeAmount := map[core.AccountType]int64{}
	typeAccountCount := map[core.AccountType]int{}
	var monthlyTotal int64
	activeCount := 0
	for _, a := range accounts {
		typeAccountCount[a.Type]++
		if b, ok := currBalances[a.ID]; ok {
			typeCount[a.Type]++
			typeAmount[a.Type] += b.Amount.Cents
			monthlyTotal += b.Amount.Cents
			activeCount++
		}
	}
	vm.MonthlyTotalFmt = formatEuros(monthlyTotal)
	vm.ActiveCount = activeCount

	for _, t := range networthTypeOrder {
		cell := NetworthTypeCellVM{
			Key:   string(t),
			Label: netWorthSectionTitles[t],
			Count: typeCount[t],
		}
		if typeCount[t] > 0 {
			cell.AmountFmt = formatEuros(typeAmount[t])
		} else {
			cell.AmountFmt = "—"
		}
		vm.Types = append(vm.Types, cell)
	}

	if monthlyTotal > 0 || activeCount > 0 {
		vm.HasData = true
	}

	// Delta vs previous month.
	var prevTotal int64
	for _, b := range prevBalances {
		prevTotal += b.Amount.Cents
	}
	_, pM := addMonths(currYear, currMonth, -1)
	vm.PrevMonthShort = italianMonthShort(pM)

	if prevTotal == 0 {
		vm.DeltaIsZero = true
	} else {
		diff := monthlyTotal - prevTotal
		pct := int((diff * 100) / prevTotal)
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
		absDiff := diff
		if absDiff < 0 {
			absDiff = -absDiff
		}
		vm.DeltaAbsFmt = formatEuros(absDiff)
	}

	// Groups (one per type that has any accounts).
	if vm.HasData {
		var maxTypeAmt int64
		for _, t := range networthTypeOrder {
			if typeAmount[t] > maxTypeAmt {
				maxTypeAmt = typeAmount[t]
			}
		}
		for _, t := range networthTypeOrder {
			if typeAccountCount[t] == 0 {
				continue
			}
			grp := NetworthGroupVM{
				Type:     t,
				Title:    netWorthSectionTitles[t],
				TotalFmt: formatEuros(typeAmount[t]),
			}
			if maxTypeAmt > 0 {
				grp.WidthPct = int((typeAmount[t] * 100) / maxTypeAmt)
			}
			for _, a := range accounts {
				if a.Type != t {
					continue
				}
				row := netWorthAccountRow{
					ID:         a.ID,
					Name:       a.Name,
					Type:       a.Type,
					Active:     a.Active,
					IsInactive: !a.Active,
				}
				if b, ok := currBalances[a.ID]; ok {
					row.Amount = formatEuros(b.Amount.Cents)
					row.AmountCents = b.Amount.Cents
					row.HasBalance = true
					row.BalanceUpdate = fmt.Sprintf("%d-%02d", b.Year, b.Month)
				} else {
					row.Amount = "—"
				}
				grp.Accounts = append(grp.Accounts, row)
			}
			sort.SliceStable(grp.Accounts, func(i, j int) bool {
				if grp.Accounts[i].IsInactive != grp.Accounts[j].IsInactive {
					return !grp.Accounts[i].IsInactive
				}
				return grp.Accounts[i].AmountCents > grp.Accounts[j].AmountCents
			})
			vm.Groups = append(vm.Groups, grp)
		}
	}

	// 12-month trend.
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

	return vm
}
