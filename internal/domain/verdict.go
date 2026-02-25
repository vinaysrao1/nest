package domain

// VerdictType represents the outcome of rule evaluation.
type VerdictType string

const (
	VerdictApprove VerdictType = "approve"
	VerdictBlock   VerdictType = "block"
	VerdictReview  VerdictType = "review"
)

// Verdict is the result of a single rule evaluation.
type Verdict struct {
	Type    VerdictType `json:"type"`
	Reason  string      `json:"reason,omitempty"`
	RuleID  string      `json:"rule_id"`
	Actions []string    `json:"actions,omitempty"`
}
