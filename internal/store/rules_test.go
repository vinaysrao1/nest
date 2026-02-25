package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestRules_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rules-crud-org")

	t.Run("create and get rule", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		rule := &domain.Rule{
			ID:         "rule-001",
			OrgID:      orgID,
			Name:       "Test Rule",
			Status:     domain.RuleStatusLive,
			Source:     `rule_id = "test"` + "\n" + `event_types = ["content"]` + "\n" + `priority = 50` + "\n\ndef evaluate(event):\n    return verdict(\"approve\")",
			EventTypes: []string{"content"},
			Priority:   50,
			Tags:       []string{"test", "automated"},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := q.CreateRule(ctx, rule); err != nil {
			t.Fatalf("CreateRule: %v", err)
		}

		got, err := q.GetRule(ctx, orgID, rule.ID)
		if err != nil {
			t.Fatalf("GetRule: %v", err)
		}

		if got.ID != rule.ID {
			t.Errorf("ID: got %q, want %q", got.ID, rule.ID)
		}
		if got.Name != rule.Name {
			t.Errorf("Name: got %q, want %q", got.Name, rule.Name)
		}
		if got.Status != rule.Status {
			t.Errorf("Status: got %q, want %q", got.Status, rule.Status)
		}
		if got.Source != rule.Source {
			t.Errorf("Source: got %q, want %q", got.Source, rule.Source)
		}
		if got.Priority != rule.Priority {
			t.Errorf("Priority: got %d, want %d", got.Priority, rule.Priority)
		}
		if got.Version != rule.Version {
			t.Errorf("Version: got %d, want %d", got.Version, rule.Version)
		}
		// TEXT[] round-trip.
		if len(got.EventTypes) != 1 || got.EventTypes[0] != "content" {
			t.Errorf("EventTypes: got %v, want [content]", got.EventTypes)
		}
		if len(got.Tags) != 2 {
			t.Errorf("Tags: got %v, want [test automated]", got.Tags)
		}
	})

	t.Run("update rule", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		rule := &domain.Rule{
			ID:         "rule-to-update",
			OrgID:      orgID,
			Name:       "Original Name",
			Status:     domain.RuleStatusDisabled,
			Source:     "source",
			EventTypes: []string{"*"},
			Priority:   0,
			Tags:       []string{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateRule(ctx, rule); err != nil {
			t.Fatalf("CreateRule: %v", err)
		}

		rule.Name = "Updated Name"
		rule.Status = domain.RuleStatusLive
		rule.Priority = 100
		rule.Version = 2

		if err := q.UpdateRule(ctx, rule); err != nil {
			t.Fatalf("UpdateRule: %v", err)
		}

		got, err := q.GetRule(ctx, orgID, rule.ID)
		if err != nil {
			t.Fatalf("GetRule after update: %v", err)
		}

		if got.Name != "Updated Name" {
			t.Errorf("Name: got %q, want %q", got.Name, "Updated Name")
		}
		if got.Status != domain.RuleStatusLive {
			t.Errorf("Status: got %q, want %q", got.Status, domain.RuleStatusLive)
		}
		if got.Priority != 100 {
			t.Errorf("Priority: got %d, want 100", got.Priority)
		}
		if got.Version != 2 {
			t.Errorf("Version: got %d, want 2", got.Version)
		}
	})

	t.Run("delete rule", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		rule := &domain.Rule{
			ID:         "rule-to-delete",
			OrgID:      orgID,
			Name:       "Delete Me",
			Status:     domain.RuleStatusDisabled,
			Source:     "source",
			EventTypes: []string{},
			Priority:   0,
			Tags:       []string{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateRule(ctx, rule); err != nil {
			t.Fatalf("CreateRule: %v", err)
		}

		if err := q.DeleteRule(ctx, orgID, rule.ID); err != nil {
			t.Fatalf("DeleteRule: %v", err)
		}

		_, err := q.GetRule(ctx, orgID, rule.ID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		if _, ok := err.(*domain.NotFoundError); !ok {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("list rules with pagination", func(t *testing.T) {
		orgID2 := seedOrg(t, q, "rules-list-org")
		now := time.Now().UTC().Truncate(time.Microsecond)
		for i := 0; i < 5; i++ {
			r := &domain.Rule{
				ID:         generateTestID(),
				OrgID:      orgID2,
				Name:       fmt.Sprintf("rule-%d", i),
				Status:     domain.RuleStatusDisabled,
				Source:     "source",
				EventTypes: []string{},
				Priority:   0,
				Tags:       []string{},
				Version:    1,
				CreatedAt:  now.Add(time.Duration(i) * time.Second),
				UpdatedAt:  now.Add(time.Duration(i) * time.Second),
			}
			if err := q.CreateRule(ctx, r); err != nil {
				t.Fatalf("CreateRule(%d): %v", i, err)
			}
		}

		result, err := q.ListRules(ctx, orgID2, domain.PageParams{Page: 1, PageSize: 3})
		if err != nil {
			t.Fatalf("ListRules: %v", err)
		}
		if result.Total != 5 {
			t.Errorf("Total: got %d, want 5", result.Total)
		}
		if len(result.Items) != 3 {
			t.Errorf("Items len: got %d, want 3", len(result.Items))
		}
		if result.TotalPages != 2 {
			t.Errorf("TotalPages: got %d, want 2", result.TotalPages)
		}
	})

	t.Run("list enabled rules returns only LIVE and BACKGROUND", func(t *testing.T) {
		orgID3 := seedOrg(t, q, "rules-enabled-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		statuses := []domain.RuleStatus{
			domain.RuleStatusLive,
			domain.RuleStatusBackground,
			domain.RuleStatusDisabled,
		}
		for i, status := range statuses {
			r := &domain.Rule{
				ID:         generateTestID(),
				OrgID:      orgID3,
				Name:       fmt.Sprintf("enabled-rule-%d", i),
				Status:     status,
				Source:     "source",
				EventTypes: []string{},
				Priority:   i * 10,
				Tags:       []string{},
				Version:    1,
				CreatedAt:  now.Add(time.Duration(i) * time.Second),
				UpdatedAt:  now.Add(time.Duration(i) * time.Second),
			}
			if err := q.CreateRule(ctx, r); err != nil {
				t.Fatalf("CreateRule(%s): %v", status, err)
			}
		}

		enabled, err := q.ListEnabledRules(ctx, orgID3)
		if err != nil {
			t.Fatalf("ListEnabledRules: %v", err)
		}
		if len(enabled) != 2 {
			t.Errorf("enabled rules count: got %d, want 2 (LIVE + BACKGROUND)", len(enabled))
		}
		for _, r := range enabled {
			if r.Status == domain.RuleStatusDisabled {
				t.Errorf("DISABLED rule %s returned by ListEnabledRules", r.ID)
			}
		}
	})

	t.Run("list enabled rules ordered by priority DESC", func(t *testing.T) {
		orgID4 := seedOrg(t, q, "rules-priority-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		priorities := []int{10, 100, 50}
		for i, pri := range priorities {
			r := &domain.Rule{
				ID:         generateTestID(),
				OrgID:      orgID4,
				Name:       fmt.Sprintf("priority-rule-%d", pri),
				Status:     domain.RuleStatusLive,
				Source:     "source",
				EventTypes: []string{},
				Priority:   pri,
				Tags:       []string{},
				Version:    1,
				CreatedAt:  now.Add(time.Duration(i) * time.Second),
				UpdatedAt:  now.Add(time.Duration(i) * time.Second),
			}
			if err := q.CreateRule(ctx, r); err != nil {
				t.Fatalf("CreateRule(priority=%d): %v", pri, err)
			}
		}

		enabled, err := q.ListEnabledRules(ctx, orgID4)
		if err != nil {
			t.Fatalf("ListEnabledRules: %v", err)
		}
		if len(enabled) != 3 {
			t.Fatalf("expected 3 enabled rules, got %d", len(enabled))
		}
		if enabled[0].Priority != 100 {
			t.Errorf("first rule priority: got %d, want 100", enabled[0].Priority)
		}
		if enabled[1].Priority != 50 {
			t.Errorf("second rule priority: got %d, want 50", enabled[1].Priority)
		}
		if enabled[2].Priority != 10 {
			t.Errorf("third rule priority: got %d, want 10", enabled[2].Priority)
		}
	})
}

func TestRules_Policies(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rules-policies-org")

	now := time.Now().UTC().Truncate(time.Microsecond)
	rule := &domain.Rule{
		ID:         "rp-rule-001",
		OrgID:      orgID,
		Name:       "Policy Rule",
		Status:     domain.RuleStatusLive,
		Source:     "source",
		EventTypes: []string{},
		Priority:   0,
		Tags:       []string{},
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := q.CreateRule(ctx, rule); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	pol1 := &domain.Policy{
		ID:            "rp-policy-001",
		OrgID:         orgID,
		Name:          "Policy One",
		StrikePenalty: 1,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	pol2 := &domain.Policy{
		ID:            "rp-policy-002",
		OrgID:         orgID,
		Name:          "Policy Two",
		StrikePenalty: 2,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for _, p := range []*domain.Policy{pol1, pol2} {
		if err := q.CreatePolicy(ctx, p); err != nil {
			t.Fatalf("CreatePolicy(%s): %v", p.ID, err)
		}
	}

	t.Run("set and get rule policies", func(t *testing.T) {
		if err := q.SetRulePolicies(ctx, rule.ID, []string{pol1.ID, pol2.ID}); err != nil {
			t.Fatalf("SetRulePolicies: %v", err)
		}

		pids, err := q.GetRulePolicies(ctx, rule.ID)
		if err != nil {
			t.Fatalf("GetRulePolicies: %v", err)
		}
		if len(pids) != 2 {
			t.Errorf("policy count: got %d, want 2", len(pids))
		}
	})

	t.Run("set empty slice clears policies", func(t *testing.T) {
		if err := q.SetRulePolicies(ctx, rule.ID, []string{}); err != nil {
			t.Fatalf("SetRulePolicies empty: %v", err)
		}

		pids, err := q.GetRulePolicies(ctx, rule.ID)
		if err != nil {
			t.Fatalf("GetRulePolicies after clear: %v", err)
		}
		if len(pids) != 0 {
			t.Errorf("expected 0 policies after clear, got %d", len(pids))
		}
	})

	t.Run("set replaces existing policies", func(t *testing.T) {
		if err := q.SetRulePolicies(ctx, rule.ID, []string{pol1.ID, pol2.ID}); err != nil {
			t.Fatalf("SetRulePolicies(2): %v", err)
		}
		if err := q.SetRulePolicies(ctx, rule.ID, []string{pol1.ID}); err != nil {
			t.Fatalf("SetRulePolicies(1): %v", err)
		}

		pids, err := q.GetRulePolicies(ctx, rule.ID)
		if err != nil {
			t.Fatalf("GetRulePolicies after replace: %v", err)
		}
		if len(pids) != 1 {
			t.Errorf("expected 1 policy after replace, got %d", len(pids))
		}
		if pids[0] != pol1.ID {
			t.Errorf("policy ID: got %q, want %q", pids[0], pol1.ID)
		}
	})

	t.Run("get rule policies for rule with no policies returns empty slice", func(t *testing.T) {
		pids, err := q.GetRulePolicies(ctx, "nonexistent-rule-id")
		if err != nil {
			t.Fatalf("GetRulePolicies(nonexistent): %v", err)
		}
		if pids == nil {
			t.Error("GetRulePolicies: got nil slice, want empty slice")
		}
	})
}

func TestRules_NotFound(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rules-notfound-org")

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "GetRule non-existent ID",
			fn: func() error {
				_, err := q.GetRule(ctx, orgID, "no-such-rule")
				return err
			},
		},
		{
			name: "UpdateRule non-existent ID",
			fn: func() error {
				return q.UpdateRule(ctx, &domain.Rule{
					ID:    "no-such-rule",
					OrgID: orgID,
				})
			},
		},
		{
			name: "DeleteRule non-existent ID",
			fn: func() error {
				return q.DeleteRule(ctx, orgID, "no-such-rule")
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatal("expected NotFoundError, got nil")
			}
			if _, ok := err.(*domain.NotFoundError); !ok {
				t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
			}
		})
	}
}
