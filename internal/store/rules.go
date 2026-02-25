package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

const (
	sqlRuleColumns = `id, org_id, name, status, source, event_types, priority, tags, version, created_at, updated_at`

	sqlListRulesCount = `SELECT COUNT(*) FROM rules WHERE org_id = $1`
	sqlListRules      = `SELECT ` + sqlRuleColumns + ` FROM rules WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	sqlGetRule = `SELECT ` + sqlRuleColumns + ` FROM rules WHERE org_id = $1 AND id = $2`

	sqlCreateRule = `INSERT INTO rules (id, org_id, name, status, source, event_types, priority, tags, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	sqlUpdateRule = `UPDATE rules SET name=$3, status=$4, source=$5, event_types=$6, priority=$7, tags=$8,
		version=$9, updated_at=now()
		WHERE org_id = $1 AND id = $2`

	sqlDeleteRule = `DELETE FROM rules WHERE org_id = $1 AND id = $2`

	sqlListEnabledRules = `SELECT ` + sqlRuleColumns + `
		FROM rules WHERE org_id = $1 AND status IN ('LIVE', 'BACKGROUND')
		ORDER BY priority DESC`

	sqlDeleteRulePolicies = `DELETE FROM rules_policies WHERE rule_id = $1`
	sqlInsertRulePolicy   = `INSERT INTO rules_policies (rule_id, policy_id) VALUES ($1, $2)`
	sqlGetRulePolicies    = `SELECT policy_id FROM rules_policies WHERE rule_id = $1`
)

// scanRule reads a single Rule from a pgx row.
func scanRule(row rowScanner) (*domain.Rule, error) {
	var r domain.Rule
	err := row.Scan(
		&r.ID,
		&r.OrgID,
		&r.Name,
		&r.Status,
		&r.Source,
		&r.EventTypes,
		&r.Priority,
		&r.Tags,
		&r.Version,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if r.EventTypes == nil {
		r.EventTypes = []string{}
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return &r, nil
}

// ListRules returns a paginated list of rules for an org, ordered by created_at DESC.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all rules for org with pagination metadata.
// Raises: error on database failure.
func (q *Queries) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error) {
	var total int
	if err := q.dbtx.QueryRow(ctx, sqlListRulesCount, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count rules: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, sqlListRules, orgID, paginationLimit(page), paginationOffset(page))
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		rules = append(rules, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}

	return buildPaginatedResult(rules, total, page), nil
}

// GetRule returns a single rule by org and rule ID.
//
// Pre-conditions: orgID and ruleID must be non-empty.
// Post-conditions: returns the rule if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error) {
	row := q.dbtx.QueryRow(ctx, sqlGetRule, orgID, ruleID)
	r, err := scanRule(row)
	if err != nil {
		return nil, notFound(err, "rule", ruleID)
	}
	return r, nil
}

// CreateRule inserts a new rule.
//
// Pre-conditions: rule.ID and rule.OrgID must be set. rule.Version should be 1.
// Post-conditions: rule is persisted.
// Raises: domain.ConflictError if duplicate (org_id, name).
func (q *Queries) CreateRule(ctx context.Context, rule *domain.Rule) error {
	eventTypes := rule.EventTypes
	if eventTypes == nil {
		eventTypes = []string{}
	}
	tags := rule.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := q.dbtx.Exec(ctx, sqlCreateRule,
		rule.ID,
		rule.OrgID,
		rule.Name,
		rule.Status,
		rule.Source,
		eventTypes,
		rule.Priority,
		tags,
		rule.Version,
		rule.CreatedAt,
		rule.UpdatedAt,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("rule with name %q already exists in org", rule.Name))
	}
	return nil
}

// UpdateRule updates an existing rule.
//
// Pre-conditions: rule.ID and rule.OrgID must be set.
// Post-conditions: rule is updated. updated_at set to now().
// Raises: domain.NotFoundError if not found (org_id + id mismatch).
// Raises: domain.ConflictError if name collides with another rule in org.
func (q *Queries) UpdateRule(ctx context.Context, rule *domain.Rule) error {
	eventTypes := rule.EventTypes
	if eventTypes == nil {
		eventTypes = []string{}
	}
	tags := rule.Tags
	if tags == nil {
		tags = []string{}
	}
	tag, err := q.dbtx.Exec(ctx, sqlUpdateRule,
		rule.OrgID,
		rule.ID,
		rule.Name,
		rule.Status,
		rule.Source,
		eventTypes,
		rule.Priority,
		tags,
		rule.Version,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("rule with name %q already exists in org", rule.Name))
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("rule %s not found", rule.ID)}
	}
	return nil
}

// DeleteRule removes a rule. Associated rules_policies entries are CASCADE deleted.
//
// Pre-conditions: orgID and ruleID must be non-empty.
// Post-conditions: rule is deleted.
// Raises: domain.NotFoundError if not found.
func (q *Queries) DeleteRule(ctx context.Context, orgID, ruleID string) error {
	tag, err := q.dbtx.Exec(ctx, sqlDeleteRule, orgID, ruleID)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("rule %s not found", ruleID)}
	}
	return nil
}

// ListEnabledRules returns all rules with status LIVE or BACKGROUND for an org,
// ordered by priority DESC (highest first). Used by the engine to build snapshots.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns rules ordered by priority DESC.
// Raises: error on database failure.
func (q *Queries) ListEnabledRules(ctx context.Context, orgID string) ([]domain.Rule, error) {
	rows, err := q.dbtx.Query(ctx, sqlListEnabledRules, orgID)
	if err != nil {
		return nil, fmt.Errorf("list enabled rules: %w", err)
	}
	defer rows.Close()

	var rules []domain.Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled rule: %w", err)
		}
		rules = append(rules, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled rules: %w", err)
	}
	if rules == nil {
		rules = []domain.Rule{}
	}
	return rules, nil
}

// SetRulePolicies replaces the set of policies associated with a rule.
// Deletes all existing associations then inserts the new set.
//
// Pre-conditions: ruleID must exist. policyIDs may be empty (clears associations).
// Post-conditions: rules_policies contains exactly the given set for this ruleID.
// Raises: error on database failure.
func (q *Queries) SetRulePolicies(ctx context.Context, ruleID string, policyIDs []string) error {
	if _, err := q.dbtx.Exec(ctx, sqlDeleteRulePolicies, ruleID); err != nil {
		return fmt.Errorf("delete rule policies: %w", err)
	}
	for _, pid := range policyIDs {
		if _, err := q.dbtx.Exec(ctx, sqlInsertRulePolicy, ruleID, pid); err != nil {
			return fmt.Errorf("insert rule policy %s: %w", pid, err)
		}
	}
	return nil
}

// GetRulePolicies returns the policy IDs associated with a rule.
//
// Pre-conditions: ruleID must be non-empty.
// Post-conditions: returns slice of policy IDs (empty if none).
// Raises: error on database failure.
func (q *Queries) GetRulePolicies(ctx context.Context, ruleID string) ([]string, error) {
	rows, err := q.dbtx.Query(ctx, sqlGetRulePolicies, ruleID)
	if err != nil {
		return nil, fmt.Errorf("get rule policies: %w", err)
	}
	defer rows.Close()

	var policyIDs []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("scan policy id: %w", err)
		}
		policyIDs = append(policyIDs, pid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rule policies: %w", err)
	}
	if policyIDs == nil {
		policyIDs = []string{}
	}
	return policyIDs, nil
}

// ListDistinctOrgIDs returns all distinct org IDs that have at least one rule.
// Used by the snapshot rebuild maintenance job.
func (q *Queries) ListDistinctOrgIDs(ctx context.Context) ([]string, error) {
	rows, err := q.dbtx.Query(ctx, `SELECT DISTINCT org_id FROM rules`)
	if err != nil {
		return nil, fmt.Errorf("list distinct org IDs: %w", err)
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var orgID string
		if err := rows.Scan(&orgID); err != nil {
			return nil, fmt.Errorf("scan org ID: %w", err)
		}
		orgIDs = append(orgIDs, orgID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate org IDs: %w", err)
	}
	if orgIDs == nil {
		orgIDs = []string{}
	}
	return orgIDs, nil
}
