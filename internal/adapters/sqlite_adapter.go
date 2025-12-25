package adapters

import (
	"context"
	"fmt"
	"spese/internal/core"
	"spese/internal/services"
	"spese/internal/sheets"
	"spese/internal/storage"
	"strconv"
)

// SQLiteAdapter adapts SQLiteRepository and ExpenseService to implement sheets.* interfaces
// This allows the HTTP handlers to work unchanged while using SQLite + AMQP backend
type SQLiteAdapter struct {
	storage *storage.SQLiteRepository
	service *services.ExpenseService
}

func NewSQLiteAdapter(storage *storage.SQLiteRepository, service *services.ExpenseService) *SQLiteAdapter {
	return &SQLiteAdapter{
		storage: storage,
		service: service,
	}
}

// Append implements sheets.ExpenseWriter
func (a *SQLiteAdapter) Append(ctx context.Context, e core.Expense) (string, error) {
	return a.service.CreateExpense(ctx, e)
}

// List implements sheets.TaxonomyReader
func (a *SQLiteAdapter) List(ctx context.Context) ([]string, []string, error) {
	return a.storage.List(ctx)
}

// ReadMonthOverview implements sheets.DashboardReader
func (a *SQLiteAdapter) ReadMonthOverview(ctx context.Context, year int, month int) (core.MonthOverview, error) {
	return a.storage.ReadMonthOverview(ctx, year, month)
}

// ListExpenses implements sheets.ExpenseLister
func (a *SQLiteAdapter) ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error) {
	return a.storage.ListExpenses(ctx, year, month)
}

// GetSecondariesByPrimary returns secondary categories for a given primary category
func (a *SQLiteAdapter) GetSecondariesByPrimary(ctx context.Context, primaryCategory string) ([]string, error) {
	return a.storage.GetSecondariesByPrimary(ctx, primaryCategory)
}

// DeleteExpense implements sheets.ExpenseDeleter
func (a *SQLiteAdapter) DeleteExpense(ctx context.Context, id string) error {
	expenseID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid expense ID: %w", err)
	}

	return a.service.DeleteExpense(ctx, expenseID)
}

// ListExpensesWithID implements sheets.ExpenseListerWithID
func (a *SQLiteAdapter) ListExpensesWithID(ctx context.Context, year int, month int) ([]sheets.ExpenseWithID, error) {
	storageExpenses, err := a.storage.ListExpensesWithID(ctx, year, month)
	if err != nil {
		return nil, err
	}

	// Convert from storage.ExpenseWithID to sheets.ExpenseWithID
	result := make([]sheets.ExpenseWithID, len(storageExpenses))
	for i, se := range storageExpenses {
		result[i] = sheets.ExpenseWithID{
			ID:      se.ID,
			Expense: se.Expense,
		}
	}

	return result, nil
}

// GetStorage returns the underlying storage repository
// This is needed for accessing recurrent expenses functionality
func (a *SQLiteAdapter) GetStorage() *storage.SQLiteRepository {
	return a.storage
}

// Income methods

// AppendIncome creates a new income entry
func (a *SQLiteAdapter) AppendIncome(ctx context.Context, i core.Income) (string, error) {
	return a.storage.AppendIncome(ctx, i)
}

// GetIncomeCategories returns all income categories
func (a *SQLiteAdapter) GetIncomeCategories(ctx context.Context) ([]string, error) {
	return a.storage.GetIncomeCategories(ctx)
}

// ReadIncomeMonthOverview returns monthly income overview
func (a *SQLiteAdapter) ReadIncomeMonthOverview(ctx context.Context, year int, month int) (core.IncomeMonthOverview, error) {
	return a.storage.ReadIncomeMonthOverview(ctx, year, month)
}

// ListIncomes returns all incomes for a given month
func (a *SQLiteAdapter) ListIncomes(ctx context.Context, year int, month int) ([]core.Income, error) {
	return a.storage.ListIncomes(ctx, year, month)
}

// ListIncomesWithID returns incomes with their IDs
func (a *SQLiteAdapter) ListIncomesWithID(ctx context.Context, year int, month int) ([]storage.IncomeWithID, error) {
	return a.storage.ListIncomesWithID(ctx, year, month)
}

// DeleteIncome deletes an income entry
func (a *SQLiteAdapter) DeleteIncome(ctx context.Context, id string) error {
	incomeID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid income ID: %w", err)
	}
	return a.storage.HardDeleteIncome(ctx, incomeID)
}
