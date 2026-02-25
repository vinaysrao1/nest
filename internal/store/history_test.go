package store_test

import (
	"context"
	"testing"
	"time"
)

func TestHistory_InsertAndGet(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "history-org")

	t.Run("insert version 1 and 2 and get ordered by version", func(t *testing.T) {
		entityType := "rule"
		entityID := "hist-rule-001"

		snap1 := map[string]any{
			"id":      entityID,
			"name":    "Rule v1",
			"version": float64(1),
		}
		snap2 := map[string]any{
			"id":      entityID,
			"name":    "Rule v2",
			"version": float64(2),
		}

		if err := q.InsertEntityHistory(ctx, entityType, entityID, orgID, 1, snap1); err != nil {
			t.Fatalf("InsertEntityHistory(v1): %v", err)
		}
		if err := q.InsertEntityHistory(ctx, entityType, entityID, orgID, 2, snap2); err != nil {
			t.Fatalf("InsertEntityHistory(v2): %v", err)
		}

		entries, err := q.GetEntityHistory(ctx, entityType, entityID, orgID)
		if err != nil {
			t.Fatalf("GetEntityHistory: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 history entries, got %d", len(entries))
		}

		// Verify ordered by version ASC.
		if entries[0].Version != 1 {
			t.Errorf("entries[0].Version: got %d, want 1", entries[0].Version)
		}
		if entries[1].Version != 2 {
			t.Errorf("entries[1].Version: got %d, want 2", entries[1].Version)
		}

		// Verify JSONB snapshot round-trip.
		if entries[0].Snapshot["name"] != "Rule v1" {
			t.Errorf("entries[0].Snapshot[name]: got %v, want %q", entries[0].Snapshot["name"], "Rule v1")
		}
		if entries[1].Snapshot["name"] != "Rule v2" {
			t.Errorf("entries[1].Snapshot[name]: got %v, want %q", entries[1].Snapshot["name"], "Rule v2")
		}
	})

	t.Run("valid_to of version 1 is closed when version 2 is inserted", func(t *testing.T) {
		entityType := "action"
		entityID := "hist-action-001"
		snap := map[string]any{"id": entityID}

		if err := q.InsertEntityHistory(ctx, entityType, entityID, orgID, 1, snap); err != nil {
			t.Fatalf("InsertEntityHistory(v1): %v", err)
		}

		// Give a small gap so valid_to of v1 is definitely before valid_from of v2.
		time.Sleep(5 * time.Millisecond)

		if err := q.InsertEntityHistory(ctx, entityType, entityID, orgID, 2, snap); err != nil {
			t.Fatalf("InsertEntityHistory(v2): %v", err)
		}

		entries, err := q.GetEntityHistory(ctx, entityType, entityID, orgID)
		if err != nil {
			t.Fatalf("GetEntityHistory: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		// Version 1's valid_to should be "now" (closed), not far future.
		// The far-future sentinel is '9999-12-31'.
		farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		if !entries[0].ValidTo.Before(farFuture) {
			t.Errorf("v1 valid_to should be closed (before far future), got %v", entries[0].ValidTo)
		}
	})

	t.Run("get history for entity with no entries returns empty slice", func(t *testing.T) {
		entries, err := q.GetEntityHistory(ctx, "rule", "nonexistent-entity", orgID)
		if err != nil {
			t.Fatalf("GetEntityHistory: %v", err)
		}
		if entries == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("insert version 1 and verify fields", func(t *testing.T) {
		entityType := "policy"
		entityID := "hist-policy-001"
		snap := map[string]any{
			"id":    entityID,
			"org_id": orgID,
			"name":  "Test Policy",
		}

		if err := q.InsertEntityHistory(ctx, entityType, entityID, orgID, 1, snap); err != nil {
			t.Fatalf("InsertEntityHistory: %v", err)
		}

		entries, err := q.GetEntityHistory(ctx, entityType, entityID, orgID)
		if err != nil {
			t.Fatalf("GetEntityHistory: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}

		e := entries[0]
		if e.ID != entityID {
			t.Errorf("ID: got %q, want %q", e.ID, entityID)
		}
		if e.EntityType != entityType {
			t.Errorf("EntityType: got %q, want %q", e.EntityType, entityType)
		}
		if e.OrgID != orgID {
			t.Errorf("OrgID: got %q, want %q", e.OrgID, orgID)
		}
		if e.Version != 1 {
			t.Errorf("Version: got %d, want 1", e.Version)
		}
		if e.ValidFrom.IsZero() {
			t.Error("ValidFrom must not be zero")
		}
		if e.ValidTo.IsZero() {
			t.Error("ValidTo must not be zero")
		}
	})
}
