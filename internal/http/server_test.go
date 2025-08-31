package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"context"
	"io"
	"spese/internal/core"
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
	srv := NewServer(":0", ew, tr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("index status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Registra Spesa") {
		t.Fatalf("index body missing heading")
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
	srv := NewServer(":0", ew, tr)

	// Wrong method
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/expenses", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
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
	if !strings.Contains(rr.Body.String(), "success") {
		t.Fatalf("expected success in body: %s", rr.Body.String())
	}

	// Append error -> 500
	var ewErr ports.ExpenseWriter = fakeExpErr{}
	srv = NewServer(":0", ewErr, tr)
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
	srv := NewServer(":0", ew, tr)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200 even if taxonomy errors, got %d", rr.Code)
	}
}

func TestStaticServesWithCacheHeader(t *testing.T) {
	chdirRepoRoot(t)
	var ew ports.ExpenseWriter = fakeExp{}
	var tr ports.TaxonomyReader = fakeTax{cats: []string{"A"}, subs: []string{"X"}}
	srv := NewServer(":0", ew, tr)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("static status=%d", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got == "" {
		t.Fatalf("expected Cache-Control header")
	}
	if !strings.Contains(rr.Body.String(), ":root") {
		t.Fatalf("unexpected static body: %s", rr.Body.String())
	}
}
