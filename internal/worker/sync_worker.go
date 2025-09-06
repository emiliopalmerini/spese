package worker

import (
	"context"
	"fmt"
	"log/slog"

	"spese/internal/amqp"
	"spese/internal/core"
	"spese/internal/sheets"
	"spese/internal/storage"
)

// SyncWorker handles synchronization of expenses from SQLite to Google Sheets
type SyncWorker struct {
	storage     *storage.SQLiteRepository
	sheets      sheets.ExpenseWriter
	batchSize   int
}

func NewSyncWorker(storage *storage.SQLiteRepository, sheets sheets.ExpenseWriter, batchSize int) *SyncWorker {
	return &SyncWorker{
		storage:   storage,
		sheets:    sheets,
		batchSize: batchSize,
	}
}

// HandleSyncMessage processes a single expense sync message from AMQP
func (w *SyncWorker) HandleSyncMessage(msg *amqp.ExpenseSyncMessage) error {
	ctx := context.Background()
	
	slog.InfoContext(ctx, "Processing sync message", 
		"id", msg.ID, 
		"version", msg.Version)

	// Get the expense from SQLite by ID
	expense, err := w.storage.GetExpense(ctx, msg.ID)
	if err != nil {
		return fmt.Errorf("get expense from storage: %w", err)
	}

	// Convert storage expense to core expense
	coreExpense := core.Expense{
		Date: core.DateParts{
			Day:   int(expense.Day),
			Month: int(expense.Month),
		},
		Description: expense.Description,
		Amount:      core.Money{Cents: expense.AmountCents},
		Primary:     expense.PrimaryCategory,
		Secondary:   expense.SecondaryCategory,
	}

	// Sync to Google Sheets
	if err := w.syncExpenseToSheets(ctx, msg.ID, coreExpense); err != nil {
		return fmt.Errorf("sync expense to sheets: %w", err)
	}

	return nil
}

// ProcessPendingExpenses processes any expenses that haven't been synced yet
// This is a backup mechanism in case AMQP messages are lost
func (w *SyncWorker) ProcessPendingExpenses(ctx context.Context) error {
	pendingExpenses, err := w.storage.GetPendingSyncExpenses(ctx, w.batchSize)
	if err != nil {
		return fmt.Errorf("get pending expenses: %w", err)
	}

	if len(pendingExpenses) == 0 {
		return nil
	}

	slog.InfoContext(ctx, "Processing pending expenses", "count", len(pendingExpenses))

	for _, pending := range pendingExpenses {
		// Get full expense data
		expense, err := w.storage.GetExpense(ctx, pending.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get expense", "id", pending.ID, "error", err)
			if err := w.storage.MarkSyncError(ctx, pending.ID); err != nil {
				slog.ErrorContext(ctx, "Failed to mark sync error", "id", pending.ID, "error", err)
			}
			continue
		}

		// Convert and sync
		coreExpense := core.Expense{
			Date: core.DateParts{
				Day:   int(expense.Day),
				Month: int(expense.Month),
			},
			Description: expense.Description,
			Amount:      core.Money{Cents: expense.AmountCents},
			Primary:     expense.PrimaryCategory,
			Secondary:   expense.SecondaryCategory,
		}

		if err := w.syncExpenseToSheets(ctx, pending.ID, coreExpense); err != nil {
			slog.ErrorContext(ctx, "Failed to sync expense", "id", pending.ID, "error", err)
			continue
		}
	}

	return nil
}

func (w *SyncWorker) syncExpenseToSheets(ctx context.Context, id int64, expense core.Expense) error {
	// Sync to Google Sheets
	ref, err := w.sheets.Append(ctx, expense)
	if err != nil {
		// Mark as sync error
		if markErr := w.storage.MarkSyncError(ctx, id); markErr != nil {
			slog.ErrorContext(ctx, "Failed to mark sync error", "id", id, "error", markErr)
		}
		return fmt.Errorf("append to sheets: %w", err)
	}

	// Mark as successfully synced
	if err := w.storage.MarkSynced(ctx, id); err != nil {
		slog.ErrorContext(ctx, "Failed to mark as synced", "id", id, "error", err)
		// Don't return error here - the sync actually worked
	}

	slog.InfoContext(ctx, "Successfully synced expense", 
		"id", id, 
		"sheets_ref", ref,
		"description", expense.Description,
		"amount_cents", expense.Amount.Cents)

	return nil
}