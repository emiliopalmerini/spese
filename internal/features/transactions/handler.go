package transactions

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/features/accounts"
	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler owns the HTTP endpoints for income + expense entry and listing.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
	Render Renderer
}

// Mount registers the GET list, GET form, and POST create endpoints.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.list)
	mux.HandleFunc("POST "+prefix, h.create)
}

// ListView is the payload for the list page.
type ListView struct {
	Transactions []Transaction
	Accounts     []accounts.Account
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	txns, err := List(r.Context(), h.Client, Filter{Last: 100}, false)
	if err != nil {
		h.Logger.Error("list transactions", "err", err)
		http.Error(w, "failed to load transactions", http.StatusBadGateway)
		return
	}
	accs, err := accounts.List(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "failed to load accounts", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "transactions/list", ListView{Transactions: txns, Accounts: accs}); err != nil {
		h.Logger.Error("render transactions list", "err", err)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	t, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := Append(r.Context(), h.Client, []Transaction{t}); err != nil {
		h.Logger.Error("append transaction", "err", err)
		http.Error(w, "failed to write transaction", http.StatusBadGateway)
		return
	}
	// Refresh and re-render so HTMX can swap.
	txns, _ := List(r.Context(), h.Client, Filter{Last: 100}, true)
	accs, _ := accounts.List(r.Context(), h.Client, false)
	_ = h.Render.Render(w, "transactions/list", ListView{Transactions: txns, Accounts: accs})
}

// parseForm builds a single-row Transaction from form values. Sign is set
// based on Kind so the caller only enters the positive amount.
func parseForm(r *http.Request) (Transaction, error) {
	kind := Kind(strings.TrimSpace(r.FormValue("kind")))
	if kind != Income && kind != Expense {
		return Transaction{}, errors.New("kind must be Income or Expense")
	}
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		return Transaction{}, errors.New("account is required")
	}
	dateStr := strings.TrimSpace(r.FormValue("date"))
	if dateStr == "" {
		return Transaction{}, errors.New("date is required")
	}
	d, err := kernel.ParseDate(dateStr)
	if err != nil {
		return Transaction{}, err
	}
	amt, err := kernel.ParseMoney(r.FormValue("amount"))
	if err != nil {
		return Transaction{}, err
	}
	if amt < 0 {
		amt = -amt
	}
	if kind == Expense {
		amt = -amt
	}
	payee := strings.TrimSpace(r.FormValue("payee"))
	if payee == "" {
		return Transaction{}, errors.New("payee/description is required")
	}
	return Transaction{
		Date:        d,
		Kind:        kind,
		Account:     account,
		Amount:      amt,
		Category:    strings.TrimSpace(r.FormValue("category")),
		Subcategory: strings.TrimSpace(r.FormValue("subcategory")),
		Payee:       payee,
		Note:        strings.TrimSpace(r.FormValue("note")),
	}, nil
}
