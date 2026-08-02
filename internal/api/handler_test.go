package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"spese/internal/features/ledger"
	"spese/internal/storage"
)

func TestMovementAPIIdempotencyAndOptimisticLocking(t *testing.T) {
	handler, service := newTestHandler(t)
	account, err := service.CreateAccount(context.Background(), ledger.AccountInput{Name: "Conto", Type: ledger.AccountAsset, Class: "Cash", InitialDate: "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := service.CreateCategory(context.Background(), ledger.CategoryInput{Name: "Casa", Kind: ledger.CategoryExpense})
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"kind": "expense", "status": "posted", "date": "2026-02-01", "accountId": account.ID,
		"amountCents": 1200, "merchant": "Forno", "allocations": []map[string]any{{"categoryId": category.ID, "amountCents": 1200}},
	}

	first := apiRequest(t, handler, http.MethodPost, "/api/v1/movements", payload, "movement-1", "")
	if first.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", first.Code, first.Body.String())
	}
	second := apiRequest(t, handler, http.MethodPost, "/api/v1/movements", payload, "movement-1", "")
	if second.Code != http.StatusCreated || second.Body.String() != first.Body.String() {
		t.Fatalf("idempotent replay mismatch: first=%s second=%s", first.Body.String(), second.Body.String())
	}

	var response struct {
		Data ledger.Movement `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	patch := payload
	patch["amountCents"] = 1300
	patch["allocations"] = []map[string]any{{"categoryId": category.ID, "amountCents": 1300}}
	conflict := apiRequest(t, handler, http.MethodPatch, "/api/v1/movements/"+response.Data.ID, patch, "", `"99"`)
	if conflict.Code != http.StatusPreconditionFailed {
		t.Fatalf("conflict status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
	if !strings.Contains(conflict.Body.String(), `"code":"version_conflict"`) {
		t.Fatalf("missing error envelope: %s", conflict.Body.String())
	}
}

func TestMutationSecurityAndRequestID(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(`{}`))
	req.Host = "spese.test"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("cross-site/missing origin status = %d, want 403", res.Code)
	}
	if res.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}
	if got := res.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("missing Content-Security-Policy")
	}
}

func TestAPINotFoundNeverFallsThroughToSPA(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/not-real", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.Code)
	}
	if contentType := res.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestOpenAPIListsImplementedEndpointFamilies(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("OpenAPI status = %d", res.Code)
	}
	for _, path := range []string{
		"/api/v1/movements", "/api/v1/accounts", "/api/v1/categories",
		"/api/v1/merchant-rules", "/api/v1/reconciliations/preview",
		"/api/v1/recurring-rules", "/api/v1/analytics/overview", "/api/v1/dictation/fallback",
	} {
		if !strings.Contains(res.Body.String(), path+":") {
			t.Errorf("OpenAPI does not contain %s", path)
		}
	}
}

func newTestHandler(t *testing.T) (http.Handler, *ledger.Service) {
	t.Helper()
	store, err := storage.Open(context.Background(), "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := ledger.New(store)
	return New(store, service), service
}

func apiRequest(t *testing.T, handler http.Handler, method, path string, payload any, idempotencyKey, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "spese.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://spese.test")
	req.Header.Set("X-Spese-CSRF", "1")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
