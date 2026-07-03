package sheetmirror

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"spese/internal/storage"
)

// MessagePublisher is the RabbitMQ publishing port used by the outbox relay.
type MessagePublisher interface {
	Publish(ctx context.Context, messageID string, body []byte, headers amqp.Table) error
	Close() error
}

// Relay drains the SQLite outbox and publishes confirmed RabbitMQ messages.
type Relay struct {
	Store          *storage.Store
	Publisher      MessagePublisher
	Logger         *slog.Logger
	PollInterval   time.Duration
	PublishTimeout time.Duration
	StaleAfter     time.Duration
}

// Run blocks until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	defer r.Publisher.Close()

	poll := r.PollInterval
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	timeout := r.PublishTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	staleAfter := r.StaleAfter
	if staleAfter <= 0 {
		staleAfter = time.Minute
	}

	for ctx.Err() == nil {
		msg, ok, err := r.Store.ClaimSheetSyncOutbox(ctx, staleAfter)
		if err != nil {
			r.logError("claim sheet sync outbox", err)
			if err := sleepContext(ctx, poll); err != nil {
				return nil
			}
			continue
		}
		if !ok {
			if err := sleepContext(ctx, poll); err != nil {
				return nil
			}
			continue
		}

		publishCtx, cancel := context.WithTimeout(ctx, timeout)
		err = r.Publisher.Publish(publishCtx, fmt.Sprintf("sheet-sync:%d", msg.ID), msg.Payload, amqp.Table{
			"x-spese-outbox-id": msg.ID,
			"x-spese-scope":     msg.Scope,
			"x-spese-attempt":   0,
		})
		cancel()
		if err != nil {
			delay := publishBackoff(msg.Attempts)
			if markErr := r.Store.MarkSheetSyncPublishFailed(ctx, msg.ID, err, delay); markErr != nil {
				r.logError("mark sheet sync publish failed", markErr)
			}
			r.logError("publish sheet sync", err)
			continue
		}
		if err := r.Store.MarkSheetSyncPublished(ctx, msg.ID); err != nil {
			r.logError("mark sheet sync published", err)
			continue
		}
		if r.Logger != nil {
			r.Logger.Info("sheet sync published", "outbox_id", msg.ID, "scope", msg.Scope)
		}
	}
	return nil
}

func (r *Relay) logError(msg string, err error) {
	if r.Logger != nil {
		r.Logger.Error(msg, "err", err)
	}
}

func publishBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(attempts) * 5 * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
