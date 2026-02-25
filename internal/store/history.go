package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

const (
	sqlInsertEntityHistory = `INSERT INTO entity_history (id, entity_type, org_id, version, snapshot, valid_from, valid_to)
		VALUES ($1, $2, $3, $4, $5, now(), '9999-12-31T23:59:59Z')`

	// Close the previous version's valid_to window when a new version is inserted.
	sqlCloseEntityHistoryVersion = `UPDATE entity_history SET valid_to = now()
		WHERE entity_type = $1 AND id = $2 AND version = $3 AND org_id = $4`

	sqlGetEntityHistory = `SELECT id, entity_type, org_id, version, snapshot, valid_from, valid_to
		FROM entity_history WHERE entity_type = $1 AND id = $2 AND org_id = $3
		ORDER BY version ASC`
)

// InsertEntityHistory appends a versioned snapshot to entity_history.
// This is append-only: only closes the previous version's valid_to window.
//
// Pre-conditions: entityType is one of "rule", "action", "policy". version >= 1.
// Post-conditions: new history entry is persisted with valid_to='9999-12-31'.
//
//	If version > 1, the previous version's valid_to is closed to now().
//
// Raises: domain.ConflictError if (entity_type, id, version) already exists.
func (q *Queries) InsertEntityHistory(ctx context.Context, entityType, id, orgID string, version int, snapshot any) error {
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	_, err = q.dbtx.Exec(ctx, sqlInsertEntityHistory,
		id,
		entityType,
		orgID,
		version,
		snapshotBytes,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("%s %s version %d already exists", entityType, id, version))
	}

	// Close the previous version's window if this is not the first version.
	if version > 1 {
		if _, err := q.dbtx.Exec(ctx, sqlCloseEntityHistoryVersion, entityType, id, version-1, orgID); err != nil {
			return fmt.Errorf("close previous entity history version: %w", err)
		}
	}
	return nil
}

// GetEntityHistory returns all history entries for an entity, ordered by version ASC.
//
// Pre-conditions: entityType and id must be non-empty.
// Post-conditions: returns history entries ordered by version ASC (empty slice if none).
// Raises: error on database failure.
func (q *Queries) GetEntityHistory(ctx context.Context, entityType, id, orgID string) ([]domain.EntityHistoryEntry, error) {
	rows, err := q.dbtx.Query(ctx, sqlGetEntityHistory, entityType, id, orgID)
	if err != nil {
		return nil, fmt.Errorf("get entity history: %w", err)
	}
	defer rows.Close()

	var entries []domain.EntityHistoryEntry
	for rows.Next() {
		var e domain.EntityHistoryEntry
		var snapshotRaw []byte
		if err := rows.Scan(
			&e.ID,
			&e.EntityType,
			&e.OrgID,
			&e.Version,
			&snapshotRaw,
			&e.ValidFrom,
			&e.ValidTo,
		); err != nil {
			return nil, fmt.Errorf("scan entity history: %w", err)
		}
		if err := json.Unmarshal(snapshotRaw, &e.Snapshot); err != nil {
			return nil, fmt.Errorf("unmarshal snapshot: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entity history: %w", err)
	}
	if entries == nil {
		entries = []domain.EntityHistoryEntry{}
	}
	return entries, nil
}
