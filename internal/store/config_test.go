package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestActions_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "actions-crud-org")

	t.Run("create and get action", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		action := &domain.Action{
			ID:         "action-001",
			OrgID:      orgID,
			Name:       "webhook-alert",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{"url": "https://example.com/webhook", "timeout_ms": float64(5000)},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := q.CreateAction(ctx, action); err != nil {
			t.Fatalf("CreateAction: %v", err)
		}

		got, err := q.GetAction(ctx, orgID, action.ID)
		if err != nil {
			t.Fatalf("GetAction: %v", err)
		}

		if got.ID != action.ID {
			t.Errorf("ID: got %q, want %q", got.ID, action.ID)
		}
		if got.Name != action.Name {
			t.Errorf("Name: got %q, want %q", got.Name, action.Name)
		}
		if got.ActionType != action.ActionType {
			t.Errorf("ActionType: got %q, want %q", got.ActionType, action.ActionType)
		}
		// JSONB config round-trip.
		if got.Config["url"] != "https://example.com/webhook" {
			t.Errorf("Config[url]: got %v, want %q", got.Config["url"], "https://example.com/webhook")
		}
		if got.Config["timeout_ms"] != float64(5000) {
			t.Errorf("Config[timeout_ms]: got %v, want 5000", got.Config["timeout_ms"])
		}
	})

	t.Run("get action by name", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		action := &domain.Action{
			ID:         "action-byname-001",
			OrgID:      orgID,
			Name:       "unique-action-name",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateAction(ctx, action); err != nil {
			t.Fatalf("CreateAction: %v", err)
		}

		got, err := q.GetActionByName(ctx, orgID, action.Name)
		if err != nil {
			t.Fatalf("GetActionByName: %v", err)
		}
		if got.ID != action.ID {
			t.Errorf("ID: got %q, want %q", got.ID, action.ID)
		}
	})

	t.Run("create action duplicate name returns ConflictError", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		a1 := &domain.Action{
			ID:         "action-dup-1",
			OrgID:      orgID,
			Name:       "dup-action",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateAction(ctx, a1); err != nil {
			t.Fatalf("CreateAction(first): %v", err)
		}

		a2 := &domain.Action{
			ID:         "action-dup-2",
			OrgID:      orgID,
			Name:       "dup-action", // same name
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		err := q.CreateAction(ctx, a2)
		if err == nil {
			t.Fatal("expected ConflictError for duplicate action name, got nil")
		}
		if _, ok := err.(*domain.ConflictError); !ok {
			t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
		}
	})

	t.Run("update action", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		action := &domain.Action{
			ID:         "action-to-update",
			OrgID:      orgID,
			Name:       "action-update-original",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{"url": "https://old.example.com"},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateAction(ctx, action); err != nil {
			t.Fatalf("CreateAction: %v", err)
		}

		action.Name = "action-update-new"
		action.Config = map[string]any{"url": "https://new.example.com"}
		action.Version = 2

		if err := q.UpdateAction(ctx, action); err != nil {
			t.Fatalf("UpdateAction: %v", err)
		}

		got, err := q.GetAction(ctx, orgID, action.ID)
		if err != nil {
			t.Fatalf("GetAction after update: %v", err)
		}
		if got.Name != "action-update-new" {
			t.Errorf("Name: got %q, want %q", got.Name, "action-update-new")
		}
		if got.Config["url"] != "https://new.example.com" {
			t.Errorf("Config[url]: got %v, want %q", got.Config["url"], "https://new.example.com")
		}
	})

	t.Run("delete action", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		action := &domain.Action{
			ID:         "action-to-delete",
			OrgID:      orgID,
			Name:       "delete-action",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{},
			Version:    1,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateAction(ctx, action); err != nil {
			t.Fatalf("CreateAction: %v", err)
		}

		if err := q.DeleteAction(ctx, orgID, action.ID); err != nil {
			t.Fatalf("DeleteAction: %v", err)
		}

		_, err := q.GetAction(ctx, orgID, action.ID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		if _, ok := err.(*domain.NotFoundError); !ok {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestActions_ItemTypes(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "action-item-types-org")

	now := time.Now().UTC().Truncate(time.Microsecond)
	action := &domain.Action{
		ID:         "action-it-001",
		OrgID:      orgID,
		Name:       "action-with-item-types",
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{},
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := q.CreateAction(ctx, action); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}

	it1 := seedItemType(t, q, orgID, "item-type-for-action-1")
	it2 := seedItemType(t, q, orgID, "item-type-for-action-2")

	t.Run("set and get action item types", func(t *testing.T) {
		if err := q.SetActionItemTypes(ctx, action.ID, []string{it1.ID, it2.ID}); err != nil {
			t.Fatalf("SetActionItemTypes: %v", err)
		}

		ids, err := q.GetActionItemTypes(ctx, action.ID)
		if err != nil {
			t.Fatalf("GetActionItemTypes: %v", err)
		}
		if len(ids) != 2 {
			t.Errorf("item type count: got %d, want 2", len(ids))
		}
	})

	t.Run("set empty clears item types", func(t *testing.T) {
		if err := q.SetActionItemTypes(ctx, action.ID, []string{}); err != nil {
			t.Fatalf("SetActionItemTypes empty: %v", err)
		}

		ids, err := q.GetActionItemTypes(ctx, action.ID)
		if err != nil {
			t.Fatalf("GetActionItemTypes after clear: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("expected 0 item types after clear, got %d", len(ids))
		}
	})
}

func TestPolicies_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "policies-crud-org")

	t.Run("create and get policy", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		policy := &domain.Policy{
			ID:            "policy-001",
			OrgID:         orgID,
			Name:          "Spam Policy",
			Description:   "Policy for spam violations",
			ParentID:      nil,
			StrikePenalty: 5,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := q.CreatePolicy(ctx, policy); err != nil {
			t.Fatalf("CreatePolicy: %v", err)
		}

		got, err := q.GetPolicy(ctx, orgID, policy.ID)
		if err != nil {
			t.Fatalf("GetPolicy: %v", err)
		}

		if got.ID != policy.ID {
			t.Errorf("ID: got %q, want %q", got.ID, policy.ID)
		}
		if got.Name != policy.Name {
			t.Errorf("Name: got %q, want %q", got.Name, policy.Name)
		}
		if got.Description != policy.Description {
			t.Errorf("Description: got %q, want %q", got.Description, policy.Description)
		}
		if got.ParentID != nil {
			t.Errorf("ParentID: expected nil, got %v", got.ParentID)
		}
		if got.StrikePenalty != policy.StrikePenalty {
			t.Errorf("StrikePenalty: got %d, want %d", got.StrikePenalty, policy.StrikePenalty)
		}
	})

	t.Run("policy with parent_id", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		parent := &domain.Policy{
			ID:            "parent-policy",
			OrgID:         orgID,
			Name:          "Parent Policy",
			StrikePenalty: 1,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := q.CreatePolicy(ctx, parent); err != nil {
			t.Fatalf("CreatePolicy(parent): %v", err)
		}

		parentID := parent.ID
		child := &domain.Policy{
			ID:            "child-policy",
			OrgID:         orgID,
			Name:          "Child Policy",
			ParentID:      &parentID,
			StrikePenalty: 2,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := q.CreatePolicy(ctx, child); err != nil {
			t.Fatalf("CreatePolicy(child): %v", err)
		}

		got, err := q.GetPolicy(ctx, orgID, child.ID)
		if err != nil {
			t.Fatalf("GetPolicy(child): %v", err)
		}
		if got.ParentID == nil {
			t.Fatal("ParentID: expected non-nil, got nil")
		}
		if *got.ParentID != parentID {
			t.Errorf("ParentID: got %q, want %q", *got.ParentID, parentID)
		}
	})

	t.Run("update policy", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		policy := &domain.Policy{
			ID:            "policy-to-update",
			OrgID:         orgID,
			Name:          "Old Policy Name",
			StrikePenalty: 1,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := q.CreatePolicy(ctx, policy); err != nil {
			t.Fatalf("CreatePolicy: %v", err)
		}

		policy.Name = "New Policy Name"
		policy.StrikePenalty = 10
		policy.Version = 2

		if err := q.UpdatePolicy(ctx, policy); err != nil {
			t.Fatalf("UpdatePolicy: %v", err)
		}

		got, err := q.GetPolicy(ctx, orgID, policy.ID)
		if err != nil {
			t.Fatalf("GetPolicy after update: %v", err)
		}
		if got.Name != "New Policy Name" {
			t.Errorf("Name: got %q, want %q", got.Name, "New Policy Name")
		}
		if got.StrikePenalty != 10 {
			t.Errorf("StrikePenalty: got %d, want 10", got.StrikePenalty)
		}
	})

	t.Run("delete policy", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		policy := &domain.Policy{
			ID:            "policy-to-delete",
			OrgID:         orgID,
			Name:          "Delete Policy",
			StrikePenalty: 0,
			Version:       1,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := q.CreatePolicy(ctx, policy); err != nil {
			t.Fatalf("CreatePolicy: %v", err)
		}

		if err := q.DeletePolicy(ctx, orgID, policy.ID); err != nil {
			t.Fatalf("DeletePolicy: %v", err)
		}

		_, err := q.GetPolicy(ctx, orgID, policy.ID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		if _, ok := err.(*domain.NotFoundError); !ok {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestItemTypes_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "item-types-crud-org")

	t.Run("create and get item type", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		it := &domain.ItemType{
			ID:         "it-001",
			OrgID:      orgID,
			Name:       "text-content",
			Kind:       domain.ItemTypeKindContent,
			Schema:     map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
			FieldRoles: map[string]any{"text_field": "text"},
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := q.CreateItemType(ctx, it); err != nil {
			t.Fatalf("CreateItemType: %v", err)
		}

		got, err := q.GetItemType(ctx, orgID, it.ID)
		if err != nil {
			t.Fatalf("GetItemType: %v", err)
		}

		if got.ID != it.ID {
			t.Errorf("ID: got %q, want %q", got.ID, it.ID)
		}
		if got.Name != it.Name {
			t.Errorf("Name: got %q, want %q", got.Name, it.Name)
		}
		if got.Kind != it.Kind {
			t.Errorf("Kind: got %q, want %q", got.Kind, it.Kind)
		}
		// JSONB schema round-trip.
		if got.Schema["type"] != "object" {
			t.Errorf("Schema[type]: got %v, want %q", got.Schema["type"], "object")
		}
		// JSONB field_roles round-trip.
		if got.FieldRoles["text_field"] != "text" {
			t.Errorf("FieldRoles[text_field]: got %v, want %q", got.FieldRoles["text_field"], "text")
		}
	})

	t.Run("create item type duplicate name returns ConflictError", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		it1 := &domain.ItemType{
			ID:         "it-dup-1",
			OrgID:      orgID,
			Name:       "dup-item-type",
			Kind:       domain.ItemTypeKindContent,
			Schema:     map[string]any{},
			FieldRoles: map[string]any{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateItemType(ctx, it1); err != nil {
			t.Fatalf("CreateItemType(first): %v", err)
		}

		it2 := &domain.ItemType{
			ID:         "it-dup-2",
			OrgID:      orgID,
			Name:       "dup-item-type", // same name
			Kind:       domain.ItemTypeKindUser,
			Schema:     map[string]any{},
			FieldRoles: map[string]any{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		err := q.CreateItemType(ctx, it2)
		if err == nil {
			t.Fatal("expected ConflictError for duplicate item type name, got nil")
		}
		if _, ok := err.(*domain.ConflictError); !ok {
			t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
		}
	})

	t.Run("update item type", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		it := &domain.ItemType{
			ID:         "it-to-update",
			OrgID:      orgID,
			Name:       "original-item-type",
			Kind:       domain.ItemTypeKindContent,
			Schema:     map[string]any{"v": "1"},
			FieldRoles: map[string]any{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateItemType(ctx, it); err != nil {
			t.Fatalf("CreateItemType: %v", err)
		}

		it.Name = "updated-item-type"
		it.Schema = map[string]any{"v": "2"}

		if err := q.UpdateItemType(ctx, it); err != nil {
			t.Fatalf("UpdateItemType: %v", err)
		}

		got, err := q.GetItemType(ctx, orgID, it.ID)
		if err != nil {
			t.Fatalf("GetItemType after update: %v", err)
		}
		if got.Name != "updated-item-type" {
			t.Errorf("Name: got %q, want %q", got.Name, "updated-item-type")
		}
		if got.Schema["v"] != "2" {
			t.Errorf("Schema[v]: got %v, want %q", got.Schema["v"], "2")
		}
	})

	t.Run("delete item type", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		it := &domain.ItemType{
			ID:         "it-to-delete",
			OrgID:      orgID,
			Name:       "delete-item-type",
			Kind:       domain.ItemTypeKindThread,
			Schema:     map[string]any{},
			FieldRoles: map[string]any{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := q.CreateItemType(ctx, it); err != nil {
			t.Fatalf("CreateItemType: %v", err)
		}

		if err := q.DeleteItemType(ctx, orgID, it.ID); err != nil {
			t.Fatalf("DeleteItemType: %v", err)
		}

		_, err := q.GetItemType(ctx, orgID, it.ID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		if _, ok := err.(*domain.NotFoundError); !ok {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}
