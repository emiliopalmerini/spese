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

// NetWorthSyncProcessor flushes pending nw_sync_queue rows into the dashboard
// sheet via the configured NetWorthWriter.
type NetWorthSyncProcessor struct {
	storage *storage.SQLiteRepository
	writer  sheets.NetWorthWriter
	config  SyncProcessorConfig

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewNetWorthSyncProcessor creates a new processor.
func NewNetWorthSyncProcessor(
	storage *storage.SQLiteRepository,
	writer sheets.NetWorthWriter,
	config SyncProcessorConfig,
) *NetWorthSyncProcessor {
	return &NetWorthSyncProcessor{
		storage: storage,
		writer:  writer,
		config:  config,
	}
}

// Start runs the loop. Returns an error if already running.
func (p *NetWorthSyncProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("net worth sync processor already running")
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})
	p.mu.Unlock()

	if err := p.storage.ResetStaleNetWorthProcessing(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to reset stale net worth processing", "error", err)
	}

	go p.runLoop(ctx)

	slog.InfoContext(ctx, "Net worth sync processor started",
		"poll_interval", p.config.PollInterval,
		"batch_size", p.config.BatchSize)
	return nil
}

// Stop signals the loop to stop and waits for completion.
func (p *NetWorthSyncProcessor) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	close(p.stopCh)
	select {
	case <-p.doneCh:
		slog.InfoContext(ctx, "Net worth sync processor stopped gracefully")
	case <-ctx.Done():
		slog.WarnContext(ctx, "Net worth sync processor stop timed out")
		return ctx.Err()
	}
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()
	return nil
}

// IsRunning reports whether the processor loop is active.
func (p *NetWorthSyncProcessor) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// ProcessOnce drains a single batch synchronously. Useful for tests and
// direct invocation outside the long-running loop.
func (p *NetWorthSyncProcessor) ProcessOnce(ctx context.Context) {
	p.processBatch(ctx)
}

func (p *NetWorthSyncProcessor) runLoop(ctx context.Context) {
	defer close(p.doneCh)

	pollTicker := time.NewTicker(p.config.PollInterval)
	defer pollTicker.Stop()

	cleanupTicker := time.NewTicker(p.config.CleanupInterval)
	defer cleanupTicker.Stop()

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

func (p *NetWorthSyncProcessor) processBatch(ctx context.Context) {
	if p.writer == nil {
		return
	}
	items, err := p.storage.DequeueNetWorthSyncBatch(ctx, int64(p.config.BatchSize))
	if err != nil {
		slog.ErrorContext(ctx, "Dequeue net worth batch failed", "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	for _, item := range items {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		if err := p.storage.MarkNetWorthSyncProcessing(ctx, item.ID); err != nil {
			slog.ErrorContext(ctx, "Mark nw processing failed", "id", item.ID, "error", err)
			continue
		}

		if err := p.processItem(ctx, item); err != nil {
			p.handleFailure(ctx, item, err)
		} else {
			if markErr := p.storage.MarkNetWorthSyncComplete(ctx, item.ID); markErr != nil {
				slog.ErrorContext(ctx, "Mark nw complete failed", "id", item.ID, "error", markErr)
			}
		}
	}
}

func (p *NetWorthSyncProcessor) processItem(ctx context.Context, item storage.NwSyncQueue) error {
	acc, err := p.storage.GetAccount(ctx, item.AccountID)
	if err != nil {
		return fmt.Errorf("get account %d: %w", item.AccountID, err)
	}
	ref, err := p.writer.UpsertBalance(ctx, acc.Name, acc.Type, int(item.Year), int(item.Month),
		core.Money{Cents: item.AmountCents})
	if err != nil {
		return fmt.Errorf("write balance to sheet: %w", err)
	}
	slog.InfoContext(ctx, "Synced net worth row", "id", item.ID, "ref", ref)
	return nil
}

func (p *NetWorthSyncProcessor) handleFailure(ctx context.Context, item storage.NwSyncQueue, processErr error) {
	slog.WarnContext(ctx, "Net worth sync item failed",
		"id", item.ID,
		"attempt", item.Attempts+1,
		"error", processErr)

	if item.Attempts+1 >= int64(p.config.MaxRetries) {
		if err := p.storage.MarkNetWorthSyncFailed(ctx, item.ID, processErr.Error()); err != nil {
			slog.ErrorContext(ctx, "Mark nw failed", "id", item.ID, "error", err)
		}
		slog.ErrorContext(ctx, "Net worth sync exhausted retries",
			"id", item.ID,
			"account_id", item.AccountID,
			"attempts", item.Attempts+1)
		return
	}
	if err := p.storage.IncrementNetWorthSyncAttempt(ctx, item.ID, processErr.Error()); err != nil {
		slog.ErrorContext(ctx, "Increment nw attempt", "id", item.ID, "error", err)
	}
}

func (p *NetWorthSyncProcessor) cleanupCompleted(ctx context.Context) {
	cutoff := time.Now().Add(-p.config.CleanupAge)
	if err := p.storage.CleanupCompletedNetWorthSyncs(ctx, cutoff); err != nil {
		slog.ErrorContext(ctx, "Cleanup completed nw syncs", "error", err)
	}
}
