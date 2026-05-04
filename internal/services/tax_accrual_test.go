package services

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"spese/internal/core"
	"spese/internal/storage"
)

func newTaxRepo(t *testing.T) *storage.SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	repo, err := storage.NewSQLiteRepository(filepath.Join(dir, "tax.db"))
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestComputeAccrualCents(t *testing.T) {
	cases := []struct {
		gross    int64
		basisPts int64
		want     int64
	}{
		{10000, 500, 500},   // 100.00 * 5%   = 5.00
		{10000, 2613, 2613}, // 100.00 * 26.13% = 26.13
		{0, 500, 0},         // zero gross
		{12345, 500, 617},   // 123.45 * 5% = 6.1725 → 617 cents (round half up)
		{12345, 0, 0},       // zero rate
	}
	for _, tc := range cases {
		got := computeAccrualCents(tc.gross, tc.basisPts)
		if got != tc.want {
			t.Errorf("compute(%d,%d) = %d, want %d", tc.gross, tc.basisPts, got, tc.want)
		}
	}
}

func TestTaxAccrualService_OnIncomeCreatedFreelance(t *testing.T) {
	repo := newTaxRepo(t)
	svc := NewTaxAccrualService(repo)
	ctx := context.Background()

	// Create a freelance income via the repo (categories are seeded).
	id, err := repo.AppendIncome(ctx, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "consulting",
		Amount:      core.Money{Cents: 100000}, // €1000
		Category:    "GFreelance",
	})
	if err != nil {
		t.Fatalf("append income: %v", err)
	}
	incomeID, _ := strconv.ParseInt(id, 10, 64)

	svc.OnIncomeCreated(ctx, incomeID, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "consulting",
		Amount:      core.Money{Cents: 100000},
		Category:    "GFreelance",
	})

	rows, err := repo.ListTaxAccrualsByIncome(ctx, incomeID)
	if err != nil {
		t.Fatalf("list accruals: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 accruals, got %d", len(rows))
	}
	byCode := map[string]storage.TaxAccrualRow{}
	for _, r := range rows {
		byCode[r.TaxCode] = r
	}
	imp, ok := byCode["imposta_sostitutiva"]
	if !ok {
		t.Fatalf("missing imposta_sostitutiva accrual")
	}
	if imp.AmountCents != 5000 { // 5% of 100000
		t.Fatalf("imposta_sostitutiva expected 5000 cents, got %d", imp.AmountCents)
	}
	inps, ok := byCode["inps"]
	if !ok {
		t.Fatalf("missing inps accrual")
	}
	if inps.AmountCents != 26130 { // 26.13% of 100000
		t.Fatalf("inps expected 26130 cents, got %d", inps.AmountCents)
	}
}

func TestTaxAccrualService_NoAccrualForNonFreelance(t *testing.T) {
	repo := newTaxRepo(t)
	svc := NewTaxAccrualService(repo)
	ctx := context.Background()

	id, err := repo.AppendIncome(ctx, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "stipendio",
		Amount:      core.Money{Cents: 200000},
		Category:    "Stipendio E",
	})
	if err != nil {
		t.Fatalf("append income: %v", err)
	}
	incomeID, _ := strconv.ParseInt(id, 10, 64)
	svc.OnIncomeCreated(ctx, incomeID, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "stipendio",
		Amount:      core.Money{Cents: 200000},
		Category:    "Stipendio E",
	})

	rows, err := repo.ListTaxAccrualsByIncome(ctx, incomeID)
	if err != nil {
		t.Fatalf("list accruals: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 accruals for non-freelance, got %d", len(rows))
	}
}

func TestTaxAccrualService_Idempotent(t *testing.T) {
	repo := newTaxRepo(t)
	svc := NewTaxAccrualService(repo)
	ctx := context.Background()

	id, err := repo.AppendIncome(ctx, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "consulting",
		Amount:      core.Money{Cents: 50000},
		Category:    "EFreelance",
	})
	if err != nil {
		t.Fatalf("append income: %v", err)
	}
	incomeID, _ := strconv.ParseInt(id, 10, 64)
	income := core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "consulting",
		Amount:      core.Money{Cents: 50000},
		Category:    "EFreelance",
	}
	svc.OnIncomeCreated(ctx, incomeID, income)
	svc.OnIncomeCreated(ctx, incomeID, income)

	rows, err := repo.ListTaxAccrualsByIncome(ctx, incomeID)
	if err != nil {
		t.Fatalf("list accruals: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 accruals after double call, got %d", len(rows))
	}
}

func TestSumTaxAccrualsByMonth(t *testing.T) {
	repo := newTaxRepo(t)
	svc := NewTaxAccrualService(repo)
	ctx := context.Background()

	id, _ := repo.AppendIncome(ctx, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "x",
		Amount:      core.Money{Cents: 100000},
		Category:    "GFreelance",
	})
	incomeID, _ := strconv.ParseInt(id, 10, 64)
	svc.OnIncomeCreated(ctx, incomeID, core.Income{
		Date:        core.NewDate(2025, 6, 15),
		Description: "x",
		Amount:      core.Money{Cents: 100000},
		Category:    "GFreelance",
	})

	total, err := svc.MonthlyAccrualByCode(ctx, "imposta_sostitutiva", 2025, 6)
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if total.Cents != 5000 {
		t.Fatalf("expected 5000 cents, got %d", total.Cents)
	}
}
