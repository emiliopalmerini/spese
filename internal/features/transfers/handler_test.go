package transfers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"spese/internal/storage"
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

func TestCreateRejectsSameAccountWithLocalizedMessage(t *testing.T) {
	form := url.Values{
		"source":      {"Conto"},
		"destination": {"Conto"},
		"date":        {"2026-05-15"},
		"amount":      {"10,00"},
	}
	req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Il conto di origine e quello di destinazione devono essere diversi." {
		t.Fatalf("unexpected validation message %q", got)
	}
}

func TestCreateRejectsUnknownAccountsBeforeAppend(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		destination string
		want        string
	}{
		{name: "source", source: "Missing", destination: "Wallet", want: "Il conto di origine non esiste."},
		{name: "destination", source: "Bank", destination: "Missing", want: "Il conto di destinazione non esiste."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openTransferTestStore(t, ctx)
			insertTransferTestAccounts(t, ctx, store)

			w := postTransfer(t, &Handler{Store: store, Logger: slog.Default()}, url.Values{
				"source":      {tt.source},
				"destination": {tt.destination},
				"date":        {"2020-05-15"},
				"amount":      {"10,00"},
			})

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
			}
			if got := strings.TrimSpace(w.Body.String()); got != tt.want {
				t.Fatalf("validation message = %q, want %q", got, tt.want)
			}
			if got := transferTransactionCount(t, ctx, store); got != 0 {
				t.Fatalf("transactions = %d, want 0", got)
			}
		})
	}
}

func TestCreateDoesNotApplyAccountActivityRules(t *testing.T) {
	ctx := context.Background()
	store := openTransferTestStore(t, ctx)
	insertTransferTestAccounts(t, ctx, store)

	w := postTransfer(t, &Handler{Store: store, Logger: slog.Default()}, url.Values{
		"source":      {"Bank"},
		"destination": {"Wallet"},
		"date":        {"2020-05-15"},
		"amount":      {"10,00"},
	})

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d: %s", http.StatusSeeOther, w.Code, w.Body.String())
	}
	if got := transferTransactionCount(t, ctx, store); got != 2 {
		t.Fatalf("transactions = %d, want 2", got)
	}
}

func openTransferTestStore(t *testing.T, ctx context.Context) *storage.Store {
	t.Helper()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "transfers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func insertTransferTestAccounts(t *testing.T, ctx context.Context, store *storage.Store) {
	t.Helper()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency, active_from, active_to) VALUES
			('Bank', 'Asset', 'Cash', 'EUR', '2025-01', ''),
			('Wallet', 'Asset', 'Cash', 'EUR', '', '2024-12');
	`); err != nil {
		t.Fatal(err)
	}
}

func postTransfer(t *testing.T, handler *Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.create(w, req)
	return w
}

func transferTransactionCount(t *testing.T, ctx context.Context, store *storage.Store) int {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
