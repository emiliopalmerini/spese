package transactions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"spese/internal/storage"
)

type captureRenderer struct {
	data any
}

func (r *captureRenderer) Render(_ http.ResponseWriter, _ string, data any) error {
	r.data = data
	return nil
}

func TestListProvidesUnfilteredHistoricalCategorySuggestions(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "transactions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency) VALUES
			('Bank', 'Asset', 'Cash', 'EUR');
		INSERT INTO transactions (date, kind, account, amount_cents, category, subcategory, payee, note) VALUES
			('2026-01-04', 'Expense', 'Bank', -1000, 'CIBO', '', 'Cafe', ''),
			('2026-01-03', 'Income', 'Bank', 2000, 'Stipendio', '', 'Employer', ''),
			('2026-01-02', 'Expense', 'Bank', -1000, 'cibo', '', 'Market', ''),
			('2026-01-01', 'Transfer', 'Bank', -500, 'Transfer', '', 'Wallet', '');
	`); err != nil {
		t.Fatal(err)
	}

	renderer := &captureRenderer{}
	handler := &Handler{Store: store, Render: renderer}
	req := httptest.NewRequest(http.MethodGet, "/transactions?category=Imported", nil)
	w := httptest.NewRecorder()

	handler.list(w, req)

	view, ok := renderer.data.(ListView)
	if !ok {
		t.Fatalf("rendered data = %T, want ListView", renderer.data)
	}
	if view.Filters.Category != "Imported" {
		t.Fatalf("category filter = %q, want Imported", view.Filters.Category)
	}
	want := []CategorySuggestion{{Name: "CIBO", Count: 2}, {Name: "Stipendio", Count: 1}}
	if !reflect.DeepEqual(view.CategorySuggestions, want) {
		t.Fatalf("suggestions = %+v, want %+v", view.CategorySuggestions, want)
	}
}

func TestCreateRejectsInvalidTransactionWithLocalizedMessage(t *testing.T) {
	form := url.Values{
		"kind":    {"Expense"},
		"account": {"Conto"},
		"date":    {"2026-05-15"},
		"amount":  {"12,50"},
	}
	req := httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "La descrizione è obbligatoria." {
		t.Fatalf("unexpected validation message %q", got)
	}
}

func TestParseListQueryBuildsInclusiveDateFilter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/transactions?from=2026-01-01&to=2026-01-31&kind=Expense&account=Bank&category=Food&q=cafe&page=2", nil)

	filter, form, page, err := parseListQuery(req.URL.Query())
	if err != nil {
		t.Fatal(err)
	}
	if filter.From.ISO() != "2026-01-01" || filter.To.ISO() != "2026-02-01" {
		t.Fatalf("date filter = %s to %s, want inclusive January", filter.From.ISO(), filter.To.ISO())
	}
	if filter.Kind != Expense || filter.Account != "Bank" || filter.Category != "Food" || filter.Query != "cafe" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if form.To != "2026-01-31" || page != 2 {
		t.Fatalf("form/page = %+v/%d", form, page)
	}
}

func TestPaginationURLPreservesFilters(t *testing.T) {
	values := url.Values{"q": {"cafe"}, "page": {"2"}}
	if got := paginationURL(values, 3); got != "/transactions?page=3&q=cafe" {
		t.Fatalf("pagination URL = %q", got)
	}
}
