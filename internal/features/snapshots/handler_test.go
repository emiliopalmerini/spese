package snapshots

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCreateRejectsEmptyBalancesWithLocalizedMessage(t *testing.T) {
	form := url.Values{"month": {"2026-05"}}
	req := httptest.NewRequest(http.MethodPost, "/snapshots", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Inserisci almeno un saldo." {
		t.Fatalf("unexpected validation message %q", got)
	}
}
