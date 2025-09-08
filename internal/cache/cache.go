package cache

import "time"

// Cache defines a generic cache interface
type Cache[T any] interface {
	// Get retrieves a value from the cache
	Get(key string) (T, bool)
	
	// Set stores a value in the cache
	Set(key string, data T)
	
	// Delete removes a key from the cache
	Delete(key string)
	
	// Size returns the current number of items in the cache
	Size() int
}

// Manager handles cache lifecycle and cleanup
type Manager struct {
	caches       []Cleaner
	stopCleanup  chan struct{}
	cleanupDone  chan struct{}
}

// Cleaner interface for caches that support cleanup
type Cleaner interface {
	CleanExpired() int
}

// NewManager creates a new cache manager
func NewManager() *Manager {
	return &Manager{
		caches:      make([]Cleaner, 0),
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
}

// Register adds a cache to the manager for cleanup
func (m *Manager) Register(cache Cleaner) {
	m.caches = append(m.caches, cache)
}

// StartCleanup begins periodic cleanup of all registered caches
func (m *Manager) StartCleanup(interval time.Duration) {
	go m.cleanup(interval)
}

func (m *Manager) cleanup(interval time.Duration) {
	defer close(m.cleanupDone)
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			totalCleaned := 0
			for _, cache := range m.caches {
				totalCleaned += cache.CleanExpired()
			}
			// Could add logging here if needed
		case <-m.stopCleanup:
			return
		}
	}
}

// Stop gracefully stops the cleanup routine
func (m *Manager) Stop() {
	if m.stopCleanup != nil {
		close(m.stopCleanup)
		<-m.cleanupDone
	}
}