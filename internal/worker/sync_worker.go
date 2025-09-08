package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"spese/internal/amqp"
	"spese/internal/core"
	"spese/internal/sheets"
	"spese/internal/storage"
)

// SyncWorker handles synchronization of expenses from SQLite to Google Sheets
type SyncWorker struct {
	storage   *storage.SQLiteRepository
	sheets    sheets.ExpenseWriter
	taxonomy  sheets.TaxonomyReader
	batchSize int
}

func NewSyncWorker(storage *storage.SQLiteRepository, sheets sheets.ExpenseWriter, taxonomy sheets.TaxonomyReader, batchSize int) *SyncWorker {
	return &SyncWorker{
		storage:   storage,
		sheets:    sheets,
		taxonomy:  taxonomy,
		batchSize: batchSize,
	}
}

// HandleSyncMessage processes a single expense sync message from AMQP
func (w *SyncWorker) HandleSyncMessage(ctx context.Context, msg *amqp.ExpenseSyncMessage) error {

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
			Day:   expense.Date.Day(),
			Month: int(expense.Date.Month()),
			Year:  expense.Date.Year(),
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
				Day:   expense.Date.Day(),
				Month: int(expense.Date.Month()),
				Year:  expense.Date.Year(),
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

// StartupSyncCheck verifies and syncs any pending expenses at worker startup
// This is useful to recover from missed AMQP messages or worker downtime
func (w *SyncWorker) StartupSyncCheck(ctx context.Context) error {
	// Get a larger batch for startup check
	pendingExpenses, err := w.storage.GetPendingSyncExpenses(ctx, w.batchSize*5)
	if err != nil {
		return fmt.Errorf("get pending expenses for startup check: %w", err)
	}

	if len(pendingExpenses) == 0 {
		slog.InfoContext(ctx, "No pending expenses found on startup")
		return nil
	}

	slog.InfoContext(ctx, "Found pending expenses on startup, processing...",
		"count", len(pendingExpenses))

	successCount := 0
	errorCount := 0

	for _, pending := range pendingExpenses {
		// Get full expense data
		expense, err := w.storage.GetExpense(ctx, pending.ID)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get expense for startup sync",
				"id", pending.ID, "error", err)
			if err := w.storage.MarkSyncError(ctx, pending.ID); err != nil {
				slog.ErrorContext(ctx, "Failed to mark sync error", "id", pending.ID, "error", err)
			}
			errorCount++
			continue
		}

		// Convert and sync
		coreExpense := core.Expense{
			Date: core.DateParts{
				Day:   expense.Date.Day(),
				Month: int(expense.Date.Month()),
				Year:  expense.Date.Year(),
			},
			Description: expense.Description,
			Amount:      core.Money{Cents: expense.AmountCents},
			Primary:     expense.PrimaryCategory,
			Secondary:   expense.SecondaryCategory,
		}

		if err := w.syncExpenseToSheets(ctx, pending.ID, coreExpense); err != nil {
			slog.ErrorContext(ctx, "Failed to sync expense during startup",
				"id", pending.ID, "error", err)
			errorCount++
			continue
		}

		successCount++
	}

	slog.InfoContext(ctx, "Startup sync completed",
		"total", len(pendingExpenses),
		"synced", successCount,
		"errors", errorCount)

	return nil
}

// SyncCategoriesIfNeeded loads categories from Google Sheets and caches them in SQLite
// with multiple invalidation strategies:
// 1. Empty cache: Always sync if no categories exist
// 2. Age-based: Refresh if cache is older than 7 days
// 3. Force refresh: Can be triggered manually via ForceRefreshCategories
func (w *SyncWorker) SyncCategoriesIfNeeded(ctx context.Context) error {
	// Check if we already have categories in the database
	count, err := w.storage.GetCategoryCount(ctx)
	if err != nil {
		return fmt.Errorf("check category count: %w", err)
	}

	// Strategy 1: Empty cache - always sync
	if count == 0 {
		slog.InfoContext(ctx, "No categories found in cache, loading from Google Sheets...")
		return w.syncCategoriesFromSheets(ctx)
	}

	// Strategy 2: Age-based invalidation (7 days)
	lastSync, err := w.storage.GetCategoryLastSync(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Could not determine last sync time, keeping current cache", "error", err)
		return nil
	}

	cacheAge := time.Since(lastSync)
	const maxCacheAge = 7 * 24 * time.Hour

	if cacheAge > maxCacheAge {
		slog.InfoContext(ctx, "Categories cache is stale, refreshing from Google Sheets",
			"last_sync", lastSync.Format(time.RFC3339),
			"age", cacheAge.Round(time.Hour))
		return w.syncCategoriesFromSheets(ctx)
	}

	slog.InfoContext(ctx, "Categories cache is fresh",
		"count", count,
		"last_sync", lastSync.Format(time.RFC3339),
		"age", cacheAge.Round(time.Hour))

	return nil
}

// ForceRefreshCategories forces a refresh of the category cache from Google Sheets
// This can be called manually or triggered by an admin endpoint
func (w *SyncWorker) ForceRefreshCategories(ctx context.Context) error {
	slog.InfoContext(ctx, "Force refreshing categories from Google Sheets")

	// Clear existing cache
	if err := w.storage.RefreshCategories(ctx); err != nil {
		return fmt.Errorf("clear category cache: %w", err)
	}

	// Reload from Google Sheets
	return w.syncCategoriesFromSheets(ctx)
}

// PeriodicCategoryRefresh can be called periodically to refresh categories
// It respects the age-based cache invalidation strategy
func (w *SyncWorker) PeriodicCategoryRefresh(ctx context.Context) error {
	return w.SyncCategoriesIfNeeded(ctx)
}

// syncCategoriesFromSheets is the internal method that actually syncs categories
func (w *SyncWorker) syncCategoriesFromSheets(ctx context.Context) error {
	// Load categories from Google Sheets
	primaryCategories, secondaryCategories, err := w.taxonomy.List(ctx)
	if err != nil {
		return fmt.Errorf("load categories from Google Sheets: %w", err)
	}

	// Sync primary categories to SQLite
	if err := w.storage.SyncCategories(ctx, primaryCategories, "primary"); err != nil {
		return fmt.Errorf("sync primary categories: %w", err)
	}

	// Sync secondary categories to SQLite
	if err := w.storage.SyncCategories(ctx, secondaryCategories, "secondary"); err != nil {
		return fmt.Errorf("sync secondary categories: %w", err)
	}

	slog.InfoContext(ctx, "Categories successfully cached",
		"primary_count", len(primaryCategories),
		"secondary_count", len(secondaryCategories))

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
