package accounts

import (
	"context"
	"errors"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Tab is the sheet tab name this slice mirrors to.
const Tab = "accounts"

var ErrAccountNameExists = errors.New("account name already exists")

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

// ListWithLatest reads every account with its most recent canonical snapshot.
func ListWithLatest(ctx context.Context, store *storage.Store, _ bool) ([]AccountRow, error) {
	return listWithLatest(ctx, store.DB())
}

func listWithLatest(ctx context.Context, db storage.SQLRunner) ([]AccountRow, error) {
	rows, err := db.QueryContext(ctx, `
		WITH canonical_snapshots AS (
			SELECT effective_month, account, balance_cents
			FROM (
				SELECT
					b.effective_month,
					sb.account,
					sb.balance_cents,
					row_number() OVER (
						PARTITION BY b.effective_month, sb.account
						ORDER BY b.captured_at DESC, b.id DESC
					) AS rn
				FROM snapshot_balances sb
				JOIN snapshot_batches b ON b.id = sb.batch_id
			)
			WHERE rn = 1
		),
		latest AS (
			SELECT account, effective_month, balance_cents
			FROM (
				SELECT
					account,
					effective_month,
					balance_cents,
					row_number() OVER (
						PARTITION BY account
						ORDER BY effective_month DESC
					) AS rn
				FROM canonical_snapshots
			)
			WHERE rn = 1
		)
		SELECT
			a.name,
			a.type,
			a.class,
			a.currency,
			a.active_from,
			a.active_to,
			a.note,
			coalesce(l.balance_cents, 0),
			coalesce(l.effective_month, '')
		FROM accounts a
		LEFT JOIN latest l ON l.account = a.name
		ORDER BY a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s with latest: %w", Tab, err)
	}
	defer rows.Close()

	var out []AccountRow
	for rows.Next() {
		var row AccountRow
		var activeFrom, activeTo, latestMonth string
		var balance int64
		if err := rows.Scan(
			&row.Account.Name,
			&row.Account.Type,
			&row.Account.Class,
			&row.Account.Currency,
			&activeFrom,
			&activeTo,
			&row.Account.Note,
			&balance,
			&latestMonth,
		); err != nil {
			return nil, fmt.Errorf("scan %s with latest: %w", Tab, err)
		}
		if activeFrom != "" {
			d, err := kernel.ParseDate(activeFrom)
			if err != nil {
				return nil, fmt.Errorf("parse active_from: %w", err)
			}
			row.Account.ActiveFrom = d
		}
		if activeTo != "" {
			d, err := kernel.ParseDate(activeTo)
			if err != nil {
				return nil, fmt.Errorf("parse active_to: %w", err)
			}
			row.Account.ActiveTo = d
		}
		row.LatestBalance = kernel.Money(balance)
		if latestMonth != "" {
			d, err := kernel.ParseDate(latestMonth)
			if err != nil {
				return nil, fmt.Errorf("parse latest month: %w", err)
			}
			row.LatestMonth = d.FirstOfMonth()
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Append writes a new account locally and enqueues a sheet mirror refresh.
func Append(ctx context.Context, store *storage.Store, a Account) error {
	var exists bool
	if err := store.DB().QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM accounts WHERE name = ?)
	`, a.Name).Scan(&exists); err != nil {
		return fmt.Errorf("check %s name: %w", Tab, err)
	}
	if exists {
		return ErrAccountNameExists
	}

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
