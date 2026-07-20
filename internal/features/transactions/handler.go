package transactions

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/features/accounts"
	"spese/internal/kernel"
	"spese/internal/storage"
)

// Renderer is the minimal template interface this slice needs.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Handler owns the HTTP endpoints for income + expense entry and listing.
type Handler struct {
	Store  *storage.Store
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
	txns, err := List(r.Context(), h.Store, Filter{}, false)
	if err != nil {
		h.Logger.Error("list transactions", "err", err)
		http.Error(w, "Impossibile caricare i movimenti.", http.StatusBadGateway)
		return
	}
	accs, err := accounts.List(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "Impossibile caricare i conti.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "transactions/list", ListView{Transactions: BuildListViewRows(txns, transactionListLimit), Accounts: accs}); err != nil {
		h.Logger.Error("render transactions list", "err", err)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Il modulo inviato non è valido.", http.StatusBadRequest)
		return
	}
	t, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := Append(r.Context(), h.Store, []Transaction{t}); err != nil {
		h.Logger.Error("append transaction", "err", err)
		http.Error(w, "Impossibile salvare il movimento. Riprova.", http.StatusBadGateway)
		return
	}
	w.Header().Set("X-Spese-Success", "Movimento salvato.")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/transactions")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// parseForm builds a single-row Transaction from form values. Sign is set
// based on Kind so the caller only enters the positive amount.
func parseForm(r *http.Request) (Transaction, error) {
	kind := Kind(strings.TrimSpace(r.FormValue("kind")))
	if kind != Income && kind != Expense {
		return Transaction{}, errors.New("Seleziona un tipo di movimento valido.")
	}
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		return Transaction{}, errors.New("Seleziona un conto.")
	}
	dateStr := strings.TrimSpace(r.FormValue("date"))
	if dateStr == "" {
		return Transaction{}, errors.New("La data è obbligatoria.")
	}
	d, err := kernel.ParseDate(dateStr)
	if err != nil {
		return Transaction{}, errors.New("Inserisci una data valida.")
	}
	amt, err := kernel.ParseMoney(r.FormValue("amount"))
	if err != nil {
		return Transaction{}, errors.New("Inserisci un importo valido.")
	}
	if amt < 0 {
		amt = -amt
	}
	if amt == 0 {
		return Transaction{}, errors.New("L'importo deve essere maggiore di zero.")
	}
	if kind == Expense {
		amt = -amt
	}
	payee := strings.TrimSpace(r.FormValue("payee"))
	if payee == "" {
		return Transaction{}, errors.New("La descrizione è obbligatoria.")
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
