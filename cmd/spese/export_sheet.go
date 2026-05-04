package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"spese/internal/config"
	"spese/internal/services"
	gsheet "spese/internal/sheets/google"
	"spese/internal/storage"
)

// runExportSheet implements `spese export-sheet --year YYYY [--dry-run]`.
// Walks SQLite for the year and idempotently upserts expenses, incomes and
// net-worth balances to Google Sheets via the existing remote writers.
func runExportSheet(args []string) int {
	fs := flag.NewFlagSet("export-sheet", flag.ContinueOnError)
	year := fs.Int("year", time.Now().Year(), "year to export (YYYY)")
	dryRun := fs.Bool("dry-run", false, "count rows but do not write to Sheets")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *year <= 0 {
		fmt.Fprintln(os.Stderr, "--year is required")
		return 2
	}

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config validation failed", "error", err)
		return 1
	}
	if cfg.DataBackend != "sqlite" {
		slog.Error("export-sheet requires DATA_BACKEND=sqlite", "got", cfg.DataBackend)
		return 1
	}

	ctx := context.Background()
	repo, err := storage.NewSQLiteRepository(cfg.SQLiteDBPath)
	if err != nil {
		slog.Error("open sqlite", "error", err, "path", cfg.SQLiteDBPath)
		return 1
	}
	defer func() { _ = repo.Close() }()

	client, err := gsheet.NewFromEnv(ctx)
	if err != nil {
		slog.Error("init Google Sheets client", "error", err)
		return 1
	}

	svc := services.NewExportSheetService(repo, client, client, client)
	counts, err := svc.Export(ctx, *year, *dryRun)
	if err != nil {
		slog.Error("export failed", "error", err)
		return 1
	}

	fmt.Printf("export-sheet year=%d dry_run=%v expenses=%d incomes=%d balances=%d errors=%d\n",
		*year, *dryRun, counts.Expenses, counts.Incomes, counts.Balances, len(counts.Errors))
	for _, e := range counts.Errors {
		fmt.Fprintln(os.Stderr, "  -", e)
	}
	if counts.HasErrors() {
		return 1
	}
	return 0
}
