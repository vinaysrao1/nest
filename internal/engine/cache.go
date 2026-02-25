package engine

import (
	"sync"
	"time"
)

// cacheEntry holds a single cached value with its expiry time.
type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// Cache is a TTL in-memory key/value cache safe for concurrent use.
// It is used to cache action config lookups (e.g. queue IDs by name) that
// change rarely but are accessed on the hot evaluation path.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

// NewCache creates a Cache with the given time-to-live for each entry.
//
// Pre-conditions: ttl must be positive.
// Post-conditions: returned Cache is ready for concurrent use.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// Get returns the cached value for key, and true if the entry exists and has
// not expired. Returns nil, false if the key is absent or the entry expired.
//
// Pre-conditions: key must be non-empty.
// Post-conditions: expired entries are not returned but remain until Purge.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

// Set stores value under key, overwriting any existing entry.
// The entry expires after the Cache's configured TTL.
//
// Pre-conditions: key must be non-empty.
func (c *Cache) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Purge removes all expired entries. Call periodically to prevent unbounded growth.
func (c *Cache) Purge() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

// Delete removes a single entry from the cache by key.
// If the key does not exist, this is a no-op.
//
// Pre-conditions: key must be non-empty.
// Post-conditions: the entry for key is removed; subsequent Get(key) returns false.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
