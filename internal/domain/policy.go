package domain

import "time"

// Policy represents a content moderation policy. Policies can form a hierarchy
// via ParentID and carry a strike penalty weight.
type Policy struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	ParentID      *string   `json:"parent_id,omitempty"`
	StrikePenalty int       `json:"strike_penalty"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
