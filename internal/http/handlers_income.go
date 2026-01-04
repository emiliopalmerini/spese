package http

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
)

func (s *Server) handleIncomes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.templates == nil {
		slog.ErrorContext(r.Context(), "Templates not loaded",
			"path", r.URL.Path,
			"component", "template_engine",
			"error_type", "configuration_error")
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	now := time.Now()

	// Get income categories
	var categories []string
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		cats, err := adapter.GetIncomeCategories(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Income categories list error", "error", err)
		} else {
			categories = cats
		}
	}

	data := struct {
		Day        int
		Month      int
		Categories []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: categories,
	}

	if err := s.templates.ExecuteTemplate(w, "income_page", data); err != nil {
		slog.ErrorContext(r.Context(), "Income template execution failed", "error", err, "template", "income_page")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleCreateIncome(w http.ResponseWriter, r *http.Request) {
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
	category := sanitizeInput(r.Form.Get("category"))

	cents, err := core.ParseDecimalToCents(amountStr)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Importo non valido</div>`))
		return
	}

	income := core.Income{
		Date:        core.NewDate(time.Now().Year(), month, day),
		Description: desc,
		Amount:      core.Money{Cents: cents},
		Category:    category,
	}
	if err := income.Validate(); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`<div class="error">Dati non validi: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	// Get adapter and create income
	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		slog.ErrorContext(r.Context(), "Income not supported with current backend")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`<div class="error">Entrate non disponibili</div>`))
		return
	}

	ref, err := adapter.AppendIncome(r.Context(), income)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to save income",
			"error", err,
			"income_description", income.Description,
			"amount_cents", income.Amount.Cents,
			"category", income.Category,
			"component", "income_writer",
			"operation", "append")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nel salvare l'entrata</div>`))
		return
	}

	// Log successful income creation
	slog.InfoContext(r.Context(), "Income created successfully",
		"income_description", income.Description,
		"amount_cents", income.Amount.Cents,
		"category", income.Category,
		"ref", ref,
		"component", "income_handler",
		"operation", "create")

	successMsg := fmt.Sprintf("Entrata registrata (#%s): %s — €%s (%s)",
		template.HTMLEscapeString(ref),
		template.HTMLEscapeString(income.Description),
		template.HTMLEscapeString(amountStr),
		template.HTMLEscapeString(income.Category))

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"form:reset": {},
		"show-notification": {"type": "success", "message": "%s", "duration": 3000},
		"page:refresh": {}
	}`, template.JSEscapeString(successMsg)))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(""))
}

func (s *Server) handleDeleteIncome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var incomeID string
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/json") || r.Method == http.MethodDelete {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.ErrorContext(r.Context(), "Read body error", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<div class="error">Errore lettura richiesta</div>`))
			return
		}

		var requestBody map[string]interface{}
		if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
			if err := json.Unmarshal(body, &requestBody); err != nil {
				slog.ErrorContext(r.Context(), "Parse JSON body error", "error", err)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<div class="error">Formato richiesta JSON non valido</div>`))
				return
			}

			if id, ok := requestBody["id"]; ok {
				incomeID = sanitizeInput(fmt.Sprintf("%v", id))
			}
		} else {
			formData, err := url.ParseQuery(string(body))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`<div class="error">Formato dati form non valido</div>`))
				return
			}
			incomeID = sanitizeInput(formData.Get("id"))
		}
	} else {
		if err := r.ParseForm(); err != nil {
			slog.ErrorContext(r.Context(), "Parse form error", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<div class="error">Formato richiesta non valido</div>`))
			return
		}
		incomeID = sanitizeInput(r.Form.Get("id"))
	}

	if incomeID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<div class="error">ID entrata mancante</div>`))
		return
	}

	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		slog.ErrorContext(r.Context(), "Income delete not supported with current backend")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`<div class="error">Cancellazione entrate non disponibile</div>`))
		return
	}

	err := adapter.DeleteIncome(r.Context(), incomeID)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to delete income",
			"error", err,
			"income_id", incomeID)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nella cancellazione dell'entrata</div>`))
		return
	}

	slog.InfoContext(r.Context(), "Income deleted successfully", "income_id", incomeID)

	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"income:deleted": {"year": %d, "month": %d},
		"income-overview:refresh": {"year": %d, "month": %d},
		"show-notification": {"type": "success", "message": "Entrata cancellata con successo", "duration": 2000}
	}`, year, month, year, month))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(""))
}

func (s *Server) handleIncomeMonthOverview(w http.ResponseWriter, r *http.Request) {
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

	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		_, _ = w.Write([]byte(`<section id="income-month-overview" class="month-overview"><div class="placeholder">Overview non disponibile</div></section>`))
		return
	}

	ov, err := adapter.ReadIncomeMonthOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Income month overview error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="income-month-overview" class="month-overview"><div class="placeholder">Errore nel caricamento overview</div></section>`))
		return
	}

	// Compute max category for progress scaling
	var maxCents int64
	var maxName string
	for _, cat := range ov.ByCategory {
		if cat.Amount.Cents > maxCents {
			maxCents = cat.Amount.Cents
			maxName = cat.Name
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
		}
	}{Year: ov.Year, Month: ov.Month, Total: formatEuros(ov.Total.Cents), MaxName: maxName, Max: formatEuros(maxCents)}

	for _, cat := range ov.ByCategory {
		width := 0
		if maxCents > 0 && cat.Amount.Cents > 0 {
			width = int((cat.Amount.Cents*100 + maxCents/2) / maxCents)
			if width > 0 && width < 2 {
				width = 2
			}
			if width > 100 {
				width = 100
			}
		}
		data.Rows = append(data.Rows, row{Name: cat.Name, Amount: formatEuros(cat.Amount.Cents), Width: width})
	}

	// Fetch detailed items with IDs
	itemsWithID, err := adapter.ListIncomesWithID(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "List incomes with ID error", "error", err, "year", year, "month", month)
	} else {
		for _, inc := range itemsWithID {
			data.Items = append(data.Items, struct {
				ID   string
				Day  int
				Desc string
				Amt  string
				Cat  string
			}{ID: inc.ID, Day: inc.Income.Date.Day(), Desc: template.HTMLEscapeString(inc.Income.Description), Amt: formatEuros(inc.Income.Amount.Cents), Cat: inc.Income.Category})
		}
	}

	if err := s.templates.ExecuteTemplate(w, "income_month_overview.html", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution error", "error", err, "template", "income_month_overview.html")
		_, _ = w.Write([]byte(`<section id="income-month-overview" class="month-overview"><div class="placeholder">Errore rendering overview</div></section>`))
	}
}

func (s *Server) handleIncomeMonthTotal(w http.ResponseWriter, r *http.Request) {
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

	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		_, _ = w.Write([]byte(`<div class="total">Non disponibile</div>`))
		return
	}

	ov, err := adapter.ReadIncomeMonthOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Income month total error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<div class="total">Errore nel caricamento</div>`))
		return
	}

	data := struct {
		Total string
	}{
		Total: formatEuros(ov.Total.Cents),
	}

	if err := s.templates.ExecuteTemplate(w, "income_month_total", data); err != nil {
		slog.ErrorContext(r.Context(), "Income month total template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="total">Errore template</div>`))
	}
}

func (s *Server) handleIncomeMonthCategories(w http.ResponseWriter, r *http.Request) {
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

	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		_, _ = w.Write([]byte(`<div class="categories"><div class="row placeholder">Non disponibile</div></div>`))
		return
	}

	ov, err := adapter.ReadIncomeMonthOverview(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "Income month categories error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<div class="categories"><div class="row placeholder">Errore nel caricamento</div></div>`))
		return
	}

	var maxCents int64
	var maxName string
	for _, cat := range ov.ByCategory {
		if cat.Amount.Cents > maxCents {
			maxCents = cat.Amount.Cents
			maxName = cat.Name
		}
	}

	type row struct {
		Name, Amount string
		Width        int
	}

	var rows []row
	for _, cat := range ov.ByCategory {
		width := 0
		if maxCents > 0 && cat.Amount.Cents > 0 {
			width = int((cat.Amount.Cents*100 + maxCents/2) / maxCents)
			if width > 0 && width < 2 {
				width = 2
			}
			if width > 100 {
				width = 100
			}
		}
		rows = append(rows, row{Name: cat.Name, Amount: formatEuros(cat.Amount.Cents), Width: width})
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

	if err := s.templates.ExecuteTemplate(w, "income_month_categories", data); err != nil {
		slog.ErrorContext(r.Context(), "Income month categories template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="categories"><div class="row placeholder">Errore template</div></div>`))
	}
}

func (s *Server) handleIncomeMonthIncomes(w http.ResponseWriter, r *http.Request) {
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

	adapter, ok := s.expWriter.(*adapters.SQLiteAdapter)
	if !ok {
		_, _ = w.Write([]byte(`<div class="incomes"><div class="row placeholder">Non disponibile</div></div>`))
		return
	}

	var items []struct {
		ID   string
		Day  int
		Desc string
		Amt  string
		Cat  string
	}

	itemsWithID, err := adapter.ListIncomesWithID(r.Context(), year, month)
	if err != nil {
		slog.ErrorContext(r.Context(), "List incomes with ID error", "error", err, "year", year, "month", month)
	} else {
		for _, inc := range itemsWithID {
			items = append(items, struct {
				ID   string
				Day  int
				Desc string
				Amt  string
				Cat  string
			}{
				ID:   inc.ID,
				Day:  inc.Income.Date.Day(),
				Desc: template.HTMLEscapeString(inc.Income.Description),
				Amt:  formatEuros(inc.Income.Amount.Cents),
				Cat:  inc.Income.Category,
			})
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
		}
	}{
		Month: month,
		Items: items,
	}

	if err := s.templates.ExecuteTemplate(w, "income_month_incomes", data); err != nil {
		slog.ErrorContext(r.Context(), "Income month incomes template execution failed", "error", err)
		_, _ = w.Write([]byte(`<div class="incomes"><div class="row placeholder">Errore template</div></div>`))
	}
}

func (s *Server) handleIncomeFormReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()

	var categories []string
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		cats, err := adapter.GetIncomeCategories(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get income categories for form reset", "error", err)
		} else {
			categories = cats
		}
	}

	data := struct {
		Day        int
		Month      int
		Categories []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: categories,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "income_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Income form reset template execution failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) handleGetIncomeCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var categories []string
	if adapter, ok := s.expWriter.(*adapters.SQLiteAdapter); ok {
		cats, err := adapter.GetIncomeCategories(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get income categories", "error", err)
			http.Error(w, "Failed to get categories", http.StatusInternalServerError)
			return
		}
		categories = cats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}
