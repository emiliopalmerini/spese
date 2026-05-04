package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"spese/internal/core"
	"spese/internal/storage"
)

type fakeRemoteExpenseWriter struct {
	mu      sync.Mutex
	calls   []core.Expense
	failID  int64
	failErr error
}

func (w *fakeRemoteExpenseWriter) UpsertExpense(_ context.Context, e core.Expense) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, e)
	if w.failID != 0 && e.ID == w.failID {
		return "", w.failErr
	}
	return "ref", nil
}

type fakeRemoteIncomeWriter struct {
	mu    sync.Mutex
	calls []core.Income
}

func (w *fakeRemoteIncomeWriter) UpsertIncome(_ context.Context, i core.Income) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, i)
	return "ref", nil
}

type fakeNWWriter struct {
	mu    sync.Mutex
	calls []struct {
		Name  string
		Type  core.AccountType
		Year  int
		Month int
		Amt   core.Money
	}
}

func (w *fakeNWWriter) UpsertBalance(_ context.Context, name string, typ core.AccountType, year, month int, amount core.Money) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, struct {
		Name  string
		Type  core.AccountType
		Year  int
		Month int
		Amt   core.Money
	}{name, typ, year, month, amount})
	return "ref", nil
}

func newExportRepo(t *testing.T) *storage.SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	repo, err := storage.NewSQLiteRepository(filepath.Join(dir, "export.db"))
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestExportSheet_Empty(t *testing.T) {
	repo := newExportRepo(t)
	exp := &fakeRemoteExpenseWriter{}
	inc := &fakeRemoteIncomeWriter{}
	nw := &fakeNWWriter{}
	svc := NewExportSheetService(repo, exp, inc, nw)

	counts, err := svc.Export(context.Background(), 2026, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if counts.Expenses != 0 || counts.Incomes != 0 || counts.Balances != 0 {
		t.Fatalf("expected zero counts, got %+v", counts)
	}
	if len(exp.calls)+len(inc.calls)+len(nw.calls) != 0 {
		t.Fatalf("no writers should be called on empty year")
	}
}

func TestExportSheet_RoundTrip(t *testing.T) {
	repo := newExportRepo(t)
	ctx := context.Background()

	if _, err := repo.Append(ctx, core.Expense{
		Date:        core.NewDate(2026, 3, 5),
		Description: "Bento",
		Amount:      core.Money{Cents: 1234},
		Primary:     "Lavoro",
		Secondary:   "Lavoro e",
	}); err != nil {
		t.Fatalf("seed expense: %v", err)
	}
	if _, err := repo.AppendIncome(ctx, core.Income{
		Date:        core.NewDate(2026, 3, 5),
		Description: "Salary",
		Amount:      core.Money{Cents: 200000},
		Category:    "ESalary",
	}); err != nil {
		t.Fatalf("seed income: %v", err)
	}

	accID, err := repo.CreateAccount(ctx, core.Account{Name: "Conto BCC", Type: core.AccountCash, Active: true})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := repo.UpsertBalanceAndEnqueueSync(ctx, core.AccountBalance{
		AccountID: accID,
		Year:      2026,
		Month:     3,
		Amount:    core.Money{Cents: 700000},
	}); err != nil {
		t.Fatalf("upsert balance: %v", err)
	}

	exp := &fakeRemoteExpenseWriter{}
	inc := &fakeRemoteIncomeWriter{}
	nw := &fakeNWWriter{}
	svc := NewExportSheetService(repo, exp, inc, nw)

	counts, err := svc.Export(ctx, 2026, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if counts.Expenses != 1 || counts.Incomes != 1 || counts.Balances != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	if len(counts.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", counts.Errors)
	}
	if exp.calls[0].ID == 0 {
		t.Fatalf("expense ID must be propagated, got %+v", exp.calls[0])
	}
	if inc.calls[0].ID == 0 {
		t.Fatalf("income ID must be propagated, got %+v", inc.calls[0])
	}
	if nw.calls[0].Name != "Conto BCC" || nw.calls[0].Type != core.AccountCash {
		t.Fatalf("balance writer got wrong account: %+v", nw.calls[0])
	}

	// Re-running is idempotent: counts repeat, but no errors.
	counts2, err := svc.Export(ctx, 2026, false)
	if err != nil {
		t.Fatalf("export 2: %v", err)
	}
	if counts2.Expenses != 1 || counts2.Incomes != 1 || counts2.Balances != 1 {
		t.Fatalf("rerun counts changed: %+v", counts2)
	}
}

func TestExportSheet_DryRun(t *testing.T) {
	repo := newExportRepo(t)
	ctx := context.Background()
	if _, err := repo.Append(ctx, core.Expense{
		Date:        core.NewDate(2026, 1, 1),
		Description: "x",
		Amount:      core.Money{Cents: 100},
		Primary:     "Casa",
		Secondary:   "Mutuo",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	exp := &fakeRemoteExpenseWriter{}
	svc := NewExportSheetService(repo, exp, nil, nil)
	counts, err := svc.Export(ctx, 2026, true)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if counts.Expenses != 1 {
		t.Fatalf("expected 1 expense counted, got %d", counts.Expenses)
	}
	if len(exp.calls) != 0 {
		t.Fatalf("dry-run must not call writer; got %d calls", len(exp.calls))
	}
}

func TestExportSheet_PerRowErrorContinues(t *testing.T) {
	repo := newExportRepo(t)
	ctx := context.Background()

	var ids []int64
	for i := 0; i < 3; i++ {
		ref, err := repo.Append(ctx, core.Expense{
			Date:        core.NewDate(2026, 2, i+1),
			Description: "x",
			Amount:      core.Money{Cents: int64(100 * (i + 1))},
			Primary:     "Casa",
			Secondary:   "Mutuo",
		})
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		var id int64
		if _, err := fmt.Sscanf(ref, "%d", &id); err != nil {
			t.Fatalf("parse id: %v", err)
		}
		ids = append(ids, id)
	}

	exp := &fakeRemoteExpenseWriter{failID: ids[1], failErr: errors.New("boom")}
	svc := NewExportSheetService(repo, exp, nil, nil)
	counts, err := svc.Export(ctx, 2026, false)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if counts.Expenses != 3 {
		t.Fatalf("all rows should be counted, got %d", counts.Expenses)
	}
	if len(counts.Errors) != 1 {
		t.Fatalf("expected exactly 1 per-row error, got %d (%v)", len(counts.Errors), counts.Errors)
	}
	if len(exp.calls) != 3 {
		t.Fatalf("writer should be called for every row, got %d", len(exp.calls))
	}
}

func TestExportSheet_InvalidYear(t *testing.T) {
	svc := NewExportSheetService(newExportRepo(t), nil, nil, nil)
	if _, err := svc.Export(context.Background(), 0, false); err == nil {
		t.Fatal("expected error for year=0")
	}
}
