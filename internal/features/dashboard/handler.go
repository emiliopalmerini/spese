// Package dashboard renders the home page from report views and recent
// transactions.
package dashboard

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/features/reports"
	"spese/internal/features/transactions"
	"spese/internal/kernel"
	"spese/internal/storage"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders the home page.
type Handler struct {
	Store  *storage.Store
	Logger *slog.Logger
	Render Renderer
}

// Mount registers the GET handler at prefix (use "/" for the home page).
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix, h.home)
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	period, err := parseDashboardPeriod(r.URL.Query().Get("month"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	income, err := reports.IncomeStatement(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("income statement", "err", err)
		http.Error(w, "Impossibile caricare la dashboard.", http.StatusBadGateway)
		return
	}
	netWorth, err := reports.NwMonthly(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("nw monthly", "err", err)
		http.Error(w, "Impossibile caricare la dashboard.", http.StatusBadGateway)
		return
	}
	balances, err := reports.BalanceSheet(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("balance sheet", "err", err)
		http.Error(w, "Impossibile caricare la dashboard.", http.StatusBadGateway)
		return
	}
	investments, err := reports.Investments(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("investments", "err", err)
		http.Error(w, "Impossibile caricare la dashboard.", http.StatusBadGateway)
		return
	}
	txns, err := transactions.List(r.Context(), h.Store, transactions.Filter{
		From: period,
		To:   nextMonth(period),
	}, false)
	if err != nil {
		h.Logger.Error("transactions", "err", err)
		http.Error(w, "Impossibile caricare la dashboard.", http.StatusBadGateway)
		return
	}

	if err := h.Render.Render(w, "dashboard/home", buildView(income, netWorth, balances, investments, txns, period)); err != nil {
		h.Logger.Error("render dashboard", "err", err)
	}
}

func parseDashboardPeriod(raw string) (kernel.Date, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return dashboardPeriod(), nil
	}
	if len(raw) != 7 {
		return kernel.Date{}, errors.New("Il mese richiesto non è valido.")
	}
	period, err := kernel.ParseDate(raw)
	if err != nil {
		return kernel.Date{}, errors.New("Il mese richiesto non è valido.")
	}
	return period.FirstOfMonth(), nil
}

func periodURL(period kernel.Date) string {
	return fmt.Sprintf("/?month=%s", period.Month())
}
