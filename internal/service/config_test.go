package service_test

import (
	"context"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// ---- Action tests -----------------------------------------------------------

// TestCreateAction_Success verifies that a new action is persisted with Version=1.
func TestCreateAction_Success(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-action-success-org")

	params := service.CreateActionParams{
		Name:       "Test Webhook",
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{"url": "https://example.com/hook"},
	}

	action, err := svc.CreateAction(ctx, orgID, params)
	if err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	if action.ID == "" {
		t.Error("action ID should not be empty")
	}
	if !isPrefix(action.ID, "act_") {
		t.Errorf("action ID should start with act_, got %q", action.ID)
	}
	if action.OrgID != orgID {
		t.Errorf("OrgID: got %q, want %q", action.OrgID, orgID)
	}
	if action.Name != params.Name {
		t.Errorf("Name: got %q, want %q", action.Name, params.Name)
	}
	if action.Version != 1 {
		t.Errorf("Version: got %d, want 1", action.Version)
	}
}

// TestCreateAction_EntityHistory verifies that entity history is written on create.
func TestCreateAction_EntityHistory(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-action-history-org")

	action, err := svc.CreateAction(ctx, orgID, service.CreateActionParams{
		Name:       "Webhook For History",
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{"url": "https://example.com"},
	})
	if err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	history, err := q.GetEntityHistory(ctx, "action", action.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("history entries after create: got %d, want 1", len(history))
	}
	if history[0].Version != 1 {
		t.Errorf("history[0].Version: got %d, want 1", history[0].Version)
	}
}

// TestUpdateAction_EntityHistory verifies that entity history is written on update and version increments.
func TestUpdateAction_EntityHistory(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-action-history-org")

	created, err := svc.CreateAction(ctx, orgID, service.CreateActionParams{
		Name:       "Action To Update",
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{"url": "https://example.com"},
	})
	if err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	newName := "Updated Action Name"
	updated, err := svc.UpdateAction(ctx, orgID, created.ID, service.UpdateActionParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateAction: %v", err)
	}

	if updated.Version != 2 {
		t.Errorf("Version after update: got %d, want 2", updated.Version)
	}

	history, err := q.GetEntityHistory(ctx, "action", created.ID, orgID)
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

// TestCreateAction_WithItemTypes verifies that item type associations are set correctly.
func TestCreateAction_WithItemTypes(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-action-itemtypes-org")

	// Seed an item type to associate.
	it := seedItemType(t, q, orgID, "content-post")

	action, err := svc.CreateAction(ctx, orgID, service.CreateActionParams{
		Name:        "Action With Item Types",
		ActionType:  domain.ActionTypeWebhook,
		Config:      map[string]any{"url": "https://example.com"},
		ItemTypeIDs: []string{it.ID},
	})
	if err != nil {
		t.Fatalf("CreateAction with item types: %v", err)
	}

	itemTypeIDs, err := q.GetActionItemTypes(ctx, action.ID)
	if err != nil {
		t.Fatalf("GetActionItemTypes: %v", err)
	}
	if len(itemTypeIDs) != 1 || itemTypeIDs[0] != it.ID {
		t.Errorf("ItemTypeIDs: got %v, want [%s]", itemTypeIDs, it.ID)
	}
}

// TestCreateAction_ValidationError verifies that missing fields return ValidationError.
func TestCreateAction_ValidationError(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-action-validation-org")

	tests := []struct {
		name   string
		params service.CreateActionParams
	}{
		{
			name: "empty name",
			params: service.CreateActionParams{
				Name:       "",
				ActionType: domain.ActionTypeWebhook,
			},
		},
		{
			name: "invalid action type",
			params: service.CreateActionParams{
				Name:       "Valid Name",
				ActionType: "INVALID_TYPE",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateAction(ctx, orgID, tc.params)
			if err == nil {
				t.Fatal("expected ValidationError, got nil")
			}
			if _, ok := err.(*domain.ValidationError); !ok {
				t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

// TestDeleteAction_NotFound verifies that deleting a non-existent action returns NotFoundError.
func TestDeleteAction_NotFound(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "delete-action-notfound-org")

	err := svc.DeleteAction(ctx, orgID, "nonexistent-action-id")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestCRUD_Actions verifies the full create-read-update-delete cycle for actions.
func TestCRUD_Actions(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "crud-actions-org")

	// Create
	created, err := svc.CreateAction(ctx, orgID, service.CreateActionParams{
		Name:       "CRUD Action",
		ActionType: domain.ActionTypeEnqueueToMRT,
		Config:     map[string]any{"queue_name": "review"},
	})
	if err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	// Read
	got, err := svc.GetAction(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetAction: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetAction ID mismatch: got %q, want %q", got.ID, created.ID)
	}

	// Update
	newName := "Updated CRUD Action"
	updated, err := svc.UpdateAction(ctx, orgID, created.ID, service.UpdateActionParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateAction: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Updated name: got %q, want %q", updated.Name, newName)
	}

	// List
	result, err := svc.ListActions(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if result.Total < 1 {
		t.Errorf("ListActions total: got %d, want >= 1", result.Total)
	}

	// Delete
	if err := svc.DeleteAction(ctx, orgID, created.ID); err != nil {
		t.Fatalf("DeleteAction: %v", err)
	}

	// Verify deleted
	_, err = svc.GetAction(ctx, orgID, created.ID)
	if err == nil {
		t.Fatal("expected NotFoundError after delete, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// ---- Policy tests -----------------------------------------------------------

// TestCreatePolicy_Success verifies that a new policy is persisted with Version=1 and history written.
func TestCreatePolicy_Success(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-policy-success-org")

	desc := "A test policy"
	params := service.CreatePolicyParams{
		Name:          "Test Policy",
		Description:   &desc,
		StrikePenalty: 5,
	}

	policy, err := svc.CreatePolicy(ctx, orgID, params)
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	if !isPrefix(policy.ID, "pol_") {
		t.Errorf("policy ID should start with pol_, got %q", policy.ID)
	}
	if policy.Version != 1 {
		t.Errorf("Version: got %d, want 1", policy.Version)
	}
	if policy.Description != desc {
		t.Errorf("Description: got %q, want %q", policy.Description, desc)
	}
	if policy.StrikePenalty != 5 {
		t.Errorf("StrikePenalty: got %d, want 5", policy.StrikePenalty)
	}

	// Verify entity history written.
	history, err := q.GetEntityHistory(ctx, "policy", policy.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("history entries: got %d, want 1", len(history))
	}
}

// TestCreatePolicy_ValidationError verifies that an empty name returns ValidationError.
func TestCreatePolicy_ValidationError(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-policy-validation-org")

	_, err := svc.CreatePolicy(ctx, orgID, service.CreatePolicyParams{
		Name: "",
	})
	if err == nil {
		t.Fatal("expected ValidationError, got nil")
	}
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestUpdatePolicy_EntityHistory verifies that updating a policy increments version and writes history.
func TestUpdatePolicy_EntityHistory(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-policy-history-org")

	created, err := svc.CreatePolicy(ctx, orgID, service.CreatePolicyParams{
		Name:          "Policy To Update",
		StrikePenalty: 1,
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	newName := "Updated Policy Name"
	newPenalty := 10
	updated, err := svc.UpdatePolicy(ctx, orgID, created.ID, service.UpdatePolicyParams{
		Name:          &newName,
		StrikePenalty: &newPenalty,
	})
	if err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	if updated.Version != 2 {
		t.Errorf("Version after update: got %d, want 2", updated.Version)
	}
	if updated.Name != newName {
		t.Errorf("Name after update: got %q, want %q", updated.Name, newName)
	}
	if updated.StrikePenalty != newPenalty {
		t.Errorf("StrikePenalty after update: got %d, want %d", updated.StrikePenalty, newPenalty)
	}

	history, err := q.GetEntityHistory(ctx, "policy", created.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("history entries after update: got %d, want 2", len(history))
	}
}

// TestCRUD_Policies verifies the full create-read-update-delete cycle for policies.
func TestCRUD_Policies(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "crud-policies-org")

	// Create
	created, err := svc.CreatePolicy(ctx, orgID, service.CreatePolicyParams{
		Name:          "CRUD Policy",
		StrikePenalty: 3,
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	// Read
	got, err := svc.GetPolicy(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetPolicy ID: got %q, want %q", got.ID, created.ID)
	}

	// Update
	newName := "Updated CRUD Policy"
	updated, err := svc.UpdatePolicy(ctx, orgID, created.ID, service.UpdatePolicyParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Updated name: got %q, want %q", updated.Name, newName)
	}

	// List
	result, err := svc.ListPolicies(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if result.Total < 1 {
		t.Errorf("ListPolicies total: got %d, want >= 1", result.Total)
	}

	// Delete
	if err := svc.DeletePolicy(ctx, orgID, created.ID); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}

	// Verify deleted
	_, err = svc.GetPolicy(ctx, orgID, created.ID)
	if err == nil {
		t.Fatal("expected NotFoundError after delete, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// ---- Item Type tests --------------------------------------------------------

// TestCreateItemType_Success verifies that a new item type is persisted without entity history.
func TestCreateItemType_Success(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-itemtype-success-org")

	params := service.CreateItemTypeParams{
		Name:       "Post Content",
		Kind:       domain.ItemTypeKindContent,
		Schema:     map[string]any{"type": "object"},
		FieldRoles: map[string]any{"text": "body"},
	}

	it, err := svc.CreateItemType(ctx, orgID, params)
	if err != nil {
		t.Fatalf("CreateItemType: %v", err)
	}

	if !isPrefix(it.ID, "ity_") {
		t.Errorf("item type ID should start with ity_, got %q", it.ID)
	}
	if it.Name != params.Name {
		t.Errorf("Name: got %q, want %q", it.Name, params.Name)
	}
	if it.Kind != params.Kind {
		t.Errorf("Kind: got %q, want %q", it.Kind, params.Kind)
	}
	// Item types have no Version field and no entity history.
	history, err := q.GetEntityHistory(ctx, "item_type", it.ID, orgID)
	if err != nil {
		t.Fatalf("GetEntityHistory: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 history entries for item type, got %d", len(history))
	}
}

// TestCreateItemType_ValidationError verifies that invalid params return ValidationError.
func TestCreateItemType_ValidationError(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "create-itemtype-validation-org")

	tests := []struct {
		name   string
		params service.CreateItemTypeParams
	}{
		{
			name: "empty name",
			params: service.CreateItemTypeParams{
				Name: "",
				Kind: domain.ItemTypeKindContent,
			},
		},
		{
			name: "invalid kind",
			params: service.CreateItemTypeParams{
				Name: "Valid Name",
				Kind: "INVALID_KIND",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateItemType(ctx, orgID, tc.params)
			if err == nil {
				t.Fatal("expected ValidationError, got nil")
			}
			if _, ok := err.(*domain.ValidationError); !ok {
				t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

// TestUpdateItemType_Success verifies that updating an item type applies non-nil fields.
func TestUpdateItemType_Success(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "update-itemtype-success-org")

	created, err := svc.CreateItemType(ctx, orgID, service.CreateItemTypeParams{
		Name: "Original Item Type",
		Kind: domain.ItemTypeKindContent,
	})
	if err != nil {
		t.Fatalf("CreateItemType: %v", err)
	}

	newName := "Updated Item Type"
	newKind := domain.ItemTypeKindUser
	updated, err := svc.UpdateItemType(ctx, orgID, created.ID, service.UpdateItemTypeParams{
		Name: &newName,
		Kind: &newKind,
	})
	if err != nil {
		t.Fatalf("UpdateItemType: %v", err)
	}

	if updated.Name != newName {
		t.Errorf("Name: got %q, want %q", updated.Name, newName)
	}
	if updated.Kind != newKind {
		t.Errorf("Kind: got %q, want %q", updated.Kind, newKind)
	}
}

// TestCRUD_ItemTypes verifies the full create-read-update-delete cycle for item types.
func TestCRUD_ItemTypes(t *testing.T) {
	svc, q, cleanup := setupConfigService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "crud-itemtypes-org")

	// Create
	created, err := svc.CreateItemType(ctx, orgID, service.CreateItemTypeParams{
		Name: "CRUD Item Type",
		Kind: domain.ItemTypeKindThread,
	})
	if err != nil {
		t.Fatalf("CreateItemType: %v", err)
	}

	// Read
	got, err := svc.GetItemType(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetItemType: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetItemType ID: got %q, want %q", got.ID, created.ID)
	}

	// Update
	newName := "Updated CRUD Item Type"
	_, err = svc.UpdateItemType(ctx, orgID, created.ID, service.UpdateItemTypeParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateItemType: %v", err)
	}

	// List
	result, err := svc.ListItemTypes(ctx, orgID, domain.PageParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListItemTypes: %v", err)
	}
	if result.Total < 1 {
		t.Errorf("ListItemTypes total: got %d, want >= 1", result.Total)
	}

	// Delete
	if err := svc.DeleteItemType(ctx, orgID, created.ID); err != nil {
		t.Fatalf("DeleteItemType: %v", err)
	}

	// Verify deleted
	_, err = svc.GetItemType(ctx, orgID, created.ID)
	if err == nil {
		t.Fatal("expected NotFoundError after delete, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// isPrefix checks if s starts with prefix.
func isPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
