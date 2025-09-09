package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
	"spese/internal/sheets"
	"spese/internal/storage"
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

// securityMetrics tracks security-related events
type securityMetrics struct {
	rateLimitHits      int64
	invalidIPAttempts  int64
	suspiciousRequests int64
}

// applicationMetrics tracks application performance and usage
type applicationMetrics struct {
	totalRequests       int64
	totalExpenses       int64
	averageResponseTime int64 // in microseconds
	uptime              time.Time
}

// Trusted proxy networks (common reverse proxies)
var trustedProxies = []*net.IPNet{
	parsecidr("127.0.0.0/8"),    // localhost
	parsecidr("10.0.0.0/8"),     // private networks
	parsecidr("172.16.0.0/12"),  // private networks
	parsecidr("192.168.0.0/16"), // private networks
}

// parsecidr is a helper to parse CIDR during initialization
func parsecidr(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse trusted proxy CIDR %s: %v", cidr, err))
	}
	return network
}

// isTrustedProxy checks if an IP is from a trusted proxy
func isTrustedProxy(ip net.IP) bool {
	for _, network := range trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// extractClientIP extracts the real client IP, validating forwarded headers
func extractClientIP(r *http.Request) string {
	// Start with the direct connection IP
	directIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If parsing fails, use RemoteAddr as-is (fallback)
		directIP = r.RemoteAddr
	}

	parsedDirectIP := net.ParseIP(directIP)
	if parsedDirectIP == nil {
		return directIP // Fallback to original if parsing fails
	}

	// If direct connection is from trusted proxy, check forwarded headers
	if isTrustedProxy(parsedDirectIP) {
		// Check X-Forwarded-For header (most common)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				clientIP := strings.TrimSpace(ips[0])
				if parsedIP := net.ParseIP(clientIP); parsedIP != nil {
					return clientIP
				}
			}
		}

		// Check X-Real-IP header (nginx)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if parsedIP := net.ParseIP(xri); parsedIP != nil {
				return xri
			}
		}
	}

	// Return direct IP if no valid forwarded IP found
	return directIP
}

// GetSecurityMetrics returns current security metrics (useful for monitoring)
func (s *Server) GetSecurityMetrics() (rateLimitHits, invalidIPAttempts, suspiciousRequests int64) {
	return atomic.LoadInt64(&s.metrics.rateLimitHits),
		atomic.LoadInt64(&s.metrics.invalidIPAttempts),
		atomic.LoadInt64(&s.metrics.suspiciousRequests)
}

// detectSuspiciousRequest analyzes request patterns for potential threats
func detectSuspiciousRequest(r *http.Request, metrics *securityMetrics) bool {
	suspicious := false

	// Check for common attack patterns in URL path
	path := strings.ToLower(r.URL.Path)
	suspiciousPatterns := []string{
		"../", "..\\", ".env", "wp-admin", "phpmyadmin",
		"admin.php", "config.php", ".git", ".ssh",
		"eval(", "javascript:", "<script", "union select",
		"base64", "0x", "etc/passwd", "cmd.exe",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(path, pattern) {
			suspicious = true
			break
		}
	}

	// Check for suspicious query parameters
	query := strings.ToLower(r.URL.RawQuery)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(query, pattern) {
			suspicious = true
			break
		}
	}

	// Check User-Agent for common bot patterns
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	suspiciousAgents := []string{
		"sqlmap", "nmap", "nikto", "gobuster", "dirb",
		"curl", "wget", "python-requests", "scanner",
		"bot", "crawler", "spider", "scraper",
	}

	for _, agent := range suspiciousAgents {
		if strings.Contains(userAgent, agent) {
			suspicious = true
			break
		}
	}

	// Check for unusual HTTP methods
	unusualMethods := []string{"TRACE", "TRACK", "DEBUG", "CONNECT"}
	for _, method := range unusualMethods {
		if r.Method == method {
			suspicious = true
			break
		}
	}

	// Check for excessively long URLs (possible overflow attempt)
	if len(r.URL.String()) > 2048 {
		suspicious = true
	}

	// Check for suspicious headers
	if r.Header.Get("X-Forwarded-For") != "" && r.Header.Get("X-Real-IP") != "" {
		// Multiple forwarding headers might indicate header manipulation
		xff := r.Header.Get("X-Forwarded-For")
		if strings.Count(xff, ",") > 5 { // More than 5 proxy hops is suspicious
			suspicious = true
		}
	}

	if suspicious && metrics != nil {
		atomic.AddInt64(&metrics.suspiciousRequests, 1)
	}

	return suspicious
}

// Simple in-memory rate limiter
type rateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientInfo
	stopCleanup  chan struct{}
	shutdownOnce sync.Once
}

type clientInfo struct {
	lastRequest time.Time
	requests    int
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		clients:     make(map[string]*clientInfo),
		stopCleanup: make(chan struct{}),
	}
	go rl.startCleanup()
	return rl
}

// startCleanup runs periodic cleanup to remove stale client entries
func (rl *rateLimiter) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupStaleEntries()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanupStaleEntries removes client entries older than 10 minutes
func (rl *rateLimiter) cleanupStaleEntries() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, client := range rl.clients {
		if client.lastRequest.Before(cutoff) {
			delete(rl.clients, ip)
		}
	}
}

// stop gracefully shuts down the rate limiter cleanup goroutine
func (rl *rateLimiter) stop() {
	rl.shutdownOnce.Do(func() {
		if rl.stopCleanup != nil {
			close(rl.stopCleanup)
		}
	})
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

// allow checks if a request from the given IP should be allowed
func (rl *rateLimiter) allow(clientIP string, metrics *securityMetrics) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	client, exists := rl.clients[clientIP]

	if !exists {
		rl.clients[clientIP] = &clientInfo{
			lastRequest: now,
			requests:    1,
		}
		return true
	}

	// Reset counter if more than 1 minute has passed
	if now.Sub(client.lastRequest) > time.Minute {
		client.requests = 1
		client.lastRequest = now
		return true
	}

	// Allow up to 60 requests per minute
	client.requests++
	client.lastRequest = now

	if client.requests > 60 {
		// Track rate limit hit
		if metrics != nil {
			atomic.AddInt64(&metrics.rateLimitHits, 1)
		}
		return false
	}

	return true
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
		"divFloat": func(a, b int64) float64 {
			return float64(a) / float64(b)
		},
		"formatDate": func(day, month, year int) string {
			return fmt.Sprintf("%02d/%02d/%d", day, month, year)
		},
		"dict": func(values ...interface{}) map[string]interface{} {
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
	mux.HandleFunc("/ui/recurrent-expenses-list", s.withSecurityHeaders(s.handleRecurrentExpensesList))
	mux.HandleFunc("/ui/recurrent-monthly-overview", s.withSecurityHeaders(s.handleRecurrentMonthlyOverview))
	mux.HandleFunc("/api/categories/secondary", s.withSecurityHeaders(s.handleGetSecondaryCategories))

	// Recurrent expenses routes
	mux.HandleFunc("/recurrent", s.withSecurityHeaders(s.handleRecurrentExpenses))
	mux.HandleFunc("/recurrent/create", s.withSecurityHeaders(s.handleCreateRecurrentExpense))
	mux.HandleFunc("/recurrent/update", s.withSecurityHeaders(s.handleUpdateRecurrentExpense))
	mux.HandleFunc("/recurrent/delete", s.withSecurityHeaders(s.handleDeleteRecurrentExpense))
	// Pattern for editing specific recurrent expense
	mux.HandleFunc("/recurrent/", s.withSecurityHeaders(s.handleRecurrentExpenseEdit))

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

// generateRequestID creates a unique request ID for tracing
func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes)
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

	// Track successful expense creation
	atomic.AddInt64(&s.appMetrics.totalExpenses, 1)

	// Log successful expense creation
	slog.InfoContext(r.Context(), "Expense created successfully",
		"expense_description", exp.Description,
		"amount_cents", exp.Amount.Cents,
		"primary_category", exp.Primary,
		"secondary_category", exp.Secondary,
		"sheets_ref", ref,
		"component", "expense_handler",
		"operation", "create")
	// Trigger client refresh for form, overview and list + show notification
	year := time.Now().Year()
	successMsg := fmt.Sprintf("Spesa registrata (#%s): %s — €%s (%s / %s)", 
		template.HTMLEscapeString(ref), 
		template.HTMLEscapeString(exp.Description),
		template.HTMLEscapeString(amountStr),
		template.HTMLEscapeString(exp.Primary), 
		template.HTMLEscapeString(exp.Secondary))
	
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"expense:created": {"year": %d, "month": %d},
		"form:reset": {},
		"overview:refresh": {"year": %d, "month": %d},
		"show-notification": {"type": "success", "message": "%s", "duration": 3000}
	}`, year, month, year, month, template.JSEscapeString(successMsg)))
	
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("")) // Empty response, notifications handled via JavaScript
}

func (s *Server) handleDeleteExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.Header().Set("Allow", "DELETE, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var expenseID string
	contentType := r.Header.Get("Content-Type")
	
	// Handle different request formats
	// HTMX with hx-vals sends JSON but may not set application/json content-type
	if strings.Contains(contentType, "application/json") || r.Method == http.MethodDelete {
		// Read body as bytes first to handle potential parsing issues
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
		
		// Try to parse as JSON
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
			// Fall back to form parsing if body doesn't look like JSON
			slog.InfoContext(r.Context(), "Body doesn't look like JSON, trying form parsing", "body", string(body))
			
			// Since we already read the body, we need to recreate the form data
			// Parse form-encoded data manually from the body
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
		// Handle form data (traditional POST requests)
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

	// Track successful expense deletion
	atomic.AddInt64(&s.appMetrics.totalExpenses, -1)

	// Log successful expense deletion
	slog.InfoContext(r.Context(), "Expense deleted successfully",
		"expense_id", expenseID,
		"component", "expense_handler",
		"operation", "delete")

	// Trigger client refresh + show notification
	now := time.Now()
	year := now.Year()
	month := int(now.Month())
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{
		"expense:deleted": {"year": %d, "month": %d},
		"overview:refresh": {"year": %d, "month": %d},
		"show-notification": {"type": "success", "message": "Spesa cancellata con successo", "duration": 2000}
	}`, year, month, year, month))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("")) // Empty response - item will be removed via targeting
}

// sanitizeInput removes potentially dangerous characters and trims whitespace
func sanitizeInput(s string) string {
	s = strings.TrimSpace(s)
	// Remove control characters except tab, newline, carriage return
	result := strings.Map(func(r rune) rune {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return -1 // remove character
		}
		return r
	}, s)
	return result
}

func (s *Server) getOverview(ctx context.Context, year, month int) (core.MonthOverview, error) {
	if s.dashReader == nil {
		return core.MonthOverview{Year: year, Month: month}, nil
	}
	// Add a small timeout to avoid hanging partials
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

// handleMonthOverview renders the monthly overview partial.
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
		Items []struct {
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
	// Fetch detailed items with IDs (cached)
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
				}{ID: e.ID, Day: e.Expense.Date.Day, Desc: template.HTMLEscapeString(e.Expense.Description), Amt: formatEuros(e.Expense.Amount.Cents), Cat: e.Expense.Primary, Sub: e.Expense.Secondary})
			}
		}
	}
	if err := s.templates.ExecuteTemplate(w, "month_overview.html", data); err != nil {
		slog.ErrorContext(r.Context(), "Template execution error", "error", err, "template", "month_overview.html", "year", year, "month", month)
		_, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Error rendering overview</div></section>`))
		return
	}
}

// handleGetSecondaryCategories returns secondary categories for a given primary category as HTML options
// Recurrent Expenses Handlers

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

	var endDate core.DateParts
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

	// Trigger client refresh for HTMX
	w.Header().Set("HX-Trigger", `{"recurrent:created": {}}`)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`<div class="success">Spesa ricorrente creata con successo</div>`))
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

	var endDate core.DateParts
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

// Helper function to parse date from string (YYYY-MM-DD format)
func parseDate(dateStr string) (core.DateParts, error) {
	parts := strings.Split(dateStr, "-")
	if len(parts) != 3 {
		return core.DateParts{}, errors.New("invalid date format")
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return core.DateParts{}, err
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return core.DateParts{}, err
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return core.DateParts{}, err
	}

	return core.DateParts{Year: year, Month: month, Day: day}, nil
}

func (s *Server) handleGetSecondaryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get primary category from query parameter or form data
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

	// Check if we have access to the SQLite repository through the adapter
	if sqliteAdapter, ok := s.taxReader.(*adapters.SQLiteAdapter); ok {
		// Use the hierarchical filtering method
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

		// Write empty option first
		_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))

		// Write filtered secondary categories as options
		for _, secondary := range secondaries {
			escapedSecondary := template.HTMLEscapeString(secondary)
			_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
		}

		slog.InfoContext(r.Context(), "Returned filtered secondary categories",
			"primary", primaryCategory,
			"count", len(secondaries))
		return
	}

	// Fallback for other adapters (memory, google sheets)
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

	// Write empty option first
	_, _ = w.Write([]byte(`<option value="">Seleziona sottocategoria</option>`))

	// Write all secondary categories as options
	for _, secondary := range secondaries {
		escapedSecondary := template.HTMLEscapeString(secondary)
		_, _ = w.Write([]byte(fmt.Sprintf(`<option value="%s">%s</option>`, escapedSecondary, escapedSecondary)))
	}
}

func formatEuros(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	euros := cents / 100
	rem := cents % 100
	s := strconv.FormatInt(euros, 10) + "," + fmt.Sprintf("%02d", rem)
	if neg {
		return "-€" + s
	}
	return "€" + s
}

// handleRecurrentExpensesList renders the recurrent expenses list partial
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

// handleRecurrentMonthlyOverview renders the recurrent expenses monthly overview partial
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

// handleRecurrentExpenseEdit handles GET /recurrent/{id}/edit for inline editing
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

// handleFormReset returns a fresh form after successful submission
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

// handleMonthTotal returns only the total amount for the month
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

// handleMonthCategories returns only the category breakdown
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

	// Calculate category data
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

// handleMonthExpenses returns only the expense list
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

	// Fetch detailed items with IDs
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
					Day:  e.Expense.Date.Day,
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
