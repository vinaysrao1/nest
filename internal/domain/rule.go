package domain

import "time"

// RuleStatus represents the operational state of a rule.
type RuleStatus string

const (
	RuleStatusLive       RuleStatus = "LIVE"
	RuleStatusBackground RuleStatus = "BACKGROUND"
	RuleStatusDisabled   RuleStatus = "DISABLED"
)

// Rule is the database representation of a Starlark rule.
// The Source field (Starlark code) is the single source of truth.
// EventTypes and Priority are DERIVED values extracted at compile time.
type Rule struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	Name       string     `json:"name"`
	Status     RuleStatus `json:"status"`
	Source     string     `json:"source"`
	EventTypes []string   `json:"event_types"`
	Priority   int        `json:"priority"`
	Tags       []string   `json:"tags"`
	Version    int        `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
