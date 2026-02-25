package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

const insertItemSQL = `
INSERT INTO items (id, org_id, item_type_id, data, submission_id, creator_id, creator_type_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

// InsertItem inserts an item into the items ledger.
// The items table has a composite PK: (org_id, id, item_type_id, submission_id).
//
// Pre-conditions: orgID must be non-empty. item.ID, item.ItemTypeID, and item.SubmissionID must be set.
// Post-conditions: item is persisted in the items ledger.
// Raises: domain.ConflictError if the exact (org_id, id, item_type_id, submission_id) combination already exists.
func (q *Queries) InsertItem(ctx context.Context, orgID string, item domain.Item) error {
	_, err := q.dbtx.Exec(ctx, insertItemSQL,
		item.ID,
		orgID,
		item.ItemTypeID,
		item.Data,
		item.SubmissionID,
		item.CreatorID,
		item.CreatorTypeID,
		item.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return &domain.ConflictError{
				Message: fmt.Sprintf("item %s already exists for org %s", item.ID, orgID),
			}
		}
		return fmt.Errorf("insert item: %w", err)
	}
	return nil
}
