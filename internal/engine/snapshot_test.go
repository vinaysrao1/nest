package engine

import (
	"strings"
	"testing"
)

// makeRule builds a CompiledRule stub for testing Snapshot without a real Starlark Program.
// Program is nil because Snapshot only uses ID, EventTypes, and Priority for indexing.
func makeRule(id string, priority int, eventTypes ...string) *CompiledRule {
	return &CompiledRule{
		ID:         id,
		EventTypes: eventTypes,
		Priority:   priority,
		Program:    nil,
		Source:     "",
	}
}

// TestNewSnapshot_IndexesByEventType verifies rules are grouped into ByEvent correctly.
func TestNewSnapshot_IndexesByEventType(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("block-rule", 100, "content"),
		makeRule("review-rule", 50, "content"),
		makeRule("approve-rule", 10, "content"),
	}
	snap := NewSnapshot("org-1", rules, nil)

	if snap.OrgID != "org-1" {
		t.Errorf("snap.OrgID = %q, want %q", snap.OrgID, "org-1")
	}
	if len(snap.Rules) != 3 {
		t.Errorf("snap.Rules length = %d, want 3", len(snap.Rules))
	}
	contentRules, ok := snap.ByEvent["content"]
	if !ok {
		t.Fatal("snap.ByEvent[\"content\"] not present")
	}
	if len(contentRules) != 3 {
		t.Errorf("ByEvent[\"content\"] length = %d, want 3", len(contentRules))
	}
}

// TestNewSnapshot_WildcardStoredUnderStarKey verifies wildcard rules are stored under "*".
func TestNewSnapshot_WildcardStoredUnderStarKey(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("wildcard-rule", 10, "*"),
		makeRule("content-rule", 50, "content"),
	}
	snap := NewSnapshot("org-2", rules, nil)

	wildcardRules, ok := snap.ByEvent["*"]
	if !ok {
		t.Fatal("snap.ByEvent[\"*\"] not present for wildcard rule")
	}
	if len(wildcardRules) != 1 || wildcardRules[0].ID != "wildcard-rule" {
		t.Errorf("ByEvent[\"*\"] = %v, want [wildcard-rule]", wildcardRules)
	}
}

// TestNewSnapshot_SortedByPriorityDesc verifies rules within each bucket are sorted descending.
func TestNewSnapshot_SortedByPriorityDesc(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("low", 10, "content"),
		makeRule("high", 100, "content"),
		makeRule("mid", 50, "content"),
	}
	snap := NewSnapshot("org-3", rules, nil)

	contentRules := snap.ByEvent["content"]
	if len(contentRules) != 3 {
		t.Fatalf("expected 3 content rules, got %d", len(contentRules))
	}
	priorities := []int{contentRules[0].Priority, contentRules[1].Priority, contentRules[2].Priority}
	if priorities[0] < priorities[1] || priorities[1] < priorities[2] {
		t.Errorf("ByEvent[\"content\"] priorities %v are not sorted descending", priorities)
	}
}

// TestRulesForEvent_MergesSpecificAndWildcard verifies specific and wildcard rules are merged.
func TestRulesForEvent_MergesSpecificAndWildcard(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("content-rule", 50, "content"),
		makeRule("wildcard-rule", 30, "*"),
	}
	snap := NewSnapshot("org-4", rules, nil)

	result := snap.RulesForEvent("content")
	if len(result) != 2 {
		t.Fatalf("RulesForEvent(\"content\") = %d rules, want 2", len(result))
	}
	if result[0].Priority < result[1].Priority {
		t.Errorf("RulesForEvent result not sorted by priority desc: %d < %d",
			result[0].Priority, result[1].Priority)
	}
	if result[0].ID != "content-rule" {
		t.Errorf("first rule ID = %q, want %q", result[0].ID, "content-rule")
	}
}

// TestRulesForEvent_UnknownEventReturnsOnlyWildcards verifies unknown events get only wildcards.
func TestRulesForEvent_UnknownEventReturnsOnlyWildcards(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("content-rule", 50, "content"),
		makeRule("wildcard-rule", 30, "*"),
	}
	snap := NewSnapshot("org-5", rules, nil)

	result := snap.RulesForEvent("image")
	if len(result) != 1 {
		t.Fatalf("RulesForEvent(\"image\") = %d rules, want 1 (wildcard only)", len(result))
	}
	if result[0].ID != "wildcard-rule" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "wildcard-rule")
	}
}

// TestRulesForEvent_NoMatchReturnsEmptySlice verifies an empty (non-nil) slice for no matches.
func TestRulesForEvent_NoMatchReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("content-rule", 50, "content"),
	}
	snap := NewSnapshot("org-6", rules, nil)

	result := snap.RulesForEvent("image")
	if result == nil {
		t.Error("RulesForEvent returned nil, want empty non-nil slice")
	}
	if len(result) != 0 {
		t.Errorf("RulesForEvent(\"image\") = %d rules, want 0", len(result))
	}
}

// TestRulesForEvent_Immutability verifies that modifying the returned slice does not
// affect subsequent calls to RulesForEvent.
func TestRulesForEvent_Immutability(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("block-rule", 100, "content"),
		makeRule("review-rule", 50, "content"),
	}
	snap := NewSnapshot("org-7", rules, nil)

	first := snap.RulesForEvent("content")
	if len(first) != 2 {
		t.Fatalf("first call: expected 2 rules, got %d", len(first))
	}

	// Overwrite elements and truncate the returned slice.
	first[0] = makeRule("injected", 999, "content")
	first = first[:1]

	// Second call must return the original 2 rules, unaffected by mutations.
	second := snap.RulesForEvent("content")
	if len(second) != 2 {
		t.Errorf("second call: expected 2 rules, got %d (slice mutation affected snapshot)", len(second))
	}
	if second[0].ID == "injected" {
		t.Error("second call: snapshot was mutated by external slice modification")
	}
}

// TestNewSnapshot_SnapshotIDContainsOrgID verifies the generated snapshot ID is org-scoped.
func TestNewSnapshot_SnapshotIDContainsOrgID(t *testing.T) {
	t.Parallel()
	snap := NewSnapshot("my-org", []*CompiledRule{}, nil)
	if snap.ID == "" {
		t.Error("snap.ID is empty")
	}
	if !strings.Contains(snap.ID, "my-org") {
		t.Errorf("snap.ID = %q, want it to contain %q", snap.ID, "my-org")
	}
}

// TestRulesForEvent_MergedSortedByPriority verifies merge result is sorted correctly
// when wildcards and specific rules have interleaved priorities.
func TestRulesForEvent_MergedSortedByPriority(t *testing.T) {
	t.Parallel()
	rules := []*CompiledRule{
		makeRule("block-rule", 100, "content"),
		makeRule("wildcard-high", 80, "*"),
		makeRule("review-rule", 50, "content"),
		makeRule("wildcard-low", 20, "*"),
	}
	snap := NewSnapshot("org-8", rules, nil)

	result := snap.RulesForEvent("content")
	if len(result) != 4 {
		t.Fatalf("RulesForEvent(\"content\") = %d rules, want 4", len(result))
	}

	for i := 1; i < len(result); i++ {
		if result[i-1].Priority < result[i].Priority {
			t.Errorf("result[%d].Priority (%d) < result[%d].Priority (%d): not sorted desc",
				i-1, result[i-1].Priority, i, result[i].Priority)
		}
	}

	wantOrder := []string{"block-rule", "wildcard-high", "review-rule", "wildcard-low"}
	for i, want := range wantOrder {
		if result[i].ID != want {
			t.Errorf("result[%d].ID = %q, want %q", i, result[i].ID, want)
		}
	}
}
