package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestOrgs_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("create and get org", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		org := &domain.Org{
			ID:        "org-crud-001",
			Name:      "Test Organization",
			Settings:  map[string]any{"plan": "pro", "max_rules": float64(100)},
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := q.CreateOrg(ctx, org); err != nil {
			t.Fatalf("CreateOrg: %v", err)
		}

		got, err := q.GetOrg(ctx, org.ID)
		if err != nil {
			t.Fatalf("GetOrg: %v", err)
		}

		if got.ID != org.ID {
			t.Errorf("ID: got %q, want %q", got.ID, org.ID)
		}
		if got.Name != org.Name {
			t.Errorf("Name: got %q, want %q", got.Name, org.Name)
		}
		// JSONB settings round-trip
		if got.Settings["plan"] != "pro" {
			t.Errorf("Settings[plan]: got %v, want %q", got.Settings["plan"], "pro")
		}
		if got.Settings["max_rules"] != float64(100) {
			t.Errorf("Settings[max_rules]: got %v, want 100", got.Settings["max_rules"])
		}
	})

	t.Run("get non-existent org returns NotFoundError", func(t *testing.T) {
		_, err := q.GetOrg(ctx, "nonexistent-org-id")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		if _, ok := err.(*domain.NotFoundError); !ok {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("org settings nil default to empty map", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		org := &domain.Org{
			ID:        "org-nil-settings",
			Name:      "Org With Nil Settings",
			Settings:  nil, // explicitly nil
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := q.CreateOrg(ctx, org); err != nil {
			t.Fatalf("CreateOrg: %v", err)
		}

		got, err := q.GetOrg(ctx, org.ID)
		if err != nil {
			t.Fatalf("GetOrg: %v", err)
		}

		if got.Settings == nil {
			t.Error("Settings must not be nil even if created with nil")
		}
	})
}
