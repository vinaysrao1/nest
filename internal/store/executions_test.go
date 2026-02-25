package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestLogRuleExecutions(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rule-exec-org")

	t.Run("empty slice is a no-op", func(t *testing.T) {
		if err := q.LogRuleExecutions(ctx, []domain.RuleExecution{}); err != nil {
			t.Fatalf("LogRuleExecutions (empty): %v", err)
		}
	})

	t.Run("nil slice is a no-op", func(t *testing.T) {
		if err := q.LogRuleExecutions(ctx, nil); err != nil {
			t.Fatalf("LogRuleExecutions (nil): %v", err)
		}
	})

	t.Run("batch insert 5 rule executions", func(t *testing.T) {
		executions := make([]domain.RuleExecution, 5)
		for i := range executions {
			executions[i] = domain.RuleExecution{
				ID:             generateTestID(),
				OrgID:          orgID,
				RuleID:         generateTestID(),
				RuleVersion:    1,
				ItemID:         generateTestID(),
				ItemTypeID:     generateTestID(),
				Verdict:        "flag",
				Reason:         "test reason",
				TriggeredRules: map[string]any{"rule-a": true},
				LatencyUs:      int64(100 + i),
				CorrelationID:  generateTestID(),
				// Use current month so the row routes to an existing partition.
				ExecutedAt: time.Now().UTC(),
			}
		}

		if err := q.LogRuleExecutions(ctx, executions); err != nil {
			t.Fatalf("LogRuleExecutions (5 entries): %v", err)
		}
	})

	t.Run("JSONB triggered_rules round-trip", func(t *testing.T) {
		corrID := generateTestID()
		exe := domain.RuleExecution{
			ID:             generateTestID(),
			OrgID:          orgID,
			RuleID:         generateTestID(),
			RuleVersion:    2,
			ItemID:         generateTestID(),
			ItemTypeID:     generateTestID(),
			Verdict:        "pass",
			Reason:         "all clear",
			TriggeredRules: map[string]any{"rule-x": "matched", "count": float64(3)},
			LatencyUs:      500,
			CorrelationID:  corrID,
			ExecutedAt:     time.Now().UTC(),
		}

		if err := q.LogRuleExecutions(ctx, []domain.RuleExecution{exe}); err != nil {
			t.Fatalf("LogRuleExecutions: %v", err)
		}

		// Read back via Pool to verify JSONB round-trip.
		var triggeredRules map[string]any
		row := q.Pool().QueryRow(ctx,
			`SELECT triggered_rules FROM rule_executions WHERE correlation_id = $1`,
			corrID)
		if err := row.Scan(&triggeredRules); err != nil {
			t.Fatalf("scan triggered_rules: %v", err)
		}
		if triggeredRules["rule-x"] != "matched" {
			t.Errorf("triggered_rules[rule-x]: got %v, want %q", triggeredRules["rule-x"], "matched")
		}
		if triggeredRules["count"] != float64(3) {
			t.Errorf("triggered_rules[count]: got %v, want 3", triggeredRules["count"])
		}
	})
}

func TestLogActionExecutions(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "action-exec-org")

	t.Run("empty slice is a no-op", func(t *testing.T) {
		if err := q.LogActionExecutions(ctx, []domain.ActionExecution{}); err != nil {
			t.Fatalf("LogActionExecutions (empty): %v", err)
		}
	})

	t.Run("nil slice is a no-op", func(t *testing.T) {
		if err := q.LogActionExecutions(ctx, nil); err != nil {
			t.Fatalf("LogActionExecutions (nil): %v", err)
		}
	})

	t.Run("batch insert 5 action executions", func(t *testing.T) {
		executions := make([]domain.ActionExecution, 5)
		for i := range executions {
			executions[i] = domain.ActionExecution{
				ID:            generateTestID(),
				OrgID:         orgID,
				ActionID:      generateTestID(),
				ItemID:        generateTestID(),
				ItemTypeID:    generateTestID(),
				Success:       i%2 == 0,
				CorrelationID: generateTestID(),
				ExecutedAt:    time.Now().UTC(),
			}
		}

		if err := q.LogActionExecutions(ctx, executions); err != nil {
			t.Fatalf("LogActionExecutions (5 entries): %v", err)
		}
	})
}
