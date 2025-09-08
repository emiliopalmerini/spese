package cache

import (
	"container/list"
	"sync"
	"time"
)

// LRU cache with TTL and size-based eviction
type LRUCache[T any] struct {
	mu       sync.Mutex
	maxSize  int
	ttl      time.Duration
	items    map[string]*list.Element
	lru      *list.List
}

type cacheItem[T any] struct {
	key       string
	data      T
	expiresAt time.Time
}

// NewLRUCache creates a new LRU cache with TTL
func NewLRUCache[T any](maxSize int, ttl time.Duration) *LRUCache[T] {
	return &LRUCache[T]{
		maxSize: maxSize,
		ttl:     ttl,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero T
	elem, exists := c.items[key]
	if !exists {
		return zero, false
	}

	item := elem.Value.(*cacheItem[T])
	
	// Check if expired
	if time.Now().After(item.expiresAt) {
		c.removeElement(elem)
		return zero, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	return item.data, true
}

// Set stores a value in the cache
func (c *LRUCache[T]) Set(key string, data T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	item := &cacheItem[T]{
		key:       key,
		data:      data,
		expiresAt: now.Add(c.ttl),
	}

	// Check if key already exists
	if elem, exists := c.items[key]; exists {
		elem.Value = item
		c.lru.MoveToFront(elem)
		return
	}

	// Add new item
	elem := c.lru.PushFront(item)
	c.items[key] = elem

	// Evict if over capacity
	if c.lru.Len() > c.maxSize {
		oldest := c.lru.Back()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}
}

// Delete removes a key from the cache
func (c *LRUCache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
	}
}

func (c *LRUCache[T]) removeElement(elem *list.Element) {
	item := elem.Value.(*cacheItem[T])
	delete(c.items, item.key)
	c.lru.Remove(elem)
}

// CleanExpired removes all expired entries and returns count of removed items
func (c *LRUCache[T]) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element
	
	for elem := c.lru.Front(); elem != nil; elem = elem.Next() {
		item := elem.Value.(*cacheItem[T])
		if now.After(item.expiresAt) {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		c.removeElement(elem)
	}

	return len(toRemove)
}

// Size returns the current number of items in the cache
func (c *LRUCache[T]) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}