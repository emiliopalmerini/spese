package http

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
	"spese/internal/log"
)

// handleHealth performs basic liveness check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Basic health check - service is alive
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    time.Since(s.appMetrics.uptime).String(),
	}
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

// handleReady performs readiness check with dependency verification
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	
	status := "ready"
	httpStatus := http.StatusOK
	checks := make(map[string]interface{})
	
	// Check templates
	if s.templates == nil {
		checks["templates"] = "failed: templates not loaded"
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	} else {
		checks["templates"] = "ok"
	}
	
	// Check expense writer dependency
	if s.expWriter != nil {
		// For sheets backend, try a lightweight operation
		if ctx.Err() == nil {
			// Test with a dummy category list call (lightweight)
			_, _, err := s.taxReader.List(ctx)
			if err != nil {
				checks["expense_writer"] = fmt.Sprintf("failed: %v", err)
				status = "not_ready"
				httpStatus = http.StatusServiceUnavailable
			} else {
				checks["expense_writer"] = "ok"
			}
		} else {
			checks["expense_writer"] = "timeout"
			status = "not_ready"
			httpStatus = http.StatusServiceUnavailable
		}
	} else {
		checks["expense_writer"] = "not_configured"
		status = "not_ready"  
		httpStatus = http.StatusServiceUnavailable
	}
	
	// Check cache health
	overviewCacheSize := s.overviewCache.Size()
	itemsCacheSize := s.itemsCache.Size()
	checks["cache"] = map[string]interface{}{
		"overview_entries": overviewCacheSize,
		"items_entries":   itemsCacheSize,
		"status":          "ok",
	}
	
	// Check rate limiter
	activeClients := s.rateLimiter.ActiveClients()
	checks["rate_limiter"] = map[string]interface{}{
		"active_clients": activeClients,
		"status":        "ok",
	}
	
	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
		"checks":    checks,
	}
	
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// handleMetrics provides application and security metrics in plain text format
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	
	// Security metrics
	securityMetrics := s.securityDetector.GetMetrics()
	rateLimitMetrics := s.rateLimiter.GetMetrics()
	traceMetrics := s.traceMiddleware.GetMetrics()
	
	// Application metrics
	totalExpenses := atomic.LoadInt64(&s.appMetrics.totalExpenses)
	cacheHits := atomic.LoadInt64(&s.appMetrics.cacheHits)
	cacheMisses := atomic.LoadInt64(&s.appMetrics.cacheMisses)
	uptime := time.Since(s.appMetrics.uptime)
	
	// Cache statistics
	overviewCacheSize := s.overviewCache.Size()
	itemsCacheSize := s.itemsCache.Size()
	activeClients := s.rateLimiter.ActiveClients()
	
	w.WriteHeader(http.StatusOK)
	
	// Write metrics in Prometheus-like format
	fmt.Fprintf(w, "# HELP http_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
	fmt.Fprintf(w, "http_requests_total %d\n\n", traceMetrics.TotalRequests)
	
	fmt.Fprintf(w, "# HELP expenses_total Total number of expenses created\n")
	fmt.Fprintf(w, "# TYPE expenses_total counter\n")
	fmt.Fprintf(w, "expenses_total %d\n\n", totalExpenses)
	
	fmt.Fprintf(w, "# HELP cache_hits_total Total cache hits\n")
	fmt.Fprintf(w, "# TYPE cache_hits_total counter\n")
	fmt.Fprintf(w, "cache_hits_total %d\n\n", cacheHits)
	
	fmt.Fprintf(w, "# HELP cache_misses_total Total cache misses\n")
	fmt.Fprintf(w, "# TYPE cache_misses_total counter\n")
	fmt.Fprintf(w, "cache_misses_total %d\n\n", cacheMisses)
	
	fmt.Fprintf(w, "# HELP cache_entries Current cache entries\n")
	fmt.Fprintf(w, "# TYPE cache_entries gauge\n")
	fmt.Fprintf(w, "cache_entries{type=\"overview\"} %d\n", overviewCacheSize)
	fmt.Fprintf(w, "cache_entries{type=\"items\"} %d\n\n", itemsCacheSize)
	
	fmt.Fprintf(w, "# HELP rate_limit_hits_total Total rate limit hits\n")
	fmt.Fprintf(w, "# TYPE rate_limit_hits_total counter\n")
	fmt.Fprintf(w, "rate_limit_hits_total %d\n\n", rateLimitMetrics.TotalHits)
	
	fmt.Fprintf(w, "# HELP suspicious_requests_total Total suspicious requests detected\n")
	fmt.Fprintf(w, "# TYPE suspicious_requests_total counter\n")
	fmt.Fprintf(w, "suspicious_requests_total %d\n\n", securityMetrics.SuspiciousRequests)
	
	fmt.Fprintf(w, "# HELP active_rate_limit_clients Currently tracked rate limit clients\n")
	fmt.Fprintf(w, "# TYPE active_rate_limit_clients gauge\n")
	fmt.Fprintf(w, "active_rate_limit_clients %d\n\n", activeClients)
	
	fmt.Fprintf(w, "# HELP uptime_seconds Application uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE uptime_seconds gauge\n")
	fmt.Fprintf(w, "uptime_seconds %.0f\n\n", uptime.Seconds())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		s.logger.ErrorContext(r.Context(), "Templates not loaded", 
			log.FieldPath, r.URL.Path,
			log.FieldComponent, log.ComponentTemplate,
			"error_type", log.ErrorTypeConfiguration)
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	
	// For hierarchical categories, load only primaries initially
	var cats, subs []string
	var err error
	
	if _, ok := s.taxReader.(*adapters.SQLiteAdapter); ok {
		// For SQLite adapter, get only primary categories initially
		// Secondary categories will be loaded dynamically via HTMX
		cats, _, err = s.taxReader.List(r.Context())
		if err != nil {
			s.logger.ErrorContext(r.Context(), "Primary categories list error", "error", err)
		}
		// Leave subs empty - will be populated dynamically
		subs = []string{}
	} else {
		// For other adapters (memory, google sheets), load all as before
		cats, subs, err = s.taxReader.List(r.Context())
		if err != nil {
			s.logger.ErrorContext(r.Context(), "Taxonomy list error", "error", err)
		}
	}
	
	data := struct {
		Day        int
		Month      int
		Categories []string
		Subcats    []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: cats,
		Subcats:    subs,
	}

	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		s.logger.ErrorContext(r.Context(), "Index template execution failed", "error", err, "template", "index.html")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleCreateExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.logger.ErrorContext(r.Context(), "Parse form error", "error", err, "method", r.Method, "url", r.URL.Path)
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
		Date:        core.DateParts{Day: day, Month: month},
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
		s.logger.ErrorContext(r.Context(), "Failed to save expense", 
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
	
	// Track successful expense creation
	atomic.AddInt64(&s.appMetrics.totalExpenses, 1)
	
	// Log successful expense creation
	s.logger.InfoContext(r.Context(), "Expense created successfully",
		"expense_description", exp.Description,
		"amount_cents", exp.Amount.Cents,
		"primary_category", exp.Primary,
		"secondary_category", exp.Secondary,
		"sheets_ref", ref,
		"component", "expense_handler",
		"operation", "create")
    // Invalidate cache for current year+month and trigger client refresh
    year := time.Now().Year()
    s.invalidateOverview(year, month)
    s.invalidateExpenses(year, month)
	w.Header().Set("HX-Trigger", `{"expense:created": {"year": `+strconv.Itoa(year)+`, "month": `+strconv.Itoa(month)+`}}`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<div class="success">Spesa registrata (#` + template.HTMLEscapeString(ref) + `): ` +
		template.HTMLEscapeString(exp.Description) +
		` — €` + template.HTMLEscapeString(amountStr) +
		` (` + template.HTMLEscapeString(exp.Primary) + ` / ` + template.HTMLEscapeString(exp.Secondary) + `)</div>`))
}

// handleMonthOverview renders the monthly overview partial
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
	// Validate month range
	if month < 1 || month > 12 {
		s.logger.WarnContext(r.Context(), "Invalid month parameter", "year", year, "month", month, "corrected_to", int(now.Month()))
		month = int(now.Month())
	}
	ov, err := s.getOverview(r.Context(), year, month)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "Month overview error", "error", err, "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Error loading overview</div></section>`))
		return
	}
	if s.templates == nil {
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Totale: ` + formatEuros(ov.Total.Cents) + `</div></section>`))
		return
	}
	// Pass data to template
	// Compute max category for progress scaling and legend
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
		// Expenses detail list
		Items []struct{
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
			width = int((r.Amount.Cents*100 + maxCents/2) / maxCents) // rounded percent
			if width > 0 && width < 2 {                               // ensure visibility for very small values
				width = 2
			}
			if width > 100 {
				width = 100
			}
		}
		data.Rows = append(data.Rows, row{Name: r.Name, Amount: formatEuros(r.Amount.Cents), Width: width})
	}
	// Fetch detailed items (cached)
	if s.expLister != nil {
		items, err := s.getExpenses(r.Context(), year, month)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "List expenses error", "error", err, "year", year, "month", month)
		} else {
			for _, e := range items {
				data.Items = append(data.Items, struct{
					Day  int
					Desc string
					Amt  string
					Cat  string
					Sub  string
				}{Day: e.Date.Day, Desc: template.HTMLEscapeString(e.Description), Amt: formatEuros(e.Amount.Cents), Cat: e.Primary, Sub: e.Secondary})
			}
		}
	}
	if err := s.templates.ExecuteTemplate(w, "month_overview.html", data); err != nil {
		s.logger.ErrorContext(r.Context(), "Template execution error", "error", err, "template", "month_overview.html", "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Error rendering overview</div></section>`))
		return
	}
}

// handleGetSecondaryCategories returns secondary categories for a given primary category as HTML options
func (s *Server) handleGetSecondaryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	
	// Get primary category from query parameter
	primaryCategory := strings.TrimSpace(r.URL.Query().Get("primary"))
	if primaryCategory == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<option value="">Seleziona prima la categoria primaria</option>`))
		return
	}
	
	// Check if we have access to the SQLite repository through the adapter
	if sqliteAdapter, ok := s.taxReader.(*adapters.SQLiteAdapter); ok {
		// Use the hierarchical filtering method
		secondaries, err := sqliteAdapter.GetSecondariesByPrimary(r.Context(), primaryCategory)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "Failed to get secondary categories for primary", 
				"primary", primaryCategory, "error", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError) 
			_, _ = w.Write([]byte(`<option value="">Errore nel caricamento</option>`))
			return
		}
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		
		// Write empty option first
		_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))
		
		// Write filtered secondary categories as options
		for _, secondary := range secondaries {
			escapedSecondary := template.HTMLEscapeString(secondary)
			_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
		}
		
		s.logger.InfoContext(r.Context(), "Returned filtered secondary categories", 
			"primary", primaryCategory, 
			"count", len(secondaries))
		return
	}
	
	// Fallback for other adapters (memory, google sheets)
	_, secondaries, err := s.taxReader.List(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "Failed to get secondary categories", 
			"primary", primaryCategory, "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<option value="">Errore nel caricamento</option>`))
		return
	}
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	// Write empty option first  
	_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))
	
	// Write all secondary categories as options
	for _, secondary := range secondaries {
		escapedSecondary := template.HTMLEscapeString(secondary)
		_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
	}
}