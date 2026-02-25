package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

// ListSigningKeys returns all signing keys for an org, ordered by created_at DESC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all signing keys for the org.
// Raises: error on database failure.
func (q *Queries) ListSigningKeys(ctx context.Context, orgID string) ([]domain.SigningKey, error) {
	const sql = `
		SELECT id, org_id, public_key, private_key, is_active, created_at
		FROM signing_keys
		WHERE org_id = $1
		ORDER BY created_at DESC`

	rows, err := q.dbtx.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("list signing keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.SigningKey, 0)
	for rows.Next() {
		var k domain.SigningKey
		if err := rows.Scan(
			&k.ID,
			&k.OrgID,
			&k.PublicKey,
			&k.PrivateKey,
			&k.IsActive,
			&k.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan signing key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error listing signing keys: %w", err)
	}
	return keys, nil
}

// GetActiveSigningKey returns the currently active signing key for an org.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns the single active key for the org.
// Raises: domain.NotFoundError if no active key exists.
func (q *Queries) GetActiveSigningKey(ctx context.Context, orgID string) (*domain.SigningKey, error) {
	const sql = `
		SELECT id, org_id, public_key, private_key, is_active, created_at
		FROM signing_keys
		WHERE org_id = $1 AND is_active = true
		LIMIT 1`

	var k domain.SigningKey
	err := q.dbtx.QueryRow(ctx, sql, orgID).Scan(
		&k.ID,
		&k.OrgID,
		&k.PublicKey,
		&k.PrivateKey,
		&k.IsActive,
		&k.CreatedAt,
	)
	if err != nil {
		return nil, notFound(err, "active signing key for org", orgID)
	}
	return &k, nil
}

// CreateSigningKey inserts a new signing key.
//
// Pre-conditions: key.ID, key.OrgID must be set.
// Post-conditions: signing key is persisted.
// Raises: error on database failure.
func (q *Queries) CreateSigningKey(ctx context.Context, key domain.SigningKey) error {
	const sql = `
		INSERT INTO signing_keys (id, org_id, public_key, private_key, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := q.dbtx.Exec(ctx, sql,
		key.ID,
		key.OrgID,
		key.PublicKey,
		key.PrivateKey,
		key.IsActive,
		key.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create signing key: %w", err)
	}
	return nil
}

// DeactivateSigningKeys sets is_active=false on all signing keys for an org.
// Used before creating a new active key in the key rotation flow.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: all signing keys for the org have is_active=false.
// Raises: error on database failure.
func (q *Queries) DeactivateSigningKeys(ctx context.Context, orgID string) error {
	const sql = `UPDATE signing_keys SET is_active = false WHERE org_id = $1`

	_, err := q.dbtx.Exec(ctx, sql, orgID)
	if err != nil {
		return fmt.Errorf("deactivate signing keys: %w", err)
	}
	return nil
}
