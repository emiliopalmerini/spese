package services

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"spese/internal/amqp"
	"spese/internal/core"
	"spese/internal/storage"
)

// ExpenseService orchestrates expense operations across SQLite and AMQP
type ExpenseService struct {
	storage    *storage.SQLiteRepository
	amqpClient *amqp.Client
}

func NewExpenseService(storage *storage.SQLiteRepository, amqpClient *amqp.Client) *ExpenseService {
	return &ExpenseService{
		storage:    storage,
		amqpClient: amqpClient,
	}
}

// CreateExpense saves an expense locally and publishes sync message
func (s *ExpenseService) CreateExpense(ctx context.Context, e core.Expense) (string, error) {
	// Save to SQLite first (fast, reliable)
	ref, err := s.storage.Append(ctx, e)
	if err != nil {
		return "", fmt.Errorf("save expense: %w", err)
	}

	// Parse ID for AMQP message
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to parse expense ID", "ref", ref, "error", err)
		return ref, nil // Return anyway, SQLite save succeeded
	}

	// Publish async sync message (non-blocking, version 1 for new expense)
	if err := s.publishSyncMessage(ctx, id, 1); err != nil {
		slog.ErrorContext(ctx, "Failed to publish sync message",
			"id", id, "error", err)
		// Don't fail the request - expense is saved locally
	}

	return ref, nil
}

// DeleteExpense soft deletes an expense locally and publishes delete message
func (s *ExpenseService) DeleteExpense(ctx context.Context, id int64) error {
	// Soft delete from SQLite first
	if err := s.storage.SoftDeleteExpense(ctx, id); err != nil {
		return fmt.Errorf("soft delete expense: %w", err)
	}

	// Publish async delete message (non-blocking)
	if err := s.publishDeleteMessage(ctx, id); err != nil {
		slog.ErrorContext(ctx, "Failed to publish delete message",
			"id", id, "error", err)
		// Don't fail the request - expense is deleted locally
	}

	return nil
}

func (s *ExpenseService) publishSyncMessage(ctx context.Context, id, version int64) error {
	if s.amqpClient == nil {
		slog.WarnContext(ctx, "AMQP client not available, skipping sync message")
		return nil
	}

	return s.amqpClient.PublishExpenseSync(ctx, id, version)
}

func (s *ExpenseService) publishDeleteMessage(ctx context.Context, id int64) error {
	if s.amqpClient == nil {
		slog.WarnContext(ctx, "AMQP client not available, skipping delete message")
		return nil
	}

	return s.amqpClient.PublishExpenseDelete(ctx, id)
}

// Close closes both storage and AMQP connections
func (s *ExpenseService) Close() error {
	var errs []error

	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			errs = append(errs, fmt.Errorf("storage: %w", err))
		}
	}

	if s.amqpClient != nil {
		if err := s.amqpClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("amqp: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close expense service: %v", errs)
	}

	return nil
}
