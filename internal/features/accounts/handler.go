package accounts

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Handler bundles the HTTP endpoints for this slice. It carries only the
// sheet client and a logger; everything else is computed per-request.
type Handler struct {
	Client *sheets.Client
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
	accs, err := List(r.Context(), h.Client, false)
	if err != nil {
		h.Logger.Error("list accounts", "err", err)
		http.Error(w, "failed to load accounts", http.StatusBadGateway)
		return
	}
	if err := h.Render.Render(w, "accounts/list", ListView{Accounts: accs}); err != nil {
		h.Logger.Error("render accounts list", "err", err)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	a, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := Append(r.Context(), h.Client, a); err != nil {
		h.Logger.Error("append account", "err", err)
		http.Error(w, "failed to write account", http.StatusBadGateway)
		return
	}
	// Re-list after write so HTMX can swap the table.
	accs, _ := List(r.Context(), h.Client, true)
	_ = h.Render.Render(w, "accounts/list", ListView{Accounts: accs})
}

// parseForm turns submitted form values into an Account, validating the
// minimum (name + type + class).
func parseForm(r *http.Request) (Account, error) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return Account{}, errors.New("name is required")
	}
	t := Type(strings.TrimSpace(r.FormValue("type")))
	if t != Asset && t != Liability {
		return Account{}, errors.New("type must be Asset or Liability")
	}
	c := Class(strings.TrimSpace(r.FormValue("class")))
	if c == "" {
		return Account{}, errors.New("class is required")
	}
	a := Account{
		Name:     name,
		Type:     t,
		Class:    c,
		Currency: defaultStr(r.FormValue("currency"), "EUR"),
		Note:     strings.TrimSpace(r.FormValue("note")),
	}
	if v := strings.TrimSpace(r.FormValue("active_from")); v != "" {
		d, err := kernel.ParseDate(v)
		if err != nil {
			return Account{}, err
		}
		a.ActiveFrom = d
	}
	if v := strings.TrimSpace(r.FormValue("active_to")); v != "" {
		d, err := kernel.ParseDate(v)
		if err != nil {
			return Account{}, err
		}
		a.ActiveTo = d
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
type ListView struct{ Accounts []Account }
