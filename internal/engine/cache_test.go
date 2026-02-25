package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestCache_SetAndGet verifies a stored value is returned before TTL expires.
func TestCache_SetAndGet(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)
	c.Set("key1", "value1")

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("Get returned ok=false for existing key")
	}
	if got != "value1" {
		t.Errorf("Get returned %v, want %q", got, "value1")
	}
}

// TestCache_MissingKey verifies Get returns false for an absent key.
func TestCache_MissingKey(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("Get returned ok=true for absent key, want false")
	}
}

// TestCache_TTLExpiry verifies that an entry is not returned after its TTL expires.
func TestCache_TTLExpiry(t *testing.T) {
	t.Parallel()
	c := NewCache(20 * time.Millisecond)
	c.Set("expiring", 42)

	// Should be present immediately.
	_, ok := c.Get("expiring")
	if !ok {
		t.Fatal("Get returned ok=false immediately after Set")
	}

	// Wait for TTL to expire.
	time.Sleep(30 * time.Millisecond)

	_, ok = c.Get("expiring")
	if ok {
		t.Error("Get returned ok=true after TTL expiry, want false")
	}
}

// TestCache_Overwrite verifies that Set overwrites an existing entry.
func TestCache_Overwrite(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)
	c.Set("key", "first")
	c.Set("key", "second")

	got, ok := c.Get("key")
	if !ok {
		t.Fatal("Get returned ok=false after overwrite")
	}
	if got != "second" {
		t.Errorf("Get returned %v, want %q", got, "second")
	}
}

// TestCache_Purge verifies that Purge removes expired entries.
func TestCache_Purge(t *testing.T) {
	t.Parallel()
	c := NewCache(20 * time.Millisecond)
	c.Set("stale", "value")

	time.Sleep(30 * time.Millisecond)
	c.Purge()

	c.mu.RLock()
	_, present := c.entries["stale"]
	c.mu.RUnlock()

	if present {
		t.Error("Purge did not remove expired entry")
	}
}

// TestCache_ConcurrentAccess verifies that concurrent Set and Get are race-free.
func TestCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			c.Set(key, n)
		}(i)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			c.Get(key)
		}(i)
	}
	wg.Wait()
}

// TestCache_Delete_Existing verifies Delete removes an existing entry.
func TestCache_Delete_Existing(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)
	c.Set("to-delete", "value")

	// Verify it exists.
	_, ok := c.Get("to-delete")
	if !ok {
		t.Fatal("Get returned ok=false before Delete")
	}

	c.Delete("to-delete")

	_, ok = c.Get("to-delete")
	if ok {
		t.Error("Get returned ok=true after Delete, want false")
	}
}

// TestCache_Delete_NonExistent verifies Delete is a no-op for absent keys.
func TestCache_Delete_NonExistent(t *testing.T) {
	t.Parallel()
	c := NewCache(time.Minute)

	// Should not panic.
	c.Delete("never-set")
}
