package transactions

import (
	"context"
	"path/filepath"
	"testing"

	"spese/internal/storage"
)

func TestListFiltersByAccountCategoryAndSearchText(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "transactions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency) VALUES
			('Bank', 'Asset', 'Cash', 'EUR'),
			('Wallet', 'Asset', 'Cash', 'EUR');
		INSERT INTO transactions (date, kind, account, amount_cents, category, subcategory, payee, note) VALUES
			('2026-01-03', 'Expense', 'Bank', -1000, 'Food', 'Lunch', 'Corner Cafe', ''),
			('2026-01-04', 'Expense', 'Wallet', -2000, 'Travel', 'Train', 'Railway', 'conference');
	`); err != nil {
		t.Fatal(err)
	}

	rows, err := List(ctx, store, Filter{Account: "Wallet", Category: "Travel", Query: "CONFERENCE"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Payee != "Railway" {
		t.Fatalf("rows = %+v, want Railway transaction", rows)
	}
}
