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

// RecurrentExpenseItem represents a recurrent expense for display
type RecurrentExpenseItem struct {
	ID          int64
	Description string
	AmountCents int64
	Category    string
	Frequency   string
}

// RecurrentExpenseDetail represents a recurrent expense with full details for editing
type RecurrentExpenseDetail struct {
	ID          int64
	Description string
	AmountCents int64
	Category    string
	Subcategory string
	Frequency   string
	StartDate   string
	EndDate     string
}

// GetActiveRecurrentExpenses returns all active recurrent expenses
func (a *SQLiteAdapter) GetActiveRecurrentExpenses(ctx context.Context) ([]RecurrentExpenseItem, error) {
	expenses, err := a.storage.GetRecurrentExpenses(ctx)
	if err != nil {
		return nil, err
	}

	var items []RecurrentExpenseItem
	for _, e := range expenses {
		items = append(items, RecurrentExpenseItem{
			ID:          e.ID,
			Description: e.Description,
			AmountCents: e.Amount.Cents,
			Category:    e.Primary,
			Frequency:   string(e.Every),
		})
	}
	return items, nil
}

// GetRecurrentExpenseByID returns a single recurrent expense with full details
func (a *SQLiteAdapter) GetRecurrentExpenseByID(ctx context.Context, id int64) (*RecurrentExpenseDetail, error) {
	expense, err := a.storage.GetRecurrentExpenseByID(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &RecurrentExpenseDetail{
		ID:          expense.ID,
		Description: expense.Description,
		AmountCents: expense.Amount.Cents,
		Category:    expense.Primary,
		Subcategory: expense.Secondary,
		Frequency:   string(expense.Every),
		StartDate:   formatDateForInput(expense.StartDate),
	}

	if !expense.EndDate.IsZero() {
		detail.EndDate = formatDateForInput(expense.EndDate)
	}

	return detail, nil
}

func formatDateForInput(d core.Date) string {
	return fmt.Sprintf("%d-%02d-%02d", d.Year(), d.Month(), d.Day())
}

// Enhanced stats methods

// YTDStats contains year-to-date totals
type YTDStats struct {
	ExpensesCents int64
	IncomeCents   int64
}

// GetYTDTotals returns year-to-date expense and income totals
func (a *SQLiteAdapter) GetYTDTotals(ctx context.Context) (*YTDStats, error) {
	now := time.Now()
	startOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())

	// Get YTD expenses
	expenses, err := a.storage.ListExpensesByDateRange(ctx, startOfYear, now)
	if err != nil {
		return nil, err
	}
	var totalExpenses int64
	for _, e := range expenses {
		totalExpenses += e.Amount.Cents
	}

	// Get YTD income - iterate through each month
	var totalIncome int64
	for month := 1; month <= int(now.Month()); month++ {
		overview, err := a.storage.ReadIncomeMonthOverview(ctx, now.Year(), month)
		if err == nil {
			totalIncome += overview.Total.Cents
		}
	}

	return &YTDStats{
		ExpensesCents: totalExpenses,
		IncomeCents:   totalIncome,
	}, nil
}

// WeekChange contains week-over-week comparison data
type WeekChange struct {
	ThisWeekCents int64
	LastWeekCents int64
	ChangePercent float64
	IsDown        bool
}

// GetWeekOverWeekChange returns expenses comparison between this week and last week
func (a *SQLiteAdapter) GetWeekOverWeekChange(ctx context.Context) (*WeekChange, error) {
	now := time.Now()

	// Calculate start of this week (Monday)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	thisWeekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)
	lastWeekEnd := thisWeekStart.AddDate(0, 0, -1)

	// Get this week's expenses
	thisWeekExpenses, err := a.storage.ListExpensesByDateRange(ctx, thisWeekStart, now)
	if err != nil {
		return nil, err
	}
	var thisWeekTotal int64
	for _, e := range thisWeekExpenses {
		thisWeekTotal += e.Amount.Cents
	}

	// Get last week's expenses
	lastWeekExpenses, err := a.storage.ListExpensesByDateRange(ctx, lastWeekStart, lastWeekEnd)
	if err != nil {
		return nil, err
	}
	var lastWeekTotal int64
	for _, e := range lastWeekExpenses {
		lastWeekTotal += e.Amount.Cents
	}

	// Calculate change percentage
	var changePercent float64
	isDown := false
	if lastWeekTotal > 0 {
		changePercent = float64(thisWeekTotal-lastWeekTotal) / float64(lastWeekTotal) * 100
		isDown = changePercent < 0
		if changePercent < 0 {
			changePercent = -changePercent
		}
	}

	return &WeekChange{
		ThisWeekCents: thisWeekTotal,
		LastWeekCents: lastWeekTotal,
		ChangePercent: changePercent,
		IsDown:        isDown,
	}, nil
}

// DailyAverage contains daily spending average data
type DailyAverage struct {
	AverageCents int64
	DaysElapsed  int
	TotalCents   int64
}

// GetDailyAverage returns average daily spending for current month
func (a *SQLiteAdapter) GetDailyAverage(ctx context.Context) (*DailyAverage, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	totalCents, err := a.GetMonthlyExpenseTotal(ctx, year, month)
	if err != nil {
		return nil, err
	}

	daysElapsed := now.Day()
	var averageCents int64
	if daysElapsed > 0 {
		averageCents = totalCents / int64(daysElapsed)
	}

	return &DailyAverage{
		AverageCents: averageCents,
		DaysElapsed:  daysElapsed,
		TotalCents:   totalCents,
	}, nil
}

// VelocityStats contains spending velocity data
type VelocityStats struct {
	MonthProgressPercent  int    // % of month elapsed
	BudgetProgressPercent int    // % of last month's total spent
	Status                string // "on-track", "ahead", "behind"
}

// GetVelocityStats returns spending velocity compared to previous month
func (a *SQLiteAdapter) GetVelocityStats(ctx context.Context) (*VelocityStats, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Get current month total
	currentTotal, _ := a.GetMonthlyExpenseTotal(ctx, year, month)

	// Get previous month total (as baseline)
	prevMonth := month - 1
	prevYear := year
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}
	prevTotal, _ := a.GetMonthlyExpenseTotal(ctx, prevYear, prevMonth)

	// Calculate month progress
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, now.Location()).Day()
	monthProgressPercent := (now.Day() * 100) / daysInMonth

	// Calculate budget progress (% of prev month spent)
	budgetProgressPercent := 0
	if prevTotal > 0 {
		budgetProgressPercent = int((currentTotal * 100) / prevTotal)
	}

	// Determine status
	status := "on-track"
	diff := budgetProgressPercent - monthProgressPercent
	if diff > 10 {
		status = "ahead"
	} else if diff < -10 {
		status = "behind"
	}

	return &VelocityStats{
		MonthProgressPercent:  monthProgressPercent,
		BudgetProgressPercent: budgetProgressPercent,
		Status:                status,
	}, nil
}

// FixedVariableRatio contains the ratio of fixed (recurring) vs variable expenses
type FixedVariableRatio struct {
	FixedCents      int64
	VariableCents   int64
	TotalCents      int64
	FixedPercent    int
	VariablePercent int
}

// GetFixedVariableRatio returns the ratio of recurring expenses vs one-off expenses
func (a *SQLiteAdapter) GetFixedVariableRatio(ctx context.Context) (*FixedVariableRatio, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Get total monthly expenses
	totalCents, _ := a.GetMonthlyExpenseTotal(ctx, year, month)

	// Get recurring expenses total (monthly cost)
	recurrentTotal := a.GetRecurrentMonthlyTotal(ctx)

	variableCents := totalCents - recurrentTotal
	if variableCents < 0 {
		variableCents = 0
	}

	fixedPercent := 0
	variablePercent := 100
	if totalCents > 0 {
		fixedPercent = int((recurrentTotal * 100) / totalCents)
		variablePercent = 100 - fixedPercent
	}

	return &FixedVariableRatio{
		FixedCents:      recurrentTotal,
		VariableCents:   variableCents,
		TotalCents:      totalCents,
		FixedPercent:    fixedPercent,
		VariablePercent: variablePercent,
	}, nil
}

// GetRecurrentMonthlyTotal returns the total monthly cost of all active recurrent expenses
func (a *SQLiteAdapter) GetRecurrentMonthlyTotal(ctx context.Context) int64 {
	expenses, err := a.storage.GetRecurrentExpenses(ctx)
	if err != nil {
		return 0
	}

	var totalMonthly int64
	for _, e := range expenses {
		switch e.Every {
		case core.Monthly:
			totalMonthly += e.Amount.Cents
		case core.Yearly:
			totalMonthly += e.Amount.Cents / 12
		case core.Weekly:
			totalMonthly += e.Amount.Cents * 4
		case core.Daily:
			totalMonthly += e.Amount.Cents * 30
		}
	}
	return totalMonthly
}

// ForecastStats contains month-end forecast data
type ForecastStats struct {
	ForecastCents int64
	BasedOn       string // "average" or "trend"
}

// GetMonthEndForecast returns projected expenses at month end
func (a *SQLiteAdapter) GetMonthEndForecast(ctx context.Context) (*ForecastStats, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Get current total
	currentTotal, _ := a.GetMonthlyExpenseTotal(ctx, year, month)

	// Get days in month and days elapsed
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, now.Location()).Day()
	daysElapsed := now.Day()

	// Simple forecast: (current total / days elapsed) * days in month
	var forecastCents int64
	if daysElapsed > 0 {
		dailyAverage := currentTotal / int64(daysElapsed)
		forecastCents = dailyAverage * int64(daysInMonth)
	}

	return &ForecastStats{
		ForecastCents: forecastCents,
		BasedOn:       "media giornaliera",
	}, nil
}

// GetIncomeCategoryBreakdown returns income totals by category for current month
func (a *SQLiteAdapter) GetIncomeCategoryBreakdown(ctx context.Context) ([]CategoryTotal, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	overview, err := a.storage.ReadIncomeMonthOverview(ctx, year, month)
	if err != nil {
		return nil, err
	}

	var cats []CategoryTotal
	for _, c := range overview.ByCategory {
		cats = append(cats, CategoryTotal{
			Name:        c.Name,
			AmountCents: c.Amount.Cents,
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
