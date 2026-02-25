package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vinaysrao1/nest/internal/domain"
)

// ruleExecutionColumns lists the columns for the rule_executions COPY operation.
// Order must match the values returned by the CopyFromSlice callback.
var ruleExecutionColumns = []string{
	"id", "org_id", "rule_id", "rule_version", "item_id", "item_type_id",
	"verdict", "reason", "triggered_rules", "latency_us", "correlation_id", "executed_at",
}

// actionExecutionColumns lists the columns for the action_executions COPY operation.
var actionExecutionColumns = []string{
	"id", "org_id", "action_id", "item_id", "item_type_id",
	"success", "correlation_id", "executed_at",
}

// LogRuleExecutions batch-inserts rule execution log entries using the COPY protocol.
// Writes to the partitioned rule_executions table (routed by executed_at).
//
// Pre-conditions: executions may be empty — the function is a no-op in that case.
// Post-conditions: all entries are persisted atomically via COPY.
// Raises: error if COPY fails or row count does not match.
func (q *Queries) LogRuleExecutions(ctx context.Context, executions []domain.RuleExecution) error {
	if len(executions) == 0 {
		return nil
	}

	rowSrc := pgx.CopyFromSlice(len(executions), func(i int) ([]any, error) {
		e := executions[i]
		triggeredJSON, err := marshalJSONB(e.TriggeredRules)
		if err != nil {
			return nil, fmt.Errorf("marshal triggered_rules for execution %s: %w", e.ID, err)
		}
		return []any{
			e.ID,
			e.OrgID,
			e.RuleID,
			e.RuleVersion,
			e.ItemID,
			e.ItemTypeID,
			e.Verdict,
			e.Reason,
			triggeredJSON,
			e.LatencyUs,
			e.CorrelationID,
			e.ExecutedAt,
		}, nil
	})

	n, err := q.dbtx.CopyFrom(ctx, pgx.Identifier{"rule_executions"}, ruleExecutionColumns, rowSrc)
	if err != nil {
		return fmt.Errorf("copy rule executions: %w", err)
	}
	if int(n) != len(executions) {
		return fmt.Errorf("copy rule executions: expected %d rows inserted, got %d", len(executions), n)
	}
	return nil
}

// LogActionExecutions batch-inserts action execution log entries using the COPY protocol.
// Writes to the partitioned action_executions table (routed by executed_at).
//
// Pre-conditions: executions may be empty — the function is a no-op in that case.
// Post-conditions: all entries are persisted atomically via COPY.
// Raises: error if COPY fails or row count does not match.
func (q *Queries) LogActionExecutions(ctx context.Context, executions []domain.ActionExecution) error {
	if len(executions) == 0 {
		return nil
	}

	rowSrc := pgx.CopyFromSlice(len(executions), func(i int) ([]any, error) {
		e := executions[i]
		return []any{
			e.ID,
			e.OrgID,
			e.ActionID,
			e.ItemID,
			e.ItemTypeID,
			e.Success,
			e.CorrelationID,
			e.ExecutedAt,
		}, nil
	})

	n, err := q.dbtx.CopyFrom(ctx, pgx.Identifier{"action_executions"}, actionExecutionColumns, rowSrc)
	if err != nil {
		return fmt.Errorf("copy action executions: %w", err)
	}
	if int(n) != len(executions) {
		return fmt.Errorf("copy action executions: expected %d rows inserted, got %d", len(executions), n)
	}
	return nil
}

// marshalJSONB marshals a map to JSON bytes for use in COPY protocol JSONB columns.
// Returns an empty JSON object if the map is nil or empty.
func marshalJSONB(v map[string]any) ([]byte, error) {
	if len(v) == 0 {
		return []byte(`{}`), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
