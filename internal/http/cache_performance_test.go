package http

import (
	"spese/internal/cache"
	"spese/internal/core"
	"testing"
	"time"
)

// TestLRUCachePerformance verifies the new LRU cache performance and eviction
func TestLRUCachePerformance(t *testing.T) {
	// Create small cache for testing eviction
	cache := cache.NewLRUCache[core.MonthOverview](3, 100*time.Millisecond) // 3 items max, 100ms TTL

	// Test basic operations
	start := time.Now()
	for i := 0; i < 1000; i++ {
		key := "test-key"
		overview := core.MonthOverview{Year: 2025, Month: 1}
		cache.Set(key, overview)
		
		if _, found := cache.Get(key); !found {
			t.Errorf("Cache miss on iteration %d", i)
		}
	}
	duration := time.Since(start)
	
	t.Logf("1000 cache operations took %v", duration)
	
	// Should be very fast (well under 1ms per operation)
	if duration > 100*time.Millisecond {
		t.Errorf("Cache operations too slow: %v", duration)
	}
}

// TestLRUCacheEviction tests size-based eviction
func TestLRUCacheEviction(t *testing.T) {
	cache := cache.NewLRUCache[string](3, time.Hour) // 3 items max
	
	// Fill beyond capacity
	cache.Set("key1", "value1")
	cache.Set("key2", "value2") 
	cache.Set("key3", "value3")
	cache.Set("key4", "value4") // Should evict key1
	
	// key1 should be evicted (LRU)
	if _, found := cache.Get("key1"); found {
		t.Error("key1 should have been evicted")
	}
	
	// Others should still exist
	if _, found := cache.Get("key2"); !found {
		t.Error("key2 should still exist")
	}
	if _, found := cache.Get("key3"); !found {
		t.Error("key3 should still exist") 
	}
	if _, found := cache.Get("key4"); !found {
		t.Error("key4 should still exist")
	}
}

// TestLRUCacheTTLExpiration tests time-based expiration
func TestLRUCacheTTLExpiration(t *testing.T) {
	cache := cache.NewLRUCache[string](100, 50*time.Millisecond) // 50ms TTL
	
	cache.Set("key1", "value1")
	
	// Should exist immediately
	if _, found := cache.Get("key1"); !found {
		t.Error("key1 should exist immediately")
	}
	
	// Wait for expiration
	time.Sleep(60 * time.Millisecond)
	
	// Should be expired
	if _, found := cache.Get("key1"); found {
		t.Error("key1 should have expired")
	}
}

// TestLRUCacheCleanExpired tests the cleanup mechanism
func TestLRUCacheCleanExpired(t *testing.T) {
	cache := cache.NewLRUCache[string](100, 50*time.Millisecond)
	
	// Add some items
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	
	// Wait for expiration
	time.Sleep(60 * time.Millisecond)
	
	// Run cleanup
	removed := cache.CleanExpired()
	
	// Should have cleaned up all 3 items
	if removed != 3 {
		t.Errorf("Expected 3 items cleaned, got %d", removed)
	}
}

// BenchmarkLRUCache benchmarks cache performance
func BenchmarkLRUCache(b *testing.B) {
	cache := cache.NewLRUCache[core.MonthOverview](1000, time.Hour)
	overview := core.MonthOverview{Year: 2025, Month: 1}
	
	b.ResetTimer()
	
	// Test mixed read/write workload
	for i := 0; i < b.N; i++ {
		key := "bench-key"
		if i%10 == 0 {
			// 10% writes
			cache.Set(key, overview)
		} else {
			// 90% reads
			cache.Get(key)
		}
	}
}

// BenchmarkCacheCleanup benchmarks the cleanup mechanism
func BenchmarkCacheCleanup(b *testing.B) {
	cache := cache.NewLRUCache[string](1000, time.Nanosecond) // Very short TTL
	
	// Fill cache with expired items
	for i := 0; i < 100; i++ {
		cache.Set("key", "value")
	}
	
	// Wait for expiration
	time.Sleep(time.Millisecond)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cache.CleanExpired()
	}
}