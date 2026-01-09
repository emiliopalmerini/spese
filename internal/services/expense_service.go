package services

import (
	"context"
	"fmt"
	"log/slog"

	"spese/internal/core"
	"spese/internal/storage"
)

// ExpenseService orchestrates expense operations with SQLite sync queue
type ExpenseService struct {
	storage *storage.SQLiteRepository
}

func NewExpenseService(storage *storage.SQLiteRepository) *ExpenseService {
	return &ExpenseService{
		storage: storage,
	}
}

// CreateExpense saves an expense and enqueues it for sync atomically
func (s *ExpenseService) CreateExpense(ctx context.Context, e core.Expense) (string, error) {
	// Use atomic transaction: save expense + enqueue sync in single transaction
	ref, err := s.storage.AppendAndEnqueueSync(ctx, e)
	if err != nil {
		return "", fmt.Errorf("save expense: %w", err)
	}

	slog.DebugContext(ctx, "Created expense and enqueued sync", "id", ref)
	return ref, nil
}

// DeleteExpense hard deletes an expense and enqueues delete sync atomically
func (s *ExpenseService) DeleteExpense(ctx context.Context, id int64) error {
	// Use atomic transaction: delete expense + enqueue delete sync
	if err := s.storage.HardDeleteAndEnqueueSync(ctx, id); err != nil {
		return fmt.Errorf("delete expense: %w", err)
	}

	slog.DebugContext(ctx, "Deleted expense and enqueued sync", "id", id)
	return nil
}

// Close closes the storage connection
func (s *ExpenseService) Close() error {
	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			return fmt.Errorf("close storage: %w", err)
		}
	}
	return nil
}
