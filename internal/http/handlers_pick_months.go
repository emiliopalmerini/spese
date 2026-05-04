package http

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"spese/internal/adapters"
)

// handleDashboardPickMonths renders the Pick Months pivot partial.
func (s *Server) handleDashboardPickMonths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	year := time.Now().Year()
	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 2000 || y > 2999 {
			http.Error(w, "invalid year", http.StatusBadRequest)
			return
		}
		year = y
	}

	a, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		_, _ = w.Write([]byte(`<div class="placeholder">Pick Months non disponibile</div>`))
		return
	}

	rows, primaries, err := a.MonthlyExpensesByPrimary(r.Context(), year)
	if err != nil {
		slog.ErrorContext(r.Context(), "monthly by primary", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore caricamento Pick Months</div>`))
		return
	}

	hasData := false
	for _, r := range rows {
		if r.Total > 0 {
			hasData = true
			break
		}
	}

	if len(primaries) == 0 {
		primaries = []string{"Senza categoria"}
	}
	sort.Strings(primaries)

	type cell struct {
		Amount string
		IsZero bool
	}
	type viewRow struct {
		Month  int
		Label  string
		Cells  []cell
		Total  string
		IsZero bool
	}
	monthLabels := []string{"Gen", "Feb", "Mar", "Apr", "Mag", "Giu", "Lug", "Ago", "Set", "Ott", "Nov", "Dic"}

	view := struct {
		Year       int
		HasData    bool
		Primaries  []string
		Rows       []viewRow
		ColTotals  []string
		GrandTotal string
	}{Year: year, HasData: hasData, Primaries: primaries}

	colTotals := make([]int64, len(primaries))
	var grand int64
	for i, row := range rows {
		v := viewRow{Month: i + 1, Label: monthLabels[i]}
		for j, p := range primaries {
			amt := row.ByPrimary[p]
			v.Cells = append(v.Cells, cell{Amount: formatEuros(amt), IsZero: amt == 0})
			colTotals[j] += amt
		}
		v.Total = formatEuros(row.Total)
		v.IsZero = row.Total == 0
		view.Rows = append(view.Rows, v)
		grand += row.Total
	}
	for _, c := range colTotals {
		view.ColTotals = append(view.ColTotals, formatEuros(c))
	}
	view.GrandTotal = formatEuros(grand)

	if err := s.templates.ExecuteTemplate(w, "pick_months", view); err != nil {
		slog.ErrorContext(r.Context(), "render pick_months", "error", err)
		_, _ = w.Write([]byte(`<div class="error">Errore template</div>`))
	}
}
