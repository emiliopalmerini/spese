package snapshots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"spese/internal/storage"
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

func TestBuildFormViewUsesAccountsAndBalancesBeforeSelectedMonth(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency, active_from, active_to) VALUES
			('Always', 'Asset', 'Cash', 'EUR', '', ''),
			('Closed', 'Asset', 'Cash', 'EUR', '', '2026-04'),
			('Future', 'Asset', 'Cash', 'EUR', '2026-06', '');
		INSERT INTO snapshot_batches (id, effective_month, captured_at) VALUES
			(1, '2026-04', '2026-04-30T20:00:00Z'),
			(2, '2026-05', '2026-05-31T20:00:00Z'),
			(3, '2026-06', '2026-06-30T20:00:00Z');
		INSERT INTO snapshot_balances (batch_id, account, balance_cents) VALUES
			(1, 'Always', 10000),
			(2, 'Always', 20000),
			(3, 'Always', 30000);
	`); err != nil {
		t.Fatal(err)
	}

	view, err := BuildFormView(ctx, store, "2026-05", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Rows) != 1 || view.Rows[0].Account.Name != "Always" {
		t.Fatalf("rows = %#v, want only Always", view.Rows)
	}
	if got := view.Rows[0].LastMonth.Month(); got != "2026-04" {
		t.Fatalf("last month = %s, want 2026-04", got)
	}
	if got := int64(view.Rows[0].LastBalance); got != 10000 {
		t.Fatalf("last balance = %d, want 10000", got)
	}
}

func TestCreateRejectsUnknownOrInactiveAccountForSubmittedMonth(t *testing.T) {
	tests := []struct {
		name    string
		account string
		want    string
	}{
		{name: "unknown", account: "Missing", want: "Il conto Missing non esiste."},
		{name: "not active yet", account: "Future", want: "Il conto Future non è attivo per il mese selezionato."},
		{name: "already closed", account: "Closed", want: "Il conto Closed non è attivo per il mese selezionato."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openTestStore(t, ctx)
			if _, err := store.DB().ExecContext(ctx, `
				INSERT INTO accounts (name, type, class, currency, active_from, active_to) VALUES
					('Future', 'Asset', 'Cash', 'EUR', '2026-06', ''),
					('Closed', 'Asset', 'Cash', 'EUR', '', '2026-04');
			`); err != nil {
				t.Fatal(err)
			}

			form := url.Values{
				"month":                       {"2026-05"},
				"balance[" + tt.account + "]": {"10,00"},
			}
			req := httptest.NewRequest(http.MethodPost, "/snapshots", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			(&Handler{Store: store}).create(w, req)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
			}
			if got := strings.TrimSpace(w.Body.String()); got != tt.want {
				t.Fatalf("validation message = %q, want %q", got, tt.want)
			}
			var count int
			if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM snapshot_batches`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("snapshot batches = %d, want 0", count)
			}
		})
	}
}

func openTestStore(t *testing.T, ctx context.Context) *storage.Store {
	t.Helper()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "snapshots.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
