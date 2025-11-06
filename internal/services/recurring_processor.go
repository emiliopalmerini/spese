package services

import (
	"context"
	"fmt"
	"log/slog"
	"spese/internal/core"
	"spese/internal/storage"
	"time"
)

// RecurringProcessor handles the automatic creation of expenses from recurring expense templates
type RecurringProcessor struct {
	storage        *storage.SQLiteRepository
	expenseService *ExpenseService
}

// NewRecurringProcessor creates a new recurring expense processor
func NewRecurringProcessor(storage *storage.SQLiteRepository, expenseService *ExpenseService) *RecurringProcessor {
	return &RecurringProcessor{
		storage:        storage,
		expenseService: expenseService,
	}
}

// ProcessDueExpenses processes all recurring expenses that are due for execution
func (p *RecurringProcessor) ProcessDueExpenses(ctx context.Context, now time.Time) (int, error) {
	if p.storage == nil || p.expenseService == nil {
		return 0, fmt.Errorf("processor not properly initialized")
	}

	// Get all active recurring expenses
	recurrentExpenses, err := p.storage.GetActiveRecurrentExpensesForProcessing(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to get active recurring expenses: %w", err)
	}

	slog.InfoContext(ctx, "Processing recurring expenses",
		"total_active", len(recurrentExpenses),
		"processing_date", now.Format("2006-01-02"))

	processedCount := 0

	for _, re := range recurrentExpenses {
		// Get the full recurrent expense from DB to access last_execution_date
		dbExpense, err := p.storage.GetRecurrentExpenseByID(ctx, re.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get recurrent expense details",
				"id", re.ID,
				"error", err)
			continue
		}

		// Check if this recurring expense is due for processing
		isDue, err := p.isDueForProcessing(ctx, dbExpense, now)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to check if expense is due",
				"id", re.ID,
				"error", err)
			continue
		}

		if !isDue {
			continue
		}

		// Create the actual expense
		expense := core.Expense{
			Date:        core.Date{Time: now},
			Description: re.Description,
			Amount:      re.Amount,
			Primary:     re.Primary,
			Secondary:   re.Secondary,
		}

		_, err = p.expenseService.CreateExpense(ctx, expense)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create expense from recurring template",
				"recurrent_id", re.ID,
				"description", re.Description,
				"error", err)
			continue
		}

		// Update last_execution_date
		err = p.storage.UpdateRecurrentLastExecution(ctx, re.ID, now)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to update last execution date",
				"recurrent_id", re.ID,
				"error", err)
			// Continue anyway - expense was created successfully
		}

		processedCount++
		slog.InfoContext(ctx, "Created expense from recurring template",
			"recurrent_id", re.ID,
			"description", re.Description,
			"amount_cents", re.Amount.Cents,
			"frequency", re.Every)
	}

	slog.InfoContext(ctx, "Recurring expense processing complete",
		"processed", processedCount,
		"total_checked", len(recurrentExpenses))

	return processedCount, nil
}

// isDueForProcessing determines if a recurring expense should be processed
func (p *RecurringProcessor) isDueForProcessing(ctx context.Context, dbExpense *core.RecurrentExpenses, now time.Time) (bool, error) {
	// Get last execution date from database
	var lastExecution time.Time

	// This requires accessing the DB record directly - we need to modify the repository method
	// For now, we'll use a simple approach: check if we've processed today

	// Get the raw DB record to access last_execution_date
	rawExpense, err := p.storage.GetRecurrentExpenseRaw(ctx, dbExpense.ID)
	if err != nil {
		return false, fmt.Errorf("get raw expense: %w", err)
	}

	if lastExecDate, ok := rawExpense.LastExecutionDate.(time.Time); ok && !lastExecDate.IsZero() {
		lastExecution = lastExecDate
	}

	switch dbExpense.Every {
	case core.Daily:
		return p.isDueDaily(lastExecution, now), nil
	case core.Weekly:
		return p.isDueWeekly(lastExecution, now), nil
	case core.Monthly:
		return p.isDueMonthly(lastExecution, now, dbExpense.StartDate.Day()), nil
	case core.Yearly:
		return p.isDueYearly(lastExecution, now, dbExpense.StartDate.Month(), dbExpense.StartDate.Day()), nil
	default:
		return false, fmt.Errorf("unknown repetition type: %s", dbExpense.Every)
	}
}

// isDueDaily checks if a daily recurring expense is due
func (p *RecurringProcessor) isDueDaily(lastExecution, now time.Time) bool {
	// If never executed, it's due
	if lastExecution.IsZero() {
		return true
	}

	// Due if last execution was before today
	lastDate := lastExecution.Format("2006-01-02")
	nowDate := now.Format("2006-01-02")
	return lastDate != nowDate
}

// isDueWeekly checks if a weekly recurring expense is due
func (p *RecurringProcessor) isDueWeekly(lastExecution, now time.Time) bool {
	// If never executed, it's due
	if lastExecution.IsZero() {
		return true
	}

	// Due if 7 or more days have passed
	daysSince := now.Sub(lastExecution).Hours() / 24
	return daysSince >= 7
}

// isDueMonthly checks if a monthly recurring expense is due
func (p *RecurringProcessor) isDueMonthly(lastExecution, now time.Time, targetDay int) bool {
	// If never executed, it's due
	if lastExecution.IsZero() {
		return true
	}

	// Already processed this month?
	if lastExecution.Year() == now.Year() && lastExecution.Month() == now.Month() {
		return false
	}

	// Check if we've reached the target day of the month
	// Handle case where target day doesn't exist in current month (e.g., Feb 31)
	targetDayThisMonth := targetDay
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if targetDay > lastDayOfMonth {
		targetDayThisMonth = lastDayOfMonth
	}

	return now.Day() >= targetDayThisMonth
}

// isDueYearly checks if a yearly recurring expense is due
func (p *RecurringProcessor) isDueYearly(lastExecution, now time.Time, targetMonth, targetDay int) bool {
	// If never executed, it's due
	if lastExecution.IsZero() {
		return true
	}

	// Already processed this year?
	if lastExecution.Year() == now.Year() {
		return false
	}

	// Check if we've reached the target month and day
	if int(now.Month()) < targetMonth {
		return false
	}

	if int(now.Month()) == targetMonth {
		// Handle case where target day doesn't exist in target month
		lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		targetDayThisMonth := targetDay
		if targetDay > lastDayOfMonth {
			targetDayThisMonth = lastDayOfMonth
		}
		return now.Day() >= targetDayThisMonth
	}

	// We're past the target month
	return true
}
