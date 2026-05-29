package snapshots

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"spese/internal/features/accounts"
	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Handler records per-account balance snapshots.
type Handler struct {
	Client *sheets.Client
	Logger *slog.Logger
}

// Mount registers the compatibility GET redirect and POST submit endpoint.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc("GET "+prefix, h.redirectToBalanceSheet)
	mux.HandleFunc("POST "+prefix, h.create)
}

// Row is one line of the entry form: an account paired with its last
// known balance to prefill the input.
type Row struct {
	Account     accounts.Account
	LastBalance kernel.Money
	LastMonth   kernel.Date
}

// FormView is the payload for the form page.
type FormView struct {
	Month kernel.Date
	Rows  []Row
}

func (h *Handler) redirectToBalanceSheet(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/reports/balance-sheet", http.StatusSeeOther)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	snaps, err := parseForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := Append(r.Context(), h.Client, snaps); err != nil {
		h.Logger.Error("append snapshots", "err", err)
		http.Error(w, "failed to write snapshots", http.StatusBadGateway)
		return
	}
	redirectAfterCreate(w, r)
}

func redirectAfterCreate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/reports/balance-sheet")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/reports/balance-sheet", http.StatusSeeOther)
}

// parseForm reads one balance input per submitted account. Inputs are named
// `balance[<account>]`; empty values are skipped.
func parseForm(r *http.Request) ([]Snapshot, error) {
	monthStr := strings.TrimSpace(r.FormValue("month"))
	month, err := kernel.ParseDate(monthStr)
	if err != nil {
		return nil, errors.New("invalid month")
	}
	month = month.FirstOfMonth()

	var snaps []Snapshot
	for k, vs := range r.Form {
		if !strings.HasPrefix(k, "balance[") || !strings.HasSuffix(k, "]") {
			continue
		}
		account := strings.TrimSuffix(strings.TrimPrefix(k, "balance["), "]")
		raw := strings.TrimSpace(vs[0])
		if raw == "" {
			continue
		}
		amt, err := kernel.ParseMoney(raw)
		if err != nil {
			return nil, errors.New("invalid balance for " + account)
		}
		snaps = append(snaps, Snapshot{
			Month:   month,
			Account: account,
			Balance: amt,
			Note:    strings.TrimSpace(r.FormValue("note[" + account + "]")),
		})
	}
	if len(snaps) == 0 {
		return nil, errors.New("no balances submitted")
	}
	return snaps, nil
}

// defaultMonth returns the requested month, or the current month if blank.
func defaultMonth(s string) kernel.Date {
	if s != "" {
		if d, err := kernel.ParseDate(s); err == nil {
			return d.FirstOfMonth()
		}
	}
	now := time.Now()
	return kernel.Date{Time: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)}
}

// BuildFormView prepares the month entry model shared by the page and the
// global action drawer.
func BuildFormView(ctx context.Context, client *sheets.Client, monthParam string, force bool) (FormView, error) {
	accs, err := accounts.List(ctx, client, false)
	if err != nil {
		return FormView{}, err
	}
	latest, err := LatestPerAccount(ctx, client, force)
	if err != nil {
		return FormView{}, err
	}
	month := defaultMonth(monthParam)
	rows := make([]Row, 0, len(accs))
	for _, a := range accs {
		if !a.IsActive(month) {
			continue
		}
		row := Row{Account: a}
		if last, ok := latest[a.Name]; ok {
			row.LastBalance = last.Balance
			row.LastMonth = last.Month
		}
		rows = append(rows, row)
	}
	return FormView{Month: month, Rows: rows}, nil
}
