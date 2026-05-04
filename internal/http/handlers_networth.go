package http

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
	"spese/internal/storage"
)

// netWorthSection groups accounts of a given type for display.
type netWorthSection struct {
	Type     core.AccountType
	Title    string
	Accounts []netWorthAccountRow
	Total    string
}

type netWorthAccountRow struct {
	ID            int64
	Name          string
	Type          core.AccountType
	Active        bool
	Amount        string
	AmountCents   int64
	HasBalance    bool
	IsInactive    bool
	BalanceUpdate string
}

var netWorthSectionTitles = map[core.AccountType]string{
	core.AccountCash:     "Cash & Liquidità",
	core.AccountRainyDay: "Rainy day",
	core.AccountLongTerm: "Long term",
}

func nwAdapter(s *Server) (*adapters.SQLiteAdapter, bool) {
	a, ok := s.expWriter.(*adapters.SQLiteAdapter)
	return a, ok
}

func parseAccountType(s string) (core.AccountType, error) {
	switch core.AccountType(s) {
	case core.AccountCash, core.AccountRainyDay, core.AccountLongTerm:
		return core.AccountType(s), nil
	}
	return "", core.ErrInvalidAccountType
}

func parseYearMonthDefaultsNow(r *http.Request) (year, month int, ok bool) {
	now := time.Now()
	year = now.Year()
	month = int(now.Month())
	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		y, err := strconv.Atoi(v)
		if err != nil || y < 2000 || y > 2999 {
			return 0, 0, false
		}
		year = y
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m < 1 || m > 12 {
			return 0, 0, false
		}
		month = m
	}
	return year, month, true
}

func buildNetWorthSections(accounts []core.Account, balances map[int64]core.AccountBalance) []netWorthSection {
	order := []core.AccountType{core.AccountCash, core.AccountRainyDay, core.AccountLongTerm}
	out := make([]netWorthSection, 0, len(order))
	for _, t := range order {
		section := netWorthSection{Type: t, Title: netWorthSectionTitles[t]}
		var total int64
		for _, a := range accounts {
			if a.Type != t {
				continue
			}
			row := netWorthAccountRow{
				ID:         a.ID,
				Name:       a.Name,
				Type:       a.Type,
				Active:     a.Active,
				IsInactive: !a.Active,
			}
			if b, ok := balances[a.ID]; ok {
				row.Amount = formatEuros(b.Amount.Cents)
				row.AmountCents = b.Amount.Cents
				row.HasBalance = true
				row.BalanceUpdate = fmt.Sprintf("%d-%02d", b.Year, b.Month)
				total += b.Amount.Cents
			} else {
				row.Amount = "—"
			}
			section.Accounts = append(section.Accounts, row)
		}
		section.Total = formatEuros(total)
		out = append(out, section)
	}
	return out
}

func (s *Server) renderNetWorthAccounts(w http.ResponseWriter, r *http.Request, year, month int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	a, ok := nwAdapter(s)
	if !ok {
		_, _ = w.Write([]byte(`<div class="placeholder">Net worth non disponibile</div>`))
		return
	}
	accounts, err := a.ListAccounts(r.Context(), true)
	if err != nil {
		slog.ErrorContext(r.Context(), "list accounts", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore caricamento conti</div>`))
		return
	}
	balances, err := a.ListBalancesByMonth(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "list balances", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore caricamento saldi</div>`))
		return
	}
	balanceByAccount := make(map[int64]core.AccountBalance, len(balances))
	for _, b := range balances {
		balanceByAccount[b.AccountID] = b
	}
	sections := buildNetWorthSections(accounts, balanceByAccount)

	total, err := a.MonthlyNetWorth(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "monthly net worth", "error", err)
		total = core.Money{}
	}

	data := struct {
		Year     int
		Month    int
		Sections []netWorthSection
		Total    string
	}{
		Year:     year,
		Month:    month,
		Sections: sections,
		Total:    formatEuros(total.Cents),
	}

	if err := s.templates.ExecuteTemplate(w, "networth_accounts", data); err != nil {
		slog.ErrorContext(r.Context(), "render networth_accounts", "error", err)
		_, _ = w.Write([]byte(`<div class="error">Errore template</div>`))
	}
}

// handleNetWorthPage renders the full /networth page.
func (s *Server) handleNetWorthPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.templates == nil {
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}
	year, month, ok := parseYearMonthDefaultsNow(r)
	if !ok {
		http.Error(w, "invalid year/month", http.StatusBadRequest)
		return
	}

	data := struct {
		Year  int
		Month int
	}{Year: year, Month: month}

	if err := s.templates.ExecuteTemplate(w, "networth_page", data); err != nil {
		slog.ErrorContext(r.Context(), "networth page render", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleNetWorthAccounts dispatches /ui/networth/accounts on method:
// GET returns the accounts partial, POST creates a new account.
func (s *Server) handleNetWorthAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		year, month, ok := parseYearMonthDefaultsNow(r)
		if !ok {
			http.Error(w, "invalid year/month", http.StatusBadRequest)
			return
		}
		s.renderNetWorthAccounts(w, r, year, month)
	case http.MethodPost:
		s.handleNetWorthAccountCreate(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleNetWorthAccountCreate creates a new account from form data.
func (s *Server) handleNetWorthAccountCreate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a, ok := nwAdapter(s)
	if !ok {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := sanitizeInput(r.Form.Get("name"))
	typeStr := strings.TrimSpace(r.Form.Get("type"))
	active := r.Form.Get("active") != "" && r.Form.Get("active") != "false"

	t, err := parseAccountType(typeStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Tipo conto non valido</div>`))
		return
	}
	acc := core.Account{Name: name, Type: t, Active: active}
	if err := acc.Validate(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Dati non validi: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	if _, err := a.CreateAccount(r.Context(), acc); err != nil {
		if errors.Is(err, storage.ErrAccountExists) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`<div class="error">Conto già esistente</div>`))
			return
		}
		slog.ErrorContext(r.Context(), "create account", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore creazione conto</div>`))
		return
	}

	w.Header().Set("HX-Trigger", `{"networth:updated":{},"dashboard:refresh":{}}`)
	year, month, _ := parseYearMonthDefaultsNow(r)
	s.renderNetWorthAccounts(w, r, year, month)
}

// handleNetWorthAccountUpdate updates an existing account by ID.
func (s *Server) handleNetWorthAccountUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.Header().Set("Allow", "PUT, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a, ok := nwAdapter(s)
	if !ok {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/networth/accounts/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := sanitizeInput(r.Form.Get("name"))
	typeStr := strings.TrimSpace(r.Form.Get("type"))
	active := r.Form.Get("active") != "" && r.Form.Get("active") != "false"

	t, err := parseAccountType(typeStr)
	if err != nil {
		http.Error(w, "invalid type", http.StatusBadRequest)
		return
	}
	acc := core.Account{ID: id, Name: name, Type: t, Active: active}
	if err := acc.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.UpdateAccount(r.Context(), acc); err != nil {
		if errors.Is(err, storage.ErrAccountExists) {
			http.Error(w, "duplicate name", http.StatusConflict)
			return
		}
		slog.ErrorContext(r.Context(), "update account", "error", err)
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", `{"networth:updated":{},"dashboard:refresh":{}}`)
	year, month, _ := parseYearMonthDefaultsNow(r)
	s.renderNetWorthAccounts(w, r, year, month)
}

// handleNetWorthBalanceUpsert writes the balance for one account/month.
func (s *Server) handleNetWorthBalanceUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a, ok := nwAdapter(s)
	if !ok {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(r.Form.Get("account_id")), 10, 64)
	if err != nil {
		http.Error(w, "invalid account_id", http.StatusBadRequest)
		return
	}
	year, err := strconv.Atoi(strings.TrimSpace(r.Form.Get("year")))
	if err != nil || year < 2000 || year > 2999 {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	month, err := strconv.Atoi(strings.TrimSpace(r.Form.Get("month")))
	if err != nil || month < 1 || month > 12 {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	cents, err := core.ParseDecimalToCents(strings.TrimSpace(r.Form.Get("amount")))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
		return
	}

	acc, err := a.GetAccount(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, storage.ErrAccountNotFound) {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "get account", "error", err)
		http.Error(w, "lookup error", http.StatusInternalServerError)
		return
	}
	if !acc.Active {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Conto inattivo</div>`))
		return
	}

	bal := core.AccountBalance{
		AccountID: accountID,
		Year:      year,
		Month:     month,
		Amount:    core.Money{Cents: cents},
	}
	if err := bal.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.UpsertBalance(r.Context(), bal); err != nil {
		slog.ErrorContext(r.Context(), "upsert balance", "error", err)
		http.Error(w, "save error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"networth:updated":{},"dashboard:refresh":{}}`)
	s.renderNetWorthAccounts(w, r, year, month)
}

// handleNetWorthMonth renders the accounts/balances partial for a specific month.
func (s *Server) handleNetWorthMonth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	year, month, ok := parseYearMonthDefaultsNow(r)
	if !ok {
		http.Error(w, "invalid year/month", http.StatusBadRequest)
		return
	}
	s.renderNetWorthAccounts(w, r, year, month)
}

// handleDashboardNetWorth renders the dashboard tile with NW + MoM delta.
func (s *Server) handleDashboardNetWorth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	a, ok := nwAdapter(s)
	if !ok {
		_, _ = w.Write([]byte(`<div class="placeholder">Net worth non disponibile</div>`))
		return
	}

	now := time.Now()
	year, month := now.Year(), int(now.Month())
	prevYear, prevMonth := year, month-1
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}

	current, err := a.MonthlyNetWorth(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "tile current", "error", err)
		_, _ = w.Write([]byte(`<div class="error">Errore caricamento</div>`))
		return
	}
	prev, err := a.MonthlyNetWorth(r.Context(), prevYear, prevMonth)
	if err != nil {
		slog.ErrorContext(r.Context(), "tile prev", "error", err)
		prev = core.Money{}
	}

	hasData := current.Cents != 0
	hasPrev := prev.Cents != 0
	var deltaCents int64
	var deltaPercent float64
	var deltaDirection string
	if hasPrev && hasData {
		deltaCents = current.Cents - prev.Cents
		deltaPercent = float64(deltaCents) / float64(prev.Cents) * 100
		switch {
		case deltaCents > 0:
			deltaDirection = "up"
		case deltaCents < 0:
			deltaDirection = "down"
		default:
			deltaDirection = "flat"
		}
	}

	data := struct {
		Year           int
		Month          int
		Total          string
		HasData        bool
		ShowDelta      bool
		DeltaAmount    string
		DeltaPercent   string
		DeltaDirection string
	}{
		Year:           year,
		Month:          month,
		Total:          formatEuros(current.Cents),
		HasData:        hasData,
		ShowDelta:      hasPrev && hasData,
		DeltaAmount:    formatEuros(deltaCents),
		DeltaPercent:   fmt.Sprintf("%.1f%%", deltaPercent),
		DeltaDirection: deltaDirection,
	}

	if err := s.templates.ExecuteTemplate(w, "dashboard_net_worth", data); err != nil {
		slog.ErrorContext(r.Context(), "tile render", "error", err)
		_, _ = w.Write([]byte(`<div class="error">Errore template</div>`))
	}
}
