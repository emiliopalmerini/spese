package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"spese/internal/core"
	"spese/internal/sheets"
	"spese/internal/storage"
)

// SyncProcessorConfig holds configuration for the sync processor
type SyncProcessorConfig struct {
	// PollInterval is how often to check for pending items (default: 10s)
	PollInterval time.Duration

	// BatchSize is the max number of items to process per poll cycle (default: 10)
	BatchSize int

	// MaxRetries is the maximum retry attempts before marking as failed (default: 3)
	MaxRetries int

	// CleanupInterval is how often to clean up completed items (default: 1h)
	CleanupInterval time.Duration

	// CleanupAge is how old completed items must be before cleanup (default: 24h)
	CleanupAge time.Duration
}

// DefaultSyncProcessorConfig returns sensible defaults
func DefaultSyncProcessorConfig() SyncProcessorConfig {
	return SyncProcessorConfig{
		PollInterval:    10 * time.Second,
		BatchSize:       10,
		MaxRetries:      3,
		CleanupInterval: 1 * time.Hour,
		CleanupAge:      24 * time.Hour,
	}
}

// SyncProcessor handles SQLite-based sync queue processing
type SyncProcessor struct {
	storage *storage.SQLiteRepository
	sheets  sheets.ExpenseWriter
	deleter sheets.ExpenseDeleter
	config  SyncProcessorConfig

	// Lifecycle management
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewSyncProcessor creates a new sync processor
func NewSyncProcessor(
	storage *storage.SQLiteRepository,
	sheetsWriter sheets.ExpenseWriter,
	deleter sheets.ExpenseDeleter,
	config SyncProcessorConfig,
) *SyncProcessor {
	return &SyncProcessor{
		storage: storage,
		sheets:  sheetsWriter,
		deleter: deleter,
		config:  config,
	}
}

// Start begins the processing loop. Returns an error if already running.
func (p *SyncProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("sync processor is already running")
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})
	p.mu.Unlock()

	// Reset any stale processing items from previous crashes
	if err := p.storage.ResetStaleProcessing(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to reset stale processing items", "error", err)
	}

	go p.runLoop(ctx)

	slog.InfoContext(ctx, "Sync processor started",
		"poll_interval", p.config.PollInterval,
		"batch_size", p.config.BatchSize)

	return nil
}

// Stop gracefully stops the processor and waits for completion.
func (p *SyncProcessor) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Signal stop
	close(p.stopCh)

	// Wait for completion or context cancellation
	select {
	case <-p.doneCh:
		slog.InfoContext(ctx, "Sync processor stopped gracefully")
	case <-ctx.Done():
		slog.WarnContext(ctx, "Sync processor stop timed out")
		return ctx.Err()
	}

	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	return nil
}

// IsRunning returns whether the processor is currently running
func (p *SyncProcessor) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// runLoop is the main processing loop
func (p *SyncProcessor) runLoop(ctx context.Context) {
	defer close(p.doneCh)

	pollTicker := time.NewTicker(p.config.PollInterval)
	defer pollTicker.Stop()

	cleanupTicker := time.NewTicker(p.config.CleanupInterval)
	defer cleanupTicker.Stop()

	// Process immediately on startup
	p.processBatch(ctx)

	for {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			p.processBatch(ctx)
		case <-cleanupTicker.C:
			p.cleanupCompleted(ctx)
		}
	}
}

// processBatch processes a single batch of pending items
func (p *SyncProcessor) processBatch(ctx context.Context) {
	// Fetch pending items
	items, err := p.storage.DequeueSyncBatch(ctx, int64(p.config.BatchSize))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to dequeue sync batch", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	slog.DebugContext(ctx, "Processing sync batch", "count", len(items))

	for _, item := range items {
		// Check if we should stop
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Mark as processing
		if err := p.storage.MarkSyncProcessing(ctx, item.ID); err != nil {
			slog.ErrorContext(ctx, "Failed to mark item as processing",
				"id", item.ID, "error", err)
			continue
		}

		// Process the item
		var processErr error
		switch item.Operation {
		case "sync":
			processErr = p.processSyncItem(ctx, item)
		case "delete":
			processErr = p.processDeleteItem(ctx, item)
		default:
			processErr = fmt.Errorf("unknown operation: %s", item.Operation)
		}

		// Handle result
		if processErr != nil {
			p.handleFailure(ctx, item, processErr)
		} else {
			p.handleSuccess(ctx, item)
		}
	}
}

// processSyncItem syncs an expense to Google Sheets
func (p *SyncProcessor) processSyncItem(ctx context.Context, item storage.SyncQueue) error {
	// Fetch the expense from database
	expense, err := p.storage.GetExpense(ctx, item.ExpenseID)
	if err != nil {
		return fmt.Errorf("get expense %d: %w", item.ExpenseID, err)
	}

	// Convert to core.Expense
	coreExpense := core.Expense{
		Date:        core.Date{Time: expense.Date},
		Description: expense.Description,
		Amount:      core.Money{Cents: expense.AmountCents},
		Primary:     expense.PrimaryCategory,
		Secondary:   expense.SecondaryCategory,
	}

	// Add timestamp for uniqueness (matching existing sync_worker.go logic)
	timestampMs := time.Now().UnixMilli()
	coreExpense.Description = fmt.Sprintf("%s [ts:%d]", expense.Description, timestampMs)

	// Sync to Google Sheets
	ref, err := p.sheets.Append(ctx, coreExpense)
	if err != nil {
		return fmt.Errorf("append to sheets: %w", err)
	}

	// Mark expense as synced in expenses table
	if err := p.storage.MarkSynced(ctx, item.ExpenseID); err != nil {
		slog.WarnContext(ctx, "Failed to mark expense as synced",
			"expense_id", item.ExpenseID, "error", err)
		// Don't fail the queue item - sync actually succeeded
	}

	slog.InfoContext(ctx, "Synced expense to Google Sheets",
		"expense_id", item.ExpenseID,
		"sheets_ref", ref)

	return nil
}

// processDeleteItem deletes an expense from Google Sheets
func (p *SyncProcessor) processDeleteItem(ctx context.Context, item storage.SyncQueue) error {
	if p.deleter == nil {
		slog.WarnContext(ctx, "No deleter configured, skipping delete",
			"expense_id", item.ExpenseID)
		return nil // Not an error - just skip
	}

	// Extract expense data from stored fields
	day := int64(0)
	if d, ok := item.ExpenseDay.(int64); ok {
		day = d
	}
	month := int64(0)
	if m, ok := item.ExpenseMonth.(int64); ok {
		month = m
	}
	description := ""
	if d, ok := item.ExpenseDescription.(string); ok {
		description = d
	}
	amountCents := int64(0)
	if a, ok := item.ExpenseAmountCents.(int64); ok {
		amountCents = a
	}
	primary := ""
	if p, ok := item.ExpensePrimary.(string); ok {
		primary = p
	}
	secondary := ""
	if s, ok := item.ExpenseSecondary.(string); ok {
		secondary = s
	}

	// Reconstruct expense data
	expenseData := core.Expense{
		Date:        core.NewDate(time.Now().Year(), int(month), int(day)),
		Description: description,
		Amount:      core.Money{Cents: amountCents},
		Primary:     primary,
		Secondary:   secondary,
	}

	// Use DeleteExpenseByData if available (Google Sheets adapter)
	if googleDeleter, ok := p.deleter.(interface {
		DeleteExpenseByData(ctx context.Context, expenseData core.Expense) error
	}); ok {
		if err := googleDeleter.DeleteExpenseByData(ctx, expenseData); err != nil {
			return fmt.Errorf("delete from Google Sheets: %w", err)
		}
	} else {
		// Fallback to ID-based deletion
		if err := p.deleter.DeleteExpense(ctx, fmt.Sprintf("%d", item.ExpenseID)); err != nil {
			return fmt.Errorf("delete expense: %w", err)
		}
	}

	slog.InfoContext(ctx, "Deleted expense from Google Sheets",
		"expense_id", item.ExpenseID)

	return nil
}

// handleSuccess marks an item as completed
func (p *SyncProcessor) handleSuccess(ctx context.Context, item storage.SyncQueue) {
	if err := p.storage.MarkSyncComplete(ctx, item.ID); err != nil {
		slog.ErrorContext(ctx, "Failed to mark sync complete",
			"id", item.ID, "error", err)
	}
}

// handleFailure handles a failed sync attempt with retry logic
func (p *SyncProcessor) handleFailure(ctx context.Context, item storage.SyncQueue, processErr error) {
	slog.WarnContext(ctx, "Sync processing failed",
		"id", item.ID,
		"operation", item.Operation,
		"attempt", item.Attempts+1,
		"error", processErr)

	if item.Attempts+1 >= int64(p.config.MaxRetries) {
		// Max retries exceeded - mark as failed
		if err := p.storage.MarkSyncFailed(ctx, item.ID, processErr.Error()); err != nil {
			slog.ErrorContext(ctx, "Failed to mark sync as failed",
				"id", item.ID, "error", err)
		}

		// Also mark the expense as having sync error
		if item.Operation == "sync" {
			if err := p.storage.MarkSyncError(ctx, item.ExpenseID); err != nil {
				slog.ErrorContext(ctx, "Failed to mark expense sync error",
					"expense_id", item.ExpenseID, "error", err)
			}
		}

		slog.ErrorContext(ctx, "Sync item failed permanently after max retries",
			"id", item.ID,
			"expense_id", item.ExpenseID,
			"attempts", item.Attempts+1)
	} else {
		// Schedule retry with exponential backoff
		if err := p.storage.IncrementSyncAttempt(ctx, item.ID, processErr.Error()); err != nil {
			slog.ErrorContext(ctx, "Failed to increment sync attempt",
				"id", item.ID, "error", err)
		}
	}
}

// cleanupCompleted removes old completed items
func (p *SyncProcessor) cleanupCompleted(ctx context.Context) {
	cutoff := time.Now().Add(-p.config.CleanupAge)
	if err := p.storage.CleanupCompletedSyncs(ctx, cutoff); err != nil {
		slog.ErrorContext(ctx, "Failed to cleanup completed syncs", "error", err)
	}
}

// Stats returns current queue statistics
func (p *SyncProcessor) Stats(ctx context.Context) (*storage.GetSyncQueueStatsRow, error) {
	return p.storage.GetSyncQueueStats(ctx)
}

// RetryFailed resets all failed items for retry
func (p *SyncProcessor) RetryFailed(ctx context.Context) error {
	return p.storage.RetryFailedSyncs(ctx)
}
