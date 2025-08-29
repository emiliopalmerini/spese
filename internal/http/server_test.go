package http

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "context"
    "spese/internal/core"
    ports "spese/internal/sheets"
)

type fakeTax struct{ cats, subs []string }
func (f fakeTax) List(ctx context.Context) ([]string, []string, error) { return f.cats, f.subs, nil }
type fakeTaxErr struct{}
func (fakeTaxErr) List(ctx context.Context) ([]string, []string, error) { return nil, nil, context.DeadlineExceeded }
type fakeExp struct{}
func (fakeExp) Append(ctx context.Context, e core.Expense) (string, error) { return "mem:1", nil }

// chdirRepoRoot attempts to set CWD to repo root so template glob works.
func chdirRepoRoot(t *testing.T) {
    t.Helper()
    // Walk up until web/templates exists
    dir, _ := os.Getwd()
    for i := 0; i < 5; i++ {
        if _, err := os.Stat(filepath.Join(dir, "web", "templates")); err == nil {
            if err := os.Chdir(dir); err != nil { t.Fatalf("chdir: %v", err) }
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
    if rr.Code != 200 { t.Fatalf("index status=%d", rr.Code) }
    if !strings.Contains(rr.Body.String(), "Registra Spesa") {
        t.Fatalf("index body missing heading")
    }

    for _, path := range []string{"/healthz", "/readyz"} {
        rr := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, path, nil)
        srv.Handler.ServeHTTP(rr, req)
        if rr.Code != 200 { t.Fatalf("%s status=%d", path, rr.Code) }
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
    if rr.Code != http.StatusMethodNotAllowed { t.Fatalf("expected 405, got %d", rr.Code) }

    // Invalid amount
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=x&amount=abc&category=A&subcategory=X"))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    srv.Handler.ServeHTTP(rr, req)
    if rr.Code != 422 { t.Fatalf("expected 422, got %d", rr.Code) }

    // Missing description
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=&amount=1.23&category=A&subcategory=X"))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    srv.Handler.ServeHTTP(rr, req)
    if rr.Code != 422 { t.Fatalf("expected 422, got %d", rr.Code) }

    // Success
    rr = httptest.NewRecorder()
    req = httptest.NewRequest(http.MethodPost, "/expenses", strings.NewReader("description=ok&amount=1.23&category=A&subcategory=X"))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    srv.Handler.ServeHTTP(rr, req)
    if rr.Code != 200 { t.Fatalf("expected 200, got %d", rr.Code) }
    if !strings.Contains(rr.Body.String(), "success") { t.Fatalf("expected success in body: %s", rr.Body.String()) }
}

func TestTemplateParseErrorPath(t *testing.T) {
    // Chdir to a temp dir with no templates to force parse error
    dir := t.TempDir()
    if err := os.Chdir(dir); err != nil { t.Fatalf("chdir temp: %v", err) }
    srv := NewServer(":0", fakeExp{}, fakeTax{cats: []string{"A"}, subs: []string{"X"}})
    // Index should fail with 500 due to missing templates
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    srv.Handler.ServeHTTP(rr, req)
    if rr.Code != http.StatusInternalServerError {
        t.Fatalf("expected 500 for missing templates, got %d", rr.Code)
    }
}

func TestTaxonomyErrorStillRenders(t *testing.T) {
    chdirRepoRoot(t)
    var ew ports.ExpenseWriter = fakeExp{}
    var tr ports.TaxonomyReader = fakeTaxErr{}
    srv := NewServer(":0", ew, tr)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    srv.Handler.ServeHTTP(rr, req)
    if rr.Code != 200 { t.Fatalf("expected 200 even if taxonomy errors, got %d", rr.Code) }
}
