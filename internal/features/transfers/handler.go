// Package transfers is the inter-account transfer slice. It writes TWO
// transactions per submitted form: a negative leg on the source account and
// a positive leg on the destination. Sum is zero, so net worth does not
// change — only the breakdown by account does.
package transfers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/features/transactions"
	"spese/internal/kernel"
	"spese/internal/storage"
)

// Handler records transfers and redirects stale page requests to movements.
type Handler struct {
	Store  *storage.Store
	Logger *slog.Logger
}

// Mount wires the compatibility GET redirect and POST create endpoint.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.redirectToTransactions)
	mux.HandleFunc("POST "+prefix, h.create)
}

func (h *Handler) redirectToTransactions(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	legs, err := parseTransfer(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := transactions.Append(r.Context(), h.Store, legs); err != nil {
		h.Logger.Error("append transfer", "err", err)
		http.Error(w, "failed to record transfer", http.StatusBadGateway)
		return
	}
	redirectAfterCreate(w, r)
}

func redirectAfterCreate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/transactions")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// parseTransfer builds the two Transfer legs from a single submitted form.
func parseTransfer(r *http.Request) ([]transactions.Transaction, error) {
	src := strings.TrimSpace(r.FormValue("source"))
	dst := strings.TrimSpace(r.FormValue("destination"))
	if src == "" || dst == "" {
		return nil, errors.New("source and destination are required")
	}
	if src == dst {
		return nil, errors.New("source and destination must differ")
	}
	dateStr := strings.TrimSpace(r.FormValue("date"))
	if dateStr == "" {
		return nil, errors.New("date is required")
	}
	d, err := kernel.ParseDate(dateStr)
	if err != nil {
		return nil, err
	}
	amt, err := kernel.ParseMoney(r.FormValue("amount"))
	if err != nil {
		return nil, err
	}
	if amt < 0 {
		amt = -amt
	}
	if amt == 0 {
		return nil, errors.New("amount must be positive")
	}
	note := strings.TrimSpace(r.FormValue("note"))

	out := []transactions.Transaction{
		{
			Date:     d,
			Kind:     transactions.Transfer,
			Account:  src,
			Amount:   -amt,
			Category: "Transfer",
			Payee:    "to " + dst,
			Note:     note,
		},
		{
			Date:     d,
			Kind:     transactions.Transfer,
			Account:  dst,
			Amount:   amt,
			Category: "Transfer",
			Payee:    "from " + src,
			Note:     note,
		},
	}
	return out, nil
}
