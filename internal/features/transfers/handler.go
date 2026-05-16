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

	"spese/internal/features/accounts"
	"spese/internal/features/transactions"
	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler renders the transfer form and handles submissions.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
	Render Renderer
}

// Mount wires GET (form) and POST (create).
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.form)
	mux.HandleFunc("POST "+prefix, h.create)
}

// FormView is the payload for the transfer form page.
type FormView struct {
	Accounts []accounts.Account
	Today    kernel.Date
}

func (h *Handler) form(w http.ResponseWriter, r *http.Request) {
	accs, err := accounts.List(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "failed to load accounts", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "transfers/form", FormView{Accounts: accs, Today: kernel.Today()}); err != nil {
		h.Logger.Error("render transfer form", "err", err)
	}
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
	if err := transactions.Append(r.Context(), h.Client, legs); err != nil {
		h.Logger.Error("append transfer", "err", err)
		http.Error(w, "failed to record transfer", http.StatusBadGateway)
		return
	}
	// Refresh the form (clears inputs, updates account list if changed).
	accs, _ := accounts.List(r.Context(), h.Client, false)
	_ = h.Render.Render(w, "transfers/form", FormView{Accounts: accs, Today: kernel.Today()})
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
