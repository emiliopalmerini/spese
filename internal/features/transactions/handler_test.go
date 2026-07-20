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
