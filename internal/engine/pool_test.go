package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
)

// newTestPool creates a Pool suitable for unit tests. The pool is stopped via t.Cleanup.
func newTestPool(t *testing.T, workerCount int) *Pool {
	t.Helper()
	registry := signal.NewRegistry()
	pool := NewPool(workerCount, registry, nil, nil)
	t.Cleanup(pool.Stop)
	return pool
}

// loadTestdataBench reads a testdata file for benchmarks.
func loadTestdataBench(b *testing.B, name string) string {
	b.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatalf("loadTestdataBench: read %s: %v", name, err)
	}
	return string(data)
}

// TestResolveVerdict_EmptyReturnsApprove verifies that empty results produce approve.
func TestResolveVerdict_EmptyReturnsApprove(t *testing.T) {
	t.Parallel()
	v := resolveVerdict(nil)
	if v.Type != domain.VerdictApprove {
		t.Errorf("resolveVerdict(nil) = %s, want approve", v.Type)
	}
}

// TestResolveVerdict_AllErrorsReturnsApprove verifies that all-error results produce approve.
func TestResolveVerdict_AllErrorsReturnsApprove(t *testing.T) {
	t.Parallel()
	results := []ruleResult{
		{ruleID: "r1", priority: 100, err: fmt.Errorf("failed")},
		{ruleID: "r2", priority: 50, err: fmt.Errorf("failed")},
	}
	v := resolveVerdict(results)
	if v.Type != domain.VerdictApprove {
		t.Errorf("resolveVerdict(all errors) = %s, want approve", v.Type)
	}
}

// TestResolveVerdict_HighestPriorityWins verifies the highest-priority rule verdict is used,
// even when a lower-priority rule has a "heavier" verdict type.
func TestResolveVerdict_HighestPriorityWins(t *testing.T) {
	t.Parallel()
	results := []ruleResult{
		{ruleID: "high", priority: 100, verdict: domain.Verdict{Type: domain.VerdictApprove, RuleID: "high"}},
		{ruleID: "low", priority: 10, verdict: domain.Verdict{Type: domain.VerdictBlock, RuleID: "low"}},
	}
	v := resolveVerdict(results)
	if v.Type != domain.VerdictApprove {
		t.Errorf("resolveVerdict: want approve (priority-100 rule wins), got %s", v.Type)
	}
	if v.RuleID != "high" {
		t.Errorf("resolveVerdict: RuleID = %q, want %q", v.RuleID, "high")
	}
}

// TestResolveVerdict_TieBrokenByWeight verifies that ties in priority are broken by verdict weight.
func TestResolveVerdict_TieBrokenByWeight(t *testing.T) {
	t.Parallel()
	results := []ruleResult{
		{ruleID: "approve-rule", priority: 50, verdict: domain.Verdict{Type: domain.VerdictApprove, RuleID: "approve-rule"}},
		{ruleID: "block-rule", priority: 50, verdict: domain.Verdict{Type: domain.VerdictBlock, RuleID: "block-rule"}},
	}
	v := resolveVerdict(results)
	if v.Type != domain.VerdictBlock {
		t.Errorf("resolveVerdict tie: want block (higher weight), got %s", v.Type)
	}
}

// TestResolveVerdict_ReviewVsApprove verifies review outweighs approve on a priority tie.
func TestResolveVerdict_ReviewVsApprove(t *testing.T) {
	t.Parallel()
	results := []ruleResult{
		{ruleID: "approve-rule", priority: 50, verdict: domain.Verdict{Type: domain.VerdictApprove, RuleID: "approve-rule"}},
		{ruleID: "review-rule", priority: 50, verdict: domain.Verdict{Type: domain.VerdictReview, RuleID: "review-rule"}},
	}
	v := resolveVerdict(results)
	if v.Type != domain.VerdictReview {
		t.Errorf("resolveVerdict tie: want review, got %s", v.Type)
	}
}

// TestPool_SwapSnapshot_NewOrg verifies SwapSnapshot stores a snapshot for a new org.
func TestPool_SwapSnapshot_NewOrg(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t, 1)

	snap := NewSnapshot("org-a", []*CompiledRule{}, nil)
	pool.SwapSnapshot("org-a", snap)

	ptrVal, ok := pool.snapshots.Load("org-a")
	if !ok {
		t.Fatal("SwapSnapshot: org-a not found in snapshots map")
	}
	loaded := ptrVal.(*atomic.Pointer[Snapshot]).Load()
	if loaded == nil {
		t.Fatal("SwapSnapshot: loaded snapshot is nil")
	}
	if loaded.ID != snap.ID {
		t.Errorf("SwapSnapshot: loaded.ID = %q, want %q", loaded.ID, snap.ID)
	}
}

// TestPool_SwapSnapshot_ExistingOrg verifies SwapSnapshot replaces an existing snapshot.
func TestPool_SwapSnapshot_ExistingOrg(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t, 1)

	snap1 := NewSnapshot("org-b", []*CompiledRule{}, nil)
	snap2 := NewSnapshot("org-b", []*CompiledRule{}, nil)
	pool.SwapSnapshot("org-b", snap1)
	pool.SwapSnapshot("org-b", snap2)

	ptrVal, ok := pool.snapshots.Load("org-b")
	if !ok {
		t.Fatal("SwapSnapshot existing: org-b not found")
	}
	loaded := ptrVal.(*atomic.Pointer[Snapshot]).Load()
	if loaded.ID != snap2.ID {
		t.Errorf("SwapSnapshot existing: loaded.ID = %q, want snap2.ID %q", loaded.ID, snap2.ID)
	}
}

// TestPool_SwapSnapshot_Concurrent verifies concurrent SwapSnapshot calls are race-free.
func TestPool_SwapSnapshot_Concurrent(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t, 2) // Stop is registered via t.Cleanup

	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			orgID := fmt.Sprintf("org-%d", n%3)
			snap := NewSnapshot(orgID, []*CompiledRule{}, nil)
			pool.SwapSnapshot(orgID, snap)
		}(i)
	}
	wg.Wait()
}

// TestPool_Stop_DrainsWorkers verifies that Stop returns promptly when no events are pending.
func TestPool_Stop_DrainsWorkers(t *testing.T) {
	t.Parallel()
	registry := signal.NewRegistry()
	pool := NewPool(4, registry, nil, nil)

	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stopped promptly.
	case <-time.After(2 * time.Second):
		t.Error("Stop did not return within 2 seconds")
	}
}

// TestPool_Evaluate_NoSnapshot verifies approve when no snapshot exists for the org.
func TestPool_Evaluate_NoSnapshot(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t, 1)

	event := domain.Event{
		ID:        "evt-001",
		EventType: "content",
		ItemType:  "text",
		OrgID:     "no-snap-org",
		Payload:   map[string]any{},
		Timestamp: time.Now(),
	}

	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("Evaluate (no snapshot): verdict = %s, want approve", result.Verdict.Type)
	}
	if result.CorrelationID != event.ID {
		t.Errorf("Evaluate: CorrelationID = %q, want %q", result.CorrelationID, event.ID)
	}
}

// TestPool_Evaluate_CancelledContext verifies a pre-cancelled context returns an error.
func TestPool_Evaluate_CancelledContext(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before submitting

	event := domain.Event{
		ID:        "evt-cancel",
		EventType: "content",
		ItemType:  "text",
		OrgID:     "org-cancel",
		Payload:   map[string]any{},
		Timestamp: time.Now(),
	}

	_, err := pool.Evaluate(ctx, event)
	if err == nil {
		t.Error("Evaluate with cancelled context: want error, got nil")
	}
}

// BenchmarkEvaluate measures end-to-end evaluation throughput.
func BenchmarkEvaluate(b *testing.B) {
	c := &Compiler{}
	src := loadTestdataBench(b, "valid_rule.star")
	rule, err := c.CompileRule(src, "valid_rule.star")
	if err != nil {
		b.Fatalf("CompileRule: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(4, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("bench-org", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("bench-org", snap)

	event := domain.Event{
		ID:        "bench-evt",
		EventType: "content",
		ItemType:  "text",
		OrgID:     "bench-org",
		Payload:   map[string]any{"text": "hello world"},
		Timestamp: time.Now(),
	}
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = pool.Evaluate(ctx, event)
		}
	})
}
