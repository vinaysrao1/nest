package store_test

import (
	"context"
	"testing"
)

func TestCounters(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "counters-org")

	t.Run("get non-existent counter returns 0 not error", func(t *testing.T) {
		sum, err := q.GetCounterSum(ctx, orgID, "entity-nonexistent", "event_view", 3600)
		if err != nil {
			t.Fatalf("GetCounterSum (no row): %v", err)
		}
		if sum != 0 {
			t.Errorf("expected 0 for non-existent counter, got %d", sum)
		}
	})

	t.Run("increment and get counter", func(t *testing.T) {
		const (
			entityID  = "entity-001"
			eventType = "click"
			window    = 3600
		)

		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, window, 5); err != nil {
			t.Fatalf("IncrementCounter: %v", err)
		}

		sum, err := q.GetCounterSum(ctx, orgID, entityID, eventType, window)
		if err != nil {
			t.Fatalf("GetCounterSum: %v", err)
		}
		if sum != 5 {
			t.Errorf("GetCounterSum: got %d, want 5", sum)
		}
	})

	t.Run("increment twice accumulates correctly", func(t *testing.T) {
		const (
			entityID  = "entity-002"
			eventType = "view"
			window    = 86400
		)

		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, window, 3); err != nil {
			t.Fatalf("first IncrementCounter: %v", err)
		}
		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, window, 7); err != nil {
			t.Fatalf("second IncrementCounter: %v", err)
		}

		sum, err := q.GetCounterSum(ctx, orgID, entityID, eventType, window)
		if err != nil {
			t.Fatalf("GetCounterSum after two increments: %v", err)
		}
		if sum != 10 {
			t.Errorf("GetCounterSum: got %d, want 10", sum)
		}
	})

	t.Run("different windows are independent keys", func(t *testing.T) {
		const (
			entityID  = "entity-003"
			eventType = "post"
		)

		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, 60, 2); err != nil {
			t.Fatalf("IncrementCounter window=60: %v", err)
		}
		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, 3600, 8); err != nil {
			t.Fatalf("IncrementCounter window=3600: %v", err)
		}

		sum60, err := q.GetCounterSum(ctx, orgID, entityID, eventType, 60)
		if err != nil {
			t.Fatalf("GetCounterSum window=60: %v", err)
		}
		if sum60 != 2 {
			t.Errorf("window=60: got %d, want 2", sum60)
		}

		sum3600, err := q.GetCounterSum(ctx, orgID, entityID, eventType, 3600)
		if err != nil {
			t.Fatalf("GetCounterSum window=3600: %v", err)
		}
		if sum3600 != 8 {
			t.Errorf("window=3600: got %d, want 8", sum3600)
		}
	})

	t.Run("different orgs are isolated", func(t *testing.T) {
		org2ID := seedOrg(t, q, "counters-org2")
		const (
			entityID  = "shared-entity"
			eventType = "action"
			window    = 300
		)

		if err := q.IncrementCounter(ctx, orgID, entityID, eventType, window, 100); err != nil {
			t.Fatalf("IncrementCounter org1: %v", err)
		}

		// org2 should see 0 for the same key.
		sum, err := q.GetCounterSum(ctx, org2ID, entityID, eventType, window)
		if err != nil {
			t.Fatalf("GetCounterSum org2: %v", err)
		}
		if sum != 0 {
			t.Errorf("org2 counter should be 0, got %d", sum)
		}
	})
}
