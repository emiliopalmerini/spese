package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
)

type ShortcutCardVM struct {
	Href        string
	Label       string
	AmountFmt   string
	Count       int
	DeltaPct    int
	DeltaSign   string
	DeltaIsZero bool
	HasData     bool
}

type DashboardShortcutsVM struct {
	Spese      ShortcutCardVM
	Entrate    ShortcutCardVM
	Ricorrenti ShortcutCardVM
}

func buildDashboardShortcuts(
	expCurr, expPrev core.MonthOverview, expCount int,
	incCurr, incPrev core.IncomeMonthOverview, incCount int,
	recurrents []core.RecurrentExpenses, now time.Time,
) DashboardShortcutsVM {
	vm := DashboardShortcutsVM{
		Spese:      ShortcutCardVM{Href: "/spese", Label: "Spese", AmountFmt: formatEuros(expCurr.Total.Cents), Count: expCount},
		Entrate:    ShortcutCardVM{Href: "/entrate", Label: "Entrate", AmountFmt: formatEuros(incCurr.Total.Cents), Count: incCount},
		Ricorrenti: ShortcutCardVM{Href: "/recurrent", Label: "Ricorrenti", DeltaIsZero: true},
	}

	vm.Spese.HasData = expCurr.Total.Cents > 0 || expCount > 0
	vm.Entrate.HasData = incCurr.Total.Cents > 0 || incCount > 0

	applyDelta(&vm.Spese, expCurr.Total.Cents, expPrev.Total.Cents)
	applyDelta(&vm.Entrate, incCurr.Total.Cents, incPrev.Total.Cents)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var monthly int64
	active := 0
	for _, re := range recurrents {
		if !re.EndDate.IsZero() {
			end := time.Date(re.EndDate.Year(), re.EndDate.Time.Month(), re.EndDate.Day(), 0, 0, 0, 0, time.UTC)
			if end.Before(today) {
				continue
			}
		}
		monthly += monthlyEquivalentCents(re)
		active++
	}
	vm.Ricorrenti.AmountFmt = formatEuros(monthly)
	vm.Ricorrenti.Count = active
	vm.Ricorrenti.HasData = active > 0

	return vm
}

func applyDelta(card *ShortcutCardVM, curr, prev int64) {
	if prev == 0 {
		card.DeltaIsZero = true
		return
	}
	diff := curr - prev
	pct := int((diff * 100) / prev)
	switch {
	case pct < 0:
		card.DeltaSign = "−"
		card.DeltaPct = -pct
	case pct > 0:
		card.DeltaSign = "+"
		card.DeltaPct = pct
	default:
		card.DeltaIsZero = true
	}
}

func (s *Server) handleDashboardShortcuts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	now := time.Now()
	year, month := now.Year(), int(now.Month())
	prevYear, prevMonth := addMonths(year, month, -1)

	var (
		expCurr, expPrev core.MonthOverview
		expCount         int
		incCurr, incPrev core.IncomeMonthOverview
		incCount         int
		recurrents       []core.RecurrentExpenses
	)

	if s.dashReader != nil {
		if ov, err := s.dashReader.ReadMonthOverview(ctx, year, month); err == nil {
			expCurr = ov
		} else {
			slog.WarnContext(ctx, "shortcuts expCurr", "error", err)
		}
		if ov, err := s.dashReader.ReadMonthOverview(ctx, prevYear, prevMonth); err == nil {
			expPrev = ov
		}
	}
	if s.expListerWithID != nil {
		if items, err := s.expListerWithID.ListExpensesWithID(ctx, year, month); err == nil {
			expCount = len(items)
		}
	}

	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		if ov, err := adapter.ReadIncomeMonthOverview(ctx, year, month); err == nil {
			incCurr = ov
		}
		if ov, err := adapter.ReadIncomeMonthOverview(ctx, prevYear, prevMonth); err == nil {
			incPrev = ov
		}
		if items, err := adapter.ListIncomesWithID(ctx, year, month); err == nil {
			incCount = len(items)
		}
		if repo := adapter.GetStorage(); repo != nil {
			if recs, err := repo.GetRecurrentExpenses(ctx); err == nil {
				recurrents = recs
			}
		}
	}

	vm := buildDashboardShortcuts(expCurr, expPrev, expCount, incCurr, incPrev, incCount, recurrents, now)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "dashboard_shortcuts", vm); err != nil {
		slog.ErrorContext(ctx, "Dashboard shortcuts template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
