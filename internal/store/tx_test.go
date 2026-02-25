package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

func TestWithTx_Commit(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := generateTestID()

	err := q.WithTx(ctx, func(tx *store.Queries) error {
		org := &domain.Org{
			ID:        orgID,
			Name:      "Transactional Org",
			Settings:  map[string]any{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		return tx.CreateOrg(ctx, org)
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}

	// Org must be visible outside the transaction.
	got, err := q.GetOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrg after commit: %v", err)
	}
	if got.ID != orgID {
		t.Errorf("ID: got %q, want %q", got.ID, orgID)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := generateTestID()
	sentinelErr := errors.New("intentional rollback error")

	err := q.WithTx(ctx, func(tx *store.Queries) error {
		org := &domain.Org{
			ID:        orgID,
			Name:      "Rollback Org",
			Settings:  map[string]any{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := tx.CreateOrg(ctx, org); err != nil {
			return err
		}
		// Return error to trigger rollback.
		return sentinelErr
	})

	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got: %v", err)
	}

	// Org must NOT be visible outside the rolled-back transaction.
	_, err = q.GetOrg(ctx, orgID)
	if err == nil {
		t.Fatal("expected NotFoundError after rollback, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}
