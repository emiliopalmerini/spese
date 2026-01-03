package http

import (
	"context"
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

func (s *Server) handleRecurrentExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get repository based on adapter type
	var repo interface {
		GetRecurrentExpenses(ctx context.Context) ([]core.RecurrentExpenses, error)
	}

	// Check if we have access to the repository through type assertion
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		slog.ErrorContext(r.Context(), "Recurrent expenses not supported with current backend")
		http.Error(w, "Recurrent expenses not available", http.StatusNotImplemented)
		return
	}

	expenses, err := repo.GetRecurrentExpenses(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get recurrent expenses", "error", err)
		http.Error(w, "Failed to load recurrent expenses", http.StatusInternalServerError)
		return
	}

	// Get categories for the form
	cats, subs, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories", "error", err)
		cats = []string{}
		subs = []string{}
	}

	now := time.Now()
	data := struct {
		RecurrentExpenses []core.RecurrentExpenses
		Categories        []string
		Subcats           []string
		Day               int
		Month             int
	}{
		RecurrentExpenses: expenses,
		Categories:        cats,
		Subcats:           subs,
		Day:               now.Day(),
		Month:             int(now.Month()),
	}

	if err := s.templates.ExecuteTemplate(w, "recurrent_page", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution failed", "error", err, "template", "recurrent_page")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleCreateRecurrentExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(r.Context(), "Parse form error", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
		return
	}

	// Parse form data
	startDateStr := r.Form.Get("start_date")
	endDateStr := r.Form.Get("end_date")
	repetitionType := r.Form.Get("repetition_type")
	description := sanitizeInput(r.Form.Get("description"))
	amountStr := strings.TrimSpace(r.Form.Get("amount"))
	primary := sanitizeInput(r.Form.Get("primary"))
	secondary := sanitizeInput(r.Form.Get("secondary"))

	// Parse dates
	startDate, err := parseDate(startDateStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Data inizio non valida</div>`))
		return
	}

	var endDate core.Date
	if endDateStr != "" {
		endDate, err = parseDate(endDateStr)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`<div class="error">Data fine non valida</div>`))
			return
		}
	}

	// Parse amount
	cents, err := core.ParseDecimalToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
		return
	}

	// Create and validate recurrent expense
	re := core.RecurrentExpenses{
		StartDate:   startDate,
		EndDate:     endDate,
		Every:       core.RepetitionTypes(repetitionType),
		Description: description,
		Amount:      core.Money{Cents: cents},
		Primary:     primary,
		Secondary:   secondary,
	}

	if err := re.Validate(); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	// Get repository
	var repo interface {
		CreateRecurrentExpense(ctx context.Context, re core.RecurrentExpenses) (int64, error)
	}

	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		slog.ErrorContext(r.Context(), "Recurrent expenses not supported with current backend")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`<div class="error">Spese ricorrenti non disponibili</div>`))
		return
	}

	id, err := repo.CreateRecurrentExpense(r.Context(), re)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to create recurrent expense", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nel salvare la spesa ricorrente</div>`))
		return
	}

	slog.InfoContext(r.Context(), "Recurrent expense created", "id", id, "description", re.Description)

	successMsg := fmt.Sprintf("Spesa ricorrente '%s' creata con successo", re.Description)
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"form:reset": {},
		"show-notification": {"type": "success", "message": "%s", "duration": 3000},
		"page:refresh": {}
	}`, template.JSEscapeString(successMsg)))

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte("")) // Empty response, notifications handled via JavaScript
}

func (s *Server) handleUpdateRecurrentExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.Header().Set("Allow", "PUT, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse ID from query params
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">ID non valido</div>`))
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(r.Context(), "Parse form error", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
		return
	}

	// Parse form data (similar to create)
	startDateStr := r.Form.Get("start_date")
	endDateStr := r.Form.Get("end_date")
	repetitionType := r.Form.Get("repetition_type")
	description := sanitizeInput(r.Form.Get("description"))
	amountStr := strings.TrimSpace(r.Form.Get("amount"))
	primary := sanitizeInput(r.Form.Get("primary"))
	secondary := sanitizeInput(r.Form.Get("secondary"))

	startDate, err := parseDate(startDateStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Data inizio non valida</div>`))
		return
	}

	var endDate core.Date
	if endDateStr != "" {
		endDate, err = parseDate(endDateStr)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`<div class="error">Data fine non valida</div>`))
			return
		}
	}

	cents, err := core.ParseDecimalToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
		return
	}

	re := core.RecurrentExpenses{
		StartDate:   startDate,
		EndDate:     endDate,
		Every:       core.RepetitionTypes(repetitionType),
		Description: description,
		Amount:      core.Money{Cents: cents},
		Primary:     primary,
		Secondary:   secondary,
	}

	if err := re.Validate(); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	// Get repository
	var repo interface {
		UpdateRecurrentExpense(ctx context.Context, id int64, re core.RecurrentExpenses) error
	}

	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		slog.ErrorContext(r.Context(), "Recurrent expenses not supported with current backend")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`<div class="error">Spese ricorrenti non disponibili</div>`))
		return
	}

	if err := repo.UpdateRecurrentExpense(r.Context(), id, re); err != nil {
		slog.ErrorContext(r.Context(), "Failed to update recurrent expense", "error", err, "id", id)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nell'aggiornare la spesa ricorrente</div>`))
		return
	}

	slog.InfoContext(r.Context(), "Recurrent expense updated", "id", id)

	// Trigger client refresh for HTMX
	w.Header().Set("HX-Trigger", `{"recurrent:updated": {}}`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<div class="success">Spesa ricorrente aggiornata con successo</div>`))
}

func (s *Server) handleDeleteRecurrentExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse ID from query params
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">ID non valido</div>`))
		return
	}

	// Get repository
	var repo interface {
		DeleteRecurrentExpense(ctx context.Context, id int64) error
	}

	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		slog.ErrorContext(r.Context(), "Recurrent expenses not supported with current backend")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`<div class="error">Spese ricorrenti non disponibili</div>`))
		return
	}

	if err := repo.DeleteRecurrentExpense(r.Context(), id); err != nil {
		slog.ErrorContext(r.Context(), "Failed to delete recurrent expense", "error", err, "id", id)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nell'eliminare la spesa ricorrente</div>`))
		return
	}

	slog.InfoContext(r.Context(), "Recurrent expense deleted", "id", id)

	// Trigger client refresh for HTMX
	w.Header().Set("HX-Trigger", `{"recurrent:deleted": {}}`)
	// Return empty content for HTMX to remove the row
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(``))
}

func (s *Server) handleRecurrentExpensesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Get repository based on adapter type
	var repo interface {
		GetRecurrentExpenses(ctx context.Context) ([]core.RecurrentExpenses, error)
	}

	// Check if we have access to the repository through type assertion
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		_, _ = w.Write([]byte(`<div id="recurrent-list" class="recurrent-expenses"><div class="empty-state"><p class="empty-message">Spese ricorrenti non disponibili con questo backend</p></div></div>`))
		return
	}

	expenses, err := repo.GetRecurrentExpenses(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get recurrent expenses", "error", err)
		_, _ = w.Write([]byte(`<div id="recurrent-list" class="recurrent-expenses"><div class="empty-state"><p class="empty-message">Errore nel caricamento delle spese ricorrenti</p></div></div>`))
		return
	}

	data := struct {
		RecurrentExpenses []core.RecurrentExpenses
	}{
		RecurrentExpenses: expenses,
	}

	if err := s.templates.ExecuteTemplate(w, "recurrent-list", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution failed", "error", err, "template", "recurrent-list")
		_, _ = w.Write([]byte(`<div id="recurrent-list" class="recurrent-expenses"><div class="empty-state"><p class="empty-message">Errore nel rendering della lista</p></div></div>`))
	}
}

func (s *Server) handleRecurrentMonthlyOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Get SQLite repository for recurrent expenses
	var repo *storage.SQLiteRepository
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		_, _ = w.Write([]byte(`<div id="recurrent-monthly-overview" class="month-overview"><div class="overview-body"><div class="row placeholder">Panoramica non disponibile con questo backend</div></div></div>`))
		return
	}

	expenses, err := repo.GetRecurrentExpenses(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get recurrent expenses for overview", "error", err)
		_, _ = w.Write([]byte(`<div id="recurrent-monthly-overview" class="month-overview"><div class="overview-body"><div class="row placeholder">Errore nel caricamento della panoramica</div></div></div>`))
		return
	}

	// Calculate monthly totals and category breakdown
	totalCents := int64(0)
	categoryTotals := make(map[string]int64)

	for _, expense := range expenses {
		// Convert to monthly amount based on frequency
		monthlyCents := int64(0)
		switch expense.Every {
		case "daily":
			monthlyCents = expense.Amount.Cents * 30 // Approximate days per month
		case "weekly":
			monthlyCents = expense.Amount.Cents * 4 // Approximate weeks per month
		case "monthly":
			monthlyCents = expense.Amount.Cents
		case "yearly":
			monthlyCents = expense.Amount.Cents / 12
		}

		totalCents += monthlyCents
		categoryTotals[expense.Primary] += monthlyCents
	}

	// Find max category for scale
	maxCents := int64(0)
	topCategory := ""
	for category, cents := range categoryTotals {
		if cents > maxCents {
			maxCents = cents
			topCategory = category
		}
	}

	// Build category breakdown with percentages
	type CategoryRow struct {
		Name   string
		Amount string
		Width  int
	}

	var categories []CategoryRow
	for category, cents := range categoryTotals {
		width := 0
		if maxCents > 0 {
			width = int((cents * 100) / maxCents)
		}
		categories = append(categories, CategoryRow{
			Name:   category,
			Amount: formatEuros(cents),
			Width:  width,
		})
	}

	data := struct {
		MonthlyTotal string
		TopCategory  string
		TopAmount    string
		Categories   []CategoryRow
	}{
		MonthlyTotal: formatEuros(totalCents),
		TopCategory:  topCategory,
		TopAmount:    formatEuros(maxCents),
		Categories:   categories,
	}

	if err := s.templates.ExecuteTemplate(w, "recurrent_monthly_overview", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution failed", "error", err, "template", "recurrent_monthly_overview")
		_, _ = w.Write([]byte(`<div id="recurrent-monthly-overview" class="month-overview"><div class="overview-body"><div class="row placeholder">Errore nel rendering della panoramica</div></div></div>`))
	}
}

func (s *Server) handleRecurrentExpenseEdit(w http.ResponseWriter, r *http.Request) {
	// Only handle paths that end with /edit
	if !strings.HasSuffix(r.URL.Path, "/edit") {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Extract ID from path like /recurrent/123/edit
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) != 3 || pathParts[0] != "recurrent" || pathParts[2] != "edit" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Get SQLite repository
	var repo *storage.SQLiteRepository
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		repo = adapter.GetStorage()
	} else {
		http.Error(w, "Backend not supported", http.StatusInternalServerError)
		return
	}

	// Get the specific recurrent expense
	expenses, err := repo.GetRecurrentExpenses(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get recurrent expenses", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var targetExpense *core.RecurrentExpenses
	for i := range expenses {
		if int64(expenses[i].ID) == int64(id) {
			targetExpense = &expenses[i]
			break
		}
	}

	if targetExpense == nil {
		http.Error(w, "Expense not found", http.StatusNotFound)
		return
	}

	// Get categories for the form
	cats, subs, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to load categories", "error", err)
		// Continue without categories
	}
	categories := cats
	subcats := subs

	data := struct {
		*core.RecurrentExpenses
		Categories []string
		Subcats    []string
	}{
		RecurrentExpenses: targetExpense,
		Categories:        categories,
		Subcats:           subcats,
	}

	if err := s.templates.ExecuteTemplate(w, "recurrent_edit_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution failed", "error", err, "template", "recurrent_edit_form")
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func (s *Server) handleRecurrentFormReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cats, _, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories for recurrent form reset", "error", err)
		cats = []string{}
	}

	data := struct {
		Categories []string
	}{
		Categories: cats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recurrent_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Recurrent form reset template execution failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
