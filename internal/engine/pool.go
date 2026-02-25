package engine

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sync/atomic"
	"time"

	"go.starlark.net/starlark"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
	"github.com/vinaysrao1/nest/internal/store"
)

// NewPool creates a Pool with the specified number of workers and starts
// each worker goroutine. The pool is ready to accept events immediately.
//
// Pre-conditions: workerCount must be positive; registry and st must not be nil.
// Post-conditions: all workers are running; pool is ready for Evaluate calls.
func NewPool(
	workerCount int,
	registry *signal.Registry,
	st *store.Queries,
	logger *slog.Logger,
) *Pool {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		workers:     make([]*Worker, workerCount),
		registry:    registry,
		store:       st,
		logger:      logger,
		eventCh:     make(chan EvalRequest, workerCount*2),
		done:        make(chan struct{}),
		actionCache: NewCache(5 * time.Minute),
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := range workerCount {
		w := &Worker{
			id:         i,
			thread:     &starlark.Thread{Name: fmt.Sprintf("worker-%d", i)},
			memo:       make(map[string]starlark.Value),
			counters:   make(map[counterKey]*atomic.Int64),
			regexCache: make(map[string]*regexp.Regexp),
			evalCache:  make(map[string]starlark.Callable),
			pool:       p,
			logger:     logger.With("worker", i),
		}
		w.predeclared = BuildUDFs(w)
		p.workers[i] = w
		p.wg.Add(1)
		go w.run()
	}

	go p.runCachePurge()

	return p
}

// runCachePurge periodically purges expired entries from the action cache.
// It exits when the pool context is cancelled (i.e., Stop is called).
func (p *Pool) runCachePurge() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.actionCache.Purge()
		case <-p.ctx.Done():
			return
		}
	}
}

// Evaluate submits an event for rule evaluation and blocks until the result
// is available or the context is cancelled.
//
// Pre-conditions: ctx must not be nil; event.OrgID must be non-empty.
// Post-conditions: returns a non-nil EvalResult on success; returns error on context cancellation.
// Raises: ctx.Err() if the context is cancelled before a result is available.
func (p *Pool) Evaluate(ctx context.Context, event domain.Event) (*EvalResult, error) {
	resultCh := make(chan EvalResult, 1)
	req := EvalRequest{
		Event:  event,
		Ctx:    ctx,
		Result: resultCh,
	}

	select {
	case p.eventCh <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case result := <-resultCh:
		if result.Error != nil {
			return &result, result.Error
		}
		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SwapSnapshot atomically replaces the snapshot for the given org.
// If no snapshot exists for the org, a new atomic pointer is created.
// The swap is lock-free on the hot evaluation path.
//
// Pre-conditions: orgID must be non-empty; snap must not be nil.
// Post-conditions: subsequent events for orgID will use snap.
func (p *Pool) SwapSnapshot(orgID string, snap *Snapshot) {
	ptr := new(atomic.Pointer[Snapshot])
	ptr.Store(snap)
	actual, loaded := p.snapshots.LoadOrStore(orgID, ptr)
	if loaded {
		actual.(*atomic.Pointer[Snapshot]).Store(snap)
	}
}

// Stop closes the event channel and waits for all workers to drain and exit.
// After Stop returns, no further events will be processed.
//
// Pre-conditions: Stop must be called at most once.
// Post-conditions: all workers have exited; done channel is closed.
func (p *Pool) Stop() {
	p.cancel()
	close(p.eventCh)
	p.wg.Wait()
	close(p.done)
}

// resolveVerdict determines the final verdict from a slice of rule results.
// Rules are already ordered by priority descending (from RulesForEvent).
// Among rules with the same priority, the heaviest verdict weight wins.
// Errors are treated as approve (rule failed, do not block).
//
// Pre-conditions: results may be empty or contain errors.
// Post-conditions: always returns a non-zero VerdictType.
func resolveVerdict(results []ruleResult) domain.Verdict {
	if len(results) == 0 {
		return domain.Verdict{Type: domain.VerdictApprove}
	}

	// Find the highest priority among successful results.
	highestPriority := -1
	for _, r := range results {
		if r.err == nil && r.verdict.Type != "" && r.priority > highestPriority {
			highestPriority = r.priority
		}
	}

	// No successful results.
	if highestPriority == -1 {
		return domain.Verdict{Type: domain.VerdictApprove}
	}

	// Among results at the highest priority, pick the heaviest verdict.
	var best *ruleResult
	for i := range results {
		r := &results[i]
		if r.err != nil || r.verdict.Type == "" {
			continue
		}
		if r.priority != highestPriority {
			continue
		}
		if best == nil || verdictWeight(r.verdict.Type) > verdictWeight(best.verdict.Type) {
			best = r
		}
	}

	if best == nil {
		return domain.Verdict{Type: domain.VerdictApprove}
	}
	return best.verdict
}

// verdictWeight assigns a numeric weight to each verdict type for tie-breaking.
// Higher weight means the verdict takes precedence.
func verdictWeight(t domain.VerdictType) int {
	switch t {
	case domain.VerdictBlock:
		return 3
	case domain.VerdictReview:
		return 2
	case domain.VerdictApprove:
		return 1
	default:
		return 0
	}
}
