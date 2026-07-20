package transactions

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
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
	Filters      ListFilterView
	Total        int
	First        int
	Last         int
	HasPrevious  bool
	HasNext      bool
	PreviousURL  string
	NextURL      string
}

// ListFilterView preserves filter values in the GET form.
type ListFilterView struct {
	Query    string
	From     string
	To       string
	Kind     string
	Account  string
	Category string
}

const transactionListPageSize = 50

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, form, page, err := parseListQuery(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	txns, err := List(r.Context(), h.Store, filter, false)
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
	rows := BuildListViewRows(txns, 0)
	total := len(rows)
	pageCount := (total + transactionListPageSize - 1) / transactionListPageSize
	if pageCount == 0 {
		page = 1
	} else if page > pageCount {
		page = pageCount
	}
	start := (page - 1) * transactionListPageSize
	end := start + transactionListPageSize
	if end > total {
		end = total
	}
	visible := rows[start:end]
	view := ListView{
		Transactions: visible,
		Accounts:     accs,
		Filters:      form,
		Total:        total,
		HasPrevious:  page > 1,
		HasNext:      end < total,
		PreviousURL:  paginationURL(r.URL.Query(), page-1),
		NextURL:      paginationURL(r.URL.Query(), page+1),
	}
	if total > 0 {
		view.First = start + 1
		view.Last = end
	}
	if err := h.Render.Render(w, "transactions/list", view); err != nil {
		h.Logger.Error("render transactions list", "err", err)
	}
}

func parseListQuery(values url.Values) (Filter, ListFilterView, int, error) {
	form := ListFilterView{
		Query:    strings.TrimSpace(values.Get("q")),
		From:     strings.TrimSpace(values.Get("from")),
		To:       strings.TrimSpace(values.Get("to")),
		Kind:     strings.TrimSpace(values.Get("kind")),
		Account:  strings.TrimSpace(values.Get("account")),
		Category: strings.TrimSpace(values.Get("category")),
	}
	filter := Filter{Query: form.Query, Account: form.Account, Category: form.Category}
	if form.From != "" {
		date, err := kernel.ParseDate(form.From)
		if err != nil {
			return Filter{}, form, 0, errors.New("La data iniziale non è valida.")
		}
		filter.From = date
	}
	if form.To != "" {
		date, err := kernel.ParseDate(form.To)
		if err != nil {
			return Filter{}, form, 0, errors.New("La data finale non è valida.")
		}
		filter.To = kernel.Date{Time: date.AddDate(0, 0, 1)}
	}
	if form.Kind != "" {
		filter.Kind = Kind(form.Kind)
		if filter.Kind != Income && filter.Kind != Expense && filter.Kind != Transfer && filter.Kind != Adjustment {
			return Filter{}, form, 0, errors.New("Il tipo di movimento non è valido.")
		}
	}
	page := 1
	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return Filter{}, form, 0, errors.New("La pagina richiesta non è valida.")
		}
		page = parsed
	}
	return filter, form, page, nil
}

func paginationURL(values url.Values, page int) string {
	copyValues := url.Values{}
	for key, entries := range values {
		copyValues[key] = append([]string(nil), entries...)
	}
	if page <= 1 {
		copyValues.Del("page")
	} else {
		copyValues.Set("page", strconv.Itoa(page))
	}
	query := copyValues.Encode()
	if query == "" {
		return "/transactions"
	}
	return fmt.Sprintf("/transactions?%s", query)
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
