package accounts

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Handler bundles the HTTP endpoints for this slice. It carries only the
// local store and a logger; everything else is computed per-request.
type Handler struct {
	Store  *storage.Store
	Logger *slog.Logger
	Render Renderer
}

// Renderer is the minimal template interface this slice needs. Each slice
// declares its own so templates can be wired centrally without coupling
// features to a specific template engine.
type Renderer interface {
	Render(w http.ResponseWriter, name string, data any) error
}

// Mount registers handlers on a mux under the given prefix (e.g. "/accounts").
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.list)
	mux.HandleFunc("POST "+prefix, h.create)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := ListWithLatest(r.Context(), h.Store, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "Impossibile caricare i conti.", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "accounts/list", ListView{Rows: rows}); err != nil {
		h.Logger.Error("render accounts list", "err", err)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Il modulo inviato non è valido.", http.StatusBadRequest)
		return
	}
	a, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := Append(r.Context(), h.Store, a); err != nil {
		if errors.Is(err, ErrAccountNameExists) {
			http.Error(w, "Esiste già un conto con questo nome.", http.StatusUnprocessableEntity)
			return
		}
		h.Logger.Error("append account", "err", err)
		http.Error(w, "Impossibile salvare il conto. Riprova.", http.StatusBadGateway)
		return
	}
	w.Header().Set("X-Spese-Success", "Conto salvato.")
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/accounts")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// parseForm turns submitted form values into a validated Account.
func parseForm(r *http.Request) (Account, error) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return Account{}, errors.New("Il nome del conto è obbligatorio.")
	}
	t := Type(strings.TrimSpace(r.FormValue("type")))
	if t != Asset && t != Liability {
		return Account{}, errors.New("Seleziona un tipo di conto valido.")
	}
	c := Class(strings.TrimSpace(r.FormValue("class")))
	switch c {
	case Cash, Investment, Property, Tax, Credit, Other:
	case "":
		return Account{}, errors.New("Seleziona una classe di conto.")
	default:
		return Account{}, errors.New("Seleziona una classe di conto valida.")
	}
	currency := strings.ToUpper(defaultStr(r.FormValue("currency"), "EUR"))
	if currency != "EUR" {
		return Account{}, errors.New("Al momento sono supportati solo conti in EUR.")
	}
	a := Account{
		Name:     name,
		Type:     t,
		Class:    c,
		Currency: currency,
		Note:     strings.TrimSpace(r.FormValue("note")),
	}
	if v := strings.TrimSpace(r.FormValue("active_from")); v != "" {
		d, err := kernel.ParseDate(v)
		if err != nil {
			return Account{}, errors.New("Inserisci una data di attivazione valida.")
		}
		a.ActiveFrom = d
	}
	if v := strings.TrimSpace(r.FormValue("active_to")); v != "" {
		d, err := kernel.ParseDate(v)
		if err != nil {
			return Account{}, errors.New("Inserisci una data di disattivazione valida.")
		}
		a.ActiveTo = d
	}
	if !a.ActiveFrom.IsZero() && !a.ActiveTo.IsZero() && a.ActiveFrom.After(a.ActiveTo.Time) {
		return Account{}, errors.New("La data di attivazione non può essere successiva alla data di disattivazione.")
	}
	return a, nil
}

func defaultStr(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

// ListView is the template payload for the list page.
type ListView struct{ Rows []AccountRow }
