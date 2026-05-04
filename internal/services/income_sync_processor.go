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

// IncomeSyncProcessor handles SQLite-based income sync queue processing
type IncomeSyncProcessor struct {
	storage      *storage.SQLiteRepository
	incomeWriter sheets.RemoteIncomeWriter
	config       SyncProcessorConfig

	// Lifecycle management
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewIncomeSyncProcessor creates a new income sync processor
func NewIncomeSyncProcessor(
	storage *storage.SQLiteRepository,
	incomeWriter sheets.RemoteIncomeWriter,
	config SyncProcessorConfig,
) *IncomeSyncProcessor {
	return &IncomeSyncProcessor{
		storage:      storage,
		incomeWriter: incomeWriter,
		config:       config,
	}
}

// Start begins the processing loop. Returns an error if already running.
func (p *IncomeSyncProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("income sync processor is already running")
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})
	p.mu.Unlock()

	// Reset any stale processing items from previous crashes
	if err := p.storage.ResetStaleIncomeProcessing(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to reset stale income processing items", "error", err)
	}

	go p.runLoop(ctx)

	slog.InfoContext(ctx, "Income sync processor started",
		"poll_interval", p.config.PollInterval,
		"batch_size", p.config.BatchSize)

	return nil
}

// Stop gracefully stops the processor and waits for completion.
func (p *IncomeSyncProcessor) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	close(p.stopCh)

	select {
	case <-p.doneCh:
		slog.InfoContext(ctx, "Income sync processor stopped gracefully")
	case <-ctx.Done():
		slog.WarnContext(ctx, "Income sync processor stop timed out")
		return ctx.Err()
	}

	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	return nil
}

// IsRunning returns whether the processor is currently running
func (p *IncomeSyncProcessor) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// runLoop is the main processing loop
func (p *IncomeSyncProcessor) runLoop(ctx context.Context) {
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

// processBatch processes a single batch of pending income items
func (p *IncomeSyncProcessor) processBatch(ctx context.Context) {
	if p.incomeWriter == nil {
		return // Income sync not configured
	}

	items, err := p.storage.DequeueIncomeSyncBatch(ctx, int64(p.config.BatchSize))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to dequeue income sync batch", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	slog.DebugContext(ctx, "Processing income sync batch", "count", len(items))

	for _, item := range items {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if err := p.storage.MarkIncomeSyncProcessing(ctx, item.ID); err != nil {
			slog.ErrorContext(ctx, "Failed to mark income item as processing",
				"id", item.ID, "error", err)
			continue
		}

		var processErr error
		switch item.Operation {
		case "sync":
			processErr = p.processSyncItem(ctx, item)
		case "delete":
			processErr = p.processDeleteItem(ctx, item)
		default:
			processErr = fmt.Errorf("unknown operation: %s", item.Operation)
		}

		if processErr != nil {
			p.handleFailure(ctx, item, processErr)
		} else {
			p.handleSuccess(ctx, item)
		}
	}
}

// processSyncItem syncs an income to Google Sheets
func (p *IncomeSyncProcessor) processSyncItem(ctx context.Context, item storage.IncomeSyncQueue) error {
	income, err := p.storage.GetIncome(ctx, item.IncomeID)
	if err != nil {
		return fmt.Errorf("get income %d: %w", item.IncomeID, err)
	}

	coreIncome := core.Income{
		ID:          item.IncomeID,
		Date:        core.Date{Time: income.Date},
		Description: income.Description,
		Amount:      core.Money{Cents: income.AmountCents},
		Category:    income.Category,
	}

	ref, err := p.incomeWriter.UpsertIncome(ctx, coreIncome)
	if err != nil {
		return fmt.Errorf("upsert income to sheets: %w", err)
	}

	if err := p.storage.MarkIncomeSynced(ctx, item.IncomeID); err != nil {
		slog.WarnContext(ctx, "Failed to mark income as synced",
			"income_id", item.IncomeID, "error", err)
	}

	slog.InfoContext(ctx, "Synced income to Google Sheets",
		"income_id", item.IncomeID,
		"sheets_ref", ref)

	return nil
}

// processDeleteItem handles income delete operations
// Note: Delete sync for income is not fully implemented yet (matching expense pattern)
func (p *IncomeSyncProcessor) processDeleteItem(ctx context.Context, item storage.IncomeSyncQueue) error {
	// For now, just log and succeed - delete sync can be implemented later if needed
	slog.WarnContext(ctx, "Income delete sync not implemented, marking as complete",
		"income_id", item.IncomeID)
	return nil
}

// handleSuccess marks an item as completed
func (p *IncomeSyncProcessor) handleSuccess(ctx context.Context, item storage.IncomeSyncQueue) {
	if err := p.storage.MarkIncomeSyncComplete(ctx, item.ID); err != nil {
		slog.ErrorContext(ctx, "Failed to mark income sync complete",
			"id", item.ID, "error", err)
	}
}

// handleFailure handles a failed sync attempt with retry logic
func (p *IncomeSyncProcessor) handleFailure(ctx context.Context, item storage.IncomeSyncQueue, processErr error) {
	slog.WarnContext(ctx, "Income sync processing failed",
		"id", item.ID,
		"operation", item.Operation,
		"attempt", item.Attempts+1,
		"error", processErr)

	if item.Attempts+1 >= int64(p.config.MaxRetries) {
		if err := p.storage.MarkIncomeSyncFailed(ctx, item.ID, processErr.Error()); err != nil {
			slog.ErrorContext(ctx, "Failed to mark income sync as failed",
				"id", item.ID, "error", err)
		}

		if item.Operation == "sync" {
			if err := p.storage.MarkIncomeSyncError(ctx, item.IncomeID); err != nil {
				slog.ErrorContext(ctx, "Failed to mark income sync error",
					"income_id", item.IncomeID, "error", err)
			}
		}

		slog.ErrorContext(ctx, "Income sync item failed permanently after max retries",
			"id", item.ID,
			"income_id", item.IncomeID,
			"attempts", item.Attempts+1)
	} else {
		if err := p.storage.IncrementIncomeSyncAttempt(ctx, item.ID, processErr.Error()); err != nil {
			slog.ErrorContext(ctx, "Failed to increment income sync attempt",
				"id", item.ID, "error", err)
		}
	}
}

// cleanupCompleted removes old completed items
func (p *IncomeSyncProcessor) cleanupCompleted(ctx context.Context) {
	cutoff := time.Now().Add(-p.config.CleanupAge)
	if err := p.storage.CleanupCompletedIncomeSyncs(ctx, cutoff); err != nil {
		slog.ErrorContext(ctx, "Failed to cleanup completed income syncs", "error", err)
	}
}

// Stats returns current income queue statistics
func (p *IncomeSyncProcessor) Stats(ctx context.Context) (*storage.GetIncomeSyncQueueStatsRow, error) {
	return p.storage.GetIncomeSyncQueueStats(ctx)
}

// RetryFailed resets all failed income items for retry
func (p *IncomeSyncProcessor) RetryFailed(ctx context.Context) error {
	return p.storage.RetryFailedIncomeSyncs(ctx)
}
