package domain

import "time"

// RuleExecution is a log entry for a single rule evaluation.
// Stored in the partitioned rule_executions table.
type RuleExecution struct {
	ID             string         `json:"id"`
	OrgID          string         `json:"org_id"`
	RuleID         string         `json:"rule_id"`
	RuleVersion    int            `json:"rule_version"`
	ItemID         string         `json:"item_id"`
	ItemTypeID     string         `json:"item_type_id"`
	Verdict        string         `json:"verdict,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	TriggeredRules map[string]any `json:"triggered_rules,omitempty"`
	LatencyUs      int64          `json:"latency_us,omitempty"`
	CorrelationID  string         `json:"correlation_id"`
	ExecutedAt     time.Time      `json:"executed_at"`
}

// ActionExecution is a log entry for a single action execution.
// Stored in the partitioned action_executions table.
type ActionExecution struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	ActionID      string    `json:"action_id"`
	ItemID        string    `json:"item_id"`
	ItemTypeID    string    `json:"item_type_id"`
	Success       bool      `json:"success"`
	CorrelationID string    `json:"correlation_id"`
	ExecutedAt    time.Time `json:"executed_at"`
}
