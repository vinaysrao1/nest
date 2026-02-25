package signal

import "sync"

// Registry is a thread-safe collection of signal adapters indexed by ID.
// Callers may Register and retrieve adapters concurrently.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry creates an empty Registry.
//
// Post-conditions: returned Registry is ready for concurrent use.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register adds or replaces an adapter in the registry.
// If an adapter with the same ID already exists it is overwritten.
//
// Pre-conditions: adapter must not be nil; adapter.ID() must be non-empty.
func (r *Registry) Register(adapter Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[adapter.ID()] = adapter
}

// Get returns the adapter with the given id, or nil if not found.
//
// Post-conditions: result is nil when no adapter matches id.
func (r *Registry) Get(id string) Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.adapters[id]
}

// All returns a snapshot of all registered adapters.
// The order of the returned slice is non-deterministic.
//
// Post-conditions: returned slice is non-nil.
func (r *Registry) All() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		result = append(result, a)
	}
	return result
}
