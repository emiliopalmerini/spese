package reports

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders read-only reports backed by local SQLite queries.
type Handler struct {
	Store  *storage.Store
	Logger *slog.Logger
	Render Renderer
}

// Mount registers GET endpoints for each report under the given prefix.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.index)
	mux.HandleFunc("GET "+prefix+"/balance-sheet", h.balanceSheet)
	mux.HandleFunc("GET "+prefix+"/income-statement", h.incomeStatement)
	mux.HandleFunc("GET "+prefix+"/nw-timeline", h.nwTimeline)
	mux.HandleFunc("GET "+prefix+"/investments", h.investments)
}

// IndexView is the menu of available reports.
type IndexView struct{}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	_ = h.Render.Render(w, "reports/index", IndexView{})
}

func (h *Handler) balanceSheet(w http.ResponseWriter, r *http.Request) {
	rows, err := BalanceSheet(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("balance sheet", "err", err)
		http.Error(w, "Impossibile caricare lo stato patrimoniale.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "reports/balance_sheet", buildBalanceSheetView(rows)); err != nil {
		h.Logger.Error("render balance sheet", "err", err)
	}
}

func (h *Handler) incomeStatement(w http.ResponseWriter, r *http.Request) {
	period, form, err := parseReportPeriod(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows, err := IncomeStatement(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("income statement", "err", err)
		http.Error(w, "Impossibile caricare il conto economico.", http.StatusBadGateway)
		return
	}
	months := make([]kernel.Date, 0, len(rows))
	for _, row := range rows {
		months = append(months, row.Month)
	}
	period, form = resolveReportPeriod(period, form, months)
	view := buildIncomeStatementView(filterIncomeRows(rows, period))
	view.Period = form
	if err := h.Render.Render(w, "reports/income_statement", view); err != nil {
		h.Logger.Error("render income statement", "err", err)
	}
}

func (h *Handler) nwTimeline(w http.ResponseWriter, r *http.Request) {
	period, form, err := parseReportPeriod(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows, err := NwMonthly(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("nw monthly", "err", err)
		http.Error(w, "Impossibile caricare l'andamento del patrimonio.", http.StatusBadGateway)
		return
	}
	months := make([]kernel.Date, 0, len(rows))
	for _, row := range rows {
		months = append(months, row.Month)
	}
	period, form = resolveReportPeriod(period, form, months)
	view := buildNwTimelineView(filterNwRows(rows, period))
	view.Period = form
	if err := h.Render.Render(w, "reports/nw_timeline", view); err != nil {
		h.Logger.Error("render net worth timeline", "err", err)
	}
}

// PeriodFilterView preserves month bounds in historical report forms.
type PeriodFilterView struct {
	From string
	To   string
	Min  string
	Max  string
}

func resolveReportPeriod(period reportPeriod, form PeriodFilterView, months []kernel.Date) (reportPeriod, PeriodFilterView) {
	var first, last kernel.Date
	for _, month := range months {
		if month.IsZero() {
			continue
		}
		month = month.FirstOfMonth()
		if first.IsZero() || month.Before(first.Time) {
			first = month
		}
		if last.IsZero() || month.After(last.Time) {
			last = month
		}
	}
	if first.IsZero() {
		last = kernel.Today().FirstOfMonth()
		first = kernel.Date{Time: last.AddDate(0, -11, 0)}
	}
	form.Min = first.Month()
	form.Max = last.Month()
	if form.From != "" || form.To != "" {
		return period, form
	}

	suggestedFrom := kernel.Date{Time: last.AddDate(0, -11, 0)}
	if first.After(suggestedFrom.Time) {
		suggestedFrom = first
	}
	period.From = suggestedFrom
	period.To = kernel.Date{Time: last.AddDate(0, 1, 0)}
	form.From = suggestedFrom.Month()
	form.To = last.Month()
	return period, form
}

type reportPeriod struct {
	From kernel.Date
	To   kernel.Date
}

func parseReportPeriod(values url.Values) (reportPeriod, PeriodFilterView, error) {
	form := PeriodFilterView{
		From: strings.TrimSpace(values.Get("from")),
		To:   strings.TrimSpace(values.Get("to")),
	}
	period := reportPeriod{}
	if form.From != "" {
		if len(form.From) != 7 {
			return period, form, errors.New("Il mese iniziale non è valido.")
		}
		date, err := kernel.ParseDate(form.From)
		if err != nil {
			return period, form, errors.New("Il mese iniziale non è valido.")
		}
		period.From = date.FirstOfMonth()
	}
	if form.To != "" {
		if len(form.To) != 7 {
			return period, form, errors.New("Il mese finale non è valido.")
		}
		date, err := kernel.ParseDate(form.To)
		if err != nil {
			return period, form, errors.New("Il mese finale non è valido.")
		}
		period.To = kernel.Date{Time: date.FirstOfMonth().AddDate(0, 1, 0)}
	}
	if !period.From.IsZero() && !period.To.IsZero() && !period.From.Before(period.To.Time) {
		return reportPeriod{}, form, errors.New("Il mese iniziale deve precedere quello finale.")
	}
	return period, form, nil
}

func filterIncomeRows(rows []IncomeRow, period reportPeriod) []IncomeRow {
	filtered := make([]IncomeRow, 0, len(rows))
	for _, row := range rows {
		if reportMonthIncluded(row.Month, period) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterNwRows(rows []NwRow, period reportPeriod) []NwRow {
	filtered := make([]NwRow, 0, len(rows))
	for _, row := range rows {
		if reportMonthIncluded(row.Month, period) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func reportMonthIncluded(month kernel.Date, period reportPeriod) bool {
	return (period.From.IsZero() || !month.Before(period.From.Time)) &&
		(period.To.IsZero() || month.Before(period.To.Time))
}

func (h *Handler) investments(w http.ResponseWriter, r *http.Request) {
	rows, err := Investments(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("investments", "err", err)
		http.Error(w, "Impossibile caricare gli investimenti.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "reports/investments", buildInvestmentsView(rows)); err != nil {
		h.Logger.Error("render investments", "err", err)
	}
}
