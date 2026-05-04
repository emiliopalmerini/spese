package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"spese/internal/core"
)

var (
	ErrAccountExists   = errors.New("account already exists")
	ErrAccountNotFound = errors.New("account not found")
)

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func accountFromRow(a Account) core.Account {
	return core.Account{
		ID:     a.ID,
		Name:   a.Name,
		Type:   core.AccountType(a.Type),
		Active: a.Active != 0,
	}
}

func balanceFromRow(b AccountBalance) core.AccountBalance {
	return core.AccountBalance{
		AccountID: b.AccountID,
		Year:      int(b.Year),
		Month:     int(b.Month),
		Amount:    core.Money{Cents: b.AmountCents},
	}
}

// CreateAccount inserts a new account. Returns ErrAccountExists when the name
// is already taken.
func (r *SQLiteRepository) CreateAccount(ctx context.Context, a core.Account) (int64, error) {
	row, err := r.queries.CreateAccount(ctx, CreateAccountParams{
		Name:   a.Name,
		Type:   string(a.Type),
		Active: boolToInt64(a.Active),
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrAccountExists
		}
		return 0, fmt.Errorf("create account: %w", err)
	}
	return row.ID, nil
}

// UpdateAccount updates an existing account by ID.
func (r *SQLiteRepository) UpdateAccount(ctx context.Context, a core.Account) error {
	err := r.queries.UpdateAccount(ctx, UpdateAccountParams{
		ID:     a.ID,
		Name:   a.Name,
		Type:   string(a.Type),
		Active: boolToInt64(a.Active),
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrAccountExists
		}
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}

// ListAccounts returns all accounts; when includeInactive is false only active
// accounts are returned.
func (r *SQLiteRepository) ListAccounts(ctx context.Context, includeInactive bool) ([]core.Account, error) {
	var rows []Account
	var err error
	if includeInactive {
		rows, err = r.readQueries.ListAllAccounts(ctx)
	} else {
		rows, err = r.readQueries.ListActiveAccounts(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]core.Account, len(rows))
	for i, a := range rows {
		out[i] = accountFromRow(a)
	}
	return out, nil
}

// GetAccount returns the account by ID. Returns ErrAccountNotFound if absent.
func (r *SQLiteRepository) GetAccount(ctx context.Context, id int64) (core.Account, error) {
	row, err := r.readQueries.GetAccountByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.Account{}, ErrAccountNotFound
		}
		return core.Account{}, fmt.Errorf("get account: %w", err)
	}
	return accountFromRow(row), nil
}

// UpsertBalance writes or overwrites the balance for (account_id, year, month).
// Returns ErrAccountNotFound if the account does not exist.
func (r *SQLiteRepository) UpsertBalance(ctx context.Context, b core.AccountBalance) error {
	if _, err := r.readQueries.GetAccountByID(ctx, b.AccountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("verify account: %w", err)
	}
	err := r.queries.UpsertAccountBalance(ctx, UpsertAccountBalanceParams{
		AccountID:   b.AccountID,
		Year:        int64(b.Year),
		Month:       int64(b.Month),
		AmountCents: b.Amount.Cents,
	})
	if err != nil {
		return fmt.Errorf("upsert balance: %w", err)
	}
	return nil
}

// ListBalancesByMonth returns all balances for a specific year/month.
func (r *SQLiteRepository) ListBalancesByMonth(ctx context.Context, year, month int) ([]core.AccountBalance, error) {
	rows, err := r.readQueries.ListBalancesForMonth(ctx, ListBalancesForMonthParams{
		Year:  int64(year),
		Month: int64(month),
	})
	if err != nil {
		return nil, fmt.Errorf("list balances by month: %w", err)
	}
	out := make([]core.AccountBalance, len(rows))
	for i, b := range rows {
		out[i] = balanceFromRow(b)
	}
	return out, nil
}

// ListBalancesByAccount returns all balances tracked for an account, newest
// month first.
func (r *SQLiteRepository) ListBalancesByAccount(ctx context.Context, accountID int64) ([]core.AccountBalance, error) {
	rows, err := r.readQueries.ListBalancesForAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list balances by account: %w", err)
	}
	out := make([]core.AccountBalance, len(rows))
	for i, b := range rows {
		out[i] = balanceFromRow(b)
	}
	return out, nil
}

// LatestBalances returns one row per account with its most recent balance.
func (r *SQLiteRepository) LatestBalances(ctx context.Context) ([]core.AccountBalance, error) {
	rows, err := r.readQueries.GetLatestBalancePerAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("latest balances: %w", err)
	}
	out := make([]core.AccountBalance, len(rows))
	for i, b := range rows {
		out[i] = balanceFromRow(b)
	}
	return out, nil
}

// MonthlyNetWorth returns the sum of all account balances for (year, month).
// Returns 0 when no balances are recorded for that month.
func (r *SQLiteRepository) MonthlyNetWorth(ctx context.Context, year, month int) (core.Money, error) {
	total, err := r.readQueries.GetMonthlyNetWorthTotal(ctx, GetMonthlyNetWorthTotalParams{
		Year:  int64(year),
		Month: int64(month),
	})
	if err != nil {
		return core.Money{}, fmt.Errorf("monthly net worth: %w", err)
	}
	return core.Money{Cents: total}, nil
}
