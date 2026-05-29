package transfers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTransfersRedirectsToTransactions(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Mount(mux, "/transfers")

	req := httptest.NewRequest(http.MethodGet, "/transfers", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/transactions" {
		t.Fatalf("expected redirect to /transactions, got %q", got)
	}
}

func TestRedirectAfterCreateUsesHXRedirectForHTMX(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/transfers", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	redirectAfterCreate(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}
	if got := w.Header().Get("HX-Redirect"); got != "/transactions" {
		t.Fatalf("expected HX-Redirect to /transactions, got %q", got)
	}
}

func TestRedirectAfterCreateFallsBackToSeeOther(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/transfers", nil)
	w := httptest.NewRecorder()

	redirectAfterCreate(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, w.Code)
	}
	if got := w.Header().Get("Location"); got != "/transactions" {
		t.Fatalf("expected redirect to /transactions, got %q", got)
	}
}
