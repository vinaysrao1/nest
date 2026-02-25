package domain

import "time"

// ActionType represents the kind of action to execute.
type ActionType string

const (
	ActionTypeWebhook      ActionType = "WEBHOOK"
	ActionTypeEnqueueToMRT ActionType = "ENQUEUE_TO_MRT"
)

// Action is a named action configuration referenced by Starlark verdict() calls.
type Action struct {
	ID         string         `json:"id"`
	OrgID      string         `json:"org_id"`
	Name       string         `json:"name"`
	ActionType ActionType     `json:"action_type"`
	Config     map[string]any `json:"config"`
	Version    int            `json:"version"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// ActionRequest is the input to action execution.
type ActionRequest struct {
	Action        Action
	ItemID        string
	Payload       map[string]any
	CorrelationID string
}

// ActionResult is the outcome of a single action execution.
type ActionResult struct {
	ActionID string `json:"action_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ActionTarget provides context about the item being acted upon.
type ActionTarget struct {
	ItemID        string         `json:"item_id"`
	ItemTypeID    string         `json:"item_type_id"`
	OrgID         string         `json:"org_id"`
	Payload       map[string]any `json:"payload"`
	CorrelationID string         `json:"correlation_id"`
}
