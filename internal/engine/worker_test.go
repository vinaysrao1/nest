package engine

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
)

// compileTestRule is a helper to compile a Starlark rule from testdata for tests.
func compileTestRule(t *testing.T, filename string) *CompiledRule {
	t.Helper()
	c := &Compiler{}
	src := loadTestdata(t, filename)
	rule, err := c.CompileRule(src, filename)
	if err != nil {
		t.Fatalf("CompileRule(%s): %v", filename, err)
	}
	return rule
}

// makeTestEvent creates a domain.Event for test use.
func makeTestEvent(orgID, eventType string, payload map[string]any) domain.Event {
	return domain.Event{
		ID:        "test-event-id",
		EventType: eventType,
		ItemType:  "text",
		OrgID:     orgID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// TestWorker_EvaluateSingleRule verifies a compiled rule is evaluated and returns a verdict.
func TestWorker_EvaluateSingleRule(t *testing.T) {
	t.Parallel()
	rule := compileTestRule(t, "valid_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-1", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-1", snap)

	// Payload with "spam" triggers block.
	event := makeTestEvent("org-1", "content", map[string]any{"text": "this is spam"})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("Verdict = %s, want block", result.Verdict.Type)
	}
	if result.Verdict.Reason != "contains spam" {
		t.Errorf("Verdict.Reason = %q, want %q", result.Verdict.Reason, "contains spam")
	}
	if len(result.TriggeredRules) != 1 {
		t.Errorf("TriggeredRules len = %d, want 1", len(result.TriggeredRules))
	}
}

// TestWorker_EvaluateApprove verifies an event that does not trigger any block condition gets approve.
func TestWorker_EvaluateApprove(t *testing.T) {
	t.Parallel()
	rule := compileTestRule(t, "valid_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-2", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-2", snap)

	event := makeTestEvent("org-2", "content", map[string]any{"text": "clean content"})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("Verdict = %s, want approve", result.Verdict.Type)
	}
}

// TestWorker_MultiRulePriorityOrder verifies that when multiple rules fire,
// the highest-priority rule's verdict is the final verdict.
func TestWorker_MultiRulePriorityOrder(t *testing.T) {
	t.Parallel()
	// block-rule: priority 100, review-rule: priority 50, approve-rule: priority 10
	blockRule := compileTestRule(t, "multi_rule_block.star")
	reviewRule := compileTestRule(t, "multi_rule_review.star")
	approveRule := compileTestRule(t, "multi_rule_approve.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-multi", []*CompiledRule{blockRule, reviewRule, approveRule}, nil)
	pool.SwapSnapshot("org-multi", snap)

	event := makeTestEvent("org-multi", "content", map[string]any{})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Highest priority rule (block at 100) wins.
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("Verdict = %s, want block (highest priority rule wins)", result.Verdict.Type)
	}
	if result.Verdict.RuleID != "block-rule" {
		t.Errorf("Verdict.RuleID = %q, want %q", result.Verdict.RuleID, "block-rule")
	}
	// All three rules should appear in triggered.
	if len(result.TriggeredRules) != 3 {
		t.Errorf("TriggeredRules len = %d, want 3", len(result.TriggeredRules))
	}
}

// TestWorker_NoMatchingRules verifies approve when no rules match the event type.
func TestWorker_NoMatchingRules(t *testing.T) {
	t.Parallel()
	blockRule := compileTestRule(t, "multi_rule_block.star") // event_types = ["content"]

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-nomatch", []*CompiledRule{blockRule}, nil)
	pool.SwapSnapshot("org-nomatch", snap)

	// Submit an "image" event - no rules match.
	event := makeTestEvent("org-nomatch", "image", map[string]any{})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("Verdict = %s, want approve (no matching rules)", result.Verdict.Type)
	}
}

// TestWorker_WildcardRule verifies a wildcard rule evaluates for any event type.
func TestWorker_WildcardRule(t *testing.T) {
	t.Parallel()
	wildcardRule := compileTestRule(t, "wildcard_rule.star") // event_types = ["*"]

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-wildcard", []*CompiledRule{wildcardRule}, nil)
	pool.SwapSnapshot("org-wildcard", snap)

	for _, eventType := range []string{"content", "image", "profile", "comment"} {
		event := makeTestEvent("org-wildcard", eventType, map[string]any{})
		result, err := pool.Evaluate(context.Background(), event)
		if err != nil {
			t.Fatalf("Evaluate(%s): %v", eventType, err)
		}
		if result.Verdict.Type != domain.VerdictApprove {
			t.Errorf("event_type=%s: Verdict = %s, want approve", eventType, result.Verdict.Type)
		}
		if len(result.TriggeredRules) != 1 {
			t.Errorf("event_type=%s: TriggeredRules len = %d, want 1", eventType, len(result.TriggeredRules))
		}
	}
}

// TestWorker_OrgIsolation verifies snapshots for different orgs are independent.
func TestWorker_OrgIsolation(t *testing.T) {
	t.Parallel()
	blockRule := compileTestRule(t, "multi_rule_block.star")

	registry := signal.NewRegistry()
	pool := NewPool(2, registry, nil, nil)
	defer pool.Stop()

	// org-block has the block rule; org-empty has no rules.
	snapBlock := NewSnapshot("org-block", []*CompiledRule{blockRule}, nil)
	snapEmpty := NewSnapshot("org-empty", []*CompiledRule{}, nil)
	pool.SwapSnapshot("org-block", snapBlock)
	pool.SwapSnapshot("org-empty", snapEmpty)

	blockEvent := makeTestEvent("org-block", "content", map[string]any{})
	blockResult, err := pool.Evaluate(context.Background(), blockEvent)
	if err != nil {
		t.Fatalf("Evaluate org-block: %v", err)
	}
	if blockResult.Verdict.Type != domain.VerdictBlock {
		t.Errorf("org-block: Verdict = %s, want block", blockResult.Verdict.Type)
	}

	emptyEvent := makeTestEvent("org-empty", "content", map[string]any{})
	emptyResult, err := pool.Evaluate(context.Background(), emptyEvent)
	if err != nil {
		t.Fatalf("Evaluate org-empty: %v", err)
	}
	if emptyResult.Verdict.Type != domain.VerdictApprove {
		t.Errorf("org-empty: Verdict = %s, want approve", emptyResult.Verdict.Type)
	}
}

// TestWorker_SnapshotReload verifies that a SwapSnapshot mid-stream is picked up by workers.
func TestWorker_SnapshotReload(t *testing.T) {
	t.Parallel()
	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	// Start with no snapshot -> approve.
	event := makeTestEvent("org-reload", "content", map[string]any{})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate before snapshot: %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("Before snapshot: Verdict = %s, want approve", result.Verdict.Type)
	}

	// Swap in a block rule.
	blockRule := compileTestRule(t, "multi_rule_block.star")
	snap := NewSnapshot("org-reload", []*CompiledRule{blockRule}, nil)
	pool.SwapSnapshot("org-reload", snap)

	// Now should block.
	result, err = pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate after snapshot: %v", err)
	}
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("After snapshot: Verdict = %s, want block", result.Verdict.Type)
	}
}

// TestWorker_StepLimitKillsInfiniteLoop verifies that a long-running rule is terminated
// by the 10M step limit and does not produce a block verdict.
// The infinite_loop_rule.star uses range(100000000) which exceeds the 10M step cap.
func TestWorker_StepLimitKillsInfiniteLoop(t *testing.T) {
	t.Parallel()

	rule := compileTestRule(t, "infinite_loop_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	snap := NewSnapshot("org-steplimit", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-steplimit", snap)

	event := makeTestEvent("org-steplimit", "content", map[string]any{})

	result, err := pool.Evaluate(context.Background(), event)
	// The rule hits the step limit and returns an error result; pool defaults to approve.
	if err != nil {
		// Error is acceptable if the rule itself errored.
		t.Logf("Evaluate returned error (expected): %v", err)
	}
	if result != nil && result.Verdict.Type == domain.VerdictBlock {
		t.Error("Infinite loop rule should not have produced a block verdict")
	}
}

// TestWorker_ActionRequestsResolvedFromSnapshot verifies that action names referenced
// in a Starlark verdict() call are resolved to ActionRequest objects using the action
// definitions stored in the snapshot.
//
// valid_rule.star returns verdict("block", actions=["webhook-1"]) when payload.text
// contains "spam". This test puts a matching action in the snapshot and verifies it
// appears in EvalResult.ActionRequests.
func TestWorker_ActionRequestsResolvedFromSnapshot(t *testing.T) {
	t.Parallel()
	rule := compileTestRule(t, "valid_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	actions := map[string]domain.Action{
		"webhook-1": {
			ID:         "act-001",
			OrgID:      "org-actions",
			Name:       "webhook-1",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{"url": "https://example.com/hook"},
			Version:    1,
		},
	}
	snap := NewSnapshot("org-actions", []*CompiledRule{rule}, actions)
	pool.SwapSnapshot("org-actions", snap)

	// Payload with "spam" triggers the block verdict with actions=["webhook-1"].
	event := makeTestEvent("org-actions", "content", map[string]any{"text": "this is spam"})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("Verdict = %s, want block", result.Verdict.Type)
	}
	if len(result.ActionRequests) != 1 {
		t.Fatalf("ActionRequests len = %d, want 1", len(result.ActionRequests))
	}
	req := result.ActionRequests[0]
	if req.Action.ID != "act-001" {
		t.Errorf("ActionRequests[0].Action.ID = %q, want %q", req.Action.ID, "act-001")
	}
	if req.Action.Name != "webhook-1" {
		t.Errorf("ActionRequests[0].Action.Name = %q, want %q", req.Action.Name, "webhook-1")
	}
	if req.Action.ActionType != domain.ActionTypeWebhook {
		t.Errorf("ActionRequests[0].Action.ActionType = %q, want %q", req.Action.ActionType, domain.ActionTypeWebhook)
	}
	if req.CorrelationID != event.ID {
		t.Errorf("ActionRequests[0].CorrelationID = %q, want %q", req.CorrelationID, event.ID)
	}
}

// TestWorker_ActionRequestsEmptyWhenNoActionsInSnapshot verifies that action names
// referenced in a verdict() call are silently skipped when no matching action exists
// in the snapshot. This handles the case where an action was deleted since the last
// snapshot rebuild.
func TestWorker_ActionRequestsEmptyWhenNoActionsInSnapshot(t *testing.T) {
	t.Parallel()
	rule := compileTestRule(t, "valid_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	// Snapshot has no actions — "webhook-1" referenced in the rule cannot be resolved.
	snap := NewSnapshot("org-noactions", []*CompiledRule{rule}, nil)
	pool.SwapSnapshot("org-noactions", snap)

	event := makeTestEvent("org-noactions", "content", map[string]any{"text": "this is spam"})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictBlock {
		t.Errorf("Verdict = %s, want block", result.Verdict.Type)
	}
	// Action name not in snapshot: ActionRequests must be nil/empty, not an error.
	if len(result.ActionRequests) != 0 {
		t.Errorf("ActionRequests len = %d, want 0 (action not in snapshot)", len(result.ActionRequests))
	}
}

// TestWorker_ApproveVerdictHasNoActionRequests verifies that approve verdicts do not
// produce ActionRequests even when the snapshot contains action definitions.
func TestWorker_ApproveVerdictHasNoActionRequests(t *testing.T) {
	t.Parallel()
	rule := compileTestRule(t, "valid_rule.star")

	registry := signal.NewRegistry()
	pool := NewPool(1, registry, nil, nil)
	defer pool.Stop()

	actions := map[string]domain.Action{
		"webhook-1": {
			ID:         "act-002",
			OrgID:      "org-approve-actions",
			Name:       "webhook-1",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{"url": "https://example.com/hook"},
			Version:    1,
		},
	}
	snap := NewSnapshot("org-approve-actions", []*CompiledRule{rule}, actions)
	pool.SwapSnapshot("org-approve-actions", snap)

	// Clean payload: rule returns approve with no actions.
	event := makeTestEvent("org-approve-actions", "content", map[string]any{"text": "clean content"})
	result, err := pool.Evaluate(context.Background(), event)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Verdict.Type != domain.VerdictApprove {
		t.Errorf("Verdict = %s, want approve", result.Verdict.Type)
	}
	if len(result.ActionRequests) != 0 {
		t.Errorf("ActionRequests len = %d, want 0 (approve verdict has no actions)", len(result.ActionRequests))
	}
}
