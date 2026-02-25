package domain

import "time"

// Event is the input to the rule engine. It represents a content moderation
// event submitted via the API.
type Event struct {
	ID         string         `json:"event_id"`
	EventType  string         `json:"event_type"`
	ItemType   string         `json:"item_type"`
	OrgID      string         `json:"org_id"`
	Payload    map[string]any `json:"payload"`
	Timestamp  time.Time      `json:"timestamp"`
	ItemID     string         `json:"item_id"`
	ItemTypeID string         `json:"item_type_id"`
}
