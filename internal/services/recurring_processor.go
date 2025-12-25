// Package services provides business logic and orchestration services.
//
// This package contains services that coordinate between the domain layer
// and infrastructure layers, implementing complex business operations
// like recurring expense processing.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"spese/internal/core"
	"spese/internal/storage"
	"time"
)

// RecurringProcessor handles the automatic creation of expenses from recurring expense templates.
// It processes configured recurrent expenses and creates actual expense entries
// based on their frequency (daily, weekly, monthly, yearly) and date ranges.
type RecurringProcessor struct {
	storage        *storage.SQLiteRepository // Database access for recurrent expenses
	expenseService *ExpenseService           // Service for creating regular expenses
}

// NewRecurringProcessor creates a new recurring expense processor.
// It requires a storage repository and an expense service to function.
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

// isDueForProcessing determines if a recurring expense should be processed.
// It uses the Strategy Pattern via GetDuenessChecker to delegate the dueness
// logic to the appropriate checker based on the expense's frequency type.
func (p *RecurringProcessor) isDueForProcessing(ctx context.Context, dbExpense *core.RecurrentExpenses, now time.Time) (bool, error) {
	// Get last execution date from database
	var lastExecution time.Time

	// Get the raw DB record to access last_execution_date
	rawExpense, err := p.storage.GetRecurrentExpenseRaw(ctx, dbExpense.ID)
	if err != nil {
		return false, fmt.Errorf("get raw expense: %w", err)
	}

	if lastExecDate, ok := rawExpense.LastExecutionDate.(time.Time); ok && !lastExecDate.IsZero() {
		lastExecution = lastExecDate
	}

	// Use the Strategy Pattern to get the appropriate dueness checker
	checker, err := GetDuenessChecker(dbExpense.Every)
	if err != nil {
		return false, err
	}

	return checker.IsDue(lastExecution, now, dbExpense.StartDate), nil
}
