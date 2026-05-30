package actions

import (
	"log/slog"
	"net/http"
	"strings"

	"spese/internal/features/accounts"
	"spese/internal/features/snapshots"
	"spese/internal/features/transactions"
	"spese/internal/kernel"
	"spese/internal/storage"
)

// Renderer is the fragment-only template interface for the global action drawer.
type Renderer interface {
	RenderFragment(w http.ResponseWriter, name string, data any) error
}

// Handler serves HTMX fragments for the global create drawer.
type Handler struct {
	Store  *storage.Store
	Logger *slog.Logger
	Render Renderer
}

// Mount registers one GET endpoint per drawer action.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix+"/new/transaction", h.transactionForm)
	mux.HandleFunc("GET "+prefix+"/new/transfer", h.transferForm)
	mux.HandleFunc("GET "+prefix+"/new/snapshot", h.snapshotForm)
	mux.HandleFunc("GET "+prefix+"/new/account", h.accountForm)
}

// AccountPickerView is shared by forms that choose one or more accounts.
type AccountPickerView struct {
	Accounts            []accounts.Account
	Today               kernel.Date
	CategorySuggestions CategorySuggestions
}

func (h *Handler) transactionForm(w http.ResponseWriter, r *http.Request) {
	view, err := h.accountPickerView(r)
	if err != nil {
		h.Logger.Error("build transaction action form", "err", err)
		http.Error(w, "failed to load transaction form", http.StatusBadGateway)
		return
	}
	h.renderFragment(w, "action_form_transaction", view)
}

func (h *Handler) transferForm(w http.ResponseWriter, r *http.Request) {
	view, err := h.accountPickerView(r)
	if err != nil {
		h.Logger.Error("build transfer action form", "err", err)
		http.Error(w, "failed to load transfer form", http.StatusBadGateway)
		return
	}
	h.renderFragment(w, "action_form_transfer", view)
}

func (h *Handler) snapshotForm(w http.ResponseWriter, r *http.Request) {
	view, err := snapshots.BuildFormView(r.Context(), h.Store, r.URL.Query().Get("month"), false)
	if err != nil {
		h.Logger.Error("build snapshot action form", "err", err)
		http.Error(w, "failed to load snapshot form", http.StatusBadGateway)
		return
	}
	h.renderFragment(w, "action_form_snapshot", view)
}

func (h *Handler) accountForm(w http.ResponseWriter, _ *http.Request) {
	h.renderFragment(w, "action_form_account", nil)
}

func (h *Handler) accountPickerView(r *http.Request) (AccountPickerView, error) {
	accs, err := accounts.List(r.Context(), h.Store, false)
	if err != nil {
		return AccountPickerView{}, err
	}
	return AccountPickerView{
		Accounts:            accs,
		Today:               kernel.Today(),
		CategorySuggestions: h.categorySuggestions(r),
	}, nil
}

func (h *Handler) categorySuggestions(r *http.Request) CategorySuggestions {
	txns, err := transactions.List(r.Context(), h.Store, transactions.Filter{}, false)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warn("load transaction category suggestions", "err", err)
		}
	}

	return buildCategorySuggestions(txns)
}

func (h *Handler) renderFragment(w http.ResponseWriter, name string, data any) {
	if err := h.Render.RenderFragment(w, name, data); err != nil {
		h.Logger.Error("render action fragment", "name", name, "err", err)
		http.Error(w, "failed to render action form", http.StatusInternalServerError)
	}
}
