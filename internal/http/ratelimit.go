package http

import (
	"sync"
	"sync/atomic"
	"time"
)

// rateLimiter implements a simple in-memory rate limiter per client IP.
type rateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientInfo
	stopCleanup  chan struct{}
	shutdownOnce sync.Once
}

type clientInfo struct {
	lastRequest time.Time
	requests    int
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		clients:     make(map[string]*clientInfo),
		stopCleanup: make(chan struct{}),
	}
	go rl.startCleanup()
	return rl
}

// startCleanup runs periodic cleanup to remove stale client entries.
func (rl *rateLimiter) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
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

// cleanupStaleEntries removes client entries older than 10 minutes.
func (rl *rateLimiter) cleanupStaleEntries() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, client := range rl.clients {
		if client.lastRequest.Before(cutoff) {
			delete(rl.clients, ip)
		}
	}
}

// stop gracefully shuts down the rate limiter cleanup goroutine.
func (rl *rateLimiter) stop() {
	rl.shutdownOnce.Do(func() {
		if rl.stopCleanup != nil {
			close(rl.stopCleanup)
		}
	})
}

// allow checks if a request from the given IP should be allowed.
// Returns false if rate limit (60 requests per minute) is exceeded.
func (rl *rateLimiter) allow(clientIP string, metrics *securityMetrics) bool {
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

	// Allow up to 60 requests per minute
	client.requests++
	client.lastRequest = now

	if client.requests > 60 {
		if metrics != nil {
			atomic.AddInt64(&metrics.rateLimitHits, 1)
		}
		return false
	}

	return true
}
