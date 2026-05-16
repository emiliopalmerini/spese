package recurring

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
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

// Handler is the CRUD-light HTTP surface for the recurring tab. Append-only;
// to disable a recurring row, set active=FALSE directly in the sheet.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
	Render Renderer
}

// Mount registers GET (list+form) and POST (create).
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.list)
	mux.HandleFunc("POST "+prefix, h.create)
}

// ListView is the template payload.
type ListView struct {
	Recurrings []Recurring
	Accounts   []accounts.Account
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	recs, err := List(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("list recurring", "err", err)
		http.Error(w, "failed to load recurring", http.StatusBadGateway)
		return
	}
	accs, err := accounts.List(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "failed to load accounts", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "recurring/list", ListView{Recurrings: recs, Accounts: accs}); err != nil {
		h.Logger.Error("render recurring list", "err", err)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	rec, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := Append(r.Context(), h.Client, rec); err != nil {
		h.Logger.Error("append recurring", "err", err)
		http.Error(w, "failed to write recurring", http.StatusBadGateway)
		return
	}
	recs, _ := List(r.Context(), h.Client, true)
	accs, _ := accounts.List(r.Context(), h.Client, false)
	_ = h.Render.Render(w, "recurring/list", ListView{Recurrings: recs, Accounts: accs})
}

func parseForm(r *http.Request) (Recurring, error) {
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		return Recurring{}, errors.New("label is required")
	}
	kind := transactions.Kind(strings.TrimSpace(r.FormValue("kind")))
	if kind != transactions.Income && kind != transactions.Expense {
		return Recurring{}, errors.New("kind must be Income or Expense")
	}
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		return Recurring{}, errors.New("account is required")
	}
	amt, err := kernel.ParseMoney(r.FormValue("amount"))
	if err != nil {
		return Recurring{}, err
	}
	if amt < 0 {
		amt = -amt
	}
	day, err := strconv.Atoi(strings.TrimSpace(r.FormValue("day_of_month")))
	if err != nil || day < 1 || day > 31 {
		return Recurring{}, errors.New("day_of_month must be 1-31")
	}
	return Recurring{
		Label:       label,
		Kind:        kind,
		Account:     account,
		Amount:      amt,
		Category:    strings.TrimSpace(r.FormValue("category")),
		Subcategory: strings.TrimSpace(r.FormValue("subcategory")),
		Payee:       strings.TrimSpace(r.FormValue("payee")),
		DayOfMonth:  day,
		Active:      r.FormValue("active") == "on" || r.FormValue("active") == "true",
		Note:        strings.TrimSpace(r.FormValue("note")),
	}, nil
}
