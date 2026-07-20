package transactions

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

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
