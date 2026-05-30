package transactions

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Tab is the sheet tab name this slice mirrors to.
const Tab = "transactions"

// Filter narrows the set of transactions returned by List.
type Filter struct {
	From kernel.Date // inclusive; zero = no lower bound
	To   kernel.Date // exclusive; zero = no upper bound
	Kind Kind        // empty = any
	Last int         // if > 0, keep only the last N rows after filtering+sorting (most recent first)
}

// List reads transactions from the local database, applies the filter, and
// returns them sorted by date descending.
func List(ctx context.Context, store *storage.Store, f Filter, _ bool) ([]Transaction, error) {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT id, date, kind, account, amount_cents, category, subcategory, payee, note
		FROM transactions
		ORDER BY date DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	defer rows.Close()

	var out []Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		if f.Kind != "" && t.Kind != f.Kind {
			continue
		}
		if !f.From.IsZero() && t.Date.Before(f.From.Time) {
			continue
		}
		if !f.To.IsZero() && !t.Date.Before(f.To.Time) {
			continue
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date.Time) })
	if f.Last > 0 && len(out) > f.Last {
		out = out[:f.Last]
	}
	return out, nil
}

// Append writes one or more transactions locally and enqueues one sheet mirror
// refresh in the same transaction.
func Append(ctx context.Context, store *storage.Store, txns []Transaction) error {
	if len(txns) == 0 {
		return nil
	}
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range txns {
		if _, err := tx.Exec(`
			INSERT INTO transactions (
				date, kind, account, amount_cents, category, subcategory, payee, note
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			t.Date.ISO(),
			string(t.Kind),
			t.Account,
			int64(t.Amount),
			t.Category,
			t.Subcategory,
			t.Payee,
			t.Note,
		); err != nil {
			return fmt.Errorf("insert %s: %w", Tab, err)
		}
	}
	if err := store.EnqueueSheetSync(tx, Tab); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", Tab, err)
	}
	return nil
}

// SheetRows returns the source-tab rows for the Google Sheets mirror.
func SheetRows(ctx context.Context, store *storage.Store) ([][]any, error) {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT id, date, kind, account, amount_cents, category, subcategory, payee, note
		FROM transactions
		ORDER BY date, id
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s mirror rows: %w", Tab, err)
	}
	defer rows.Close()

	var out [][]any
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, []any{
			t.Date.ISO(),
			string(t.Kind),
			t.Account,
			t.Amount.Float(),
			t.Category,
			t.Subcategory,
			t.Payee,
			t.Note,
			t.ID,
		})
	}
	return out, rows.Err()
}

type transactionScanner interface {
	Scan(dest ...any) error
}

func scanTransaction(row transactionScanner) (Transaction, error) {
	var id int64
	var dateStr, kind string
	var amount int64
	t := Transaction{}
	if err := row.Scan(
		&id,
		&dateStr,
		&kind,
		&t.Account,
		&amount,
		&t.Category,
		&t.Subcategory,
		&t.Payee,
		&t.Note,
	); err != nil {
		return Transaction{}, fmt.Errorf("scan %s: %w", Tab, err)
	}
	d, err := kernel.ParseDate(dateStr)
	if err != nil {
		return Transaction{}, fmt.Errorf("parse transaction date: %w", err)
	}
	t.ID = strconv.FormatInt(id, 10)
	t.Date = d
	t.Kind = Kind(kind)
	t.Amount = kernel.Money(amount)
	return t, nil
}
