package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

const (
	sqlGetOrg    = `SELECT id, name, settings, created_at, updated_at FROM orgs WHERE id = $1`
	sqlCreateOrg = `INSERT INTO orgs (id, name, settings, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`
)

// GetOrg returns an org by ID.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns the org if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetOrg(ctx context.Context, orgID string) (*domain.Org, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetOrg, orgID)

	var org domain.Org
	err := row.Scan(
		&org.ID,
		&org.Name,
		&org.Settings,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return nil, notFound(err, "org", orgID)
	}
	if org.Settings == nil {
		org.Settings = map[string]any{}
	}
	return &org, nil
}

// CreateOrg inserts a new org.
//
// Pre-conditions: org.ID and org.Name must be set.
// Post-conditions: org is persisted.
// Raises: domain.ConflictError if ID already exists.
func (q *Queries) CreateOrg(ctx context.Context, org *domain.Org) error {
	settings := org.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	_, err := q.dbtx.Exec(ctx, sqlCreateOrg,
		org.ID,
		org.Name,
		settings,
		org.CreatedAt,
		org.UpdatedAt,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("org %s already exists", org.ID))
	}
	return nil
}
