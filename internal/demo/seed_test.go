package demo

import (
	"context"
	"path/filepath"
	"testing"

	"spese/internal/kernel"
	"spese/internal/storage"
)

func TestSeedCreatesCurrentRepresentativeDataset(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := Seed(ctx, store, kernel.Today()); err != nil {
		t.Fatal(err)
	}

	assertCount(t, store, "accounts", 5)
	assertCount(t, store, "snapshot_batches", 12)
	assertCountAtLeast(t, store, "transactions", 80)

	var currentMonthTransactions int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*) FROM transactions WHERE substr(date, 1, 7) = ?
	`, kernel.Today().Month()).Scan(&currentMonthTransactions); err != nil {
		t.Fatal(err)
	}
	if currentMonthTransactions == 0 {
		t.Fatal("expected transactions in the current month")
	}

	if err := Seed(ctx, store, kernel.Today()); err != nil {
		t.Fatal(err)
	}
	assertCount(t, store, "accounts", 5)
	assertCount(t, store, "snapshot_batches", 12)
}

func assertCount(t *testing.T, store *storage.Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func assertCountAtLeast(t *testing.T, store *storage.Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.DB().QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got < want {
		t.Fatalf("%s count = %d, want at least %d", table, got, want)
	}
}
