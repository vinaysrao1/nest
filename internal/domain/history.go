package domain

import "time"

// EntityHistoryEntry is a versioned snapshot of any entity (rule, action, policy).
// Stored in the generic entity_history table.
type EntityHistoryEntry struct {
	ID         string         `json:"id"`
	EntityType string         `json:"entity_type"`
	OrgID      string         `json:"org_id"`
	Version    int            `json:"version"`
	Snapshot   map[string]any `json:"snapshot"`
	ValidFrom  time.Time      `json:"valid_from"`
	ValidTo    time.Time      `json:"valid_to"`
}
