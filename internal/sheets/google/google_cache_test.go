package google

import (
	"testing"
	"time"
)

func TestRowCacheExpiration(t *testing.T) {
	c := &Client{
		cacheValidDuration: 100 * time.Millisecond, // Short TTL for testing
	}

	// Initial state: cache should be expired
	c.mu.Lock()
	isValid := time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if isValid {
		t.Error("cache should start expired")
	}

	// Manually set cache to valid state
	c.mu.Lock()
	c.cachedRowCount = 10
	c.cacheExpiresAt = time.Now().Add(c.cacheValidDuration)
	c.mu.Unlock()

	// Cache should be valid now
	c.mu.Lock()
	isValid = time.Now().Before(c.cacheExpiresAt)
	rowCount := c.cachedRowCount
	c.mu.Unlock()
	if !isValid {
		t.Error("cache should be valid immediately after update")
	}
	if rowCount != 10 {
		t.Errorf("cached row count should be 10, got %d", rowCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Cache should be expired now
	c.mu.Lock()
	isValid = time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if isValid {
		t.Error("cache should be expired after TTL")
	}
}

func TestInvalidateRowCache(t *testing.T) {
	c := &Client{
		cacheValidDuration: 10 * time.Minute,
	}

	// Set cache to valid state
	c.mu.Lock()
	c.cachedRowCount = 42
	c.cacheExpiresAt = time.Now().Add(c.cacheValidDuration)
	c.mu.Unlock()

	// Verify cache is valid
	c.mu.Lock()
	isValid := time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if !isValid {
		t.Error("cache should be valid before invalidation")
	}

	// Invalidate
	c.InvalidateRowCache()

	// Verify cache is now expired
	c.mu.Lock()
	isValid = time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if isValid {
		t.Error("cache should be expired after invalidation")
	}
}

func TestCacheInitialState(t *testing.T) {
	c := &Client{
		cacheValidDuration: 2 * time.Minute,
	}

	// Verify initial state
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedRowCount != 0 {
		t.Errorf("initial cachedRowCount should be 0, got %d", c.cachedRowCount)
	}

	if time.Now().Before(c.cacheExpiresAt) {
		t.Error("initial cacheExpiresAt should be in the past (expired)")
	}

	if c.cacheValidDuration != 2*time.Minute {
		t.Errorf("cache duration should be 2 minutes, got %v", c.cacheValidDuration)
	}
}

func TestCacheNextRowCalculation(t *testing.T) {
	c := &Client{
		cacheValidDuration: 2 * time.Minute,
	}

	tests := []struct {
		name           string
		cachedRowCount int
		expectedNext   int
	}{
		{"empty sheet", 0, 1},
		{"one row", 1, 2},
		{"ten rows", 10, 11},
		{"hundred rows", 100, 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.mu.Lock()
			c.cachedRowCount = tt.cachedRowCount
			c.cacheExpiresAt = time.Now().Add(c.cacheValidDuration)
			c.mu.Unlock()

			nextRow := tt.cachedRowCount + 1
			if nextRow != tt.expectedNext {
				t.Errorf("expected next row %d, got %d", tt.expectedNext, nextRow)
			}
		})
	}
}

func TestCacheMutexProtection(t *testing.T) {
	c := &Client{
		cacheValidDuration: 2 * time.Minute,
	}

	// Write and read concurrently to verify mutex protection
	done := make(chan struct{})
	errors := make(chan string, 10)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			c.mu.Lock()
			c.cachedRowCount = i
			c.cacheExpiresAt = time.Now().Add(c.cacheValidDuration)
			c.mu.Unlock()
		}
		done <- struct{}{}
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			c.mu.Lock()
			_ = c.cachedRowCount
			_ = c.cacheExpiresAt
			c.mu.Unlock()
		}
		done <- struct{}{}
	}()

	// Invalidator goroutine
	go func() {
		for i := 0; i < 50; i++ {
			c.InvalidateRowCache()
			time.Sleep(1 * time.Millisecond)
		}
		done <- struct{}{}
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	if len(errors) > 0 {
		t.Fatalf("concurrent access errors: %v", <-errors)
	}
}

func TestCacheValidityCheck(t *testing.T) {
	c := &Client{
		cacheValidDuration: 100 * time.Millisecond,
	}

	// Set cache with specific timestamp
	c.mu.Lock()
	c.cachedRowCount = 50
	c.cacheExpiresAt = time.Now().Add(50 * time.Millisecond) // Expires in 50ms
	c.mu.Unlock()

	// Immediately check: should be valid
	c.mu.Lock()
	valid := time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if !valid {
		t.Error("cache should be valid immediately after setting")
	}

	// Wait for partial expiration
	time.Sleep(75 * time.Millisecond)

	// Check again: should be expired
	c.mu.Lock()
	valid = time.Now().Before(c.cacheExpiresAt)
	c.mu.Unlock()
	if valid {
		t.Error("cache should be expired after TTL")
	}
}
