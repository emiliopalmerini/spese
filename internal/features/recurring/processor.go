package recurring

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"spese/internal/features/transactions"
	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Processor scans the recurring tab on a fixed interval and fires due
// transactions. Idempotent: if a transaction with the recurring's marker
// already exists for the current month, it is skipped.
type Processor struct {
	Client   *sheets.Client
	Logger   *slog.Logger
	Interval time.Duration
}

// Run starts the processing loop. Returns when ctx is cancelled.
func (p *Processor) Run(ctx context.Context) error {
	if p.Interval <= 0 {
		p.Interval = 6 * time.Hour
	}
	t := time.NewTicker(p.Interval)
	defer t.Stop()

	// Fire once on startup so a freshly-restarted server doesn't wait an
	// interval before catching today's recurrings.
	if n, err := p.RunOnce(ctx, kernel.Today()); err != nil {
		p.Logger.Error("recurring run", "err", err)
	} else if n > 0 {
		p.Logger.Info("recurring fired on startup", "count", n)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if n, err := p.RunOnce(ctx, kernel.Today()); err != nil {
				p.Logger.Error("recurring run", "err", err)
			} else if n > 0 {
				p.Logger.Info("recurring fired", "count", n)
			}
		}
	}
}

// RunOnce evaluates all recurring rows once against `today` and returns the
// number of fired transactions.
func (p *Processor) RunOnce(ctx context.Context, today kernel.Date) (int, error) {
	recs, err := List(ctx, p.Client, true)
	if err != nil {
		return 0, fmt.Errorf("list recurring: %w", err)
	}

	monthStart := today.FirstOfMonth()
	monthEnd := kernel.Date{Time: monthStart.AddDate(0, 1, 0)}
	monthTxns, err := transactions.List(ctx, p.Client, transactions.Filter{
		From: monthStart,
		To:   monthEnd,
	}, true)
	if err != nil {
		return 0, fmt.Errorf("list month transactions: %w", err)
	}

	var due []transactions.Transaction
	for _, r := range recs {
		if !r.Active || r.DayOfMonth <= 0 || r.DayOfMonth > today.Day() {
			continue
		}
		if alreadyFired(monthTxns, r.Marker()) {
			continue
		}
		fireDate := dateForDayInMonth(today, r.DayOfMonth)
		amt := r.Amount
		if amt < 0 {
			amt = -amt
		}
		if r.Kind == transactions.Expense {
			amt = -amt
		}
		due = append(due, transactions.Transaction{
			Date:        fireDate,
			Kind:        r.Kind,
			Account:     r.Account,
			Amount:      amt,
			Category:    r.Category,
			Subcategory: r.Subcategory,
			Payee:       r.Payee,
			Note:        strings.TrimSpace(r.Marker() + " " + r.Note),
		})
	}
	if len(due) == 0 {
		return 0, nil
	}
	if err := transactions.Append(ctx, p.Client, due); err != nil {
		return 0, fmt.Errorf("append recurring transactions: %w", err)
	}
	return len(due), nil
}

func alreadyFired(txns []transactions.Transaction, marker string) bool {
	for _, t := range txns {
		if strings.Contains(t.Note, marker) {
			return true
		}
	}
	return false
}

// dateForDayInMonth returns a Date for the same year/month as `today` but
// with the requested day. If the requested day exceeds the month's length
// (e.g. day=31 in February), it snaps to the last day of the month.
func dateForDayInMonth(today kernel.Date, day int) kernel.Date {
	first := today.FirstOfMonth().Time
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return kernel.Date{Time: time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, first.Location())}
}
