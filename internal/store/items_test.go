package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestItems_Insert(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "items-insert-org")
	it := seedItemType(t, q, orgID, "items-insert-type")

	t.Run("insert item succeeds", func(t *testing.T) {
		item := domain.Item{
			ID:            generateTestID(),
			OrgID:         orgID,
			ItemTypeID:    it.ID,
			Data:          map[string]any{"text": "hello world", "score": float64(42)},
			SubmissionID:  generateTestID(),
			CreatorID:     "creator-001",
			CreatorTypeID: "user",
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertItem(ctx, orgID, item); err != nil {
			t.Fatalf("InsertItem: %v", err)
		}
	})

	t.Run("duplicate composite PK returns ConflictError", func(t *testing.T) {
		submID := generateTestID()
		itemID := generateTestID()
		item := domain.Item{
			ID:           itemID,
			OrgID:        orgID,
			ItemTypeID:   it.ID,
			Data:         map[string]any{"key": "value"},
			SubmissionID: submID,
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertItem(ctx, orgID, item); err != nil {
			t.Fatalf("first InsertItem: %v", err)
		}

		// Inserting exact same composite PK (org_id, id, item_type_id, submission_id) must conflict.
		if err := q.InsertItem(ctx, orgID, item); err == nil {
			t.Fatal("expected ConflictError, got nil")
		} else {
			var ce *domain.ConflictError
			if !errors.As(err, &ce) {
				t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
			}
		}
	})

	t.Run("JSONB data with nested map and array", func(t *testing.T) {
		item := domain.Item{
			ID:           generateTestID(),
			OrgID:        orgID,
			ItemTypeID:   it.ID,
			Data:         map[string]any{"nested": map[string]any{"level": float64(2)}, "tags": []any{"a", "b"}},
			SubmissionID: generateTestID(),
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertItem(ctx, orgID, item); err != nil {
			t.Fatalf("InsertItem with nested JSONB: %v", err)
		}
	})

	t.Run("different submission_id allows re-submission of same item", func(t *testing.T) {
		itemID := generateTestID()
		baseItem := domain.Item{
			ID:           itemID,
			OrgID:        orgID,
			ItemTypeID:   it.ID,
			Data:         map[string]any{"v": float64(1)},
			SubmissionID: generateTestID(),
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertItem(ctx, orgID, baseItem); err != nil {
			t.Fatalf("first insert: %v", err)
		}

		// Same itemID + itemTypeID but different submissionID is valid per composite PK design.
		baseItem.SubmissionID = generateTestID()
		if err := q.InsertItem(ctx, orgID, baseItem); err != nil {
			t.Fatalf("second insert with different submission_id: %v", err)
		}
	})
}
