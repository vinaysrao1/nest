package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

// TestErrorTypes verifies that all error types implement the error interface
// and return the expected message from Error().
func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"NotFoundError", &domain.NotFoundError{Message: "not found"}, "not found"},
		{"ForbiddenError", &domain.ForbiddenError{Message: "forbidden"}, "forbidden"},
		{"ConflictError", &domain.ConflictError{Message: "conflict"}, "conflict"},
		{"ValidationError", &domain.ValidationError{Message: "invalid", Details: map[string]string{"field": "bad"}}, "invalid"},
		{"ConfigError", &domain.ConfigError{Message: "missing var"}, "missing var"},
		{"CompileError", &domain.CompileError{Message: "syntax error", Line: 5, Column: 10}, "syntax error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("got %q, want %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

// TestEnumValues verifies that all enum constants have the expected string values.
func TestEnumValues(t *testing.T) {
	t.Run("RuleStatus", func(t *testing.T) {
		if domain.RuleStatusLive != "LIVE" {
			t.Error("RuleStatusLive should be LIVE")
		}
		if domain.RuleStatusBackground != "BACKGROUND" {
			t.Error("RuleStatusBackground should be BACKGROUND")
		}
		if domain.RuleStatusDisabled != "DISABLED" {
			t.Error("RuleStatusDisabled should be DISABLED")
		}
	})

	t.Run("VerdictType", func(t *testing.T) {
		if domain.VerdictApprove != "approve" {
			t.Error("VerdictApprove should be approve")
		}
		if domain.VerdictBlock != "block" {
			t.Error("VerdictBlock should be block")
		}
		if domain.VerdictReview != "review" {
			t.Error("VerdictReview should be review")
		}
	})

	t.Run("ActionType", func(t *testing.T) {
		if domain.ActionTypeWebhook != "WEBHOOK" {
			t.Error("ActionTypeWebhook should be WEBHOOK")
		}
		if domain.ActionTypeEnqueueToMRT != "ENQUEUE_TO_MRT" {
			t.Error("ActionTypeEnqueueToMRT should be ENQUEUE_TO_MRT")
		}
	})

	t.Run("UserRole", func(t *testing.T) {
		if domain.UserRoleAdmin != "ADMIN" {
			t.Error("UserRoleAdmin should be ADMIN")
		}
		if domain.UserRoleModerator != "MODERATOR" {
			t.Error("UserRoleModerator should be MODERATOR")
		}
		if domain.UserRoleAnalyst != "ANALYST" {
			t.Error("UserRoleAnalyst should be ANALYST")
		}
	})

	t.Run("MRTJobStatus", func(t *testing.T) {
		if domain.MRTJobStatusPending != "PENDING" {
			t.Error("MRTJobStatusPending should be PENDING")
		}
		if domain.MRTJobStatusAssigned != "ASSIGNED" {
			t.Error("MRTJobStatusAssigned should be ASSIGNED")
		}
		if domain.MRTJobStatusDecided != "DECIDED" {
			t.Error("MRTJobStatusDecided should be DECIDED")
		}
	})

	t.Run("ItemTypeKind", func(t *testing.T) {
		if domain.ItemTypeKindContent != "CONTENT" {
			t.Error("ItemTypeKindContent should be CONTENT")
		}
		if domain.ItemTypeKindUser != "USER" {
			t.Error("ItemTypeKindUser should be USER")
		}
		if domain.ItemTypeKindThread != "THREAD" {
			t.Error("ItemTypeKindThread should be THREAD")
		}
	})
}

// TestJSONSensitiveFieldsExcluded verifies that fields tagged json:"-" are
// not included in JSON output.
func TestJSONSensitiveFieldsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		excluded string
	}{
		{
			"User.Password",
			domain.User{ID: "u1", Password: "secret123", Email: "a@b.com", Name: "Test", Role: domain.UserRoleAdmin},
			"secret123",
		},
		{
			"ApiKey.KeyHash",
			domain.ApiKey{ID: "k1", KeyHash: "hashvalue", Prefix: "nest_", Name: "key1"},
			"hashvalue",
		},
		{
			"SigningKey.PrivateKey",
			domain.SigningKey{ID: "s1", PrivateKey: "privkey", PublicKey: "pubkey"},
			"privkey",
		},
		{
			"PasswordResetToken.TokenHash",
			domain.PasswordResetToken{ID: "t1", TokenHash: "tokenhash", UserID: "u1"},
			"tokenhash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("empty JSON")
			}
			if strings.Contains(string(data), tt.excluded) {
				t.Errorf("JSON contains sensitive field %q: %s", tt.excluded, string(data))
			}
		})
	}
}

// TestJSONRoundTrip verifies that key domain types serialize to valid JSON
// and deserialize back to a generic map without error.
func TestJSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name  string
		value any
	}{
		{
			"Event",
			domain.Event{
				ID: "e1", EventType: "content", ItemType: "post", OrgID: "org1",
				Payload: map[string]any{"text": "hello"}, Timestamp: now,
			},
		},
		{
			"Rule",
			domain.Rule{
				ID: "r1", OrgID: "org1", Name: "test-rule", Status: domain.RuleStatusLive,
				Source: "def evaluate(event): pass", EventTypes: []string{"content"},
				Priority: 100, Tags: []string{"spam"}, Version: 1, CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			"Verdict",
			domain.Verdict{Type: domain.VerdictBlock, Reason: "spam", RuleID: "r1", Actions: []string{"webhook-1"}},
		},
		{
			"Action",
			domain.Action{
				ID: "a1", OrgID: "org1", Name: "webhook-1", ActionType: domain.ActionTypeWebhook,
				Config: map[string]any{"url": "https://example.com"}, Version: 1, CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			"Policy",
			domain.Policy{
				ID: "p1", OrgID: "org1", Name: "hate-speech", Description: "Hate speech policy",
				StrikePenalty: 3, Version: 1, CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			"Org",
			domain.Org{
				ID: "org1", Name: "Test Org", Settings: map[string]any{"feature": true},
				CreatedAt: now, UpdatedAt: now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("empty JSON output")
			}
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
		})
	}
}

// TestPaginatedResult verifies that PaginatedResult works with different type
// parameters and that pagination metadata round-trips correctly via JSON.
func TestPaginatedResult(t *testing.T) {
	t.Run("WithStrings", func(t *testing.T) {
		pr := domain.PaginatedResult[string]{
			Items:      []string{"a", "b", "c"},
			Total:      10,
			Page:       1,
			PageSize:   3,
			TotalPages: 4,
		}
		data, err := json.Marshal(pr)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var result domain.PaginatedResult[string]
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if result.Total != 10 || result.Page != 1 || result.TotalPages != 4 {
			t.Errorf("pagination metadata mismatch: %+v", result)
		}
		if len(result.Items) != 3 {
			t.Errorf("expected 3 items, got %d", len(result.Items))
		}
	})

	t.Run("WithRules", func(t *testing.T) {
		prRules := domain.PaginatedResult[domain.Rule]{
			Items:      []domain.Rule{{ID: "r1", Name: "test"}},
			Total:      1,
			Page:       1,
			PageSize:   10,
			TotalPages: 1,
		}
		data, err := json.Marshal(prRules)
		if err != nil {
			t.Fatalf("marshal rules: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("empty JSON for PaginatedResult[Rule]")
		}
	})
}

// TestCompileErrorJSON verifies that CompileError serializes with the correct
// field names and that zero-value fields are omitted.
func TestCompileErrorJSON(t *testing.T) {
	t.Run("AllFields", func(t *testing.T) {
		ce := domain.CompileError{
			Message:  "undefined: foo",
			Line:     10,
			Column:   5,
			Filename: "rule.star",
		}
		data, err := json.Marshal(ce)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m["message"] != "undefined: foo" {
			t.Errorf("message mismatch: %v", m["message"])
		}
		if m["line"] != float64(10) {
			t.Errorf("line mismatch: %v", m["line"])
		}
		if m["column"] != float64(5) {
			t.Errorf("column mismatch: %v", m["column"])
		}
		if m["filename"] != "rule.star" {
			t.Errorf("filename mismatch: %v", m["filename"])
		}
	})

	t.Run("ZeroValuesOmitted", func(t *testing.T) {
		ce := domain.CompileError{Message: "error"}
		data, err := json.Marshal(ce)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, exists := m["line"]; exists {
			t.Error("line should be omitted when zero")
		}
		if _, exists := m["column"]; exists {
			t.Error("column should be omitted when zero")
		}
		if _, exists := m["filename"]; exists {
			t.Error("filename should be omitted when zero")
		}
	})
}

// TestResponseTypes verifies that EvalResultResponse and TriggeredRule
// serialize with the expected JSON field names.
func TestResponseTypes(t *testing.T) {
	resp := domain.EvalResultResponse{
		ItemID:  "item-1",
		Verdict: domain.VerdictBlock,
		TriggeredRules: []domain.TriggeredRule{
			{RuleID: "r1", Version: 1, Verdict: domain.VerdictBlock, Reason: "spam", LatencyUs: 1500},
		},
		Actions: []domain.ActionResult{
			{ActionID: "a1", Success: true},
			{ActionID: "a2", Success: false, Error: "timeout"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["item_id"] != "item-1" {
		t.Errorf("item_id mismatch: got %v", m["item_id"])
	}
	if m["verdict"] != "block" {
		t.Errorf("verdict mismatch: got %v", m["verdict"])
	}

	triggeredRules, ok := m["triggered_rules"].([]any)
	if !ok {
		t.Fatalf("triggered_rules is not a slice: %T", m["triggered_rules"])
	}
	if len(triggeredRules) != 1 {
		t.Errorf("expected 1 triggered rule, got %d", len(triggeredRules))
	}

	actions, ok := m["actions"].([]any)
	if !ok {
		t.Fatalf("actions is not a slice: %T", m["actions"])
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}
