// Package engine implements the Starlark rule evaluation pipeline for Nest.
// It provides a goroutine worker pool where each worker owns an isolated
// Starlark thread, per-worker memoization, and lock-free atomic counters.
// The engine hot-reloads rule snapshots via atomic.Pointer without stopping
// the evaluation pipeline.
package engine

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
	"github.com/vinaysrao1/nest/internal/store"
)

// counterKey identifies a time-bucketed counter for a specific entity/event/window.
// The bucket field is time.Now().Unix() / windowSeconds.
type counterKey struct {
	orgID         string
	entityID      string
	eventType     string
	windowSeconds int
	bucket        int64
}

// evalContext holds per-event evaluation state for a single Starlark evaluation.
// It is created fresh for each event and never shared between goroutines.
type evalContext struct {
	ctx          context.Context
	event        domain.Event
	signalCache  map[string]domain.SignalOutput
	logs         []string
	actionNames  []string
	enqueuedJobs []domain.MRTJob
}

// ruleResult is the outcome of evaluating a single rule against an event.
type ruleResult struct {
	ruleID    string
	version   int
	priority  int
	verdict   domain.Verdict
	latencyUs int64
	err       error
}

// EvalRequest is submitted to the pool's event channel for evaluation.
// The caller receives the outcome on the Result channel.
type EvalRequest struct {
	Event  domain.Event
	Ctx    context.Context
	Result chan<- EvalResult
}

// EvalResult is the complete outcome of evaluating all matching rules for an event.
type EvalResult struct {
	Verdict        domain.Verdict
	TriggeredRules []domain.TriggeredRule
	ActionRequests []domain.ActionRequest
	Logs           []string
	LatencyUs      int64
	CorrelationID  string
	Error          error
}

// counterCleanupInterval is how many events are processed between stale counter
// cleanup passes. A value of 100 keeps memory bounded without adding per-event cost.
const counterCleanupInterval = 100

// Worker is a single evaluation goroutine. Each worker owns its Starlark thread,
// a per-event memo map, per-worker atomic counters, and an eval cache keyed by
// rule ID. Workers reference the shared Pool for cross-worker operations such as
// CounterSum and signal registry lookups.
//
// Invariant: Worker fields must NOT be accessed from outside the worker's goroutine
// except for the counters map which is read by Pool.CounterSum under counterMu.
type Worker struct {
	id          int
	thread      *starlark.Thread
	predeclared starlark.StringDict
	memo        map[string]starlark.Value
	counterMu   sync.RWMutex
	counters    map[counterKey]*atomic.Int64
	regexCache  map[string]*regexp.Regexp
	evalCache   map[string]starlark.Callable
	lastSnap    string
	eventCount  int
	pool        *Pool
	logger      *slog.Logger
	currentCtx  *evalContext
}

// Pool manages a goroutine worker pool for Starlark rule evaluation.
// Rule snapshots are stored in the snapshots sync.Map keyed by org ID,
// with values of type *atomic.Pointer[Snapshot] to allow lock-free
// hot-reload per org.
//
// Invariant: Pool.eventCh is the sole entry point from external callers.
// No mutex protects the hot path — workers read snapshot via atomic.Pointer.
type Pool struct {
	workers     []*Worker
	snapshots   sync.Map // map[orgID string] -> *atomic.Pointer[Snapshot]
	registry    *signal.Registry
	store       *store.Queries
	logger      *slog.Logger
	eventCh     chan EvalRequest
	done        chan struct{}
	wg          sync.WaitGroup
	actionCache *Cache
	ctx         context.Context
	cancel      context.CancelFunc
}

// ActionCache returns the pool's action cache for external invalidation.
// Used by the composition root to wire CacheInvalidator for MRTService.
func (p *Pool) ActionCache() *Cache {
	return p.actionCache
}

// CounterSum aggregates the per-worker atomic counter values for the given
// (orgID, entityID, eventType, windowSeconds) key across all pool workers.
//
// Each worker maintains its own map of atomic.Int64 counters keyed by
// counterKey. This method reads across all workers to produce a total count
// for the current time bucket. Per-worker counter maps are only written by
// the owning worker goroutine; the *atomic.Int64 values themselves are safe
// to read concurrently via Load().
//
// Pre-conditions: windowSeconds must be positive.
// Post-conditions: returns 0 if no worker has a counter for the key.
func (p *Pool) CounterSum(orgID, entityID, eventType string, windowSeconds int) int64 {
	key := buildCounterKey(orgID, entityID, eventType, windowSeconds)
	var total int64
	for _, w := range p.workers {
		w.counterMu.RLock()
		counter, ok := w.counters[key]
		w.counterMu.RUnlock()
		if ok {
			total += counter.Load()
		}
	}
	return total
}
