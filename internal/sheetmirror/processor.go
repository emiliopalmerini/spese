// Package sheetmirror syncs local SQLite source data to sheet targets.
package sheetmirror

import (
	"context"
	"log/slog"
	"time"

	"spese/internal/features/accounts"
	"spese/internal/features/snapshots"
	"spese/internal/features/transactions"
	"spese/internal/storage"
)

// Processor rebuilds source tabs from SQLite. Rebuilding keeps the mirror
// idempotent: older bilanci for the same account/month never leak into the
// sheet.
type Processor struct {
	Store  *storage.Store
	Client SheetWriter
	Logger *slog.Logger
}

// SheetWriter is the output port used by the mirror worker. Google Sheets and
// local test sinks both implement it.
type SheetWriter interface {
	ReplaceRows(ctx context.Context, tab string, rows [][]any) error
}

// Export rewrites all source tabs from the local database.
func (p *Processor) Export(ctx context.Context) error {
	start := time.Now()
	if err := p.replaceAccounts(ctx); err != nil {
		return err
	}
	if err := p.replaceTransactions(ctx); err != nil {
		return err
	}
	if err := p.replaceSnapshots(ctx); err != nil {
		return err
	}
	if p.Logger != nil {
		p.Logger.Info("sheet mirror exported", "elapsed", time.Since(start))
	}
	return nil
}

func (p *Processor) replaceAccounts(ctx context.Context) error {
	rows, err := accounts.SheetRows(ctx, p.Store)
	if err != nil {
		return err
	}
	return p.Client.ReplaceRows(ctx, accounts.Tab, prependHeader(rows, []any{
		"name", "type", "class", "currency", "active_from", "active_to", "note",
	}))
}

func (p *Processor) replaceTransactions(ctx context.Context) error {
	rows, err := transactions.SheetRows(ctx, p.Store)
	if err != nil {
		return err
	}
	return p.Client.ReplaceRows(ctx, transactions.Tab, prependHeader(rows, []any{
		"date", "kind", "account", "amount", "category", "subcategory", "payee", "note", "id",
	}))
}

func (p *Processor) replaceSnapshots(ctx context.Context) error {
	rows, err := snapshots.SheetRows(ctx, p.Store)
	if err != nil {
		return err
	}
	return p.Client.ReplaceRows(ctx, snapshots.Tab, prependHeader(rows, []any{
		"month", "account", "balance", "note",
	}))
}

func prependHeader(rows [][]any, header []any) [][]any {
	out := make([][]any, 0, len(rows)+1)
	out = append(out, header)
	out = append(out, rows...)
	return out
}
