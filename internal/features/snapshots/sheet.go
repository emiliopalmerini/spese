package snapshots

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/storage"
)

// Tab is the sheet tab name this slice mirrors to.
const Tab = "snapshots"

// List reads canonical snapshot rows from the local database. If several
// bilanci exist for the same account and month, only the newest batch is
// returned so month totals cannot double-count older submissions.
func List(ctx context.Context, store *storage.Store, _ bool) ([]Snapshot, error) {
	rows, err := store.DB().QueryContext(ctx, canonicalSnapshotsSQL+`
		SELECT effective_month, account, balance_cents, note
		FROM canonical_snapshots
		ORDER BY effective_month, account
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LatestPerAccountBefore returns the most recent snapshot strictly before
// month per account, keyed by account name.
func LatestPerAccountBefore(ctx context.Context, store *storage.Store, month kernel.Date, force bool) (map[string]Snapshot, error) {
	all, err := List(ctx, store, force)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]Snapshot)
	for _, s := range all {
		if !s.Month.Before(month.Time) {
			continue
		}
		cur, ok := latest[s.Account]
		if !ok || s.Month.After(cur.Month.Time) {
			latest[s.Account] = s
		}
	}
	return latest, nil
}

// Append writes one bilancio batch locally and enqueues a canonical sheet
// mirror refresh in the same transaction.
func Append(ctx context.Context, store *storage.Store, snaps []Snapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	batch, err := tx.Exec(`
		INSERT INTO snapshot_batches (effective_month)
		VALUES (?)
	`, snaps[0].Month.FirstOfMonth().Month())
	if err != nil {
		return fmt.Errorf("insert snapshot batch: %w", err)
	}
	batchID, err := batch.LastInsertId()
	if err != nil {
		return fmt.Errorf("snapshot batch id: %w", err)
	}
	for _, s := range snaps {
		if _, err := tx.Exec(`
			INSERT INTO snapshot_balances (batch_id, account, balance_cents, note)
			VALUES (?, ?, ?, ?)
		`, batchID, s.Account, int64(s.Balance), s.Note); err != nil {
			return fmt.Errorf("insert snapshot balance: %w", err)
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

// SheetRows returns canonical source-tab rows for the Google Sheets mirror.
func SheetRows(ctx context.Context, store *storage.Store) ([][]any, error) {
	snaps, err := List(ctx, store, false)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, len(snaps))
	for i, s := range snaps {
		rows[i] = []any{
			s.Month.Month(),
			s.Account,
			s.Balance.Float(),
			s.Note,
		}
	}
	return rows, nil
}

const canonicalSnapshotsSQL = `
	WITH canonical_snapshots AS (
		SELECT effective_month, account, balance_cents, note
		FROM (
			SELECT
				b.effective_month,
				sb.account,
				sb.balance_cents,
				sb.note,
				row_number() OVER (
					PARTITION BY b.effective_month, sb.account
					ORDER BY b.captured_at DESC, b.id DESC
				) AS rn
			FROM snapshot_balances sb
			JOIN snapshot_batches b ON b.id = sb.batch_id
		)
		WHERE rn = 1
	)
`

type snapshotScanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(row snapshotScanner) (Snapshot, error) {
	var monthStr string
	var amount int64
	s := Snapshot{}
	if err := row.Scan(&monthStr, &s.Account, &amount, &s.Note); err != nil {
		return Snapshot{}, fmt.Errorf("scan %s: %w", Tab, err)
	}
	month, err := kernel.ParseDate(monthStr)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot month: %w", err)
	}
	s.Month = month.FirstOfMonth()
	s.Balance = kernel.Money(amount)
	return s, nil
}

// dateOrEmpty is unused now but kept for the symmetry with accounts.sheet.go.
var _ = kernel.Today
