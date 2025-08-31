package http

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"spese/internal/core"
	"spese/internal/sheets"
	appweb "spese/web"
)

type Server struct {
	http.Server
	templates   *template.Template
	expWriter   sheets.ExpenseWriter
	taxReader   sheets.TaxonomyReader
	dashReader  sheets.DashboardReader
	rateLimiter *rateLimiter

	// cache for month overviews
	cacheMu       sync.Mutex
	overviewCache map[string]cachedOverview
	cacheTTL      time.Duration
}

// Simple in-memory rate limiter
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientInfo
}

type clientInfo struct {
	lastRequest time.Time
	requests    int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		clients: make(map[string]*clientInfo),
	}
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
func NewServer(addr string, ew sheets.ExpenseWriter, tr sheets.TaxonomyReader, dr sheets.DashboardReader) *Server {
	mux := http.NewServeMux()

	s := &Server{
		Server: http.Server{
			Addr:    addr,
			Handler: mux,
		},
		expWriter:     ew,
		taxReader:     tr,
		dashReader:    dr,
		rateLimiter:   newRateLimiter(),
		overviewCache: make(map[string]cachedOverview),
		cacheTTL:      5 * time.Minute,
	}

	// Parse embedded templates at startup.
	t, err := template.ParseFS(appweb.TemplatesFS, "templates/*.html")
	if err != nil {
		log.Printf("warning: failed parsing templates: %v", err)
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
		log.Printf("warning: failed to mount embedded static FS: %v", err)
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

// withSecurityHeaders adds security headers and rate limiting to responses
func (s *Server) withSecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract client IP (considering proxies)
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.Header.Get("X-Real-IP")
		}
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Apply rate limiting to POST requests (expense creation)
		if r.Method == http.MethodPost && !s.rateLimiter.allow(clientIP) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://unpkg.com; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next(w, r)
	}
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
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	cats, subs, err := s.taxReader.List(r.Context())
	if err != nil {
		log.Printf("taxonomy list error: %v", err)
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
		log.Printf("parse form error: %v", err)
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
		log.Printf("append error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div class="error">Errore nel salvataggio</div>`))
		return
	}
	// Invalidate cache for current year+month and trigger client refresh
	year := time.Now().Year()
	s.invalidateOverview(year, month)
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

type cachedOverview struct {
	at   time.Time
	data core.MonthOverview
}

func (s *Server) cacheKey(year, month int) string {
	return strconv.Itoa(year) + "-" + strconv.Itoa(month)
}

func (s *Server) invalidateOverview(year, month int) {
	s.cacheMu.Lock()
	delete(s.overviewCache, s.cacheKey(year, month))
	s.cacheMu.Unlock()
}

func (s *Server) getOverview(ctx context.Context, year, month int) (core.MonthOverview, error) {
	key := s.cacheKey(year, month)
	now := time.Now()
	s.cacheMu.Lock()
	if c, ok := s.overviewCache[key]; ok && now.Sub(c.at) < s.cacheTTL {
		data := c.data
		s.cacheMu.Unlock()
		return data, nil
	}
	s.cacheMu.Unlock()
	if s.dashReader == nil {
		return core.MonthOverview{Year: year, Month: month}, nil
	}
	data, err := s.dashReader.ReadMonthOverview(ctx, year, month)
	if err != nil {
		return core.MonthOverview{}, err
	}
	s.cacheMu.Lock()
	s.overviewCache[key] = cachedOverview{at: now, data: data}
	s.cacheMu.Unlock()
	return data, nil
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
	ov, err := s.getOverview(r.Context(), year, month)
	if err != nil {
		log.Printf("month overview error: %v", err)
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
	if err := s.templates.ExecuteTemplate(w, "month_overview.html", data); err != nil {
		log.Printf("template error: %v", err)
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
