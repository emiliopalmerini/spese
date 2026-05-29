// Package dashboard renders the home page from report views and recent
// transactions.
package dashboard

import (
	"log/slog"
	"net/http"

	"spese/internal/features/reports"
	"spese/internal/features/transactions"
	"spese/internal/sheets"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders the home page.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
	Render Renderer
}

// Mount registers the GET handler at prefix (use "/" for the home page).
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix, h.home)
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	income, err := reports.IncomeStatement(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("income statement", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}
	netWorth, err := reports.NwMonthly(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("nw monthly", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}
	balances, err := reports.BalanceSheet(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("balance sheet", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}
	investments, err := reports.Investments(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("investments", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}
	period := dashboardPeriod(income)
	txns, err := transactions.List(r.Context(), h.Client, transactions.Filter{
		From: period,
		To:   nextMonth(period),
	}, false)
	if err != nil {
		h.Logger.Error("transactions", "err", err)
		http.Error(w, "failed to load dashboard", http.StatusBadGateway)
		return
	}

	_ = h.Render.Render(w, "dashboard/home", buildView(income, netWorth, balances, investments, txns, period))
}
