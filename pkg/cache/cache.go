package cache

import (
	"sync"
	"time"
)

// Entry represents a cached value with expiration time.
type Entry[T any] struct {
	Value     T
	ExpiresAt time.Time
}

// Cache provides a thread-safe TTL cache with generic value types.
type Cache[T any] struct {
	mu      sync.RWMutex
	entries map[string]Entry[T]
	ttl     time.Duration
}

// New creates a new Cache with the given TTL.
func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{
		entries: make(map[string]Entry[T]),
		ttl:     ttl,
	}
}

// Get retrieves a value from the cache. Returns the value and true if found and not expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		var zero T
		return zero, false
	}

	if time.Now().After(entry.ExpiresAt) {
		var zero T
		return zero, false
	}

	return entry.Value, true
}

// Set stores a value in the cache with the configured TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = Entry[T]{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// SetWithTTL stores a value with a custom TTL.
func (c *Cache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = Entry[T]{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a value from the cache.
func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from the cache.
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]Entry[T])
}

// Cleanup removes all expired entries from the cache.
func (c *Cache[T]) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}

// Len returns the number of entries in the cache (including expired).
func (c *Cache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
