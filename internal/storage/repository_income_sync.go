package storage

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"spese/internal/core"
)

// Income Sync Queue methods

// EnqueueIncomeSync adds a sync operation to the income queue
func (r *SQLiteRepository) EnqueueIncomeSync(ctx context.Context, incomeID int64) (IncomeSyncQueue, error) {
	item, err := r.queries.EnqueueIncomeSync(ctx, incomeID)
	if err != nil {
		return IncomeSyncQueue{}, fmt.Errorf("enqueue income sync: %w", err)
	}
	slog.DebugContext(ctx, "Enqueued income sync operation", "id", item.ID, "income_id", incomeID)
	return item, nil
}

// EnqueueIncomeDelete adds a delete operation to the income queue with income data
func (r *SQLiteRepository) EnqueueIncomeDelete(ctx context.Context, incomeID int64, day, month int, description string, amountCents int64, category string) (IncomeSyncQueue, error) {
	item, err := r.queries.EnqueueIncomeDelete(ctx, EnqueueIncomeDeleteParams{
		IncomeID:          incomeID,
		IncomeDay:         int64(day),
		IncomeMonth:       int64(month),
		IncomeDescription: description,
		IncomeAmountCents: amountCents,
		IncomeCategory:    category,
	})
	if err != nil {
		return IncomeSyncQueue{}, fmt.Errorf("enqueue income delete: %w", err)
	}
	slog.DebugContext(ctx, "Enqueued income delete operation", "id", item.ID, "income_id", incomeID)
	return item, nil
}

// DequeueIncomeSyncBatch fetches a batch of pending income items ready for processing
func (r *SQLiteRepository) DequeueIncomeSyncBatch(ctx context.Context, limit int64) ([]IncomeSyncQueue, error) {
	items, err := r.queries.DequeueIncomeSyncBatch(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("dequeue income sync batch: %w", err)
	}
	return items, nil
}

// MarkIncomeSyncProcessing marks an income item as being processed
func (r *SQLiteRepository) MarkIncomeSyncProcessing(ctx context.Context, id int64) error {
	err := r.queries.MarkIncomeSyncProcessing(ctx, id)
	if err != nil {
		return fmt.Errorf("mark income sync processing: %w", err)
	}
	return nil
}

// MarkIncomeSyncComplete marks an income sync queue item as successfully completed
func (r *SQLiteRepository) MarkIncomeSyncComplete(ctx context.Context, id int64) error {
	err := r.queries.MarkIncomeSyncComplete(ctx, id)
	if err != nil {
		return fmt.Errorf("mark income sync complete: %w", err)
	}
	slog.DebugContext(ctx, "Income sync queue item completed", "id", id)
	return nil
}

// MarkIncomeSyncFailed marks an income sync queue item as failed after max retries exceeded
func (r *SQLiteRepository) MarkIncomeSyncFailed(ctx context.Context, id int64, errorMsg string) error {
	err := r.queries.MarkIncomeSyncFailed(ctx, MarkIncomeSyncFailedParams{
		ID:        id,
		LastError: errorMsg,
	})
	if err != nil {
		return fmt.Errorf("mark income sync failed: %w", err)
	}
	slog.WarnContext(ctx, "Income sync queue item marked as failed", "id", id, "error", errorMsg)
	return nil
}

// IncrementIncomeSyncAttempt increments attempt count and schedules next retry
func (r *SQLiteRepository) IncrementIncomeSyncAttempt(ctx context.Context, id int64, errorMsg string) error {
	err := r.queries.IncrementIncomeSyncAttempt(ctx, IncrementIncomeSyncAttemptParams{
		ID:        id,
		LastError: errorMsg,
	})
	if err != nil {
		return fmt.Errorf("increment income sync attempt: %w", err)
	}
	return nil
}

// RetryFailedIncomeSyncs resets failed income items back to pending for manual retry
func (r *SQLiteRepository) RetryFailedIncomeSyncs(ctx context.Context) error {
	err := r.queries.RetryFailedIncomeSyncs(ctx)
	if err != nil {
		return fmt.Errorf("retry failed income syncs: %w", err)
	}
	slog.InfoContext(ctx, "Reset failed income sync items for retry")
	return nil
}

// CleanupCompletedIncomeSyncs removes completed income items older than the specified time
func (r *SQLiteRepository) CleanupCompletedIncomeSyncs(ctx context.Context, olderThan time.Time) error {
	err := r.queries.CleanupCompletedIncomeSyncs(ctx, olderThan)
	if err != nil {
		return fmt.Errorf("cleanup completed income syncs: %w", err)
	}
	return nil
}

// ResetStaleIncomeProcessing resets income items stuck in processing state (crash recovery)
func (r *SQLiteRepository) ResetStaleIncomeProcessing(ctx context.Context) error {
	err := r.queries.ResetStaleIncomeProcessing(ctx)
	if err != nil {
		return fmt.Errorf("reset stale income processing: %w", err)
	}
	return nil
}

// GetIncomeSyncQueueStats returns counts by status for monitoring
func (r *SQLiteRepository) GetIncomeSyncQueueStats(ctx context.Context) (*GetIncomeSyncQueueStatsRow, error) {
	stats, err := r.queries.GetIncomeSyncQueueStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get income sync queue stats: %w", err)
	}
	return &stats, nil
}

// MarkIncomeSynced marks an income as successfully synced
func (r *SQLiteRepository) MarkIncomeSynced(ctx context.Context, id int64) error {
	err := r.queries.MarkIncomeSynced(ctx, id)
	if err != nil {
		return fmt.Errorf("mark income synced: %w", err)
	}
	slog.InfoContext(ctx, "Income marked as synced", "id", id)
	return nil
}

// MarkIncomeSyncError marks an income as having sync errors
func (r *SQLiteRepository) MarkIncomeSyncError(ctx context.Context, id int64) error {
	err := r.queries.MarkIncomeSyncError(ctx, id)
	if err != nil {
		return fmt.Errorf("mark income sync error: %w", err)
	}
	slog.WarnContext(ctx, "Income marked with sync error", "id", id)
	return nil
}

// GetIncome retrieves a single income by ID
func (r *SQLiteRepository) GetIncome(ctx context.Context, id int64) (*Income, error) {
	income, err := r.readQueries.GetIncome(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get income by id: %w", err)
	}
	return &income, nil
}

// AppendIncomeAndEnqueueSync creates an income and enqueues it for sync in a single atomic transaction
func (r *SQLiteRepository) AppendIncomeAndEnqueueSync(ctx context.Context, i core.Income) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	txQueries := r.queries.WithTx(tx)

	// Format date as string for SQLite
	dateStr := fmt.Sprintf("%04d-%02d-%02d", i.Date.Year(), i.Date.Month(), i.Date.Day())

	// Create income
	income, err := txQueries.CreateIncome(ctx, CreateIncomeParams{
		Date:        dateStr,
		Description: i.Description,
		AmountCents: i.Amount.Cents,
		Category:    i.Category,
	})
	if err != nil {
		return "", fmt.Errorf("create income: %w", err)
	}

	// Enqueue for sync
	_, err = txQueries.EnqueueIncomeSync(ctx, income.ID)
	if err != nil {
		return "", fmt.Errorf("enqueue income sync: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transaction: %w", err)
	}

	slog.InfoContext(ctx, "Income saved and enqueued for sync",
		"id", income.ID,
		"description", income.Description,
		"amount_cents", income.AmountCents,
		"date", dateStr)

	return strconv.FormatInt(income.ID, 10), nil
}

// HardDeleteIncomeAndEnqueueSync deletes an income and enqueues delete operation atomically
func (r *SQLiteRepository) HardDeleteIncomeAndEnqueueSync(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	txQueries := r.queries.WithTx(tx)

	// Get income data inside transaction to avoid TOCTOU race
	income, err := txQueries.GetIncome(ctx, id)
	if err != nil {
		return fmt.Errorf("get income: %w", err)
	}

	// Delete income
	if err := txQueries.HardDeleteIncome(ctx, id); err != nil {
		return fmt.Errorf("delete income: %w", err)
	}

	// Enqueue delete operation with income data for Google Sheets sync
	_, err = txQueries.EnqueueIncomeDelete(ctx, EnqueueIncomeDeleteParams{
		IncomeID:          id,
		IncomeDay:         int64(income.Date.Day()),
		IncomeMonth:       int64(income.Date.Month()),
		IncomeDescription: income.Description,
		IncomeAmountCents: income.AmountCents,
		IncomeCategory:    income.Category,
	})
	if err != nil {
		return fmt.Errorf("enqueue income delete: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	slog.InfoContext(ctx, "Income deleted and enqueued for sync",
		"id", id,
		"description", income.Description)

	return nil
}
