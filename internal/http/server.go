package http

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"spese/internal/cache"
	"spese/internal/core"
	"spese/internal/log"
	"spese/internal/middleware/ratelimit"
	"spese/internal/middleware/security"
	"spese/internal/middleware/trace"
	"spese/internal/sheets"
	appweb "spese/web"
)

// applicationMetrics tracks application performance and usage
type applicationMetrics struct {
	totalRequests       int64
	totalExpenses       int64
	cacheHits          int64
	cacheMisses        int64
	averageResponseTime int64  // in microseconds
	uptime             time.Time
}

type Server struct {
	http.Server
	logger      *log.Logger
	templates   *template.Template
	expWriter   sheets.ExpenseWriter
	taxReader   sheets.TaxonomyReader
	dashReader  sheets.DashboardReader
	expLister   sheets.ExpenseLister
	
	// Middleware components
	rateLimiter       *ratelimit.Limiter
	securityDetector  *security.Detector
	traceMiddleware   *trace.Middleware
	cacheManager      *cache.Manager
	headersMiddleware *security.HeadersMiddleware

	// LRU cache for month overviews with eviction policy
	overviewCache *cache.LRUCache[core.MonthOverview]
	itemsCache    *cache.LRUCache[[]core.Expense]
	
	// Application metrics
	appMetrics   *applicationMetrics
	shutdownOnce sync.Once
}

// NewServer configures routes and templates, returning a ready-to-run http.Server
func NewServer(addr string, logger *log.Logger, ew sheets.ExpenseWriter, tr sheets.TaxonomyReader, dr sheets.DashboardReader, lr sheets.ExpenseLister) *Server {
	mux := http.NewServeMux()

	// Initialize middleware components
	detector := security.NewDetector()
	rateLimiterConfig := ratelimit.DefaultConfig()
	rateLimiter := ratelimit.NewLimiter(rateLimiterConfig)
	
	traceMiddleware := trace.NewMiddleware(detector.ExtractClientIP)
	headersConfig := security.DefaultHeadersConfig()
	headersMiddleware := security.NewHeadersMiddleware(headersConfig)
	
	// Initialize cache components
	overviewCache := cache.NewLRUCache[core.MonthOverview](100, 5*time.Minute)
	itemsCache := cache.NewLRUCache[[]core.Expense](200, 5*time.Minute)
	
	cacheManager := cache.NewManager()
	cacheManager.Register(overviewCache)
	cacheManager.Register(itemsCache)
	cacheManager.StartCleanup(10 * time.Minute)

	s := &Server{
		Server: http.Server{
			Addr:    addr,
			Handler: mux,
		},
		logger:            logger,
		expWriter:         ew,
		taxReader:         tr,
		dashReader:        dr,
		expLister:         lr,
		rateLimiter:       rateLimiter,
		securityDetector:  detector,
		traceMiddleware:   traceMiddleware,
		cacheManager:      cacheManager,
		headersMiddleware: headersMiddleware,
		overviewCache:     overviewCache,
		itemsCache:        itemsCache,
		appMetrics:        &applicationMetrics{uptime: time.Now()},
	}

	// Parse embedded templates at startup
	t, err := template.ParseFS(appweb.TemplatesFS, "templates/*.html")
	if err != nil {
		logger.Warn("Failed parsing templates", log.FieldError, err, log.FieldComponent, log.ComponentTemplate)
	}
	s.templates = t

	// Static assets (served from embedded FS)
	if sub, err := fs.Sub(appweb.StaticFS, "static"); err == nil {
		staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
		staticWithCache := security.StaticAssetMiddleware(3600)(staticHandler)
		mux.Handle("/static/", staticWithCache)
	} else {
		logger.Warn("Failed to mount embedded static FS", log.FieldError, err, log.FieldComponent, log.ComponentHTTP)
	}

	// Setup middleware chain
	middlewareChain := func(handler http.HandlerFunc) http.Handler {
		// Apply middleware in order: logging -> trace -> headers -> rate limit (POST only) -> handler
		h := http.Handler(handler)
		h = s.rateLimitMiddleware(h)
		h = s.suspiciousRequestMiddleware(h)
		h = headersMiddleware.Middleware(h)
		h = traceMiddleware.Middleware(h)
		h = log.Middleware(logger.WithComponent(log.ComponentHTTP))(h)
		return h
	}

	// Routes with middleware
	mux.Handle("/", middlewareChain(s.handleIndex))
	mux.Handle("/expenses", middlewareChain(s.handleCreateExpense))
	mux.Handle("/ui/month-overview", middlewareChain(s.handleMonthOverview))
	mux.Handle("/api/categories/secondary", middlewareChain(s.handleGetSecondaryCategories))

	// Health/metrics endpoints without security middleware (for monitoring)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)

	return s
}

// rateLimitMiddleware applies rate limiting to POST requests only
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			clientIP := s.securityDetector.ExtractClientIP(r)
			if !s.rateLimiter.Allow(clientIP) {
				s.logger.WarnContext(r.Context(), "Rate limit exceeded",
					log.FieldClientIP, clientIP,
					log.FieldMethod, r.Method,
					log.FieldPath, r.URL.Path,
					log.FieldComponent, log.ComponentRateLimit)
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// suspiciousRequestMiddleware detects and logs suspicious requests
func (s *Server) suspiciousRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.securityDetector.DetectSuspiciousRequest(r) {
			clientIP := s.securityDetector.ExtractClientIP(r)
			requestID := trace.GetRequestID(r.Context())
			
			fields := log.NewFields().
				WithRequestID(requestID).
				WithClientIP(clientIP).
				WithHTTPRequest(r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("User-Agent"), "").
				WithComponent(log.ComponentSecurity)
			
			s.logger.WarnContext(r.Context(), "Suspicious request detected", fields.ToSlice()...)
		}
		next.ServeHTTP(w, r)
	})
}

// Shutdown gracefully shuts down the server and cleanup routines
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error
	
	// Ensure shutdown logic runs only once
	s.shutdownOnce.Do(func() {
		// Stop cache manager
		if s.cacheManager != nil {
			s.cacheManager.Stop()
		}
		
		// Stop rate limiter
		if s.rateLimiter != nil {
			s.rateLimiter.Stop()
		}
		
		// Shutdown HTTP server
		shutdownErr = s.Server.Shutdown(ctx)
	})
	
	return shutdownErr
}

// Cache management methods
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
		s.logger.DebugContext(ctx, "Overview cache hit", 
			log.FieldYear, year, 
			log.FieldMonth, month,
			log.FieldComponent, log.ComponentCache)
		atomic.AddInt64(&s.appMetrics.cacheHits, 1)
		return data, nil
	}
	
	// Track cache miss
	atomic.AddInt64(&s.appMetrics.cacheMisses, 1)
	
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
	s.logger.DebugContext(ctx, "Overview cached", 
		log.FieldYear, year, 
		log.FieldMonth, month, 
		"total_cents", data.Total.Cents, 
		"categories", len(data.ByCategory),
		log.FieldComponent, log.ComponentCache)
	return data, nil
}

func (s *Server) getExpenses(ctx context.Context, year, month int) ([]core.Expense, error) {
	key := s.cacheKey(year, month)
	
	// Check cache first
	if items, found := s.itemsCache.Get(key); found {
		s.logger.DebugContext(ctx, "Expenses cache hit", 
			log.FieldYear, year, 
			log.FieldMonth, month, 
			"count", len(items),
			log.FieldComponent, log.ComponentCache)
		atomic.AddInt64(&s.appMetrics.cacheHits, 1)
		// Return a copy to prevent external mutation
		result := make([]core.Expense, len(items))
		copy(result, items)
		return result, nil
	}
	
	// Track cache miss
	atomic.AddInt64(&s.appMetrics.cacheMisses, 1)
	
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
	s.logger.DebugContext(ctx, "Expenses cached", 
		log.FieldYear, year, 
		log.FieldMonth, month, 
		"count", len(items),
		log.FieldComponent, log.ComponentCache)
	return items, nil
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