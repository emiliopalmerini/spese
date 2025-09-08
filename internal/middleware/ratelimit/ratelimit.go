package ratelimit

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter provides rate limiting functionality
type Limiter struct {
	mu           sync.Mutex
	clients      map[string]*clientInfo
	stopCleanup  chan struct{}
	shutdownOnce sync.Once
	
	// Configuration
	requestsPerMinute int
	cleanupInterval   time.Duration
}

type clientInfo struct {
	lastRequest time.Time
	requests    int
}

// Config holds rate limiter configuration
type Config struct {
	RequestsPerMinute int
	CleanupInterval   time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		RequestsPerMinute: 60,
		CleanupInterval:   5 * time.Minute,
	}
}

// NewLimiter creates a new rate limiter
func NewLimiter(config Config) *Limiter {
	if config.RequestsPerMinute <= 0 {
		config = DefaultConfig()
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	
	rl := &Limiter{
		clients:           make(map[string]*clientInfo),
		stopCleanup:       make(chan struct{}),
		requestsPerMinute: config.RequestsPerMinute,
		cleanupInterval:   config.CleanupInterval,
	}
	go rl.startCleanup()
	return rl
}

// Allow checks if a request from the given IP should be allowed
func (rl *Limiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	client, exists := rl.clients[clientIP]

	if !exists {
		rl.clients[clientIP] = &clientInfo{
			lastRequest: now,
			requests:    1,
		}
		return true
	}

	// Reset counter if more than 1 minute has passed
	if now.Sub(client.lastRequest) > time.Minute {
		client.requests = 1
		client.lastRequest = now
		return true
	}

	// Check limit
	client.requests++
	client.lastRequest = now

	return client.requests <= rl.requestsPerMinute
}

// startCleanup runs periodic cleanup to remove stale client entries
func (rl *Limiter) startCleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupStaleEntries()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanupStaleEntries removes client entries older than 10 minutes
func (rl *Limiter) cleanupStaleEntries() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, client := range rl.clients {
		if client.lastRequest.Before(cutoff) {
			delete(rl.clients, ip)
		}
	}
}

// ActiveClients returns the number of currently tracked clients
func (rl *Limiter) ActiveClients() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.clients)
}

// Stop gracefully shuts down the rate limiter cleanup goroutine
func (rl *Limiter) Stop() {
	rl.shutdownOnce.Do(func() {
		if rl.stopCleanup != nil {
			close(rl.stopCleanup)
		}
	})
}

// GetMetrics returns current rate limiting metrics
func (rl *Limiter) GetMetrics() Metrics {
	rl.mu.Lock()
	clientCount := int64(len(rl.clients))
	rl.mu.Unlock()
	
	return Metrics{
		TotalHits:   0, // This would need to be tracked if needed
		ClientCount: clientCount,
	}
}

// Metrics for monitoring rate limit performance
type Metrics struct {
	TotalHits   int64
	ClientCount int64
}

// MetricsCollector tracks rate limiting metrics
type MetricsCollector struct {
	totalHits   int64
	clientCount int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// RecordHit records a rate limit hit
func (m *MetricsCollector) RecordHit() {
	atomic.AddInt64(&m.totalHits, 1)
}

// UpdateClientCount updates the active client count
func (m *MetricsCollector) UpdateClientCount(count int64) {
	atomic.StoreInt64(&m.clientCount, count)
}

// GetMetrics returns current metrics
func (m *MetricsCollector) GetMetrics() Metrics {
	return Metrics{
		TotalHits:   atomic.LoadInt64(&m.totalHits),
		ClientCount: atomic.LoadInt64(&m.clientCount),
	}
}

// Middleware creates HTTP middleware for rate limiting
func (rl *Limiter) Middleware(extractIP func(*http.Request) string, onLimit func(http.ResponseWriter, *http.Request)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractIP(r)
			
			if !rl.Allow(clientIP) {
				if onLimit != nil {
					onLimit(w, r)
				} else {
					w.Header().Set("Retry-After", "60")
					http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				}
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}