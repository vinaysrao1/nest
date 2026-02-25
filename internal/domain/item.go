package domain

import "time"

// ItemTypeKind categorizes what kind of content an item type represents.
type ItemTypeKind string

const (
	ItemTypeKindContent ItemTypeKind = "CONTENT"
	ItemTypeKindUser    ItemTypeKind = "USER"
	ItemTypeKindThread  ItemTypeKind = "THREAD"
)

// ItemType defines the schema and field roles for a category of items.
type ItemType struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	Name       string         `json:"name"`
	Kind       ItemTypeKind   `json:"kind"`
	Schema     map[string]any `json:"schema"`
	FieldRoles map[string]any `json:"field_roles"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Item is a submitted content item stored in the items ledger.
type Item struct {
	ID            string         `json:"id"`
	OrgID         string         `json:"org_id"`
	ItemTypeID    string         `json:"item_type_id"`
	Data          map[string]any `json:"data"`
	SubmissionID  string         `json:"submission_id"`
	CreatorID     string         `json:"creator_id,omitempty"`
	CreatorTypeID string         `json:"creator_type_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
