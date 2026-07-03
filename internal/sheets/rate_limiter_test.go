package sheets

import (
	"context"
	"testing"
	"time"
)

func TestWriteRateLimiterSpacesRequestsWithoutBurst(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	var sleeps []time.Duration
	limiter := newWriteRateLimiter(6*time.Second, func() time.Time {
		return now
	}, func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		now = now.Add(d)
		return nil
	})

	for range 3 {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}

	want := []time.Duration{6 * time.Second, 6 * time.Second, 6 * time.Second}
	if len(sleeps) != len(want) {
		t.Fatalf("sleeps = %v, want %v", sleeps, want)
	}
	for i := range want {
		if sleeps[i] != want[i] {
			t.Fatalf("sleeps = %v, want %v", sleeps, want)
		}
	}
}

func TestNewWriteRateLimiterValidatesRate(t *testing.T) {
	if limiter, err := NewWriteRateLimiter(0); err != nil || limiter != nil {
		t.Fatalf("NewWriteRateLimiter(0) = (%v, %v), want nil nil", limiter, err)
	}
	if _, err := NewWriteRateLimiter(-1); err == nil {
		t.Fatal("NewWriteRateLimiter(-1) error = nil")
	}
}
