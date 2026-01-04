package http

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"spese/internal/adapters"
	"spese/internal/sheets"
	appweb "spese/web"
)

type Server struct {
	http.Server
	templates       *template.Template
	expWriter       sheets.ExpenseWriter
	taxReader       sheets.TaxonomyReader
	dashReader      sheets.DashboardReader
	expLister       sheets.ExpenseLister
	expListerWithID sheets.ExpenseListerWithID
	expDeleter      sheets.ExpenseDeleter
	rateLimiter     *rateLimiter

	shutdownOnce sync.Once

	// Security and application metrics
	metrics    *securityMetrics
	appMetrics *applicationMetrics
}

// applicationMetrics tracks application performance and usage
type applicationMetrics struct {
	totalRequests       int64
	totalExpenses       int64
	averageResponseTime int64 // in microseconds
	uptime              time.Time
}

// GetSecurityMetrics returns current security metrics (useful for monitoring)
func (s *Server) GetSecurityMetrics() (rateLimitHits, invalidIPAttempts, suspiciousRequests int64) {
	return atomic.LoadInt64(&s.metrics.rateLimitHits),
		atomic.LoadInt64(&s.metrics.invalidIPAttempts),
		atomic.LoadInt64(&s.metrics.suspiciousRequests)
}

// Shutdown gracefully shuts down the server and cleanup routines
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	// Ensure shutdown logic runs only once
	s.shutdownOnce.Do(func() {
		// Stop rate limiter cleanup goroutine
		if s.rateLimiter != nil {
			s.rateLimiter.stop()
		}

		// Shutdown HTTP server
		shutdownErr = s.Server.Shutdown(ctx)
	})

	return shutdownErr
}

// NewServer configures routes and templates, returning a ready-to-run http.Server.
func NewServer(addr string, ew sheets.ExpenseWriter, tr sheets.TaxonomyReader, dr sheets.DashboardReader, lr sheets.ExpenseLister, ed sheets.ExpenseDeleter, lrwid sheets.ExpenseListerWithID) *Server {
	mux := http.NewServeMux()

	s := &Server{
		Server: http.Server{
			Addr:    addr,
			Handler: mux,
		},
		expWriter:       ew,
		taxReader:       tr,
		dashReader:      dr,
		expLister:       lr,
		expListerWithID: lrwid,
		expDeleter:      ed,
		rateLimiter:     newRateLimiter(),
		metrics:         &securityMetrics{},
		appMetrics:      &applicationMetrics{uptime: time.Now()},
	}

	// Parse embedded templates at startup with custom functions.
	funcMap := template.FuncMap{
		"divFloat": func(a, b int64) float64 { // Safe float division for template calculations
			return float64(a) / float64(b)
		},
		"formatDate": func(day, month, year int) string { // Format date components as DD/MM/YYYY
			return fmt.Sprintf("%02d/%02d/%d", day, month, year)
		},
		"not": func(v bool) bool { // Logical NOT for template conditionals
			return !v
		},
		"dict": func(values ...interface{}) map[string]interface{} { // Create map from key-value pairs for template data
			if len(values)%2 != 0 {
				return nil
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil
				}
				dict[key] = values[i+1]
			}
			return dict
		},
	}

	t, err := template.New("").Funcs(funcMap).ParseFS(appweb.TemplatesFS, "templates/**/*.html")
	if err != nil {
		slog.Error("Failed parsing templates", "error", err)
		panic(fmt.Sprintf("Failed to parse templates: %v", err))
	}
	s.templates = t

	// Static assets (served from embedded FS)
	if sub, err := fs.Sub(appweb.StaticFS, "static"); err == nil {
		static := http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
		mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Tiny cache for static assets
			w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
			static.ServeHTTP(w, r)
		}))
	} else {
		slog.Warn("Failed to mount embedded static FS", "error", err)
	}

	// Add security middleware
	mux.HandleFunc("/", s.withSecurityHeaders(s.handleIndex))
	mux.HandleFunc("/healthz", s.handleHealth)  // Updated to server method
	mux.HandleFunc("/readyz", s.handleReady)    // Updated to server method
	mux.HandleFunc("/metrics", s.handleMetrics) // Metrics endpoint (no auth for now)
	mux.HandleFunc("/expenses", s.withSecurityHeaders(s.handleCreateExpense))
	mux.HandleFunc("/expenses/delete", s.withSecurityHeaders(s.handleDeleteExpense))
	// UI partials
	mux.HandleFunc("/ui/month-overview", s.withSecurityHeaders(s.handleMonthOverview))
	mux.HandleFunc("/ui/month-total", s.withSecurityHeaders(s.handleMonthTotal))
	mux.HandleFunc("/ui/month-categories", s.withSecurityHeaders(s.handleMonthCategories))
	mux.HandleFunc("/ui/month-expenses", s.withSecurityHeaders(s.handleMonthExpenses))
	mux.HandleFunc("/ui/notifications", s.withSecurityHeaders(s.handleNotifications))
	mux.HandleFunc("/ui/form-reset", s.withSecurityHeaders(s.handleFormReset))
	mux.HandleFunc("/ui/recurrent-form-reset", s.withSecurityHeaders(s.handleRecurrentFormReset))
	mux.HandleFunc("/ui/recurrent-expenses-list", s.withSecurityHeaders(s.handleRecurrentExpensesList))
	mux.HandleFunc("/ui/recurrent-monthly-overview", s.withSecurityHeaders(s.handleRecurrentMonthlyOverview))
	mux.HandleFunc("/api/categories/secondary", s.withSecurityHeaders(s.handleGetSecondaryCategories))
	mux.HandleFunc("/api/categories", s.withSecurityHeaders(s.handleGetAllCategories))
	mux.HandleFunc("/api/income-categories", s.withSecurityHeaders(s.handleGetIncomeCategories))

	// Recurrent expenses routes
	mux.HandleFunc("/recurrent", s.withSecurityHeaders(s.handleRecurrentExpenses))
	mux.HandleFunc("/recurrent/create", s.withSecurityHeaders(s.handleCreateRecurrentExpense))
	mux.HandleFunc("/recurrent/update", s.withSecurityHeaders(s.handleUpdateRecurrentExpense))
	mux.HandleFunc("/recurrent/delete", s.withSecurityHeaders(s.handleDeleteRecurrentExpense))
	// Pattern for editing specific recurrent expense
	mux.HandleFunc("/recurrent/", s.withSecurityHeaders(s.handleRecurrentExpenseEdit))

	// Income routes
	mux.HandleFunc("/entrate", s.withSecurityHeaders(s.handleIncomes))
	mux.HandleFunc("/incomes", s.withSecurityHeaders(s.handleCreateIncome))
	mux.HandleFunc("/incomes/delete", s.withSecurityHeaders(s.handleDeleteIncome))
	// Income UI partials
	mux.HandleFunc("/ui/income-month-overview", s.withSecurityHeaders(s.handleIncomeMonthOverview))
	mux.HandleFunc("/ui/income-month-total", s.withSecurityHeaders(s.handleIncomeMonthTotal))
	mux.HandleFunc("/ui/income-month-categories", s.withSecurityHeaders(s.handleIncomeMonthCategories))
	mux.HandleFunc("/ui/income-month-incomes", s.withSecurityHeaders(s.handleIncomeMonthIncomes))
	mux.HandleFunc("/ui/income-form-reset", s.withSecurityHeaders(s.handleIncomeFormReset))

	return s
}

// withSecurityHeaders adds security headers, rate limiting, and request logging to responses
func (s *Server) withSecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract real client IP with proper validation
		clientIP := extractClientIP(r)

		// Generate request ID for tracing
		requestID := generateRequestID()

		// Detect suspicious request patterns
		if detectSuspiciousRequest(r, s.metrics) {
			slog.WarnContext(r.Context(), "Suspicious request detected",
				"request_id", requestID,
				"client_ip", clientIP,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"user_agent", r.Header.Get("User-Agent"),
				"action", "security_threat_detected")
		}

		// Add request context with metadata and request ID
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		// Enhanced structured request logging
		slog.InfoContext(ctx, "HTTP request started",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"client_ip", clientIP,
			"user_agent", r.Header.Get("User-Agent"),
			"referer", r.Header.Get("Referer"),
			"content_length", r.ContentLength,
			"protocol", r.Proto)

		// Apply rate limiting to POST requests (expense creation)
		if r.Method == http.MethodPost && !s.rateLimiter.allow(clientIP, s.metrics) {
			slog.WarnContext(ctx, "Rate limit exceeded",
				"request_id", requestID,
				"client_ip", clientIP,
				"method", r.Method,
				"path", r.URL.Path,
				"action", "rate_limit_blocked")
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		// Modern security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Enhanced Content Security Policy with stricter rules
		csp := "default-src 'self'; " +
			"script-src 'self' https://unpkg.com 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data:; " +
			"connect-src 'self'; " +
			"font-src 'self'; " +
			"object-src 'none'; " +
			"media-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		// Additional modern security headers
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

		// HSTS header (only for HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		// Track total requests
		atomic.AddInt64(&s.appMetrics.totalRequests, 1)

		// Create a custom response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(rw, r)

		// Enhanced request completion logging
		duration := time.Since(start)
		durationMs := duration.Milliseconds()

		// Update average response time
		atomic.StoreInt64(&s.appMetrics.averageResponseTime, durationMs*1000) // convert to microseconds

		// Use appropriate log level based on status code
		logLevel := slog.LevelInfo
		if rw.statusCode >= 400 && rw.statusCode < 500 {
			logLevel = slog.LevelWarn
		} else if rw.statusCode >= 500 {
			logLevel = slog.LevelError
		}

		slog.Log(ctx, logLevel, "HTTP request completed",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status_code", rw.statusCode,
			"duration_ms", durationMs,
			"duration_human", duration.String(),
			"client_ip", clientIP,
			"success", rw.statusCode < 400)
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

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

	// Check rate limiter
	s.rateLimiter.mu.Lock()
	activeClients := len(s.rateLimiter.clients)
	s.rateLimiter.mu.Unlock()

	checks["rate_limiter"] = map[string]interface{}{
		"active_clients": activeClients,
		"status":         "ok",
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
	rateLimitHits, _, suspiciousRequests := s.GetSecurityMetrics()

	// Application metrics
	totalRequests := atomic.LoadInt64(&s.appMetrics.totalRequests)
	totalExpenses := atomic.LoadInt64(&s.appMetrics.totalExpenses)
	uptime := time.Since(s.appMetrics.uptime)

	// Rate limiter statistics
	s.rateLimiter.mu.Lock()
	activeClients := len(s.rateLimiter.clients)
	s.rateLimiter.mu.Unlock()

	w.WriteHeader(http.StatusOK)

	// Write metrics in Prometheus-like format
	fmt.Fprintf(w, "# HELP http_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
	fmt.Fprintf(w, "http_requests_total %d\n\n", totalRequests)

	fmt.Fprintf(w, "# HELP expenses_total Total number of expenses created\n")
	fmt.Fprintf(w, "# TYPE expenses_total counter\n")
	fmt.Fprintf(w, "expenses_total %d\n\n", totalExpenses)
	fmt.Fprintf(w, "# HELP rate_limit_hits_total Total rate limit hits\n")
	fmt.Fprintf(w, "# TYPE rate_limit_hits_total counter\n")
	fmt.Fprintf(w, "rate_limit_hits_total %d\n\n", rateLimitHits)

	fmt.Fprintf(w, "# HELP suspicious_requests_total Total suspicious requests detected\n")
	fmt.Fprintf(w, "# TYPE suspicious_requests_total counter\n")
	fmt.Fprintf(w, "suspicious_requests_total %d\n\n", suspiciousRequests)

	fmt.Fprintf(w, "# HELP active_rate_limit_clients Currently tracked rate limit clients\n")
	fmt.Fprintf(w, "# TYPE active_rate_limit_clients gauge\n")
	fmt.Fprintf(w, "active_rate_limit_clients %d\n\n", activeClients)

	fmt.Fprintf(w, "# HELP uptime_seconds Application uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE uptime_seconds gauge\n")
	fmt.Fprintf(w, "uptime_seconds %.0f\n\n", uptime.Seconds())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		slog.ErrorContext(r.Context(), "Templates not loaded",
			"path", r.URL.Path,
			"component", "template_engine",
			"error_type", "configuration_error")
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
			slog.ErrorContext(r.Context(), "Primary categories list error", "error", err)
		}
		// Leave subs empty - will be populated dynamically
		subs = []string{}
	} else {
		// For other adapters (memory, google sheets), load all as before
		cats, subs, err = s.taxReader.List(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Taxonomy list error", "error", err)
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

	if err := s.templates.ExecuteTemplate(w, "index_page", data); err != nil {
		slog.ErrorContext(r.Context(), "Index template execution failed", "error", err, "template", "index_page")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleCreateExpense processes expense creation requests from the web form.
// It parses form data, validates input, creates an Expense entity, and saves it to the database.
// Returns HTMX-compatible HTML fragments for success or error states.
//
// Expected form fields:
//   - day: Day of month (1-31)
//   - month: Month (1-12)
//   - description: Expense description (required, max 200 chars)
//   - amount: Monetary amount (decimal string like "12.34")
//   - primary: Primary category (required)
//   - secondary: Secondary category (required)

// handleGetSecondaryCategories returns secondary categories for a given primary category as HTML options
// handleNotifications provides a dedicated endpoint for flash messages
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Get notification details from query params
	msgType := r.URL.Query().Get("type") // success, error, info
	message := r.URL.Query().Get("message")
	duration := r.URL.Query().Get("duration") // in milliseconds

	if message == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	data := struct {
		Type     string
		Message  string
		Duration string
	}{
		Type:     msgType,
		Message:  message,
		Duration: duration,
	}

	if err := s.templates.ExecuteTemplate(w, "notification", data); err != nil {
		slog.ErrorContext(r.Context(), "Notification template execution failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
