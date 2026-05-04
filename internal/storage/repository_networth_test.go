package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"spese/internal/core"
)

func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	repo, err := NewSQLiteRepository(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func mustCreateAccount(t *testing.T, r *SQLiteRepository, a core.Account) int64 {
	t.Helper()
	id, err := r.CreateAccount(context.Background(), a)
	if err != nil {
		t.Fatalf("create account %q: %v", a.Name, err)
	}
	return id
}

func TestCreateAndListAccounts(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	mustCreateAccount(t, r, core.Account{Name: "Conto BCC", Type: core.AccountCash, Active: true})
	mustCreateAccount(t, r, core.Account{Name: "Trade Republic", Type: core.AccountLongTerm, Active: true})
	mustCreateAccount(t, r, core.Account{Name: "Old Wallet", Type: core.AccountCash, Active: false})

	active, err := r.ListAccounts(ctx, false)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active accounts, got %d", len(active))
	}

	all, err := r.ListAccounts(ctx, true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 accounts including inactive, got %d", len(all))
	}
}

func TestCreateAccountDuplicateName(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	a := core.Account{Name: "Dup", Type: core.AccountCash, Active: true}

	if _, err := r.CreateAccount(ctx, a); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := r.CreateAccount(ctx, a)
	if !errors.Is(err, ErrAccountExists) {
		t.Fatalf("expected ErrAccountExists, got %v", err)
	}
}

func TestUpsertBalanceOverwrites(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	id := mustCreateAccount(t, r, core.Account{Name: "BCC", Type: core.AccountCash, Active: true})

	if err := r.UpsertBalance(ctx, core.AccountBalance{
		AccountID: id, Year: 2025, Month: 6, Amount: core.Money{Cents: 10000},
	}); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := r.UpsertBalance(ctx, core.AccountBalance{
		AccountID: id, Year: 2025, Month: 6, Amount: core.Money{Cents: 25000},
	}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	bs, err := r.ListBalancesByMonth(ctx, 2025, 6)
	if err != nil {
		t.Fatalf("list balances: %v", err)
	}
	if len(bs) != 1 {
		t.Fatalf("expected 1 balance after upsert, got %d", len(bs))
	}
	if bs[0].Amount.Cents != 25000 {
		t.Fatalf("expected 25000 cents, got %d", bs[0].Amount.Cents)
	}
}

func TestUpsertBalanceUnknownAccount(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	err := r.UpsertBalance(ctx, core.AccountBalance{
		AccountID: 9999, Year: 2025, Month: 1, Amount: core.Money{Cents: 100},
	})
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestMonthlyNetWorthSum(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	a := mustCreateAccount(t, r, core.Account{Name: "BCC", Type: core.AccountCash, Active: true})
	b := mustCreateAccount(t, r, core.Account{Name: "TR", Type: core.AccountLongTerm, Active: true})

	if err := r.UpsertBalance(ctx, core.AccountBalance{AccountID: a, Year: 2025, Month: 6, Amount: core.Money{Cents: 1500}}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertBalance(ctx, core.AccountBalance{AccountID: b, Year: 2025, Month: 6, Amount: core.Money{Cents: 4000}}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertBalance(ctx, core.AccountBalance{AccountID: a, Year: 2025, Month: 5, Amount: core.Money{Cents: 999}}); err != nil {
		t.Fatal(err)
	}

	tot, err := r.MonthlyNetWorth(ctx, 2025, 6)
	if err != nil {
		t.Fatalf("monthly: %v", err)
	}
	if tot.Cents != 5500 {
		t.Fatalf("expected 5500 cents, got %d", tot.Cents)
	}

	// Empty month → zero, not error
	zero, err := r.MonthlyNetWorth(ctx, 2025, 1)
	if err != nil {
		t.Fatalf("monthly empty: %v", err)
	}
	if zero.Cents != 0 {
		t.Fatalf("expected 0 cents for empty month, got %d", zero.Cents)
	}
}

func TestLatestBalancesPerAccount(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	a := mustCreateAccount(t, r, core.Account{Name: "BCC", Type: core.AccountCash, Active: true})
	b := mustCreateAccount(t, r, core.Account{Name: "TR", Type: core.AccountLongTerm, Active: true})

	for _, ent := range []core.AccountBalance{
		{AccountID: a, Year: 2025, Month: 1, Amount: core.Money{Cents: 100}},
		{AccountID: a, Year: 2025, Month: 6, Amount: core.Money{Cents: 600}},
		{AccountID: a, Year: 2024, Month: 12, Amount: core.Money{Cents: 1200}},
		{AccountID: b, Year: 2025, Month: 3, Amount: core.Money{Cents: 300}},
	} {
		if err := r.UpsertBalance(ctx, ent); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := r.LatestBalances(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest rows, got %d", len(latest))
	}

	byAccount := map[int64]core.AccountBalance{}
	for _, l := range latest {
		byAccount[l.AccountID] = l
	}
	if got := byAccount[a]; got.Year != 2025 || got.Month != 6 || got.Amount.Cents != 600 {
		t.Fatalf("unexpected latest for account a: %+v", got)
	}
	if got := byAccount[b]; got.Year != 2025 || got.Month != 3 || got.Amount.Cents != 300 {
		t.Fatalf("unexpected latest for account b: %+v", got)
	}
}

func TestListBalancesByAccount(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	a := mustCreateAccount(t, r, core.Account{Name: "BCC", Type: core.AccountCash, Active: true})

	for _, ent := range []core.AccountBalance{
		{AccountID: a, Year: 2025, Month: 1, Amount: core.Money{Cents: 100}},
		{AccountID: a, Year: 2025, Month: 3, Amount: core.Money{Cents: 300}},
		{AccountID: a, Year: 2024, Month: 12, Amount: core.Money{Cents: 50}},
	} {
		if err := r.UpsertBalance(ctx, ent); err != nil {
			t.Fatal(err)
		}
	}

	bs, err := r.ListBalancesByAccount(ctx, a)
	if err != nil {
		t.Fatalf("list by account: %v", err)
	}
	if len(bs) != 3 {
		t.Fatalf("expected 3 balances, got %d", len(bs))
	}
	// Newest first: 2025-03, 2025-01, 2024-12
	if bs[0].Year != 2025 || bs[0].Month != 3 {
		t.Fatalf("expected newest 2025-03 first, got %d-%d", bs[0].Year, bs[0].Month)
	}
	if bs[2].Year != 2024 || bs[2].Month != 12 {
		t.Fatalf("expected oldest 2024-12 last, got %d-%d", bs[2].Year, bs[2].Month)
	}
}

func TestGetAccountNotFound(t *testing.T) {
	r := newTestRepo(t)
	_, err := r.GetAccount(context.Background(), 12345)
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestUpdateAccount(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	id := mustCreateAccount(t, r, core.Account{Name: "Old", Type: core.AccountCash, Active: true})

	if err := r.UpdateAccount(ctx, core.Account{ID: id, Name: "New", Type: core.AccountLongTerm, Active: false}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := r.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New" || got.Type != core.AccountLongTerm || got.Active {
		t.Fatalf("unexpected updated account: %+v", got)
	}
}
