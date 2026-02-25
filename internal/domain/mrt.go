package domain

import "time"

// MRTJobStatus represents the lifecycle state of a manual review job.
type MRTJobStatus string

const (
	MRTJobStatusPending  MRTJobStatus = "PENDING"
	MRTJobStatusAssigned MRTJobStatus = "ASSIGNED"
	MRTJobStatusDecided  MRTJobStatus = "DECIDED"
)

// MRT decision verdict constants.
const (
	MRTDecisionApprove = "APPROVE"
	MRTDecisionBlock   = "BLOCK"
	MRTDecisionSkip    = "SKIP"
	MRTDecisionRoute   = "ROUTE"
)

// MRTQueue is a named queue for manual review jobs.
type MRTQueue struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	IsDefault   bool       `json:"is_default"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// MRTJob is a manual review job assigned to a queue.
type MRTJob struct {
	ID            string         `json:"id"`
	OrgID         string         `json:"org_id"`
	QueueID       string         `json:"queue_id"`
	ItemID        string         `json:"item_id"`
	ItemTypeID    string         `json:"item_type_id"`
	Payload       map[string]any `json:"payload"`
	Status        MRTJobStatus   `json:"status"`
	AssignedTo    *string        `json:"assigned_to,omitempty"`
	PolicyIDs     []string       `json:"policy_ids"`
	EnqueueSource string         `json:"enqueue_source"`
	SourceInfo    map[string]any `json:"source_info"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// MRTDecision is the outcome of a manual review.
type MRTDecision struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	JobID         string    `json:"job_id"`
	UserID        string    `json:"user_id"`
	Verdict       string    `json:"verdict"`
	ActionIDs     []string  `json:"action_ids"`
	PolicyIDs     []string  `json:"policy_ids"`
	Reason        string    `json:"reason,omitempty"`
	TargetQueueID *string   `json:"target_queue_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
