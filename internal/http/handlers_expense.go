package http

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
	"spese/internal/sheets"
)

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(r.Context(), "Parse form error", "error", err, "method", r.Method, "url", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
		return
	}

	now := time.Now()
	day := now.Day()
	month := int(now.Month())
	if v := strings.TrimSpace(r.Form.Get("day")); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			day = d
		}
	}
	if v := strings.TrimSpace(r.Form.Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}

	desc := sanitizeInput(r.Form.Get("description"))
	amountStr := strings.TrimSpace(r.Form.Get("amount"))
	primary := sanitizeInput(r.Form.Get("primary"))
	secondary := sanitizeInput(r.Form.Get("secondary"))

	cents, err := core.ParseDecimalToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
		return
	}

	exp := core.Expense{
		Date:        core.NewDate(time.Now().Year(), month, day),
		Description: desc,
		Amount:      core.Money{Cents: cents},
		Primary:     primary,
		Secondary:   secondary,
	}
	if err := exp.Validate(); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Invalid data: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	ref, err := s.expWriter.Append(r.Context(), exp)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to save expense",
			"error", err,
			"expense_description", exp.Description,
			"amount_cents", exp.Amount.Cents,
			"primary_category", exp.Primary,
			"secondary_category", exp.Secondary,
			"component", "expense_writer",
			"operation", "append")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Error saving expense</div>`))
		return
	}

	atomic.AddInt64(&s.appMetrics.totalExpenses, 1)

	slog.InfoContext(r.Context(), "Expense created successfully",
		"expense_description", exp.Description,
		"amount_cents", exp.Amount.Cents,
		"primary_category", exp.Primary,
		"secondary_category", exp.Secondary,
		"sheets_ref", ref,
		"component", "expense_handler",
		"operation", "create")

	successMsg := fmt.Sprintf("Spesa registrata (#%s): %s — €%s (%s / %s)",
		template.HTMLEscapeString(ref),
		template.HTMLEscapeString(exp.Description),
		template.HTMLEscapeString(amountStr),
		template.HTMLEscapeString(exp.Primary),
		template.HTMLEscapeString(exp.Secondary))

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"form:reset": {},
		"show-notification": {"type": "success", "message": "%s", "duration": 3000},
		"page:refresh": {}
	}`, template.JSEscapeString(successMsg)))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(""))
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var expenseID string
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") || r.Method == http.MethodDelete {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.ErrorContext(r.Context(), "Read body error", "error", err, "method", r.Method, "url", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<div class="error">Errore lettura richiesta</div>`))
			return
		}

		slog.InfoContext(r.Context(), "Delete expense request body",
			"method", r.Method,
			"content_type", contentType,
			"body", string(body),
			"headers", r.Header,
			"body_length", len(body))

		var requestBody map[string]interface{}
		if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
			if err := json.Unmarshal(body, &requestBody); err != nil {
				slog.ErrorContext(r.Context(), "Parse JSON body error", "error", err, "method", r.Method, "url", r.URL.Path, "content_type", contentType, "body", string(body))
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<div class="error">Formato richiesta JSON non valido</div>`))
				return
			}

			if id, ok := requestBody["id"]; ok {
				expenseID = sanitizeInput(fmt.Sprintf("%v", id))
			}

			slog.InfoContext(r.Context(), "Delete expense request (JSON)", "method", r.Method, "json_body", requestBody, "expense_id", expenseID)
		} else {
			slog.InfoContext(r.Context(), "Body doesn't look like JSON, trying form parsing", "body", string(body))

			formData, err := url.ParseQuery(string(body))
			if err != nil {
				slog.ErrorContext(r.Context(), "Parse form data from body error", "error", err, "method", r.Method, "url", r.URL.Path, "content_type", contentType, "body", string(body))
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<div class="error">Formato dati form non valido</div>`))
				return
			}

			expenseID = sanitizeInput(formData.Get("id"))
			slog.InfoContext(r.Context(), "Delete expense request (Form fallback)", "method", r.Method, "form_data", formData, "expense_id", expenseID)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			slog.ErrorContext(r.Context(), "Parse form error", "error", err, "method", r.Method, "url", r.URL.Path, "content_type", contentType)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
			return
		}

		expenseID = sanitizeInput(r.Form.Get("id"))
		slog.InfoContext(r.Context(), "Delete expense request (Form)", "method", r.Method, "form_values", r.Form, "expense_id", expenseID)
	}

	if expenseID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">ID spesa mancante</div>`))
		return
	}

	if s.expDeleter == nil {
		slog.ErrorContext(r.Context(), "Expense deleter not configured")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Servizio di cancellazione non disponibile</div>`))
		return
	}

	err := s.expDeleter.DeleteExpense(r.Context(), expenseID)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to delete expense",
			"error", err,
			"expense_id", expenseID,
			"component", "expense_deleter",
			"operation", "delete")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nella cancellazione della spesa</div>`))
		return
	}

	atomic.AddInt64(&s.appMetrics.totalExpenses, -1)

	slog.InfoContext(r.Context(), "Expense deleted successfully",
		"expense_id", expenseID,
		"component", "expense_handler",
		"operation", "delete")

	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"expense:deleted": {"year": %d, "month": %d},
		"overview:refresh": {"year": %d, "month": %d},
		"show-notification": {"type": "success", "message": "Spesa cancellata con successo", "duration": 2000}
	}`, year, month, year, month))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(""))
}

func (s *Server) getOverview(ctx context.Context, year, month int) (core.MonthOverview, error) {
	if s.dashReader == nil {
		return core.MonthOverview{Year: year, Month: month}, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	data, err := s.dashReader.ReadMonthOverview(cctx, year, month)
	if err != nil {
		return core.MonthOverview{}, fmt.Errorf("read month overview (year=%d, month=%d): %w", year, month, err)
	}
	return data, nil
}

func (s *Server) getExpenses(ctx context.Context, year, month int) ([]core.Expense, error) {
	if s.expLister == nil {
		return nil, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	items, err := s.expLister.ListExpenses(cctx, year, month)
	if err != nil {
		return nil, fmt.Errorf("list month expenses (year=%d, month=%d): %w", year, month, err)
	}
	return items, nil
}

func (s *Server) getExpensesWithID(ctx context.Context, year, month int) ([]sheets.ExpenseWithID, error) {
	if s.expListerWithID == nil {
		return nil, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	items, err := s.expListerWithID.ListExpensesWithID(cctx, year, month)
	if err != nil {
		return nil, fmt.Errorf("list month expenses with ID (year=%d, month=%d): %w", year, month, err)
	}
	return items, nil
}

func (s *Server) handleMonthOverview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			year = y
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}
	if month < 1 || month > 12 {
		slog.WarnContext(r.Context(), "Invalid month parameter", "year", year, "month", month, "corrected_to", int(now.Month()))
		month = int(now.Month())
	}
	ov, err := s.getOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Month overview error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Error loading overview</div></section>`))
		return
	}
	if s.templates == nil {
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Totale: ` + formatEuros(ov.Total.Cents) + `</div></section>`))
		return
	}

	var maxCents int64
	var maxName string
	for _, r := range ov.ByCategory {
		if r.Amount.Cents > maxCents {
			maxCents = r.Amount.Cents
			maxName = r.Name
		}
	}
	type row struct {
		Name, Amount string
		Width        int
	}
	data := struct {
		Year    int
		Month   int
		Total   string
		MaxName string
		Max     string
		Rows    []row
		Items   []struct {
			ID   string
			Day  int
			Desc string
			Amt  string
			Cat  string
			Sub  string
		}
	}{Year: ov.Year, Month: ov.Month, Total: formatEuros(ov.Total.Cents), MaxName: maxName, Max: formatEuros(maxCents)}
	for _, r := range ov.ByCategory {
		width := 0
		if maxCents > 0 && r.Amount.Cents > 0 {
			width = int((r.Amount.Cents*100 + maxCents/2) / maxCents)
			if width > 0 && width < 2 {
				width = 2
			}
			if width > 100 {
				width = 100
			}
		}
		data.Rows = append(data.Rows, row{Name: r.Name, Amount: formatEuros(r.Amount.Cents), Width: width})
	}
	if s.expListerWithID != nil {
		itemsWithID, err := s.getExpensesWithID(r.Context(), year, month)
		if err != nil {
			slog.ErrorContext(r.Context(), "List expenses with ID error", "error", err, "year", year, "month", month)
		} else {
			for _, e := range itemsWithID {
				data.Items = append(data.Items, struct {
					ID   string
					Day  int
					Desc string
					Amt  string
					Cat  string
					Sub  string
				}{ID: e.ID, Day: e.Expense.Date.Day(), Desc: template.HTMLEscapeString(e.Expense.Description), Amt: formatEuros(e.Expense.Amount.Cents), Cat: e.Expense.Primary, Sub: e.Expense.Secondary})
			}
		}
	}
	if err := s.templates.ExecuteTemplate(w, "month_overview.html", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution error", "error", err, "template", "month_overview.html", "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Error rendering overview</div></section>`))
		return
	}
}

func (s *Server) handleGetSecondaryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	primaryCategory := strings.TrimSpace(r.URL.Query().Get("primary"))
	if primaryCategory == "" {
		primaryCategory = strings.TrimSpace(r.FormValue("primary"))
	}
	if primaryCategory == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<option value="">Seleziona prima la categoria primaria</option>`))
		return
	}

	if sqliteAdapter, ok := s.taxReader.(*adapters.SQLiteAdapter); ok {
		secondaries, err := sqliteAdapter.GetSecondariesByPrimary(r.Context(), primaryCategory)
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get secondary categories for primary",
				"primary", primaryCategory, "error", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`<option value="">Errore nel caricamento</option>`))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))

		for _, secondary := range secondaries {
			escapedSecondary := template.HTMLEscapeString(secondary)
			_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
		}

		slog.InfoContext(r.Context(), "Returned filtered secondary categories",
			"primary", primaryCategory,
			"count", len(secondaries))
		return
	}

	_, secondaries, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get secondary categories",
			"primary", primaryCategory, "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<option value="">Errore nel caricamento</option>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))

	for _, secondary := range secondaries {
		escapedSecondary := template.HTMLEscapeString(secondary)
		_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
	}
}

func (s *Server) handleGetAllCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sqliteAdapter, ok := s.taxReader.(*adapters.SQLiteAdapter)
	if !ok {
		// Fallback for non-SQLite adapters
		primaries, secondaries, err := s.taxReader.List(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get categories", "error", err)
			http.Error(w, "Failed to get categories", http.StatusInternalServerError)
			return
		}
		// Return simple structure for non-SQLite
		type simpleCat struct {
			Primary     string   `json:"primary"`
			Secondaries []string `json:"secondaries"`
		}
		result := make([]simpleCat, len(primaries))
		for i, p := range primaries {
			result[i] = simpleCat{Primary: p, Secondaries: secondaries}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	categories, err := sqliteAdapter.GetAllCategoriesWithSubs(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get all categories", "error", err)
		http.Error(w, "Failed to get categories", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}

func (s *Server) handleFormReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()
	cats, _, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories for form reset", "error", err)
		cats = []string{}
	}

	data := struct {
		Day        int
		Month      int
		Categories []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: cats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "expense_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Form reset template execution failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) handleMonthTotal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			year = y
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}

	ov, err := s.getOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Month total error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<div class="total">Errore nel caricamento</div>`))
		return
	}

	data := struct {
		Total string
	}{
		Total: formatEuros(ov.Total.Cents),
	}

	if err := s.templates.ExecuteTemplate(w, "month_total", data); err != nil {
		slog.ErrorContext(r.Context(), "Month total template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="total">Errore template</div>`))
	}
}

func (s *Server) handleMonthCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			year = y
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}

	ov, err := s.getOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Month categories error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<div class="categories"><div class="row placeholder">Errore nel caricamento</div></div>`))
		return
	}

	var maxCents int64
	var maxName string
	for _, r := range ov.ByCategory {
		if r.Amount.Cents > maxCents {
			maxCents = r.Amount.Cents
			maxName = r.Name
		}
	}

	type row struct {
		Name, Amount string
		Width        int
	}

	var rows []row
	for _, r := range ov.ByCategory {
		width := 0
		if maxCents > 0 && r.Amount.Cents > 0 {
			width = int((r.Amount.Cents*100 + maxCents/2) / maxCents)
			if width > 0 && width < 2 {
				width = 2
			}
			if width > 100 {
				width = 100
			}
		}
		rows = append(rows, row{Name: r.Name, Amount: formatEuros(r.Amount.Cents), Width: width})
	}

	data := struct {
		MaxName string
		Max     string
		Rows    []row
	}{
		MaxName: maxName,
		Max:     formatEuros(maxCents),
		Rows:    rows,
	}

	if err := s.templates.ExecuteTemplate(w, "month_categories", data); err != nil {
		slog.ErrorContext(r.Context(), "Month categories template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="categories"><div class="row placeholder">Errore template</div></div>`))
	}
}

func (s *Server) handleMonthExpenses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			year = y
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}

	var items []struct {
		ID   string
		Day  int
		Desc string
		Amt  string
		Cat  string
		Sub  string
	}

	if s.expListerWithID != nil {
		itemsWithID, err := s.getExpensesWithID(r.Context(), year, month)
		if err != nil {
			slog.ErrorContext(r.Context(), "List expenses with ID error", "error", err, "year", year, "month", month)
		} else {
			for _, e := range itemsWithID {
				items = append(items, struct {
					ID   string
					Day  int
					Desc string
					Amt  string
					Cat  string
					Sub  string
				}{
					ID:   e.ID,
					Day:  e.Expense.Date.Day(),
					Desc: template.HTMLEscapeString(e.Expense.Description),
					Amt:  formatEuros(e.Expense.Amount.Cents),
					Cat:  e.Expense.Primary,
					Sub:  e.Expense.Secondary,
				})
			}
		}
	}

	data := struct {
		Month int
		Items []struct {
			ID   string
			Day  int
			Desc string
			Amt  string
			Cat  string
			Sub  string
		}
	}{
		Month: month,
		Items: items,
	}

	if err := s.templates.ExecuteTemplate(w, "month_expenses", data); err != nil {
		slog.ErrorContext(r.Context(), "Month expenses template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="expenses"><div class="row placeholder">Errore template</div></div>`))
	}
}
