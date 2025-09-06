package http

import (
	"container/list"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"spese/internal/core"
	"spese/internal/sheets"
	appweb "spese/web"
)

// LRU cache with TTL and size-based eviction
type lruCache[T any] struct {
	mu       sync.Mutex
	maxSize  int
	ttl      time.Duration
	items    map[string]*list.Element
	lru      *list.List
}

type cacheItem[T any] struct {
	key       string
	data      T
	expiresAt time.Time
}

func newLRUCache[T any](maxSize int, ttl time.Duration) *lruCache[T] {
	return &lruCache[T]{
		maxSize: maxSize,
		ttl:     ttl,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}
}

func (c *lruCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero T
	elem, exists := c.items[key]
	if !exists {
		return zero, false
	}

	item := elem.Value.(*cacheItem[T])
	
	// Check if expired
	if time.Now().After(item.expiresAt) {
		c.removeElement(elem)
		return zero, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	return item.data, true
}

func (c *lruCache[T]) Set(key string, data T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	item := &cacheItem[T]{
		key:       key,
		data:      data,
		expiresAt: now.Add(c.ttl),
	}

	// Check if key already exists
	if elem, exists := c.items[key]; exists {
		elem.Value = item
		c.lru.MoveToFront(elem)
		return
	}

	// Add new item
	elem := c.lru.PushFront(item)
	c.items[key] = elem

	// Evict if over capacity
	if c.lru.Len() > c.maxSize {
		oldest := c.lru.Back()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}
}

func (c *lruCache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
	}
}

func (c *lruCache[T]) removeElement(elem *list.Element) {
	item := elem.Value.(*cacheItem[T])
	delete(c.items, item.key)
	c.lru.Remove(elem)
}

// CleanExpired removes all expired entries
func (c *lruCache[T]) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element
	
	for elem := c.lru.Front(); elem != nil; elem = elem.Next() {
		item := elem.Value.(*cacheItem[T])
		if now.After(item.expiresAt) {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		c.removeElement(elem)
	}

	return len(toRemove)
}

type Server struct {
    http.Server
    templates   *template.Template
    expWriter   sheets.ExpenseWriter
    taxReader   sheets.TaxonomyReader
    dashReader  sheets.DashboardReader
    expLister   sheets.ExpenseLister
    rateLimiter *rateLimiter

    // LRU cache for month overviews with eviction policy
    overviewCache *lruCache[core.MonthOverview]
    itemsCache    *lruCache[[]core.Expense]
    
    // Cache cleanup management
    stopCacheCleanup chan struct{}
    shutdownOnce     sync.Once
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

// startCacheCleanup runs periodic cleanup for both caches
func (s *Server) startCacheCleanup() {
	ticker := time.NewTicker(10 * time.Minute) // Cleanup every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			overviewCleaned := s.overviewCache.CleanExpired()
			itemsCleaned := s.itemsCache.CleanExpired()
			if overviewCleaned > 0 || itemsCleaned > 0 {
				slog.Debug("Cache cleanup completed", 
					"overview_entries_removed", overviewCleaned,
					"items_entries_removed", itemsCleaned)
			}
		case <-s.stopCacheCleanup:
			return
		}
	}
}

// Shutdown gracefully shuts down the server and cleanup routines
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error
	
	// Ensure shutdown logic runs only once
	s.shutdownOnce.Do(func() {
		// Stop cache cleanup goroutine
		if s.stopCacheCleanup != nil {
			close(s.stopCacheCleanup)
		}
		
		// Stop rate limiter cleanup goroutine  
		if s.rateLimiter != nil {
			s.rateLimiter.stop()
		}
		
		// Shutdown HTTP server
		shutdownErr = s.Server.Shutdown(ctx)
	})
	
	return shutdownErr
}

func (rl *rateLimiter) allow(clientIP string) bool {
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

	return client.requests <= 60
}

// NewServer configures routes and templates, returning a ready-to-run http.Server.
func NewServer(addr string, ew sheets.ExpenseWriter, tr sheets.TaxonomyReader, dr sheets.DashboardReader, lr sheets.ExpenseLister) *Server {
    mux := http.NewServeMux()

    s := &Server{
        Server: http.Server{
            Addr:    addr,
            Handler: mux,
        },
        expWriter:     ew,
        taxReader:     tr,
        dashReader:    dr,
        expLister:     lr,
        rateLimiter:      newRateLimiter(),
        overviewCache:    newLRUCache[core.MonthOverview](100, 5*time.Minute), // Max 100 entries, 5min TTL
        itemsCache:       newLRUCache[[]core.Expense](200, 5*time.Minute),     // Max 200 entries, 5min TTL
        stopCacheCleanup: make(chan struct{}),
    }

    // Start periodic cache cleanup
    go s.startCacheCleanup()

	// Parse embedded templates at startup.
	t, err := template.ParseFS(appweb.TemplatesFS, "templates/*.html")
	if err != nil {
		slog.Warn("Failed parsing templates", "error", err)
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
	mux.HandleFunc("/healthz", handleHealth)
	mux.HandleFunc("/readyz", handleReady)
	mux.HandleFunc("/expenses", s.withSecurityHeaders(s.handleCreateExpense))
	// UI partials
	mux.HandleFunc("/ui/month-overview", s.withSecurityHeaders(s.handleMonthOverview))

	return s
}


// withSecurityHeaders adds security headers, rate limiting, and request logging to responses
func (s *Server) withSecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Extract client IP (considering proxies)
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.Header.Get("X-Real-IP")
		}
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Generate request ID for tracing
		requestID := generateRequestID()
		
		// Add request context with metadata and request ID
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)
		
		slog.InfoContext(ctx, "Request started", 
			"request_id", requestID,
			"method", r.Method, 
			"url", r.URL.Path, 
			"client_ip", clientIP, 
			"user_agent", r.Header.Get("User-Agent"))

		// Apply rate limiting to POST requests (expense creation)
		if r.Method == http.MethodPost && !s.rateLimiter.allow(clientIP) {
			slog.WarnContext(ctx, "Rate limit exceeded", "client_ip", clientIP, "method", r.Method, "url", r.URL.Path)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://unpkg.com 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Create a custom response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(rw, r)
		
		// Log request completion
		duration := time.Since(start)
		slog.InfoContext(ctx, "Request completed", 
			"request_id", requestID,
			"method", r.Method, 
			"url", r.URL.Path, 
			"status", rw.statusCode, 
			"duration_ms", duration.Milliseconds(),
			"client_ip", clientIP)
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

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	// In futuro: aggiungere check integrazione Sheets.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		slog.ErrorContext(r.Context(), "Templates not loaded", "url", r.URL.Path)
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	cats, subs, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Taxonomy list error", "error", err)
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
		slog.ErrorContext(r.Context(), "Index template execution failed", "error", err, "template", "index.html")
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
		_, _ = w.Write([]byte(`<div class="error">Dati non validi: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}

	ref, err := s.expWriter.Append(r.Context(), exp)
	if err != nil {
		slog.ErrorContext(r.Context(), "Expense append error", "error", err, "expense", exp.Description, "amount", exp.Amount.Cents)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nel salvataggio</div>`))
		return
	}
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

func (s *Server) cacheKey(year, month int) string {
	return strconv.Itoa(year) + "-" + strconv.Itoa(month)
}

func (s *Server) invalidateOverview(year, month int) {
    s.overviewCache.Delete(s.cacheKey(year, month))
}

func (s *Server) invalidateExpenses(year, month int) {
    s.itemsCache.Delete(s.cacheKey(year, month))
}

func (s *Server) getOverview(ctx context.Context, year, month int) (core.MonthOverview, error) {
    key := s.cacheKey(year, month)
    
    // Check cache first
    if data, found := s.overviewCache.Get(key); found {
        slog.DebugContext(ctx, "Overview cache hit", "year", year, "month", month)
        return data, nil
    }
    
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
    
    // Cache the result
    s.overviewCache.Set(key, data)
    slog.DebugContext(ctx, "Overview cached", "year", year, "month", month, "total_cents", data.Total.Cents, "categories", len(data.ByCategory))
    return data, nil
}

func (s *Server) getExpenses(ctx context.Context, year, month int) ([]core.Expense, error) {
    key := s.cacheKey(year, month)
    
    // Check cache first
    if items, found := s.itemsCache.Get(key); found {
        slog.DebugContext(ctx, "Expenses cache hit", "year", year, "month", month, "count", len(items))
        // Return a copy to prevent external mutation
        result := make([]core.Expense, len(items))
        copy(result, items)
        return result, nil
    }
    
    if s.expLister == nil {
        return nil, nil
    }
    cctx, cancel := context.WithTimeout(ctx, 7*time.Second)
    defer cancel()
    items, err := s.expLister.ListExpenses(cctx, year, month)
    if err != nil {
        return nil, fmt.Errorf("list month expenses (year=%d, month=%d): %w", year, month, err)
    }
    
    // Cache the result
    s.itemsCache.Set(key, items)
    slog.DebugContext(ctx, "Expenses cached", "year", year, "month", month, "count", len(items))
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
        _, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Errore caricando panoramica</div></section>`))
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
            slog.ErrorContext(r.Context(), "List expenses error", "error", err, "year", year, "month", month)
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
        slog.ErrorContext(r.Context(), "Template execution error", "error", err, "template", "month_overview.html", "year", year, "month", month)
        _, _ = w.Write([]byte(`<section id="month-overview" class="month-overview"><div class="placeholder">Errore rendering panoramica</div></section>`))
        return
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
