package engine

import (
	"fmt"
	"sync/atomic"
	"time"

	"go.starlark.net/starlark"
)

// counterUDF returns a Starlark built-in that increments and reads a
// time-bucketed counter for an entity/event-type pair.
//
// Signature: counter(entity_id, event_type, window_seconds) -> int
//
// The counter is per-worker and stored in the worker's counters map. After
// incrementing the worker-local counter, the UDF calls Pool.CounterSum to
// aggregate across all workers in the pool, returning the total count within
// the current time bucket.
//
// Counters are never persisted; they reset when the process restarts. For
// persistent rate-limiting across restarts, use the store.IncrementCounter path.
func counterUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("counter", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var entityID, eventType string
		var windowSeconds int
		if err := starlark.UnpackPositionalArgs("counter", args, kwargs, 3, &entityID, &eventType, &windowSeconds); err != nil {
			return nil, err
		}

		if windowSeconds <= 0 {
			return nil, fmt.Errorf("counter: window_seconds must be positive, got %d", windowSeconds)
		}

		orgID := ""
		if w.currentCtx != nil {
			orgID = w.currentCtx.event.OrgID
		}

		key := buildCounterKey(orgID, entityID, eventType, windowSeconds)
		incrementWorkerCounter(w, key)

		total := w.pool.CounterSum(orgID, entityID, eventType, windowSeconds)
		return starlark.MakeInt64(total), nil
	})
}

// cleanStaleCounters removes counter entries whose time bucket is more than one
// bucket behind the current bucket. Call this periodically (e.g., every 100 events)
// to bound memory usage of the per-worker counters map.
//
// counterMu is acquired exclusively because we delete map entries.
func (w *Worker) cleanStaleCounters() {
	now := time.Now().Unix()
	w.counterMu.Lock()
	defer w.counterMu.Unlock()
	for key := range w.counters {
		currentBucket := now / int64(key.windowSeconds)
		if key.bucket < currentBucket-1 {
			delete(w.counters, key)
		}
	}
}

// buildCounterKey constructs a counterKey for the current time bucket.
// The bucket is time.Now().Unix() divided by windowSeconds, so all events
// within the same window map to the same bucket.
func buildCounterKey(orgID, entityID, eventType string, windowSeconds int) counterKey {
	bucket := time.Now().Unix() / int64(windowSeconds)
	return counterKey{
		orgID:         orgID,
		entityID:      entityID,
		eventType:     eventType,
		windowSeconds: windowSeconds,
		bucket:        bucket,
	}
}

// incrementWorkerCounter atomically increments the counter for key in w.counters.
// If no counter exists for the key, a new atomic.Int64 is created and stored.
// counterMu guards map reads and writes; the *atomic.Int64 value is safe to
// Add/Load concurrently once stored under the lock.
func incrementWorkerCounter(w *Worker, key counterKey) {
	w.counterMu.Lock()
	counter, ok := w.counters[key]
	if !ok {
		counter = new(atomic.Int64)
		w.counters[key] = counter
	}
	w.counterMu.Unlock()
	counter.Add(1)
}
