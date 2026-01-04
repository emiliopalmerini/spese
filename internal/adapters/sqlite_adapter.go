package adapters

import (
	"context"
	"fmt"
	"time"

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

// GetAllCategoriesWithSubs returns all categories with their subcategories
func (a *SQLiteAdapter) GetAllCategoriesWithSubs(ctx context.Context) ([]storage.CategoryWithSubs, error) {
	return a.storage.GetAllCategoriesWithSubs(ctx)
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

// Dashboard methods

// Transaction represents a unified view of expense or income for dashboard
type Transaction struct {
	ID          string // ID for delete/edit
	Type        string // "expense" or "income"
	Description string
	Category    string
	AmountCents int64
	Date        time.Time
}

// TrendPoint represents a data point for the trend chart
type TrendPoint struct {
	Date        string
	AmountCents int64
}

// CategoryTotal represents a category with its total amount
type CategoryTotal struct {
	Name        string
	AmountCents int64
}

// GetMonthlyExpenseTotal returns total expenses for a given month in cents
func (a *SQLiteAdapter) GetMonthlyExpenseTotal(ctx context.Context, year, month int) (int64, error) {
	overview, err := a.storage.ReadMonthOverview(ctx, year, month)
	if err != nil {
		return 0, err
	}
	return overview.Total.Cents, nil
}

// GetMonthlyIncomeTotal returns total income for a given month in cents
func (a *SQLiteAdapter) GetMonthlyIncomeTotal(ctx context.Context, year, month int) (int64, error) {
	overview, err := a.storage.ReadIncomeMonthOverview(ctx, year, month)
	if err != nil {
		return 0, err
	}
	return overview.Total.Cents, nil
}

// GetRecentTransactions returns the most recent transactions (expenses and incomes combined)
func (a *SQLiteAdapter) GetRecentTransactions(ctx context.Context, limit int) ([]Transaction, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Get recent expenses
	expenses, err := a.storage.ListExpensesWithID(ctx, year, month)
	if err != nil {
		return nil, err
	}

	// Get recent incomes
	incomes, err := a.storage.ListIncomesWithID(ctx, year, month)
	if err != nil {
		return nil, err
	}

	// Combine and sort by date
	var transactions []Transaction
	for _, e := range expenses {
		transactions = append(transactions, Transaction{
			ID:          e.ID,
			Type:        "expense",
			Description: e.Expense.Description,
			Category:    e.Expense.Primary,
			AmountCents: e.Expense.Amount.Cents,
			Date:        e.Expense.Date.Time,
		})
	}
	for _, i := range incomes {
		transactions = append(transactions, Transaction{
			ID:          i.ID,
			Type:        "income",
			Description: i.Income.Description,
			Category:    i.Income.Category,
			AmountCents: i.Income.Amount.Cents,
			Date:        i.Income.Date.Time,
		})
	}

	// Sort by date descending
	for i := 0; i < len(transactions)-1; i++ {
		for j := i + 1; j < len(transactions); j++ {
			if transactions[j].Date.After(transactions[i].Date) {
				transactions[i], transactions[j] = transactions[j], transactions[i]
			}
		}
	}

	// Limit results
	if len(transactions) > limit {
		transactions = transactions[:limit]
	}

	return transactions, nil
}

// GetExpenseTrend returns expense totals grouped by date for a given period
func (a *SQLiteAdapter) GetExpenseTrend(ctx context.Context, period string) ([]TrendPoint, error) {
	now := time.Now()
	var startDate time.Time

	switch period {
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	case "3months":
		startDate = now.AddDate(0, -3, 0)
	case "6months":
		startDate = now.AddDate(0, -6, 0)
	case "year":
		startDate = now.AddDate(-1, 0, 0)
	default:
		startDate = now.AddDate(0, -1, 0)
	}

	// Get all expenses in range and group by date
	expenses, err := a.storage.ListExpensesByDateRange(ctx, startDate, now)
	if err != nil {
		return nil, err
	}

	// Group by date using ISO format for proper sorting
	type dateEntry struct {
		isoDate string // 2006-01-02 for sorting
		display string // DD/MM for display
		amount  int64
	}
	dateMap := make(map[string]*dateEntry)
	for _, e := range expenses {
		isoDate := e.Date.Time.Format("2006-01-02")
		if entry, exists := dateMap[isoDate]; exists {
			entry.amount += e.Amount.Cents
		} else {
			dateMap[isoDate] = &dateEntry{
				isoDate: isoDate,
				display: e.Date.Time.Format("02/01"),
				amount:  e.Amount.Cents,
			}
		}
	}

	// Convert to sorted list
	var entries []*dateEntry
	for _, entry := range dateMap {
		entries = append(entries, entry)
	}

	// Sort by ISO date (proper chronological order)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].isoDate > entries[j].isoDate {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Convert to TrendPoint with display date
	var points []TrendPoint
	for _, entry := range entries {
		points = append(points, TrendPoint{
			Date:        entry.display,
			AmountCents: entry.amount,
		})
	}

	return points, nil
}

// GetCategoryBreakdown returns expense totals by primary category for a given period
func (a *SQLiteAdapter) GetCategoryBreakdown(ctx context.Context, period string) ([]CategoryTotal, error) {
	now := time.Now()
	var startDate time.Time

	switch period {
	case "week":
		// Current week (Monday to now)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		startDate = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	case "month":
		// Current calendar month (1st of month to now)
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case "quarter":
		// Current quarter (Q1: Jan-Mar, Q2: Apr-Jun, Q3: Jul-Sep, Q4: Oct-Dec)
		quarterMonth := ((int(now.Month())-1)/3)*3 + 1
		startDate = time.Date(now.Year(), time.Month(quarterMonth), 1, 0, 0, 0, 0, now.Location())
	case "year":
		// Current calendar year (Jan 1 to now)
		startDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	default:
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	expenses, err := a.storage.ListExpensesByDateRange(ctx, startDate, now)
	if err != nil {
		return nil, err
	}

	// Group by category
	catMap := make(map[string]int64)
	for _, e := range expenses {
		catMap[e.Primary] += e.Amount.Cents
	}

	// Convert to sorted list (by amount descending)
	var cats []CategoryTotal
	for name, amount := range catMap {
		cats = append(cats, CategoryTotal{
			Name:        name,
			AmountCents: amount,
		})
	}

	// Sort by amount descending
	for i := 0; i < len(cats)-1; i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[j].AmountCents > cats[i].AmountCents {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}

	return cats, nil
}

// ListIncomeCategories returns all income category names
func (a *SQLiteAdapter) ListIncomeCategories(ctx context.Context) ([]string, error) {
	return a.storage.GetIncomeCategories(ctx)
}
