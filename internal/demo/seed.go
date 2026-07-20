// Package demo creates representative local-only data for UI development.
package demo

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Seed replaces the supplied store contents with a rolling twelve-month demo.
// Callers must ensure the store points to a disposable development database.
func Seed(ctx context.Context, store *storage.Store, today kernel.Date) error {
	if today.IsZero() {
		today = kernel.Today()
	}
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"snapshot_balances", "snapshot_batches", "transactions", "accounts", "sheet_sync_outbox"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear demo %s: %w", table, err)
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO accounts (name, type, class, currency, active_from, note) VALUES
			('Conto principale', 'Asset', 'Cash', 'EUR', ?, 'Entrate e spese quotidiane'),
			('Contanti', 'Asset', 'Cash', 'EUR', ?, 'Piccole spese'),
			('Broker ETF', 'Asset', 'Investment', 'EUR', ?, 'Portafoglio a lungo termine'),
			('Casa', 'Asset', 'Property', 'EUR', ?, 'Valore indicativo immobile'),
			('Carta di credito', 'Liability', 'Credit', 'EUR', ?, 'Saldo carta da rimborsare')
	`, demoMonth(today, -11).Month(), demoMonth(today, -11).Month(), demoMonth(today, -11).Month(), demoMonth(today, -11).Month(), demoMonth(today, -11).Month()); err != nil {
		return fmt.Errorf("insert demo accounts: %w", err)
	}

	firstMonth := demoMonth(today, -11)
	for i := 0; i < 12; i++ {
		month := demoMonth(firstMonth, i)
		if err := insertMonth(tx, month, i); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit demo data: %w", err)
	}
	return nil
}

func insertMonth(tx storage.Tx, month kernel.Date, index int) error {
	day := func(value int) string {
		return kernel.Date{Time: month.AddDate(0, 0, value-1)}.ISO()
	}
	groceries := int64(28500 + (index%4)*1800)
	utilities := int64(12000 + (index%3)*2500)
	leisure := int64(9000 + (index%5)*1700)
	freelance := int64(0)
	if index%3 == 2 {
		freelance = 65000 + int64(index)*2500
	}

	rows := []struct {
		date, kind, account  string
		amount               int64
		category, sub, payee string
	}{
		{day(2), "Income", "Conto principale", 320000, "Lavoro", "Stipendio", "Azienda Demo"},
		{day(3), "Expense", "Conto principale", -105000, "Casa", "Affitto", "Proprietario"},
		{day(6), "Expense", "Conto principale", -groceries, "Alimentari", "Spesa", "Supermercato"},
		{day(15), "Expense", "Conto principale", -(groceries - 6000), "Alimentari", "Spesa", "Mercato"},
		{day(8), "Expense", "Conto principale", -6500, "Trasporti", "Abbonamento", "Trasporto pubblico"},
		{day(10), "Expense", "Conto principale", -utilities, "Casa", "Utenze", "Fornitore energia"},
		{day(20), "Expense", "Conto principale", -leisure, "Tempo libero", "Uscite", "Ristorante"},
		{day(5), "Transfer", "Conto principale", -50000, "Investimenti", "ETF", "Giroconto Broker"},
		{day(5), "Transfer", "Broker ETF", 50000, "Investimenti", "ETF", "Giroconto Broker"},
	}
	if freelance > 0 {
		rows = append(rows, struct {
			date, kind, account  string
			amount               int64
			category, sub, payee string
		}{day(18), "Income", "Conto principale", freelance, "Lavoro", "Freelance", "Cliente Demo"})
	}
	for _, row := range rows {
		if _, err := tx.Exec(`
			INSERT INTO transactions (date, kind, account, amount_cents, category, subcategory, payee)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, row.date, row.kind, row.account, row.amount, row.category, row.sub, row.payee); err != nil {
			return fmt.Errorf("insert demo transaction for %s: %w", month.Month(), err)
		}
	}

	capturedAt := kernel.Date{Time: month.AddDate(0, 1, -1)}.ISO() + "T20:00:00Z"
	result, err := tx.Exec(`
		INSERT INTO snapshot_batches (effective_month, captured_at, note)
		VALUES (?, ?, 'Dati dimostrativi')
	`, month.Month(), capturedAt)
	if err != nil {
		return fmt.Errorf("insert demo snapshot for %s: %w", month.Month(), err)
	}
	batchID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("demo snapshot id: %w", err)
	}

	balances := []struct {
		account string
		amount  int64
	}{
		{"Conto principale", 1250000 + int64(index)*115000},
		{"Contanti", 18000 + int64(index%3)*2500},
		{"Broker ETF", 6200000 + int64(index)*560000 + int64(index*index)*3500},
		{"Casa", 21000000 + int64(index/6)*250000},
		{"Carta di credito", -(42000 + int64(index%4)*7500)},
	}
	for _, balance := range balances {
		if _, err := tx.Exec(`
			INSERT INTO snapshot_balances (batch_id, account, balance_cents)
			VALUES (?, ?, ?)
		`, batchID, balance.account, balance.amount); err != nil {
			return fmt.Errorf("insert demo balance for %s: %w", balance.account, err)
		}
	}
	return nil
}

func demoMonth(date kernel.Date, offset int) kernel.Date {
	return kernel.Date{Time: date.FirstOfMonth().AddDate(0, offset, 0)}
}
