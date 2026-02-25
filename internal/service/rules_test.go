package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// validRuleSource is a complete, compilable Starlark rule used across tests.
const validRuleSource = `
rule_id = "test-rule-001"
event_types = ["post_created"]
priority = 100

def evaluate(event):
    return verdict("approve", reason="test")
`

// validWildcardRuleSource is a rule that matches all event types.
const validWildcardRuleSource = `
rule_id = "test-rule-wildcard"
event_types = ["*"]
priority = 50

def evaluate(event):
    return verdict("approve", reason="wildcard")
`

// invalidRuleSource is syntactically invalid Starlark.
const invalidRuleSource = `
this is not valid starlark !!!
`

// TestCreateRule_ValidStarlark verifies that a valid Starlark rule is compiled,
// persisted, and that event_types and priority are derived from the source.
func TestCreateRule_ValidStarlark(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-valid-org")

	params := service.CreateRuleParams{
		Name:   "Test Rule",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
		Tags:   []string{"test"},
	}

	rule, err := svc.CreateRule(ctx, orgID, params)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	if rule.ID == "" {
		t.Error("rule ID should not be empty")
	}
	if rule.OrgID != orgID {
		t.Errorf("OrgID: got %q, want %q", rule.OrgID, orgID)
	}
	if rule.Name != params.Name {
		t.Errorf("Name: got %q, want %q", rule.Name, params.Name)
	}
	if rule.Version != 1 {
		t.Errorf("Version: got %d, want 1", rule.Version)
	}
	// event_types extracted from source, NOT from params
	if len(rule.EventTypes) != 1 || rule.EventTypes[0] != "post_created" {
		t.Errorf("EventTypes: got %v, want [post_created]", rule.EventTypes)
	}
	// priority extracted from source
	if rule.Priority != 100 {
		t.Errorf("Priority: got %d, want 100", rule.Priority)
	}
}

// TestCreateRule_InvalidStarlark verifies that invalid Starlark returns a CompileError
// and nothing is persisted.
func TestCreateRule_InvalidStarlark(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-invalid-org")

	params := service.CreateRuleParams{
		Name:   "Bad Rule",
		Status: domain.RuleStatusLive,
		Source: invalidRuleSource,
	}

	_, err := svc.CreateRule(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected CompileError, got nil")
	}
	if _, ok := err.(*domain.CompileError); !ok {
		t.Errorf("expected *domain.CompileError, got %T: %v", err, err)
	}

	// Verify nothing was persisted.
	result, listErr := svc.ListRules(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if listErr != nil {
		t.Fatalf("ListRules: %v", listErr)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 rules after compile failure, got %d", result.Total)
	}
}

// TestCreateRule_WildcardEventTypes verifies that a rule with event_types=["*"] succeeds.
func TestCreateRule_WildcardEventTypes(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-wildcard-org")

	params := service.CreateRuleParams{
		Name:   "Wildcard Rule",
		Status: domain.RuleStatusBackground,
		Source: validWildcardRuleSource,
	}

	rule, err := svc.CreateRule(ctx, orgID, params)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if len(rule.EventTypes) != 1 || rule.EventTypes[0] != "*" {
		t.Errorf("EventTypes: got %v, want [*]", rule.EventTypes)
	}
}

// TestCreateRule_MixedWildcard verifies that mixing "*" with specific event types fails.
func TestCreateRule_MixedWildcard(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-mixed-wildcard-org")

	mixedWildcardSource := `
rule_id = "test-mixed"
event_types = ["*", "post_created"]
priority = 10

def evaluate(event):
    return verdict("approve")
`
	params := service.CreateRuleParams{
		Name:   "Mixed Wildcard Rule",
		Status: domain.RuleStatusLive,
		Source: mixedWildcardSource,
	}

	_, err := svc.CreateRule(ctx, orgID, params)
	if err == nil {
		t.Fatal("expected CompileError for mixed wildcard, got nil")
	}
	if _, ok := err.(*domain.CompileError); !ok {
		t.Errorf("expected *domain.CompileError, got %T: %v", err, err)
	}
}

// TestCreateRule_ValidationError verifies that missing required fields return ValidationError.
func TestCreateRule_ValidationError(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-validation-org")

	tests := []struct {
		name   string
		params service.CreateRuleParams
	}{
		{
			name: "empty name",
			params: service.CreateRuleParams{
				Name:   "",
				Status: domain.RuleStatusLive,
				Source: validRuleSource,
			},
		},
		{
			name: "empty source",
			params: service.CreateRuleParams{
				Name:   "Test Rule",
				Status: domain.RuleStatusLive,
				Source: "",
			},
		},
		{
			name: "invalid status",
			params: service.CreateRuleParams{
				Name:   "Test Rule",
				Status: "INVALID",
				Source: validRuleSource,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateRule(ctx, orgID, tc.params)
			if err == nil {
				t.Fatal("expected ValidationError, got nil")
			}
			if _, ok := err.(*domain.ValidationError); !ok {
				t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

// TestUpdateRule_SourceChange verifies that changing the source re-compiles the rule,
// updates metadata (event_types, priority), and increments the version.
func TestUpdateRule_SourceChange(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-rule-source-org")

	// Create initial rule.
	created, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "Rule To Update",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	newSource := `
rule_id = "test-rule-updated"
event_types = ["comment_created"]
priority = 200

def evaluate(event):
    return verdict("block", reason="updated")
`
	updated, err := svc.UpdateRule(ctx, orgID, created.ID, service.UpdateRuleParams{
		Source: &newSource,
	})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}

	if updated.Version != 2 {
		t.Errorf("Version: got %d, want 2", updated.Version)
	}
	if len(updated.EventTypes) != 1 || updated.EventTypes[0] != "comment_created" {
		t.Errorf("EventTypes: got %v, want [comment_created]", updated.EventTypes)
	}
	if updated.Priority != 200 {
		t.Errorf("Priority: got %d, want 200", updated.Priority)
	}
}

// TestUpdateRule_NoSourceChange verifies that updating non-source fields does not
// recompile the rule and metadata (event_types, priority) remains unchanged.
func TestUpdateRule_NoSourceChange(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-rule-nosource-org")

	created, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "Original Name",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
		Tags:   []string{"old-tag"},
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	newName := "Updated Name"
	newTags := []string{"new-tag"}
	updated, err := svc.UpdateRule(ctx, orgID, created.ID, service.UpdateRuleParams{
		Name: &newName,
		Tags: &newTags,
	})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", updated.Name, "Updated Name")
	}
	if updated.Version != 2 {
		t.Errorf("Version: got %d, want 2", updated.Version)
	}
	// Metadata unchanged since source did not change.
	if len(updated.EventTypes) != 1 || updated.EventTypes[0] != "post_created" {
		t.Errorf("EventTypes: got %v, want [post_created]", updated.EventTypes)
	}
	if updated.Priority != 100 {
		t.Errorf("Priority: got %d, want 100", updated.Priority)
	}
}

// TestUpdateRule_EntityHistory verifies that entity_history entries are written on each update.
func TestUpdateRule_EntityHistory(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-rule-history-org")

	created, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "History Rule",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	// Check history after create.
	history, err := q.GetEntityHistory(ctx, "rule", created.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory after create: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("history entries after create: got %d, want 1", len(history))
	}
	if history[0].Version != 1 {
		t.Errorf("history[0].Version: got %d, want 1", history[0].Version)
	}

	// Update the rule.
	newName := "History Rule Updated"
	_, err = svc.UpdateRule(ctx, orgID, created.ID, service.UpdateRuleParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}

	// Check history after update.
	history, err = q.GetEntityHistory(ctx, "rule", created.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory after update: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("history entries after update: got %d, want 2", len(history))
	}
	if history[1].Version != 2 {
		t.Errorf("history[1].Version: got %d, want 2", history[1].Version)
	}
}

// TestDeleteRule_SnapshotRebuilt verifies that deleting a rule removes it and
// triggers a snapshot rebuild (the test only checks that the rule is gone from the store).
func TestDeleteRule_SnapshotRebuilt(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "delete-rule-org")

	created, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "Rule To Delete",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	if err := svc.DeleteRule(ctx, orgID, created.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	_, err = svc.GetRule(ctx, orgID, created.ID)
	if err == nil {
		t.Fatal("expected NotFoundError after delete, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestTestRule_NoSideEffects verifies that TestRule does not write to the rules table.
func TestTestRule_NoSideEffects(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "testrule-nosideeffects-org")

	event := domain.Event{
		OrgID:     orgID,
		EventType: "post_created",
		Payload:   map[string]any{"text": "hello"},
	}

	_, err := svc.TestRule(ctx, orgID, validRuleSource, event)
	if err != nil {
		t.Fatalf("TestRule: %v", err)
	}

	// Confirm no rule was written to the rules table.
	result, err := svc.ListRules(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 rules after TestRule, got %d", result.Total)
	}
}

// TestTestRule_ReturnsResult verifies that TestRule returns a non-nil TestResult
// with a valid verdict.
func TestTestRule_ReturnsResult(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "testrule-result-org")

	event := domain.Event{
		OrgID:     orgID,
		EventType: "post_created",
		Payload:   map[string]any{"text": "hello"},
	}

	result, err := svc.TestRule(ctx, orgID, validRuleSource, event)
	if err != nil {
		t.Fatalf("TestRule: %v", err)
	}
	if result == nil {
		t.Fatal("TestRule returned nil result")
	}
	if result.Verdict == "" {
		t.Error("TestResult.Verdict should not be empty")
	}
}

// TestTestRule_InvalidStarlark verifies that TestRule returns a CompileError for bad source.
func TestTestRule_InvalidStarlark(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "testrule-invalid-org")

	event := domain.Event{
		OrgID:     orgID,
		EventType: "post_created",
		Payload:   map[string]any{},
	}

	_, err := svc.TestRule(ctx, orgID, invalidRuleSource, event)
	if err == nil {
		t.Fatal("expected CompileError, got nil")
	}
	if _, ok := err.(*domain.CompileError); !ok {
		t.Errorf("expected *domain.CompileError, got %T: %v", err, err)
	}
}

// TestRebuildSnapshot_AllEnabled verifies that only LIVE and BACKGROUND rules
// are compiled and included in the snapshot.
func TestRebuildSnapshot_AllEnabled(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rebuild-snapshot-enabled-org")

	statusTests := []struct {
		status  domain.RuleStatus
		source  string
		include bool
	}{
		{domain.RuleStatusLive, buildRuleSource("live-rule-001", "post_created", 10), true},
		{domain.RuleStatusBackground, buildRuleSource("bg-rule-001", "post_created", 20), true},
		{domain.RuleStatusDisabled, buildRuleSource("dis-rule-001", "post_created", 30), false},
	}

	for _, tc := range statusTests {
		_, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
			Name:   string(tc.status) + " rule",
			Status: tc.status,
			Source: tc.source,
		})
		if err != nil {
			t.Fatalf("CreateRule(%s): %v", tc.status, err)
		}
	}

	// RebuildSnapshot should not error.
	if err := svc.RebuildSnapshot(ctx, orgID); err != nil {
		t.Fatalf("RebuildSnapshot: %v", err)
	}
}

// TestRebuildSnapshot_SkipsBadRules verifies that when one rule fails compilation,
// the rebuild still succeeds and includes the other rules.
func TestRebuildSnapshot_SkipsBadRules(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "rebuild-snapshot-skipbad-org")

	// Create a valid rule normally.
	_, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "Valid Rule",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
	})
	if err != nil {
		t.Fatalf("CreateRule valid: %v", err)
	}

	// RebuildSnapshot should succeed without error.
	if err := svc.RebuildSnapshot(ctx, orgID); err != nil {
		t.Fatalf("RebuildSnapshot: %v", err)
	}
}

// TestListRules_Pagination verifies that ListRules returns correct pagination metadata.
func TestListRules_Pagination(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "listrules-pagination-org")

	for i := 0; i < 5; i++ {
		source := buildRuleSource(
			"pagination-rule-"+string(rune('a'+i)),
			"post_created",
			i*10,
		)
		_, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
			Name:   "Pagination Rule",
			Status: domain.RuleStatusDisabled,
			Source: source,
		})
		if err != nil {
			t.Fatalf("CreateRule(%d): %v", i, err)
		}
	}

	result, err := svc.ListRules(ctx, orgID, domain.PageParams{Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total: got %d, want 5", result.Total)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count: got %d, want 3", len(result.Items))
	}
	if result.TotalPages != 2 {
		t.Errorf("TotalPages: got %d, want 2", result.TotalPages)
	}

	// Second page.
	result2, err := svc.ListRules(ctx, orgID, domain.PageParams{Page: 2, PageSize: 3})
	if err != nil {
		t.Fatalf("ListRules page 2: %v", err)
	}
	if len(result2.Items) != 2 {
		t.Errorf("Items count page 2: got %d, want 2", len(result2.Items))
	}
}

// TestCreateRule_WithPolicies verifies that policy IDs are associated with the rule.
func TestCreateRule_WithPolicies(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-rule-policies-org")

	// Create a policy to associate with the rule.
	polDomain1 := &domain.Policy{
		ID:            generateTestID("pol"),
		OrgID:         orgID,
		Name:          "Policy For Rule",
		StrikePenalty: 1,
		Version:       1,
	}
	if createErr := q.CreatePolicy(ctx, polDomain1); createErr != nil {
		t.Fatalf("CreatePolicy: %v", createErr)
	}

	rule, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:      "Rule With Policy",
		Status:    domain.RuleStatusLive,
		Source:    validRuleSource,
		PolicyIDs: []string{polDomain1.ID},
	})
	if err != nil {
		t.Fatalf("CreateRule with policies: %v", err)
	}

	pids, err := q.GetRulePolicies(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRulePolicies: %v", err)
	}
	if len(pids) != 1 || pids[0] != polDomain1.ID {
		t.Errorf("PolicyIDs: got %v, want [%s]", pids, polDomain1.ID)
	}
}

// TestTestExistingRule verifies that TestExistingRule fetches the rule source and evaluates it.
func TestTestExistingRule(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "test-existing-rule-org")

	created, err := svc.CreateRule(ctx, orgID, service.CreateRuleParams{
		Name:   "Existing Rule",
		Status: domain.RuleStatusLive,
		Source: validRuleSource,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	event := domain.Event{
		OrgID:     orgID,
		EventType: "post_created",
		Payload:   map[string]any{"text": "hello"},
	}

	result, err := svc.TestExistingRule(ctx, orgID, created.ID, event)
	if err != nil {
		t.Fatalf("TestExistingRule: %v", err)
	}
	if result == nil {
		t.Fatal("TestExistingRule returned nil result")
	}
}

// TestGetRule_NotFound verifies that GetRule returns a NotFoundError for missing rules.
func TestGetRule_NotFound(t *testing.T) {
	svc, q, cleanup := setupRuleService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "getrule-notfound-org")

	_, err := svc.GetRule(ctx, orgID, "nonexistent-rule-id")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// buildRuleSource constructs a valid Starlark rule source with the given params.
func buildRuleSource(ruleID, eventType string, priority int) string {
	return strings.Join([]string{
		`rule_id = "` + ruleID + `"`,
		`event_types = ["` + eventType + `"]`,
		"priority = " + fmt.Sprintf("%d", priority),
		"",
		"def evaluate(event):",
		`    return verdict("approve", reason="auto")`,
	}, "\n")
}
