package domain

// EvalResultResponse is the per-item response shape returned by POST /api/v1/items.
type EvalResultResponse struct {
	ItemID         string          `json:"item_id"`
	Verdict        VerdictType     `json:"verdict"`
	TriggeredRules []TriggeredRule `json:"triggered_rules"`
	Actions        []ActionResult  `json:"actions"`
}

// TriggeredRule is a per-rule evaluation result included in EvalResultResponse.
type TriggeredRule struct {
	RuleID    string      `json:"rule_id"`
	Version   int         `json:"version"`
	Verdict   VerdictType `json:"verdict"`
	Reason    string      `json:"reason,omitempty"`
	LatencyUs int64       `json:"latency_us"`
}
