package reports

import (
	"context"
	"path/filepath"
	"testing"

	"spese/internal/kernel"
	"spese/internal/storage"
)

func TestNwMonthlyCarriesBalancesForwardAcrossPartialSnapshots(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO accounts (name, type, class, currency) VALUES
			('Bank', 'Asset', 'Cash', 'EUR'),
			('Broker', 'Asset', 'Investment', 'EUR');

		INSERT INTO snapshot_batches (id, effective_month, captured_at) VALUES
			(1, '2026-01', '2026-01-31T20:00:00Z'),
			(2, '2026-03', '2026-03-31T20:00:00Z');

		INSERT INTO snapshot_balances (batch_id, account, balance_cents) VALUES
			(1, 'Bank', 100000),
			(1, 'Broker', 200000),
			(2, 'Bank', 150000);
	`); err != nil {
		t.Fatal(err)
	}

	rows, err := NwMonthly(ctx, store, false)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		month string
		value kernel.Money
	}{
		{month: "2026-01", value: 300000},
		{month: "2026-02", value: 300000},
		{month: "2026-03", value: 350000},
	}
	if len(rows) != len(want) {
		t.Fatalf("len(rows) = %d, want %d", len(rows), len(want))
	}
	for i, expected := range want {
		if rows[i].Month.Month() != expected.month || rows[i].NetWorth != expected.value {
			t.Errorf("rows[%d] = (%s, %s), want (%s, %s)", i, rows[i].Month.Month(), rows[i].NetWorth, expected.month, expected.value)
		}
	}
}
