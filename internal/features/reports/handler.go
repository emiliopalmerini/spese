package reports

import (
	"log/slog"
	"net/http"
	"strings"

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
	rows, err := IncomeStatement(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("income statement", "err", err)
		http.Error(w, "Impossibile caricare il conto economico.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "reports/income_statement", buildIncomeStatementView(rows)); err != nil {
		h.Logger.Error("render income statement", "err", err)
	}
}

func (h *Handler) nwTimeline(w http.ResponseWriter, r *http.Request) {
	rows, err := NwMonthly(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("nw monthly", "err", err)
		http.Error(w, "Impossibile caricare l'andamento del patrimonio.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "reports/nw_timeline", buildNwTimelineView(rows)); err != nil {
		h.Logger.Error("render net worth timeline", "err", err)
	}
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
