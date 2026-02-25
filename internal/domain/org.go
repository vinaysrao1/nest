package domain

import "time"

// Org represents a tenant organization.
type Org struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Settings  map[string]any `json:"settings"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
