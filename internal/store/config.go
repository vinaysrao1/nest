package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

// ---- Actions ----------------------------------------------------------------

const (
	sqlActionColumns = `id, org_id, name, action_type, config, version, created_at, updated_at`

	sqlListActionsCount = `SELECT COUNT(*) FROM actions WHERE org_id = $1`
	sqlListActions      = `SELECT ` + sqlActionColumns + ` FROM actions WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	sqlListAllActions   = `SELECT ` + sqlActionColumns + ` FROM actions WHERE org_id = $1 ORDER BY name ASC`

	sqlGetAction       = `SELECT ` + sqlActionColumns + ` FROM actions WHERE org_id = $1 AND id = $2`
	sqlGetActionByName = `SELECT ` + sqlActionColumns + ` FROM actions WHERE org_id = $1 AND name = $2`

	sqlCreateAction = `INSERT INTO actions (id, org_id, name, action_type, config, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	sqlUpdateAction = `UPDATE actions SET name=$3, action_type=$4, config=$5, version=$6, updated_at=now()
		WHERE org_id = $1 AND id = $2`

	sqlDeleteAction = `DELETE FROM actions WHERE org_id = $1 AND id = $2`

	sqlDeleteActionItemTypes = `DELETE FROM actions_item_types WHERE action_id = $1`
	sqlInsertActionItemType  = `INSERT INTO actions_item_types (action_id, item_type_id) VALUES ($1, $2)`
	sqlGetActionItemTypes    = `SELECT item_type_id FROM actions_item_types WHERE action_id = $1`
)

// scanAction reads a single Action from a pgx row.
func scanAction(row rowScanner) (*domain.Action, error) {
	var a domain.Action
	err := row.Scan(
		&a.ID,
		&a.OrgID,
		&a.Name,
		&a.ActionType,
		&a.Config,
		&a.Version,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if a.Config == nil {
		a.Config = map[string]any{}
	}
	return &a, nil
}

// ListActions returns a paginated list of actions for an org, ordered by created_at DESC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all actions for org with pagination metadata.
// Raises: error on database failure.
func (q *Queries) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error) {
	var total int
	if err := q.dbtx.QueryRow(ctx, sqlListActionsCount, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count actions: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, sqlListActions, orgID, paginationLimit(page), paginationOffset(page))
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	defer rows.Close()

	var actions []domain.Action
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan action: %w", err)
		}
		actions = append(actions, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate actions: %w", err)
	}

	return buildPaginatedResult(actions, total, page), nil
}

// ListAllActionsByOrg returns all actions for an org as a map from action name to Action.
// This is intended for use during snapshot builds where pagination is not appropriate.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns map of all actions for org; empty map if none.
// Raises: error on database failure.
func (q *Queries) ListAllActionsByOrg(ctx context.Context, orgID string) (map[string]domain.Action, error) {
	rows, err := q.dbtx.Query(ctx, sqlListAllActions, orgID)
	if err != nil {
		return nil, fmt.Errorf("list all actions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]domain.Action)
	for rows.Next() {
		a, scanErr := scanAction(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan action: %w", scanErr)
		}
		result[a.Name] = *a
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate actions: %w", err)
	}
	return result, nil
}

// GetAction returns a single action by org and action ID.
//
// Pre-conditions: orgID and actionID must be non-empty.
// Post-conditions: returns the action if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetAction, orgID, actionID)
	a, err := scanAction(row)
	if err != nil {
		return nil, notFound(err, "action", actionID)
	}
	return a, nil
}

// GetActionByName returns an action by its unique (org_id, name).
//
// Pre-conditions: orgID and name must be non-empty.
// Post-conditions: returns the action if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetActionByName(ctx context.Context, orgID, name string) (*domain.Action, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetActionByName, orgID, name)
	a, err := scanAction(row)
	if err != nil {
		return nil, notFound(err, "action", name)
	}
	return a, nil
}

// CreateAction inserts a new action.
//
// Pre-conditions: action.ID and action.OrgID must be set.
// Post-conditions: action is persisted.
// Raises: domain.ConflictError if (org_id, name) unique constraint violated.
func (q *Queries) CreateAction(ctx context.Context, action *domain.Action) error {
	cfg := action.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	_, err := q.dbtx.Exec(ctx, sqlCreateAction,
		action.ID,
		action.OrgID,
		action.Name,
		action.ActionType,
		cfg,
		action.Version,
		action.CreatedAt,
		action.UpdatedAt,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("action %s already exists in org", action.Name))
	}
	return nil
}

// UpdateAction updates an existing action.
//
// Pre-conditions: action.ID and action.OrgID must be set.
// Post-conditions: action is updated. updated_at set to now().
// Raises: domain.NotFoundError if not found.
func (q *Queries) UpdateAction(ctx context.Context, action *domain.Action) error {
	cfg := action.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	tag, err := q.dbtx.Exec(ctx, sqlUpdateAction,
		action.OrgID,
		action.ID,
		action.Name,
		action.ActionType,
		cfg,
		action.Version,
	)
	if err != nil {
		return fmt.Errorf("update action: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("action %s not found", action.ID)}
	}
	return nil
}

// DeleteAction removes an action.
//
// Pre-conditions: orgID and actionID must be non-empty.
// Post-conditions: action is deleted.
// Raises: domain.NotFoundError if not found.
func (q *Queries) DeleteAction(ctx context.Context, orgID, actionID string) error {
	tag, err := q.dbtx.Exec(ctx, sqlDeleteAction, orgID, actionID)
	if err != nil {
		return fmt.Errorf("delete action: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("action %s not found", actionID)}
	}
	return nil
}

// SetActionItemTypes replaces the set of item types associated with an action.
// Deletes all existing associations then inserts the new set.
//
// Pre-conditions: actionID must exist. itemTypeIDs may be empty.
// Post-conditions: actions_item_types contains exactly the given set for this actionID.
// Raises: error on database failure.
func (q *Queries) SetActionItemTypes(ctx context.Context, actionID string, itemTypeIDs []string) error {
	if _, err := q.dbtx.Exec(ctx, sqlDeleteActionItemTypes, actionID); err != nil {
		return fmt.Errorf("delete action item types: %w", err)
	}
	for _, itID := range itemTypeIDs {
		if _, err := q.dbtx.Exec(ctx, sqlInsertActionItemType, actionID, itID); err != nil {
			return fmt.Errorf("insert action item type %s: %w", itID, err)
		}
	}
	return nil
}

// GetActionItemTypes returns item type IDs associated with an action.
//
// Pre-conditions: actionID must be non-empty.
// Post-conditions: returns slice of item type IDs (empty if none).
// Raises: error on database failure.
func (q *Queries) GetActionItemTypes(ctx context.Context, actionID string) ([]string, error) {
	rows, err := q.dbtx.Query(ctx, sqlGetActionItemTypes, actionID)
	if err != nil {
		return nil, fmt.Errorf("get action item types: %w", err)
	}
	defer rows.Close()

	var itemTypeIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan item type id: %w", err)
		}
		itemTypeIDs = append(itemTypeIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate action item types: %w", err)
	}
	if itemTypeIDs == nil {
		itemTypeIDs = []string{}
	}
	return itemTypeIDs, nil
}

// ---- Policies ---------------------------------------------------------------

const (
	sqlPolicyColumns = `id, org_id, name, COALESCE(description, ''), parent_id, strike_penalty, version, created_at, updated_at`

	sqlListPoliciesCount = `SELECT COUNT(*) FROM policies WHERE org_id = $1`
	sqlListPolicies      = `SELECT ` + sqlPolicyColumns + ` FROM policies WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	sqlGetPolicy = `SELECT ` + sqlPolicyColumns + ` FROM policies WHERE org_id = $1 AND id = $2`

	sqlCreatePolicy = `INSERT INTO policies (id, org_id, name, description, parent_id, strike_penalty, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	sqlUpdatePolicy = `UPDATE policies SET name=$3, description=$4, parent_id=$5, strike_penalty=$6,
		version=$7, updated_at=now() WHERE org_id = $1 AND id = $2`

	sqlDeletePolicy = `DELETE FROM policies WHERE org_id = $1 AND id = $2`
)

// scanPolicy reads a single Policy from a pgx row.
func scanPolicy(row rowScanner) (*domain.Policy, error) {
	var p domain.Policy
	err := row.Scan(
		&p.ID,
		&p.OrgID,
		&p.Name,
		&p.Description,
		&p.ParentID,
		&p.StrikePenalty,
		&p.Version,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPolicies returns a paginated list of policies for an org, ordered by created_at DESC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all policies for org with pagination metadata.
// Raises: error on database failure.
func (q *Queries) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error) {
	var total int
	if err := q.dbtx.QueryRow(ctx, sqlListPoliciesCount, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count policies: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, sqlListPolicies, orgID, paginationLimit(page), paginationOffset(page))
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()

	var policies []domain.Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		policies = append(policies, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate policies: %w", err)
	}

	return buildPaginatedResult(policies, total, page), nil
}

// GetPolicy returns a single policy by org and policy ID.
//
// Pre-conditions: orgID and policyID must be non-empty.
// Post-conditions: returns the policy if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetPolicy, orgID, policyID)
	p, err := scanPolicy(row)
	if err != nil {
		return nil, notFound(err, "policy", policyID)
	}
	return p, nil
}

// CreatePolicy inserts a new policy.
//
// Pre-conditions: policy.ID and policy.OrgID must be set.
// Post-conditions: policy is persisted.
// Raises: error on database failure.
func (q *Queries) CreatePolicy(ctx context.Context, policy *domain.Policy) error {
	_, err := q.dbtx.Exec(ctx, sqlCreatePolicy,
		policy.ID,
		policy.OrgID,
		policy.Name,
		policy.Description,
		policy.ParentID,
		policy.StrikePenalty,
		policy.Version,
		policy.CreatedAt,
		policy.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create policy: %w", err)
	}
	return nil
}

// UpdatePolicy updates an existing policy.
//
// Pre-conditions: policy.ID and policy.OrgID must be set.
// Post-conditions: policy is updated. updated_at set to now().
// Raises: domain.NotFoundError if not found.
func (q *Queries) UpdatePolicy(ctx context.Context, policy *domain.Policy) error {
	tag, err := q.dbtx.Exec(ctx, sqlUpdatePolicy,
		policy.OrgID,
		policy.ID,
		policy.Name,
		policy.Description,
		policy.ParentID,
		policy.StrikePenalty,
		policy.Version,
	)
	if err != nil {
		return fmt.Errorf("update policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("policy %s not found", policy.ID)}
	}
	return nil
}

// DeletePolicy removes a policy.
//
// Pre-conditions: orgID and policyID must be non-empty.
// Post-conditions: policy is deleted.
// Raises: domain.NotFoundError if not found.
func (q *Queries) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	tag, err := q.dbtx.Exec(ctx, sqlDeletePolicy, orgID, policyID)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("policy %s not found", policyID)}
	}
	return nil
}

// ---- Item Types -------------------------------------------------------------

const (
	sqlItemTypeColumns = `id, org_id, name, kind, schema, field_roles, created_at, updated_at`

	sqlListItemTypesCount = `SELECT COUNT(*) FROM item_types WHERE org_id = $1`
	sqlListItemTypes      = `SELECT ` + sqlItemTypeColumns + ` FROM item_types WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	sqlGetItemType = `SELECT ` + sqlItemTypeColumns + ` FROM item_types WHERE org_id = $1 AND id = $2`

	sqlCreateItemType = `INSERT INTO item_types (id, org_id, name, kind, schema, field_roles, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	sqlUpdateItemType = `UPDATE item_types SET name=$3, kind=$4, schema=$5, field_roles=$6, updated_at=now()
		WHERE org_id = $1 AND id = $2`

	sqlDeleteItemType = `DELETE FROM item_types WHERE org_id = $1 AND id = $2`
)

// scanItemType reads a single ItemType from a pgx row.
func scanItemType(row rowScanner) (*domain.ItemType, error) {
	var it domain.ItemType
	err := row.Scan(
		&it.ID,
		&it.OrgID,
		&it.Name,
		&it.Kind,
		&it.Schema,
		&it.FieldRoles,
		&it.CreatedAt,
		&it.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if it.Schema == nil {
		it.Schema = map[string]any{}
	}
	if it.FieldRoles == nil {
		it.FieldRoles = map[string]any{}
	}
	return &it, nil
}

// ListItemTypes returns a paginated list of item types for an org, ordered by created_at DESC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all item types for org with pagination metadata.
// Raises: error on database failure.
func (q *Queries) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error) {
	var total int
	if err := q.dbtx.QueryRow(ctx, sqlListItemTypesCount, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count item types: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, sqlListItemTypes, orgID, paginationLimit(page), paginationOffset(page))
	if err != nil {
		return nil, fmt.Errorf("list item types: %w", err)
	}
	defer rows.Close()

	var itemTypes []domain.ItemType
	for rows.Next() {
		it, err := scanItemType(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item type: %w", err)
		}
		itemTypes = append(itemTypes, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item types: %w", err)
	}

	return buildPaginatedResult(itemTypes, total, page), nil
}

// GetItemType returns a single item type by org and item type ID.
//
// Pre-conditions: orgID and itemTypeID must be non-empty.
// Post-conditions: returns the item type if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetItemType, orgID, itemTypeID)
	it, err := scanItemType(row)
	if err != nil {
		return nil, notFound(err, "item_type", itemTypeID)
	}
	return it, nil
}

// CreateItemType inserts a new item type.
//
// Pre-conditions: itemType.ID and itemType.OrgID must be set.
// Post-conditions: item type is persisted.
// Raises: domain.ConflictError if (org_id, name) unique constraint violated.
func (q *Queries) CreateItemType(ctx context.Context, itemType *domain.ItemType) error {
	schema := itemType.Schema
	if schema == nil {
		schema = map[string]any{}
	}
	fieldRoles := itemType.FieldRoles
	if fieldRoles == nil {
		fieldRoles = map[string]any{}
	}
	_, err := q.dbtx.Exec(ctx, sqlCreateItemType,
		itemType.ID,
		itemType.OrgID,
		itemType.Name,
		itemType.Kind,
		schema,
		fieldRoles,
		itemType.CreatedAt,
		itemType.UpdatedAt,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("item_type %s already exists in org", itemType.Name))
	}
	return nil
}

// UpdateItemType updates an existing item type.
//
// Pre-conditions: itemType.ID and itemType.OrgID must be set.
// Post-conditions: item type is updated. updated_at set to now().
// Raises: domain.NotFoundError if not found.
func (q *Queries) UpdateItemType(ctx context.Context, itemType *domain.ItemType) error {
	schema := itemType.Schema
	if schema == nil {
		schema = map[string]any{}
	}
	fieldRoles := itemType.FieldRoles
	if fieldRoles == nil {
		fieldRoles = map[string]any{}
	}
	tag, err := q.dbtx.Exec(ctx, sqlUpdateItemType,
		itemType.OrgID,
		itemType.ID,
		itemType.Name,
		itemType.Kind,
		schema,
		fieldRoles,
	)
	if err != nil {
		return fmt.Errorf("update item type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("item_type %s not found", itemType.ID)}
	}
	return nil
}

// DeleteItemType removes an item type.
//
// Pre-conditions: orgID and itemTypeID must be non-empty.
// Post-conditions: item type is deleted.
// Raises: domain.NotFoundError if not found.
func (q *Queries) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error {
	tag, err := q.dbtx.Exec(ctx, sqlDeleteItemType, orgID, itemTypeID)
	if err != nil {
		return fmt.Errorf("delete item type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("item_type %s not found", itemTypeID)}
	}
	return nil
}
