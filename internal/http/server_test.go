package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"spese/internal/core"
	"strings"
	"testing"
	"time"

	ports "spese/internal/sheets"
)

type fakeTax struct{ cats, subs []string }

func (f fakeTax) List(ctx context.Context) ([]string, []string, error) { return f.cats, f.subs, nil }

type fakeTaxErr struct{}

func (fakeTaxErr) List(ctx context.Context) ([]string, []string, error) {
	return nil, nil, context.DeadlineExceeded
}

type fakeExp struct{}

func (fakeExp) Append(ctx context.Context, e core.Expense) (string, error) { return "mem:1", nil }

type fakeExpErr struct{}

func (fakeExpErr) Append(ctx context.Context, e core.Expense) (string, error) {
	return "", context.DeadlineExceeded
}

type badReader struct{}

func (badReader) Read(p []byte) (int, error) { return 0, context.DeadlineExceeded }

type fakeDash struct {
	ov  core.MonthOverview
	err error
}

func (f fakeDash) ReadMonthOverview(ctx context.Context, year int, month int) (core.MonthOverview, error) {
	if f.err != nil {
		return core.MonthOverview{}, f.err
	}
	if f.ov.Year == 0 {
		f.ov.Year = year
	}
	if f.ov.Month == 0 {
		f.ov.Month = month
	}
	return f.ov, nil
}

// Test month overview endpoint
func TestHandleMonthOverview(t *testing.T) {
	chdirRepoRoot(t)

	// Test successful overview
	mockOverview := core.MonthOverview{
		Year:  2025,
		Month: 1,
		Total: core.Money{Cents: 12345}, // €123.45
		ByCategory: []core.CategoryAmount{
			{Name: "Food", Amount: core.Money{Cents: 8000}},      // €80.00
			{Name: "Transport", Amount: core.Money{Cents: 4345}}, // €43.45
		},
	}
	mockExpensesWithID := []ports.ExpenseWithID{
		{ID: "1", Expense: core.Expense{Date: core.NewDate(2025, 1, 1), Description: "Groceries", Amount: core.Money{Cents: 5000}, Primary: "Food", Secondary: "Supermarket"}},
		{ID: "2", Expense: core.Expense{Date: core.NewDate(2025, 1, 2), Description: "Bus ticket", Amount: core.Money{Cents: 345}, Primary: "Transport", Secondary: "Public"}},
	}

	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"Food", "Transport"}, subs: []string{"Supermarket", "Public"}}
	var dr ports.DashboardReader = fakeDash{ov: mockOverview}
	var lr ports.ExpenseLister = fakeList{}
	var lrWithID ports.ExpenseListerWithID = fakeListWithID{items: mockExpensesWithID}
	srv := NewServer(":0", ew, tr, dr, lr, nil, lrWithID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/month-overview", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected HTML content type, got %s", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "€123,45") {
		t.Fatalf("expected total amount in body, got: %s", body)
	}
	if !strings.Contains(body, "Food") {
		t.Fatalf("expected Food category in body, got: %s", body)
	}
	if !strings.Contains(body, "Groceries") {
		t.Fatalf("expected expense details in body, got: %s", body)
	}
}

// Test month overview with query parameters
func TestHandleMonthOverviewWithParams(t *testing.T) {
	chdirRepoRoot(t)
	mockOverview := core.MonthOverview{Year: 2024, Month: 6, Total: core.Money{Cents: 5000}}

	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{}
	var dr ports.DashboardReader = fakeDash{ov: mockOverview}
	var lr ports.ExpenseLister = fakeList{}
	srv := NewServer(":0", ew, tr, dr, lr, nil, nil)

	// Test with valid year/month params
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/month-overview?year=2024&month=6", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Test with invalid month (should default to current)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/month-overview?month=13", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Test with invalid month (should default to current)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/month-overview?month=0", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// Test month overview error handling
func TestHandleMonthOverviewErrors(t *testing.T) {
	chdirRepoRoot(t)

	// Test dashboard read error
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{}
	var dr ports.DashboardReader = fakeDash{err: context.DeadlineExceeded}
	var lr ports.ExpenseLister = fakeList{}
	srv := NewServer(":0", ew, tr, dr, lr, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/month-overview", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Error loading overview") {
		t.Fatalf("expected error message in body, got: %s", body)
	}

	// Test with missing templates
	srv.templates = nil
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/month-overview", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "month-overview") {
		t.Fatalf("expected fallback HTML in body, got: %s", body)
	}
}

// Test format euros function
func TestFormatEuros(t *testing.T) {
	tests := []struct {
		cents    int64
		expected string
	}{
		{0, "€0,00"},
		{1, "€0,01"},
		{99, "€0,99"},
		{100, "€1,00"},
		{123, "€1,23"},
		{12345, "€123,45"},
		{-100, "-€1,00"},
		{-12345, "-€123,45"},
		{1000000, "€10000,00"},
	}

	for _, tt := range tests {
		got := formatEuros(tt.cents)
		if got != tt.expected {
			t.Fatalf("formatEuros(%d) = %q, want %q", tt.cents, got, tt.expected)
		}
	}
}

type fakeList struct{ items []core.Expense }

func (f fakeList) ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error) {
	return f.items, nil
}

type fakeListWithID struct{ items []ports.ExpenseWithID }

func (f fakeListWithID) ListExpensesWithID(ctx context.Context, year int, month int) ([]ports.ExpenseWithID, error) {
	return f.items, nil
}

type fakeListErr struct{}

func (fakeListErr) ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error) {
	return nil, context.DeadlineExceeded
}

// chdirRepoRoot attempts to set CWD to repo root so template glob works.
func chdirRepoRoot(t *testing.T) {
	t.Helper()
	// Walk up until web/templates exists
	dir, _ := os.Getwd()
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "web", "templates")); err == nil {
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			return
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate repo root with web/templates")
}

func TestIndexAndHealth(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("index status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "stat-hero") {
		t.Fatalf("dashboard body missing stat-hero section")
	}

	for _, path := range []string{"/healthz", "/readyz"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("%s status=%d", path, rr.Code)
		}
	}
}

func TestCreateExpenseValidationAndSuccess(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	// Wrong method
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/expenses", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if got := rr.Header().Get("Allow"); got != "POST" {
		t.Fatalf("expected Allow: POST, got %s", got)
	}

	// Invalid amount
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=x&amount=abc&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 422 {
		t.Fatalf("expected 422, got %d", rr.Code)
	}

	// ParseForm error via broken body
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", nil)
	req.Body = io.NopCloser(badReader{})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	// Missing description
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=&amount=1.23&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 422 {
		t.Fatalf("expected 422, got %d", rr.Code)
	}

	// Success (explicit day/month)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("day=2&month=3&description=ok&amount=1.23&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Check HTMX trigger header contains dashboard refresh
	hxTrigger := rr.Header().Get("HX-Trigger")
	if !strings.Contains(hxTrigger, "dashboard:refresh") {
		t.Fatalf("expected HX-Trigger header with dashboard:refresh, got %s", hxTrigger)
	}

	// Success with invalid day/month params (should use current)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("day=abc&month=xyz&description=ok&amount=1.23&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Append error -> 500
	var ewErr ports.ExpenseWriter = fakeExpErr{}
	srv = NewServer(":0", ewErr, tr, fakeDash{}, fakeList{}, nil, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=ok&amount=1.23&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// With embedded templates we no longer expect template parse errors at runtime.

func TestTaxonomyErrorStillRenders(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTaxErr{}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200 even if taxonomy errors, got %d", rr.Code)
	}
}

// Test index handler with missing templates
func TestIndexMissingTemplates(t *testing.T) {
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)
	srv.templates = nil // Simulate missing templates

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// Test sanitizeInput function
func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  normal text  ", "normal text"},
		{"text\x01with\x02control\x03chars", "textwithcontrolchars"},
		{"text\twith\ntab\rand\rcarriage", "text\twith\ntab\rand\rcarriage"},
		{"\x00\x1F\x7F", "\x7F"}, // Keep DEL (0x7F) but remove others
	}

	for _, tt := range tests {
		got := sanitizeInput(tt.input)
		if got != tt.expected {
			t.Fatalf("sanitizeInput(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStaticServesWithCacheHeader(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("static status=%d", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got == "" {
		t.Fatalf("expected Cache-Control header")
	}
	// Check for @import (modular CSS structure) or :root (legacy/inlined CSS)
	body := rr.Body.String()
	if !strings.Contains(body, "@import") && !strings.Contains(body, ":root") {
		t.Fatalf("unexpected static body: %s", body)
	}
}

// Test rate limiter functionality
func TestRateLimiterBehavior(t *testing.T) {
	rl := newRateLimiter()
	metrics := &securityMetrics{}

	// First request should be allowed
	if !rl.allow("192.168.1.1", metrics) {
		t.Fatal("first request should be allowed")
	}

	// Multiple requests within limit should be allowed
	for i := 0; i < 59; i++ {
		if !rl.allow("192.168.1.1", metrics) {
			t.Fatalf("request %d should be allowed", i+2)
		}
	}

	// 61st request should be blocked
	if rl.allow("192.168.1.1", metrics) {
		t.Fatal("61st request should be blocked")
	}

	// Different IP should be allowed
	if !rl.allow("192.168.1.2", metrics) {
		t.Fatal("different IP should be allowed")
	}
}

// Test rate limiter reset after time window
func TestRateLimiterReset(t *testing.T) {
	rl := newRateLimiter()
	metrics := &securityMetrics{}

	// Fill up the rate limit
	for i := 0; i < 60; i++ {
		rl.allow("192.168.1.1", metrics)
	}

	// Should be blocked
	if rl.allow("192.168.1.1", metrics) {
		t.Fatal("should be rate limited")
	}

	// Simulate time passage by directly modifying the client info
	rl.mu.Lock()
	client := rl.clients["192.168.1.1"]
	client.lastRequest = time.Now().Add(-2 * time.Minute)
	rl.mu.Unlock()

	// Should be allowed again
	if !rl.allow("192.168.1.1", metrics) {
		t.Fatal("should be allowed after time window reset")
	}
}

// Test rate limiter cleanup mechanism
func TestRateLimiterCleanup(t *testing.T) {
	rl := newRateLimiter()
	defer rl.stop() // Ensure cleanup goroutine is stopped
	metrics := &securityMetrics{}

	// Add some clients
	rl.allow("192.168.1.1", metrics)
	rl.allow("192.168.1.2", metrics)
	rl.allow("192.168.1.3", metrics)

	// Verify clients exist
	rl.mu.Lock()
	initialCount := len(rl.clients)
	rl.mu.Unlock()
	if initialCount != 3 {
		t.Fatalf("expected 3 clients, got %d", initialCount)
	}

	// Manually set old timestamps to simulate stale entries
	rl.mu.Lock()
	oldTime := time.Now().Add(-15 * time.Minute)
	rl.clients["192.168.1.1"].lastRequest = oldTime
	rl.clients["192.168.1.2"].lastRequest = oldTime
	// Keep 192.168.1.3 recent
	rl.mu.Unlock()

	// Run cleanup manually
	rl.cleanupStaleEntries()

	// Check that stale entries were removed
	rl.mu.Lock()
	finalCount := len(rl.clients)
	_, exists1 := rl.clients["192.168.1.1"]
	_, exists2 := rl.clients["192.168.1.2"]
	_, exists3 := rl.clients["192.168.1.3"]
	rl.mu.Unlock()

	if finalCount != 1 {
		t.Fatalf("expected 1 client after cleanup, got %d", finalCount)
	}
	if exists1 {
		t.Error("stale client 192.168.1.1 should have been cleaned up")
	}
	if exists2 {
		t.Error("stale client 192.168.1.2 should have been cleaned up")
	}
	if !exists3 {
		t.Error("recent client 192.168.1.3 should still exist")
	}
}

// Test server shutdown cleanup
func TestServerShutdownCleanup(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	// Verify rate limiter is running
	if srv.rateLimiter == nil {
		t.Fatal("rate limiter should be initialized")
	}

	// Add some activity
	srv.rateLimiter.allow("192.168.1.1", srv.metrics)

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("server shutdown failed: %v", err)
	}

	// Verify cleanup goroutine was stopped by attempting to access after short delay
	time.Sleep(10 * time.Millisecond)
	// The cleanup should have stopped; we can't easily test the goroutine state directly
	// but we verified the Shutdown method completes without error
}

// Test security headers middleware
func TestSecurityHeaders(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)

	// Check security headers
	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Content-Security-Policy",
		"Referrer-Policy",
	}

	for _, header := range headers {
		if got := rr.Header().Get(header); got == "" {
			t.Fatalf("missing security header: %s", header)
		}
	}
}

// Test rate limiting for POST requests
func TestRateLimitingPOST(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	// Fill up rate limit
	for i := 0; i < 60; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=test&amount=1.00&primary=A&secondary=X"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.168.1.1:12345"
		srv.Handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d failed: %d", i+1, rr.Code)
		}
	}

	// 61st request should be rate limited
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=test&amount=1.00&primary=A&secondary=X"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.1:12345"
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("expected Retry-After: 60, got %s", got)
	}
}

// Test client IP extraction for rate limiting
func TestClientIPExtraction(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr, fakeDash{}, fakeList{}, nil, nil)

	tests := []struct {
		name            string
		xForwardedFor   string
		xRealIP         string
		remoteAddr      string
		expectedRateKey string
	}{
		{"X-Forwarded-For header", "10.0.0.1", "", "192.168.1.1:12345", "10.0.0.1"},
		{"X-Real-IP header", "", "10.0.0.2", "192.168.1.1:12345", "10.0.0.2"},
		{"RemoteAddr fallback", "", "", "192.168.1.1:12345", "192.168.1.1:12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear rate limiter state
			srv.rateLimiter = newRateLimiter()

			// Fill up rate limit for the expected IP
			for i := 0; i < 60; i++ {
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=test&amount=1.00&primary=A&secondary=X"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tt.xForwardedFor != "" {
					req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
				}
				if tt.xRealIP != "" {
					req.Header.Set("X-Real-IP", tt.xRealIP)
				}
				req.RemoteAddr = tt.remoteAddr
				srv.Handler.ServeHTTP(rr, req)
			}

			// Next request should be rate limited
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=test&amount=1.00&primary=A&secondary=X"))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			req.RemoteAddr = tt.remoteAddr
			srv.Handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429, got %d", rr.Code)
			}
		})
	}
}
