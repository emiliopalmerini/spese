package reports

import (
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/sheets"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders read-only reports backed by the v_* tabs.
type Handler struct {
	Client *sheets.Client
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

// BalanceSheetView is the payload for the balance-sheet report.
type BalanceSheetView struct{ Rows []BalanceRow }

func (h *Handler) balanceSheet(w http.ResponseWriter, r *http.Request) {
	rows, err := BalanceSheet(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("balance sheet", "err", err)
		http.Error(w, "failed to load balance sheet", http.StatusBadGateway)
		return
	}
	_ = h.Render.Render(w, "reports/balance_sheet", BalanceSheetView{Rows: rows})
}

// IncomeStatementView is the payload for the income-statement report.
type IncomeStatementView struct{ Rows []IncomeRow }

func (h *Handler) incomeStatement(w http.ResponseWriter, r *http.Request) {
	rows, err := IncomeStatement(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("income statement", "err", err)
		http.Error(w, "failed to load income statement", http.StatusBadGateway)
		return
	}
	_ = h.Render.Render(w, "reports/income_statement", IncomeStatementView{Rows: rows})
}

// NwTimelineView is the payload for the NW timeline report.
type NwTimelineView struct{ Rows []NwRow }

func (h *Handler) nwTimeline(w http.ResponseWriter, r *http.Request) {
	rows, err := NwMonthly(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("nw monthly", "err", err)
		http.Error(w, "failed to load NW timeline", http.StatusBadGateway)
		return
	}
	_ = h.Render.Render(w, "reports/nw_timeline", NwTimelineView{Rows: rows})
}

// InvestmentsView is the payload for the investments report.
type InvestmentsView struct{ Rows []InvestmentRow }

func (h *Handler) investments(w http.ResponseWriter, r *http.Request) {
	rows, err := Investments(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("investments", "err", err)
		http.Error(w, "failed to load investments", http.StatusBadGateway)
		return
	}
	_ = h.Render.Render(w, "reports/investments", InvestmentsView{Rows: rows})
}
