package sheets

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WriteRateLimiter spaces write requests with no burst. A rate of 10/minute
// means every Google Sheets write waits for a 6 second slot.
type WriteRateLimiter struct {
	interval time.Duration

	mu    sync.Mutex
	next  time.Time
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

// NewWriteRateLimiter returns a limiter for write requests per minute. A rate
// of 0 disables limiting.
func NewWriteRateLimiter(ratePerMinute int) (*WriteRateLimiter, error) {
	if ratePerMinute < 0 {
		return nil, fmt.Errorf("rate must be >= 0")
	}
	if ratePerMinute == 0 {
		return nil, nil
	}
	return newWriteRateLimiter(time.Minute/time.Duration(ratePerMinute), time.Now, sleepContext), nil
}

func newWriteRateLimiter(interval time.Duration, now func() time.Time, sleep func(context.Context, time.Duration) error) *WriteRateLimiter {
	return &WriteRateLimiter{interval: interval, now: now, sleep: sleep}
}

// Wait blocks until the next write slot is available.
func (l *WriteRateLimiter) Wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return nil
	}

	l.mu.Lock()
	now := l.now()
	scheduled := now.Add(l.interval)
	if !l.next.IsZero() && l.next.After(scheduled) {
		scheduled = l.next
	}
	l.next = scheduled.Add(l.interval)
	wait := scheduled.Sub(now)
	l.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	return l.sleep(ctx, wait)
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
