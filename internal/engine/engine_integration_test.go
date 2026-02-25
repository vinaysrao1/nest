package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
)

// TestIntegration_CompileSnapshotPoolEvaluate exercises the full pipeline:
// Compiler -> Snapshot -> Pool -> Evaluate -> verify verdict.
func TestIntegration_CompileSnapshotPoolEvaluate(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	src := loadTestdata(t, "valid_rule.star")
	rule, err := c.CompileRule(src, "valid_rule.star")
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(2, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-integ", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-integ", snap)

	tests := []struct {
		name        string
		payload     map[string]any
		wantVerdict domain.VerdictType
	}{
		{"spam payload blocks", map[string]any{"text": "this is spam"}, domain.VerdictBlock},
		{"clean payload approves", map[string]any{"text": "clean"}, domain.VerdictApprove},
		{"empty payload approves", map[string]any{}, domain.VerdictApprove},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := domain.Event{
				ID:        "evt-" + tc.name,
				EventType: "content",
				ItemType:  "text",
				OrgID:     "org-integ",
				Payload:   tc.payload,
				Timestamp: time.Now(),
			}
			result, evalErr := pool.Evaluate(context.Background(), event)
			if evalErr != nil {
				t.Fatalf("Evaluate: %v", evalErr)
			}
			if result.Verdict.Type != tc.wantVerdict {
				t.Errorf("Verdict = %s, want %s", result.Verdict.Type, tc.wantVerdict)
			}
		})
	}
}

// TestIntegration_MultiRulePriorityCorrectness verifies a 3-rule setup with
// different priorities produces the correct highest-priority verdict.
func TestIntegration_MultiRulePriorityCorrectness(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	var rules []*CompiledRule
	for _, filename := range []string{"multi_rule_block.star", "multi_rule_review.star", "multi_rule_approve.star"} {
		src := loadTestdata(t, filename)
		rule, err := c.CompileRule(src, filename)
		if err != nil {
			t.Fatalf("CompileRule(%s): %v", filename, err)
		}
		rules = append(rules, rule)
	}

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-priority", rules, nil)
	pool.SwapSnapshot("org-priority", snap)

	event := domain.Event{
		ID:        "evt-priority",
		EventType: "content",
		ItemType:  "text",
		OrgID:     "org-priority",
		Payload:   map[string]any{},
		Timestamp: time.Now(),
	}
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// block-rule (priority=100) > review-rule (50) > approve-rule (10)
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("Verdict = %s, want block (priority 100 rule)", result.Verdict.Type)
	}
}

// TestIntegration_WildcardRuleEvaluatesForAnyEvent verifies wildcards fire for all event types.
func TestIntegration_WildcardRuleEvaluatesForAnyEvent(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	src := loadTestdata(t, "wildcard_rule.star")
	rule, err := c.CompileRule(src, "wildcard_rule.star")
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(2, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-wc", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-wc", snap)

	for _, eventType := range []string{"content", "image", "profile", "comment", "transaction"} {
		event := domain.Event{
			ID:        "evt-wc",
			EventType: eventType,
			ItemType:  "text",
			OrgID:     "org-wc",
			Payload:   map[string]any{},
			Timestamp: time.Now(),
		}
		result, evalErr := pool.Evaluate(context.Background(), event)
		if evalErr != nil {
			t.Fatalf("Evaluate(%s): %v", eventType, evalErr)
		}
		if len(result.TriggeredRules) != 1 {
			t.Errorf("event_type=%s: TriggeredRules len = %d, want 1", eventType, len(result.TriggeredRules))
		}
		if result.TriggeredRules[0].RuleID != "test-wildcard" {
			t.Errorf("event_type=%s: TriggeredRules[0].RuleID = %q, want %q",
				eventType, result.TriggeredRules[0].RuleID, "test-wildcard")
		}
	}
}

// TestIntegration_OrgIsolation verifies that different orgs have separate snapshots.
func TestIntegration_OrgIsolation(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	blockSrc := loadTestdata(t, "multi_rule_block.star")
	blockRule, err := c.CompileRule(blockSrc, "multi_rule_block.star")
	if err != nil {
		t.Fatalf("CompileRule block: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(2, registry, nil, nil)
	defer pool.Stop()

	// org-a has a block rule; org-b has no rules.
	pool.SwapSnapshot("org-a", NewSnapshot("org-a", []*CompiledRule{blockRule}, nil))
	pool.SwapSnapshot("org-b", NewSnapshot("org-b", []*CompiledRule{}, nil))

	makeEvent := func(orgID string) domain.Event {
		return domain.Event{
			ID:        "evt-isolation",
			EventType: "content",
			ItemType:  "text",
			OrgID:     orgID,
			Payload:   map[string]any{},
			Timestamp: time.Now(),
		}
	}

	resultA, err := pool.Evaluate(context.Background(), makeEvent("org-a"))
	if err != nil {
		t.Fatalf("Evaluate org-a: %v", err)
	}
	if resultA.Verdict.Type != domain.VerdictBlock {
		t.Errorf("org-a: Verdict = %s, want block", resultA.Verdict.Type)
	}

	resultB, err := pool.Evaluate(context.Background(), makeEvent("org-b"))
	if err != nil {
		t.Fatalf("Evaluate org-b: %v", err)
	}
	if resultB.Verdict.Type != domain.VerdictApprove {
		t.Errorf("org-b: Verdict = %s, want approve", resultB.Verdict.Type)
	}
}

// TestIntegration_ConcurrentEvaluation verifies the pool handles concurrent evaluations safely.
func TestIntegration_ConcurrentEvaluation(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	src := loadTestdata(t, "valid_rule.star")
	rule, err := c.CompileRule(src, "valid_rule.star")
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(4, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-concurrent", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-concurrent", snap)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var payload map[string]any
			if n%2 == 0 {
				payload = map[string]any{"text": "this is spam"}
			} else {
				payload = map[string]any{"text": "clean"}
			}
			event := domain.Event{
				ID:        fmt.Sprintf("evt-concurrent-%d", n),
				EventType: "content",
				ItemType:  "text",
				OrgID:     "org-concurrent",
				Payload:   payload,
				Timestamp: time.Now(),
			}
			result, evalErr := pool.Evaluate(context.Background(), event)
			if evalErr != nil {
				errs <- evalErr
				return
			}
			wantVerdict := domain.VerdictApprove
			if n%2 == 0 {
				wantVerdict = domain.VerdictBlock
			}
			if result.Verdict.Type != wantVerdict {
				errs <- fmt.Errorf("goroutine %d: Verdict = %s, want %s", n, result.Verdict.Type, wantVerdict)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestIntegration_SnapshotHotSwap verifies zero-downtime snapshot replacement.
func TestIntegration_SnapshotHotSwap(t *testing.T) {
	t.Parallel()

	c := &Compiler{}
	blockSrc := loadTestdata(t, "multi_rule_block.star")
	blockRule, err := c.CompileRule(blockSrc, "multi_rule_block.star")
	if err != nil {
		t.Fatalf("CompileRule block: %v", err)
	}
	approveSrc := loadTestdata(t, "multi_rule_approve.star")
	approveRule, err := c.CompileRule(approveSrc, "multi_rule_approve.star")
	if err != nil {
		t.Fatalf("CompileRule approve: %v", err)
	}

	registry := signal.NewRegistry()
	pool := NewPool(2, registry, nil, nil)
	defer pool.Stop()

	event := domain.Event{
		ID:        "evt-hotswap",
		EventType: "content",
		ItemType:  "text",
		OrgID:     "org-hotswap",
		Payload:   map[string]any{},
		Timestamp: time.Now(),
	}

	// Start with block rule.
	pool.SwapSnapshot("org-hotswap", NewSnapshot("org-hotswap", []*CompiledRule{blockRule}, nil))

	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate (block snap): %v", err)
	}
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("After block snapshot: Verdict = %s, want block", result.Verdict.Type)
	}

	// Hot-swap to approve-only snapshot.
	pool.SwapSnapshot("org-hotswap", NewSnapshot("org-hotswap", []*CompiledRule{approveRule}, nil))

	result, err = pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate (approve snap): %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("After approve snapshot: Verdict = %s, want approve", result.Verdict.Type)
	}
}
