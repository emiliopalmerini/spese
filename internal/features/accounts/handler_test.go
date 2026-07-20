package accounts

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateRejectsInvalidAccountWithLocalizedMessage(t *testing.T) {
	form := url.Values{
		"type":  {"Asset"},
		"class": {"Cash"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Il nome del conto è obbligatorio." {
		t.Fatalf("unexpected validation message %q", got)
	}
}

func TestCreateRejectsUnsupportedCurrency(t *testing.T) {
	form := url.Values{
		"name":     {"Broker USD"},
		"type":     {"Asset"},
		"class":    {"Investment"},
		"currency": {"USD"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Al momento sono supportati solo conti in EUR." {
		t.Fatalf("unexpected validation message %q", got)
	}
}
