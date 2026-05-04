package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"spese/internal/core"
	"spese/internal/sheets"
	"spese/internal/storage"
)

// ExportSheetService walks SQLite for a year and idempotently upserts every
// expense, income, and net-worth balance to the configured remote writers.
//
// Re-running with the same data is a no-op for the sheet (id-based upsert
// updates rows in place). The service is the engine behind the
// `spese export-sheet` CLI introduced in ADR-0007 step 4.
type ExportSheetService struct {
	storage   *storage.SQLiteRepository
	expWriter sheets.RemoteExpenseWriter
	incWriter sheets.RemoteIncomeWriter
	nwWriter  sheets.NetWorthWriter
}

// NewExportSheetService wires the repository and the three remote writers.
// Any of the writers may be nil to disable that slice of the export.
func NewExportSheetService(
	repo *storage.SQLiteRepository,
	exp sheets.RemoteExpenseWriter,
	inc sheets.RemoteIncomeWriter,
	nw sheets.NetWorthWriter,
) *ExportSheetService {
	return &ExportSheetService{storage: repo, expWriter: exp, incWriter: inc, nwWriter: nw}
}

// ExportCounts summarizes the result of an export run.
type ExportCounts struct {
	Expenses int
	Incomes  int
	Balances int
	Errors   []error
}

// HasErrors reports whether the run accumulated any per-row errors.
func (c ExportCounts) HasErrors() bool { return len(c.Errors) > 0 }

// Export iterates SQLite expenses, incomes and balances for the given year
// and upserts each row to the configured remote writers. With dryRun=true
// rows are counted but no writes happen. Per-row errors are collected so a
// single failure does not abort the run; the count plus errors is returned.
func (s *ExportSheetService) Export(ctx context.Context, year int, dryRun bool) (ExportCounts, error) {
	if year <= 0 {
		return ExportCounts{}, fmt.Errorf("export-sheet: invalid year %d", year)
	}
	if s.storage == nil {
		return ExportCounts{}, errors.New("export-sheet: storage not configured")
	}
	var counts ExportCounts

	for month := 1; month <= 12; month++ {
		if err := s.exportExpenses(ctx, year, month, dryRun, &counts); err != nil {
			return counts, err
		}
		if err := s.exportIncomes(ctx, year, month, dryRun, &counts); err != nil {
			return counts, err
		}
		if err := s.exportBalances(ctx, year, month, dryRun, &counts); err != nil {
			return counts, err
		}
	}
	slog.InfoContext(ctx, "Export complete",
		"year", year,
		"dry_run", dryRun,
		"expenses", counts.Expenses,
		"incomes", counts.Incomes,
		"balances", counts.Balances,
		"errors", len(counts.Errors))
	return counts, nil
}

func (s *ExportSheetService) exportExpenses(ctx context.Context, year, month int, dryRun bool, counts *ExportCounts) error {
	rows, err := s.storage.ListExpensesWithID(ctx, year, month)
	if err != nil {
		return fmt.Errorf("list expenses %d/%d: %w", year, month, err)
	}
	for _, r := range rows {
		id, err := strconv.ParseInt(r.ID, 10, 64)
		if err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("parse expense id %q: %w", r.ID, err))
			continue
		}
		e := r.Expense
		e.ID = id
		counts.Expenses++
		if dryRun || s.expWriter == nil {
			continue
		}
		if _, err := s.expWriter.UpsertExpense(ctx, e); err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("upsert expense %d: %w", id, err))
		}
	}
	return nil
}

func (s *ExportSheetService) exportIncomes(ctx context.Context, year, month int, dryRun bool, counts *ExportCounts) error {
	rows, err := s.storage.ListIncomesWithID(ctx, year, month)
	if err != nil {
		return fmt.Errorf("list incomes %d/%d: %w", year, month, err)
	}
	for _, r := range rows {
		id, err := strconv.ParseInt(r.ID, 10, 64)
		if err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("parse income id %q: %w", r.ID, err))
			continue
		}
		i := r.Income
		i.ID = id
		counts.Incomes++
		if dryRun || s.incWriter == nil {
			continue
		}
		if _, err := s.incWriter.UpsertIncome(ctx, i); err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("upsert income %d: %w", id, err))
		}
	}
	return nil
}

func (s *ExportSheetService) exportBalances(ctx context.Context, year, month int, dryRun bool, counts *ExportCounts) error {
	balances, err := s.storage.ListBalancesByMonth(ctx, year, month)
	if err != nil {
		return fmt.Errorf("list balances %d/%d: %w", year, month, err)
	}
	for _, b := range balances {
		acc, err := s.storage.GetAccount(ctx, b.AccountID)
		if err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("get account %d: %w", b.AccountID, err))
			continue
		}
		counts.Balances++
		if dryRun || s.nwWriter == nil {
			continue
		}
		if _, err := s.nwWriter.UpsertBalance(ctx, acc.Name, acc.Type, b.Year, b.Month, core.Money{Cents: b.Amount.Cents}); err != nil {
			counts.Errors = append(counts.Errors, fmt.Errorf("upsert balance acc=%d %d/%d: %w", b.AccountID, b.Year, b.Month, err))
		}
	}
	return nil
}
