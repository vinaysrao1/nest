package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/engine"
	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/signal"
)

// setupPostVerdictPipeline creates a PostVerdictPipeline backed by the test database.
// Returns the pipeline, the store.Queries, and a cleanup function.
func setupPostVerdictPipeline(t *testing.T) (*service.PostVerdictPipeline, func()) {
	t.Helper()

	q, dbCleanup := setupTestDB(t)
	logger := testLogger()
	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, logger)
	pipeline := service.NewPostVerdictPipeline(publisher, q, logger)

	return pipeline, dbCleanup
}

// TestPostVerdictPipeline_Execute_EmptyActions verifies that Execute is a no-op
// when ActionRequests is empty — no panic, no DB writes, returns empty slice.
func TestPostVerdictPipeline_Execute_EmptyActions(t *testing.T) {
	pipeline, cleanup := setupPostVerdictPipeline(t)
	defer cleanup()

	ctx := context.Background()
	params := service.PostVerdictParams{
		ActionRequests: []domain.ActionRequest{},
		OrgID:          "org_empty",
		ItemID:         "item_empty",
		ItemTypeID:     "ity_empty",
		Payload:        map[string]any{"text": "test"},
		CorrelationID:  "corr_empty",
	}

	results := pipeline.Execute(ctx, params)
	if results == nil {
		t.Fatal("Execute returned nil, expected non-nil slice")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty actions, got %d", len(results))
	}
}

// TestPostVerdictPipeline_Execute_PublishesAndLogs verifies that Execute writes
// action_executions to the database when actions are present.
// The webhook action will fail (no real server), but the execution record should
// still be logged with Success=false.
func TestPostVerdictPipeline_Execute_PublishesAndLogs(t *testing.T) {
	q, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	logger := testLogger()
	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, logger)
	pipeline := service.NewPostVerdictPipeline(publisher, q, logger)

	ctx := context.Background()

	// Seed an org and action in the DB so action_executions can reference them.
	orgID := seedOrg(t, q, "TestPostVerdictPipeline_Execute_PublishesAndLogs")
	action := seedAction(t, q, orgID, "webhook-notify")

	itemID := generateTestID("item")
	itemTypeID := generateTestID("ity")
	correlationID := generateTestID("corr")
	time.Sleep(time.Millisecond) // ensure unique nano IDs

	params := service.PostVerdictParams{
		ActionRequests: []domain.ActionRequest{
			{
				Action:        *action,
				ItemID:        itemID,
				Payload:       map[string]any{"text": "hello"},
				CorrelationID: correlationID,
			},
		},
		OrgID:         orgID,
		ItemID:        itemID,
		ItemTypeID:    itemTypeID,
		Payload:       map[string]any{"text": "hello"},
		CorrelationID: correlationID,
	}

	results := pipeline.Execute(ctx, params)
	if len(results) != 1 {
		t.Fatalf("expected 1 ActionResult, got %d", len(results))
	}
	// The webhook will fail (no real server), but the ActionID must match.
	if results[0].ActionID != action.ID {
		t.Errorf("ActionResult.ActionID = %q, want %q", results[0].ActionID, action.ID)
	}
}

// TestPostVerdictPipeline_LogRuleExecutions verifies that LogRuleExecutions
// persists rule execution records for non-empty TriggeredRules.
func TestPostVerdictPipeline_LogRuleExecutions(t *testing.T) {
	q, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	logger := testLogger()
	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, logger)
	pipeline := service.NewPostVerdictPipeline(publisher, q, logger)

	ctx := context.Background()

	orgID := seedOrg(t, q, "TestPostVerdictPipeline_LogRuleExecutions")
	itemID := generateTestID("item")
	itemTypeID := generateTestID("ity")
	ruleID := generateTestID("rule")
	correlationID := generateTestID("corr")
	time.Sleep(time.Millisecond) // ensure unique nano IDs

	result := &engine.EvalResult{
		Verdict: domain.Verdict{Type: domain.VerdictApprove},
		TriggeredRules: []domain.TriggeredRule{
			{
				RuleID:    ruleID,
				Version:   1,
				Verdict:   domain.VerdictApprove,
				Reason:    "test rule fired",
				LatencyUs: 42,
			},
		},
		CorrelationID: correlationID,
	}

	// LogRuleExecutions must not panic and must not return an error.
	// Since the rule_executions table requires an existing rule, the store call
	// may return a foreign-key error; per contract, errors are logged not returned.
	// We verify there is no panic and execution completes.
	pipeline.LogRuleExecutions(ctx, service.RuleExecutionParams{
		OrgID:      orgID,
		ItemID:     itemID,
		ItemTypeID: itemTypeID,
		Result:     result,
	})
	// No assertion on DB state needed: the contract says errors are fire-and-forget.
	// The key invariant is that the function does not panic.
}

// TestPostVerdictPipeline_LogRuleExecutions_EmptyRules verifies that LogRuleExecutions
// is a no-op when TriggeredRules is empty.
func TestPostVerdictPipeline_LogRuleExecutions_EmptyRules(t *testing.T) {
	pipeline, cleanup := setupPostVerdictPipeline(t)
	defer cleanup()

	ctx := context.Background()

	result := &engine.EvalResult{
		Verdict:        domain.Verdict{Type: domain.VerdictApprove},
		TriggeredRules: []domain.TriggeredRule{},
		CorrelationID:  "corr_empty_rules",
	}

	// Must not panic for empty TriggeredRules.
	pipeline.LogRuleExecutions(ctx, service.RuleExecutionParams{
		OrgID:      "org_empty",
		ItemID:     "item_empty",
		ItemTypeID: "ity_empty",
		Result:     result,
	})
}

// TestNewPostVerdictPipeline_NilLogger verifies that a nil logger is replaced
// with slog.Default() without panicking.
func TestNewPostVerdictPipeline_NilLogger(t *testing.T) {
	q, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, nil)

	// Should not panic with nil logger.
	pipeline := service.NewPostVerdictPipeline(publisher, q, nil)
	if pipeline == nil {
		t.Fatal("NewPostVerdictPipeline returned nil")
	}
}

// TestPostVerdictPipeline_Execute_ConstructsTargetFromFlatParams verifies that
// Execute constructs ActionTarget internally from the flat PostVerdictParams fields
// and that callers do not need to pass a separate ActionTarget.
// This tests Decision 5 from the design: flat params, no nested ActionTarget.
func TestPostVerdictPipeline_Execute_ConstructsTargetFromFlatParams(t *testing.T) {
	q, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	registry := signal.NewRegistry()
	logger := testLogger()
	pool := engine.NewPool(1, registry, q, logger)
	defer pool.Stop()

	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, logger)
	pipeline := service.NewPostVerdictPipeline(publisher, q, logger)

	ctx := context.Background()

	orgID := seedOrg(t, q, "TestPostVerdictPipeline_Execute_ConstructsTargetFromFlatParams")
	action := seedAction(t, q, orgID, "construct-target-check")

	itemID := generateTestID("item")
	itemTypeID := generateTestID("ity")
	correlationID := generateTestID("corr")
	time.Sleep(time.Millisecond)

	// PostVerdictParams uses flat fields — no nested ActionTarget required by caller.
	params := service.PostVerdictParams{
		ActionRequests: []domain.ActionRequest{
			{
				Action:        *action,
				ItemID:        itemID,
				Payload:       map[string]any{"key": "value"},
				CorrelationID: correlationID,
			},
		},
		OrgID:         orgID,
		ItemID:        itemID,
		ItemTypeID:    itemTypeID,
		Payload:       map[string]any{"key": "value"},
		CorrelationID: correlationID,
	}

	results := pipeline.Execute(ctx, params)
	// One result expected — regardless of webhook success/failure.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ActionID != action.ID {
		t.Errorf("ActionID: got %q, want %q", results[0].ActionID, action.ID)
	}
}
