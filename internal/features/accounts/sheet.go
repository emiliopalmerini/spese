package accounts

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Tab is the sheet tab name this slice mirrors to.
const Tab = "accounts"

// List reads every account row from the local database.
func List(ctx context.Context, store *storage.Store, _ bool) ([]Account, error) {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT name, type, class, currency, active_from, active_to, note
		FROM accounts
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		var activeFrom, activeTo string
		if err := rows.Scan(&a.Name, &a.Type, &a.Class, &a.Currency, &activeFrom, &activeTo, &a.Note); err != nil {
			return nil, fmt.Errorf("scan %s: %w", Tab, err)
		}
		if activeFrom != "" {
			d, err := kernel.ParseDate(activeFrom)
			if err != nil {
				return nil, fmt.Errorf("parse active_from: %w", err)
			}
			a.ActiveFrom = d
		}
		if activeTo != "" {
			d, err := kernel.ParseDate(activeTo)
			if err != nil {
				return nil, fmt.Errorf("parse active_to: %w", err)
			}
			a.ActiveTo = d
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Append writes a new account locally and enqueues a sheet mirror refresh.
func Append(ctx context.Context, store *storage.Store, a Account) error {
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO accounts (name, type, class, currency, active_from, active_to, note)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, a.Name, string(a.Type), string(a.Class), a.Currency, dateOrEmpty(a.ActiveFrom), dateOrEmpty(a.ActiveTo), a.Note); err != nil {
		return fmt.Errorf("insert %s: %w", Tab, err)
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
	accs, err := List(ctx, store, false)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, len(accs))
	for i, a := range accs {
		rows[i] = []any{
			a.Name,
			string(a.Type),
			string(a.Class),
			a.Currency,
			dateOrEmpty(a.ActiveFrom),
			dateOrEmpty(a.ActiveTo),
			a.Note,
		}
	}
	return rows, nil
}

func dateOrEmpty(d kernel.Date) any {
	if d.IsZero() {
		return ""
	}
	return d.Month()
}
