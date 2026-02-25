package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// ---- Actions ----------------------------------------------------------------

// CreateActionParams holds inputs required to create a new action.
type CreateActionParams struct {
	Name        string
	ActionType  domain.ActionType
	Config      map[string]any
	ItemTypeIDs []string
}

// UpdateActionParams holds optional fields that may be changed on an action update.
// A nil pointer means "do not change this field".
type UpdateActionParams struct {
	Name        *string
	ActionType  *domain.ActionType
	Config      *map[string]any
	ItemTypeIDs *[]string
}

// ---- Policies ---------------------------------------------------------------

// CreatePolicyParams holds inputs required to create a new policy.
type CreatePolicyParams struct {
	Name          string
	Description   *string
	ParentID      *string
	StrikePenalty int
}

// UpdatePolicyParams holds optional fields that may be changed on a policy update.
// A nil pointer means "do not change this field".
type UpdatePolicyParams struct {
	Name          *string
	Description   *string
	ParentID      *string
	StrikePenalty *int
}

// ---- Item Types -------------------------------------------------------------

// CreateItemTypeParams holds inputs required to create a new item type.
type CreateItemTypeParams struct {
	Name       string
	Kind       domain.ItemTypeKind
	Schema     map[string]any
	FieldRoles map[string]any
}

// UpdateItemTypeParams holds optional fields that may be changed on an item type update.
// A nil pointer means "do not change this field".
type UpdateItemTypeParams struct {
	Name       *string
	Kind       *domain.ItemTypeKind
	Schema     *map[string]any
	FieldRoles *map[string]any
}

// ---- ConfigService ----------------------------------------------------------

// ConfigService manages the CRUD lifecycle of actions, policies, and item types.
// Actions and policies are versioned (entity history tracked). Item types are not.
type ConfigService struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewConfigService constructs a ConfigService with the required dependencies.
//
// Pre-conditions: st and logger must be non-nil.
// Post-conditions: returned ConfigService is ready for use.
func NewConfigService(st *store.Queries, logger *slog.Logger) *ConfigService {
	return &ConfigService{store: st, logger: logger}
}

// ---- Action methods ---------------------------------------------------------

// CreateAction validates, persists a new action, writes entity history in the same
// transaction, and sets item type associations if provided.
//
// Pre-conditions: orgID non-empty; params.Name non-empty; params.ActionType is valid.
// Post-conditions: action persisted with Version=1; entity history written.
// Raises: *domain.ValidationError for invalid params; *domain.ConflictError on duplicate name.
func (s *ConfigService) CreateAction(ctx context.Context, orgID string, params CreateActionParams) (*domain.Action, error) {
	if err := validateCreateActionParams(params); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	action := domain.Action{
		ID:         fmt.Sprintf("act_%d", now.UnixNano()),
		OrgID:      orgID,
		Name:       params.Name,
		ActionType: params.ActionType,
		Config:     params.Config,
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if action.Config == nil {
		action.Config = map[string]any{}
	}

	itemTypeIDs := params.ItemTypeIDs
	if itemTypeIDs == nil {
		itemTypeIDs = []string{}
	}

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.CreateAction(ctx, &action); err != nil {
			return err
		}
		if len(itemTypeIDs) > 0 {
			if err := tx.SetActionItemTypes(ctx, action.ID, itemTypeIDs); err != nil {
				return fmt.Errorf("set action item types: %w", err)
			}
		}
		if err := tx.InsertEntityHistory(ctx, "action", action.ID, orgID, 1, action); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("config.CreateAction: %w", err)
	}

	s.logger.Info("action created", "org_id", orgID, "action_id", action.ID, "name", action.Name)
	return &action, nil
}

// UpdateAction fetches the existing action, applies non-nil param fields,
// increments the version, and persists with entity history.
//
// Pre-conditions: orgID and actionID non-empty.
// Post-conditions: action updated; version incremented; entity history written.
// Raises: *domain.NotFoundError if action does not exist.
func (s *ConfigService) UpdateAction(ctx context.Context, orgID, actionID string, params UpdateActionParams) (*domain.Action, error) {
	existing, err := s.store.GetAction(ctx, orgID, actionID)
	if err != nil {
		return nil, err
	}

	if params.Name != nil {
		existing.Name = *params.Name
	}
	if params.ActionType != nil {
		existing.ActionType = *params.ActionType
	}
	if params.Config != nil {
		existing.Config = *params.Config
	}
	existing.Version++

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.UpdateAction(ctx, existing); err != nil {
			return err
		}
		if params.ItemTypeIDs != nil {
			if err := tx.SetActionItemTypes(ctx, actionID, *params.ItemTypeIDs); err != nil {
				return fmt.Errorf("set action item types: %w", err)
			}
		}
		if err := tx.InsertEntityHistory(ctx, "action", actionID, orgID, existing.Version, existing); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("config.UpdateAction: %w", err)
	}

	s.logger.Info("action updated", "org_id", orgID, "action_id", actionID, "version", existing.Version)
	return existing, nil
}

// DeleteAction removes an action by org and action ID.
//
// Pre-conditions: orgID and actionID non-empty.
// Post-conditions: action is deleted.
// Raises: *domain.NotFoundError if action does not exist.
func (s *ConfigService) DeleteAction(ctx context.Context, orgID, actionID string) error {
	return s.store.DeleteAction(ctx, orgID, actionID)
}

// GetAction returns a single action by org and action ID.
//
// Pre-conditions: orgID and actionID non-empty.
// Post-conditions: returns the action if found.
// Raises: *domain.NotFoundError if not found.
func (s *ConfigService) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error) {
	return s.store.GetAction(ctx, orgID, actionID)
}

// ListActions returns a paginated list of actions for an org.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns paginated result.
// Raises: error on database failure.
func (s *ConfigService) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error) {
	return s.store.ListActions(ctx, orgID, page)
}

// ---- Policy methods ---------------------------------------------------------

// CreatePolicy validates, persists a new policy, and writes entity history in the same transaction.
//
// Pre-conditions: orgID non-empty; params.Name non-empty.
// Post-conditions: policy persisted with Version=1; entity history written.
// Raises: *domain.ValidationError for invalid params.
func (s *ConfigService) CreatePolicy(ctx context.Context, orgID string, params CreatePolicyParams) (*domain.Policy, error) {
	if params.Name == "" {
		return nil, &domain.ValidationError{
			Message: "policy name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}

	description := ""
	if params.Description != nil {
		description = *params.Description
	}

	now := time.Now().UTC()
	policy := domain.Policy{
		ID:            fmt.Sprintf("pol_%d", now.UnixNano()),
		OrgID:         orgID,
		Name:          params.Name,
		Description:   description,
		ParentID:      params.ParentID,
		StrikePenalty: params.StrikePenalty,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.CreatePolicy(ctx, &policy); err != nil {
			return err
		}
		if err := tx.InsertEntityHistory(ctx, "policy", policy.ID, orgID, 1, policy); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("config.CreatePolicy: %w", err)
	}

	s.logger.Info("policy created", "org_id", orgID, "policy_id", policy.ID, "name", policy.Name)
	return &policy, nil
}

// UpdatePolicy fetches the existing policy, applies non-nil param fields,
// increments the version, and persists with entity history.
//
// Pre-conditions: orgID and policyID non-empty.
// Post-conditions: policy updated; version incremented; entity history written.
// Raises: *domain.NotFoundError if policy does not exist.
func (s *ConfigService) UpdatePolicy(ctx context.Context, orgID, policyID string, params UpdatePolicyParams) (*domain.Policy, error) {
	existing, err := s.store.GetPolicy(ctx, orgID, policyID)
	if err != nil {
		return nil, err
	}

	if params.Name != nil {
		existing.Name = *params.Name
	}
	if params.Description != nil {
		existing.Description = *params.Description
	}
	if params.ParentID != nil {
		existing.ParentID = params.ParentID
	}
	if params.StrikePenalty != nil {
		existing.StrikePenalty = *params.StrikePenalty
	}
	existing.Version++

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.UpdatePolicy(ctx, existing); err != nil {
			return err
		}
		if err := tx.InsertEntityHistory(ctx, "policy", policyID, orgID, existing.Version, existing); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("config.UpdatePolicy: %w", err)
	}

	s.logger.Info("policy updated", "org_id", orgID, "policy_id", policyID, "version", existing.Version)
	return existing, nil
}

// DeletePolicy removes a policy by org and policy ID.
//
// Pre-conditions: orgID and policyID non-empty.
// Post-conditions: policy is deleted.
// Raises: *domain.NotFoundError if policy does not exist.
func (s *ConfigService) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	return s.store.DeletePolicy(ctx, orgID, policyID)
}

// GetPolicy returns a single policy by org and policy ID.
//
// Pre-conditions: orgID and policyID non-empty.
// Post-conditions: returns the policy if found.
// Raises: *domain.NotFoundError if not found.
func (s *ConfigService) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error) {
	return s.store.GetPolicy(ctx, orgID, policyID)
}

// ListPolicies returns a paginated list of policies for an org.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns paginated result.
// Raises: error on database failure.
func (s *ConfigService) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error) {
	return s.store.ListPolicies(ctx, orgID, page)
}

// ---- Item Type methods ------------------------------------------------------

// CreateItemType validates and persists a new item type. Item types are not versioned
// and do not receive entity history entries.
//
// Pre-conditions: orgID non-empty; params.Name non-empty; params.Kind is valid.
// Post-conditions: item type is persisted.
// Raises: *domain.ValidationError for invalid params; *domain.ConflictError on duplicate name.
func (s *ConfigService) CreateItemType(ctx context.Context, orgID string, params CreateItemTypeParams) (*domain.ItemType, error) {
	if err := validateCreateItemTypeParams(params); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	itemType := domain.ItemType{
		ID:         fmt.Sprintf("ity_%d", now.UnixNano()),
		OrgID:      orgID,
		Name:       params.Name,
		Kind:       params.Kind,
		Schema:     params.Schema,
		FieldRoles: params.FieldRoles,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if itemType.Schema == nil {
		itemType.Schema = map[string]any{}
	}
	if itemType.FieldRoles == nil {
		itemType.FieldRoles = map[string]any{}
	}

	if err := s.store.CreateItemType(ctx, &itemType); err != nil {
		return nil, fmt.Errorf("config.CreateItemType: %w", err)
	}

	s.logger.Info("item type created", "org_id", orgID, "item_type_id", itemType.ID, "name", itemType.Name)
	return &itemType, nil
}

// UpdateItemType fetches the existing item type and applies non-nil param fields.
// Item types have no version or entity history.
//
// Pre-conditions: orgID and itemTypeID non-empty.
// Post-conditions: item type is updated.
// Raises: *domain.NotFoundError if item type does not exist.
func (s *ConfigService) UpdateItemType(ctx context.Context, orgID, itemTypeID string, params UpdateItemTypeParams) (*domain.ItemType, error) {
	existing, err := s.store.GetItemType(ctx, orgID, itemTypeID)
	if err != nil {
		return nil, err
	}

	if params.Name != nil {
		existing.Name = *params.Name
	}
	if params.Kind != nil {
		existing.Kind = *params.Kind
	}
	if params.Schema != nil {
		existing.Schema = *params.Schema
	}
	if params.FieldRoles != nil {
		existing.FieldRoles = *params.FieldRoles
	}

	if err := s.store.UpdateItemType(ctx, existing); err != nil {
		return nil, fmt.Errorf("config.UpdateItemType: %w", err)
	}

	s.logger.Info("item type updated", "org_id", orgID, "item_type_id", itemTypeID)
	return existing, nil
}

// DeleteItemType removes an item type by org and item type ID.
//
// Pre-conditions: orgID and itemTypeID non-empty.
// Post-conditions: item type is deleted.
// Raises: *domain.NotFoundError if item type does not exist.
func (s *ConfigService) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error {
	return s.store.DeleteItemType(ctx, orgID, itemTypeID)
}

// GetItemType returns a single item type by org and item type ID.
//
// Pre-conditions: orgID and itemTypeID non-empty.
// Post-conditions: returns the item type if found.
// Raises: *domain.NotFoundError if not found.
func (s *ConfigService) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error) {
	return s.store.GetItemType(ctx, orgID, itemTypeID)
}

// ListItemTypes returns a paginated list of item types for an org.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns paginated result.
// Raises: error on database failure.
func (s *ConfigService) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error) {
	return s.store.ListItemTypes(ctx, orgID, page)
}

// ---- Org settings -----------------------------------------------------------

// GetOrgSettings returns the settings map for an org.
// This is a thin wrapper that retrieves the org and returns its Settings field.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns the org's Settings map.
// Raises: *domain.NotFoundError if org does not exist; error on database failure.
func (s *ConfigService) GetOrgSettings(ctx context.Context, orgID string) (map[string]any, error) {
	org, err := s.store.GetOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("config.GetOrgSettings: %w", err)
	}
	return org.Settings, nil
}

// ---- Validation helpers -----------------------------------------------------

// validateCreateActionParams validates required fields of CreateActionParams.
func validateCreateActionParams(params CreateActionParams) error {
	if params.Name == "" {
		return &domain.ValidationError{
			Message: "action name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}
	switch params.ActionType {
	case domain.ActionTypeWebhook, domain.ActionTypeEnqueueToMRT:
		// valid
	default:
		return &domain.ValidationError{
			Message: fmt.Sprintf("invalid action type %q", params.ActionType),
			Details: map[string]string{"action_type": "must be WEBHOOK or ENQUEUE_TO_MRT"},
		}
	}
	return nil
}

// validateCreateItemTypeParams validates required fields of CreateItemTypeParams.
func validateCreateItemTypeParams(params CreateItemTypeParams) error {
	if params.Name == "" {
		return &domain.ValidationError{
			Message: "item type name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}
	switch params.Kind {
	case domain.ItemTypeKindContent, domain.ItemTypeKindUser, domain.ItemTypeKindThread:
		// valid
	default:
		return &domain.ValidationError{
			Message: fmt.Sprintf("invalid item type kind %q", params.Kind),
			Details: map[string]string{"kind": "must be CONTENT, USER, or THREAD"},
		}
	}
	return nil
}
