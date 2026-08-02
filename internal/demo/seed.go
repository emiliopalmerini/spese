// Package demo creates representative local-only data for UI development.
package demo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

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

	for _, table := range []string{
		"recurring_occurrences", "recurring_rules", "account_reconciliations", "reconciliation_batches",
		"movement_allocations", "postings", "movements", "merchant_rules",
		"accounts", "sheet_sync_outbox",
	} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear demo %s: %w", table, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM categories WHERE parent_id IS NOT NULL"); err != nil {
		return fmt.Errorf("clear demo subcategories: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM categories"); err != nil {
		return fmt.Errorf("clear demo categories: %w", err)
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
	if _, err := tx.Exec(`
		INSERT INTO recurring_rules (id, kind, frequency, interval_count, start_date, day_of_month,
			timezone, amount_cents, amount_mode, account_id, category_id, merchant, note, state, mode,
			next_due, created_at, updated_at)
		SELECT ?, 'expense', 'monthly', 1, ?, 3, 'Europe/Rome', 105000, 'fixed', a.id, c.id,
			'Proprietario', 'Affitto demo', 'active', 'auto_post', ?, ?, ?
		FROM accounts a JOIN categories c ON c.name = 'Affitto'
		WHERE a.name = 'Conto principale'
	`, uuid.NewString(), firstMonth.ISO(), demoMonth(today, 1).FirstOfMonth().AddDate(0, 0, 2).Format("2006-01-02"), today.ISO()+"T12:00:00Z", today.ISO()+"T12:00:00Z"); err != nil {
		return fmt.Errorf("insert demo recurring rule: %w", err)
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
		if row.kind != "Transfer" {
			if err := insertLedgerMovement(tx, row.date, row.kind, row.account, row.amount, row.category, row.sub, row.payee); err != nil {
				return err
			}
		}
	}
	if err := insertDemoTransfer(tx, day(5)); err != nil {
		return err
	}

	capturedAt := kernel.Date{Time: month.AddDate(0, 1, -1)}.ISO() + "T20:00:00Z"
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
	reconciliationBatchID := uuid.NewString()
	if _, err := tx.Exec(`
		INSERT INTO reconciliation_batches (id, period, status, created_at, committed_at)
		VALUES (?, ?, 'committed', ?, ?)
	`, reconciliationBatchID, month.Month(), capturedAt, capturedAt); err != nil {
		return fmt.Errorf("insert demo reconciliation batch: %w", err)
	}
	for _, balance := range balances {
		if _, err := tx.Exec(`
			INSERT INTO account_reconciliations (id, batch_id, account_id, closed_through,
				expected_balance_cents, actual_balance_cents, difference_cents, created_at)
			SELECT ?, ?, id, ?, ?, ?, 0, ? FROM accounts WHERE name = ?
		`, uuid.NewString(), reconciliationBatchID, kernel.Date{Time: month.AddDate(0, 1, -1)}.ISO(), balance.amount, balance.amount, capturedAt, balance.account); err != nil {
			return fmt.Errorf("insert demo reconciliation for %s: %w", balance.account, err)
		}
	}
	return nil
}

func insertLedgerMovement(tx storage.Tx, date, legacyKind, account string, signedAmount int64, parent, child, merchant string) error {
	kind, categoryKind := "expense", "expense"
	posting := signedAmount
	if legacyKind == "Income" {
		kind, categoryKind = "income", "income"
	}
	amount := signedAmount
	if amount < 0 {
		amount = -amount
	}
	now := date + "T12:00:00Z"
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO categories (id, kind, name, icon, color, created_at, updated_at)
		VALUES (?, ?, ?, 'shapes', ?, ?, ?)
	`, uuid.NewString(), categoryKind, parent, demoColor(parent), now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO categories (id, parent_id, kind, name, icon, color, created_at, updated_at)
		SELECT ?, id, ?, ?, 'shapes', color, ?, ? FROM categories
		WHERE kind = ? AND parent_id IS NULL AND name = ?
	`, uuid.NewString(), categoryKind, child, now, now, categoryKind, parent); err != nil {
		return err
	}
	movementID := uuid.NewString()
	if _, err := tx.Exec(`
		INSERT INTO movements (id, kind, status, business_date, amount_cents, merchant, origin, created_at, updated_at)
		VALUES (?, ?, 'posted', ?, ?, ?, 'manual', ?, ?)
	`, movementID, kind, date, amount, merchant, now, now); err != nil {
		return fmt.Errorf("insert demo ledger movement: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at)
		SELECT ?, ?, id, ?, ? FROM accounts WHERE name = ?
	`, uuid.NewString(), movementID, posting, now, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO movement_allocations (id, movement_id, category_id, amount_cents, created_at)
		SELECT ?, ?, child.id, ?, ? FROM categories child
		JOIN categories parent ON parent.id = child.parent_id
		WHERE child.kind = ? AND parent.name = ? AND child.name = ?
	`, uuid.NewString(), movementID, amount, now, categoryKind, parent, child); err != nil {
		return err
	}
	return nil
}

func insertDemoTransfer(tx storage.Tx, date string) error {
	id, now := uuid.NewString(), date+"T12:00:00Z"
	if _, err := tx.Exec(`
		INSERT INTO movements (id, kind, status, business_date, amount_cents, merchant, origin, created_at, updated_at)
		VALUES (?, 'transfer', 'posted', ?, 50000, 'Giroconto Broker', 'manual', ?, ?)
	`, id, date, now, now); err != nil {
		return err
	}
	for _, posting := range []struct {
		account string
		amount  int64
	}{{"Conto principale", -50000}, {"Broker ETF", 50000}} {
		if _, err := tx.Exec(`
			INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at)
			SELECT ?, ?, id, ?, ? FROM accounts WHERE name = ?
		`, uuid.NewString(), id, posting.amount, now, posting.account); err != nil {
			return err
		}
	}
	return nil
}

func demoColor(category string) string {
	colors := map[string]string{"Casa": "#865D36", "Alimentari": "#2E6F95", "Trasporti": "#725B86", "Tempo libero": "#9A4F66", "Lavoro": "#6A7B35"}
	if color := colors[category]; color != "" {
		return color
	}
	return "#725B86"
}

func demoMonth(date kernel.Date, offset int) kernel.Date {
	return kernel.Date{Time: date.FirstOfMonth().AddDate(0, offset, 0)}
}
