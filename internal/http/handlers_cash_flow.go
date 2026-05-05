package http

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spese/internal/adapters"
	"spese/internal/services"
)

// handleDashboardCashFlow renders the cash flow detail panel for a year.
func (s *Server) handleDashboardCashFlow(w http.ResponseWriter, r *http.Request) {
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
		_, _ = w.Write([]byte(`<div class="placeholder">Flusso di Cassa non disponibile</div>`))
		return
	}

	svc := services.NewCashFlowService(a.GetStorage())
	rows, err := svc.BuildYear(r.Context(), year)
	if err != nil {
		slog.ErrorContext(r.Context(), "cash flow build", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore caricamento Flusso di Cassa</div>`))
		return
	}

	monthLabels := []string{"Gen", "Feb", "Mar", "Apr", "Mag", "Giu", "Lug", "Ago", "Set", "Ott", "Nov", "Dic"}

	type viewCell struct {
		Amount     string
		IsNegative bool
		IsZero     bool
	}
	type viewRow struct {
		Label string
		Group string
		Cells []viewCell
		Total string
		IsNeg bool
	}
	type sectionView struct {
		Name string
		Rows []viewRow
	}

	sections := []sectionView{}
	colTotals := [12]int64{}
	var grand int64
	bySection := map[string]int{}

	for _, r := range rows {
		idx, ok := bySection[r.Section]
		if !ok {
			sections = append(sections, sectionView{Name: r.Section})
			idx = len(sections) - 1
			bySection[r.Section] = idx
		}
		row := viewRow{Label: r.Label, Group: r.Group, IsNeg: r.Total < 0}
		for i, c := range r.Months {
			row.Cells = append(row.Cells, viewCell{
				Amount:     formatEuros(c),
				IsNegative: c < 0,
				IsZero:     c == 0,
			})
			colTotals[i] += c
		}
		row.Total = formatEuros(r.Total)
		sections[idx].Rows = append(sections[idx].Rows, row)
		grand += r.Total
	}

	hasData := len(rows) > 0

	colTotalStrs := make([]string, 12)
	for i, c := range colTotals {
		colTotalStrs[i] = formatEuros(c)
	}

	view := struct {
		Year       int
		HasData    bool
		Months     []string
		Sections   []sectionView
		ColTotals  []string
		GrandTotal string
	}{
		Year:       year,
		HasData:    hasData,
		Months:     monthLabels,
		Sections:   sections,
		ColTotals:  colTotalStrs,
		GrandTotal: formatEuros(grand),
	}

	if err := s.templates.ExecuteTemplate(w, "cash_flow_panel", view); err != nil {
		slog.ErrorContext(r.Context(), "render cash flow", "error", err)
		_, _ = w.Write([]byte(`<div class="error">Errore template</div>`))
	}
}
