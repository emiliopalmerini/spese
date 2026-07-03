package accounts

import (
	"context"
	"database/sql"
	"testing"

	"spese/internal/kernel"
	"spese/internal/storage"

	_ "github.com/mattn/go-sqlite3"
)

func TestListWithLatestReturnsLatestCanonicalSnapshotPerAccount(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := storage.MigrateSQLite(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency, note) VALUES
			('Broker', 'Asset', 'Investment', 'EUR', 'Long term'),
			('Wallet', 'Asset', 'Cash', 'EUR', '');

		INSERT INTO snapshot_batches (id, effective_month, captured_at) VALUES
			(1, '2026-04', '2026-04-30T20:00:00Z'),
			(2, '2026-05', '2026-05-31T20:00:00Z'),
			(3, '2026-05', '2026-05-31T21:00:00Z');

		INSERT INTO snapshot_balances (batch_id, account, balance_cents) VALUES
			(1, 'Broker', 100000),
			(2, 'Broker', 110000),
			(3, 'Broker', 115000);
	`); err != nil {
		t.Fatal(err)
	}

	rows, err := listWithLatest(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}

	broker := rows[0]
	if broker.Account.Name != "Broker" {
		t.Fatalf("first account = %q, want Broker", broker.Account.Name)
	}
	if broker.LatestBalance != kernel.Money(115000) {
		t.Fatalf("Broker latest balance = %s, want 1.150,00", broker.LatestBalance)
	}
	if broker.LatestMonth.Month() != "2026-05" {
		t.Fatalf("Broker latest month = %q, want 2026-05", broker.LatestMonth.Month())
	}

	wallet := rows[1]
	if wallet.Account.Name != "Wallet" {
		t.Fatalf("second account = %q, want Wallet", wallet.Account.Name)
	}
	if wallet.LatestBalance != 0 {
		t.Fatalf("Wallet latest balance = %s, want zero", wallet.LatestBalance)
	}
	if !wallet.LatestMonth.IsZero() {
		t.Fatalf("Wallet latest month = %q, want zero", wallet.LatestMonth.ISO())
	}
}
