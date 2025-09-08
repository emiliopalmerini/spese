package adapters

import (
	"context"
	"spese/internal/core"
	"spese/internal/services"
	"spese/internal/storage"
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

// GetStorage returns the underlying storage repository
// This is needed for accessing recurrent expenses functionality
func (a *SQLiteAdapter) GetStorage() *storage.SQLiteRepository {
	return a.storage
}
