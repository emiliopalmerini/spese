// Package sheetmirror syncs the canonical SQLite ledger to derived sheet tabs.
package sheetmirror

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"spese/internal/storage"
)

type Processor struct {
	Store  *storage.Store
	Client SheetWriter
	Logger *slog.Logger
}

type SheetWriter interface {
	ReplaceRows(ctx context.Context, tab string, rows [][]any) error
}

// Export fully replaces every v2 ledger tab. A repeated RabbitMQ delivery is
// therefore idempotent and Google Sheets is never used as a read model.
func (p *Processor) Export(ctx context.Context) error {
	start := time.Now()
	tabs := []struct {
		name   string
		header []any
		query  string
	}{
		{"accounts", []any{"id", "name", "type", "class", "currency", "initial_balance_cents", "initial_date", "archived_at", "version"}, `SELECT id, name, type, class, currency, initial_balance_cents, initial_date, archived_at, version FROM accounts ORDER BY name`},
		{"categories", []any{"id", "parent_id", "kind", "name", "icon", "color", "sort_order", "archived_at", "version"}, `SELECT id, parent_id, kind, name, icon, color, sort_order, archived_at, version FROM categories ORDER BY kind, parent_id, sort_order, name`},
		{"movements", []any{"id", "date", "kind", "status", "amount_cents", "merchant", "description", "note", "origin", "voided_at", "version"}, `SELECT id, business_date, kind, status, amount_cents, merchant, description, note, origin, voided_at, version FROM movements ORDER BY business_date, id`},
		{"postings", []any{"id", "movement_id", "account_id", "amount_cents"}, `SELECT id, movement_id, account_id, amount_cents FROM postings ORDER BY movement_id, id`},
		{"allocations", []any{"id", "movement_id", "category_id", "amount_cents"}, `SELECT id, movement_id, category_id, amount_cents FROM movement_allocations ORDER BY movement_id, id`},
		{"reconciliations", []any{"id", "period", "account_id", "closed_through", "expected_balance_cents", "actual_balance_cents", "difference_cents"}, `SELECT r.id, b.period, r.account_id, r.closed_through, r.expected_balance_cents, r.actual_balance_cents, r.difference_cents FROM account_reconciliations r JOIN reconciliation_batches b ON b.id = r.batch_id ORDER BY r.closed_through, r.account_id`},
		{"recurring_rules", []any{"id", "kind", "frequency", "interval", "start_date", "end_date", "day_of_month", "timezone", "amount_cents", "amount_mode", "account_id", "category_id", "merchant", "state", "mode", "next_due", "version"}, `SELECT id, kind, frequency, interval_count, start_date, end_date, day_of_month, timezone, amount_cents, amount_mode, account_id, category_id, merchant, state, mode, next_due, version FROM recurring_rules ORDER BY next_due, id`},
	}
	for _, tab := range tabs {
		rows, err := queryRows(ctx, p.Store.DB(), tab.query)
		if err != nil {
			return fmt.Errorf("export %s: %w", tab.name, err)
		}
		if err := p.Client.ReplaceRows(ctx, tab.name, prependHeader(rows, tab.header)); err != nil {
			return fmt.Errorf("replace %s: %w", tab.name, err)
		}
	}
	if p.Logger != nil {
		p.Logger.Info("sheet mirror exported", "elapsed", time.Since(start), "tabs", len(tabs))
	}
	return nil
}

func queryRows(ctx context.Context, db *sql.DB, query string) ([][]any, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result [][]any
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		for i, value := range values {
			if bytes, ok := value.([]byte); ok {
				values[i] = string(bytes)
			}
			if value == nil {
				values[i] = ""
			}
		}
		result = append(result, values)
	}
	return result, rows.Err()
}

func prependHeader(rows [][]any, header []any) [][]any {
	out := make([][]any, 0, len(rows)+1)
	out = append(out, header)
	out = append(out, rows...)
	return out
}
