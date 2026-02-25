package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/engine"
	"github.com/vinaysrao1/nest/internal/store"
)

// CreateRuleParams holds the inputs required to create a new rule.
type CreateRuleParams struct {
	Name      string
	Status    domain.RuleStatus
	Source    string
	Tags      []string
	PolicyIDs []string
}

// UpdateRuleParams holds the optional fields that may be changed on a rule update.
// A nil pointer means "do not change this field".
type UpdateRuleParams struct {
	Name      *string
	Status    *domain.RuleStatus
	Source    *string
	Tags      *[]string
	PolicyIDs *[]string
}

// TestResult is the outcome of evaluating a Starlark source against a sample event.
type TestResult struct {
	Verdict   domain.VerdictType `json:"verdict"`
	Reason    string             `json:"reason"`
	RuleID    string             `json:"rule_id"`
	Actions   []string           `json:"actions"`
	Logs      []string           `json:"logs"`
	LatencyUs int64              `json:"latency_us"`
}

// RuleService manages the lifecycle of Starlark rules: creation, update, deletion,
// retrieval, testing, and snapshot management.
type RuleService struct {
	store    *store.Queries
	compiler *engine.Compiler
	pool     *engine.Pool
	logger   *slog.Logger
}

// NewRuleService constructs a RuleService with the required dependencies.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned RuleService is ready for use.
func NewRuleService(
	st *store.Queries,
	compiler *engine.Compiler,
	pool *engine.Pool,
	logger *slog.Logger,
) *RuleService {
	return &RuleService{
		store:    st,
		compiler: compiler,
		pool:     pool,
		logger:   logger,
	}
}

// CreateRule compiles the Starlark source, persists the rule with entity history,
// and triggers a snapshot rebuild for the org.
//
// Pre-conditions: orgID non-empty; params.Name non-empty; params.Source non-empty;
// params.Status is a valid RuleStatus.
// Post-conditions: rule is persisted with Version=1; snapshot is rebuilt.
// Raises: *domain.ValidationError for invalid params; *domain.CompileError for bad source;
// store errors (ConflictError) on duplicate ID.
func (s *RuleService) CreateRule(ctx context.Context, orgID string, params CreateRuleParams) (*domain.Rule, error) {
	if err := validateCreateRuleParams(params); err != nil {
		return nil, err
	}

	compiled, err := s.compiler.CompileRule(params.Source, params.Name)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rule := domain.Rule{
		ID:         fmt.Sprintf("rul_%d", now.UnixNano()),
		OrgID:      orgID,
		Name:       params.Name,
		Status:     params.Status,
		Source:     params.Source,
		EventTypes: compiled.EventTypes,
		Priority:   compiled.Priority,
		Tags:       params.Tags,
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if rule.Tags == nil {
		rule.Tags = []string{}
	}
	if rule.EventTypes == nil {
		rule.EventTypes = []string{}
	}

	policyIDs := params.PolicyIDs
	if policyIDs == nil {
		policyIDs = []string{}
	}

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.CreateRule(ctx, &rule); err != nil {
			return err
		}
		if err := tx.SetRulePolicies(ctx, rule.ID, policyIDs); err != nil {
			return fmt.Errorf("set rule policies: %w", err)
		}
		if err := tx.InsertEntityHistory(ctx, "rule", rule.ID, orgID, 1, rule); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("rules.CreateRule: %w", err)
	}

	s.rebuildSnapshot(ctx, orgID)
	s.logger.Info("rule created", "org_id", orgID, "rule_id", rule.ID, "name", rule.Name)
	return &rule, nil
}

// UpdateRule fetches the existing rule, applies non-nil param fields, re-compiles
// if the source changed, increments the version, and persists with entity history.
//
// Pre-conditions: orgID and ruleID non-empty.
// Post-conditions: rule is updated; version incremented; snapshot rebuilt.
// Raises: *domain.NotFoundError if rule does not exist; *domain.CompileError for bad source.
func (s *RuleService) UpdateRule(ctx context.Context, orgID, ruleID string, params UpdateRuleParams) (*domain.Rule, error) {
	existing, err := s.store.GetRule(ctx, orgID, ruleID)
	if err != nil {
		return nil, err
	}

	if params.Name != nil {
		existing.Name = *params.Name
	}
	if params.Status != nil {
		existing.Status = *params.Status
	}
	if params.Tags != nil {
		existing.Tags = *params.Tags
	}

	if params.Source != nil {
		compiled, compErr := s.compiler.CompileRule(*params.Source, existing.Name)
		if compErr != nil {
			return nil, compErr
		}
		existing.Source = *params.Source
		existing.EventTypes = compiled.EventTypes
		existing.Priority = compiled.Priority
	}

	existing.Version++

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.UpdateRule(ctx, existing); err != nil {
			return err
		}
		if params.PolicyIDs != nil {
			if err := tx.SetRulePolicies(ctx, ruleID, *params.PolicyIDs); err != nil {
				return fmt.Errorf("set rule policies: %w", err)
			}
		}
		if err := tx.InsertEntityHistory(ctx, "rule", ruleID, orgID, existing.Version, existing); err != nil {
			return fmt.Errorf("insert entity history: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("rules.UpdateRule: %w", err)
	}

	s.rebuildSnapshot(ctx, orgID)
	s.logger.Info("rule updated", "org_id", orgID, "rule_id", ruleID, "version", existing.Version)
	return existing, nil
}

// DeleteRule removes the rule from the store and triggers a snapshot rebuild.
//
// Pre-conditions: orgID and ruleID non-empty.
// Post-conditions: rule is deleted; snapshot rebuilt without the deleted rule.
// Raises: *domain.NotFoundError if rule does not exist.
func (s *RuleService) DeleteRule(ctx context.Context, orgID, ruleID string) error {
	if err := s.store.DeleteRule(ctx, orgID, ruleID); err != nil {
		return err
	}
	s.rebuildSnapshot(ctx, orgID)
	s.logger.Info("rule deleted", "org_id", orgID, "rule_id", ruleID)
	return nil
}

// GetRule returns a single rule by org and rule ID.
//
// Pre-conditions: orgID and ruleID non-empty.
// Post-conditions: returns the rule if found.
// Raises: *domain.NotFoundError if not found.
func (s *RuleService) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error) {
	return s.store.GetRule(ctx, orgID, ruleID)
}

// ListRules returns a paginated list of rules for an org.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns paginated result.
// Raises: error on database failure.
func (s *RuleService) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error) {
	return s.store.ListRules(ctx, orgID, page)
}

// TestRule compiles the given source, creates a temporary snapshot under a unique
// org namespace, evaluates the provided event through the pool, and returns the result.
// This does NOT write to the rules table.
//
// Pre-conditions: orgID non-empty; source non-empty; event is a valid domain.Event.
// Post-conditions: returns TestResult; no rules table rows written.
// Raises: *domain.CompileError for bad source; error on evaluation failure.
func (s *RuleService) TestRule(ctx context.Context, orgID string, source string, event domain.Event) (*TestResult, error) {
	compiled, err := s.compiler.CompileRule(source, "test-rule")
	if err != nil {
		return nil, err
	}

	testOrgID := fmt.Sprintf("_test_%d", time.Now().UnixNano())
	snap := engine.NewSnapshot(testOrgID, []*engine.CompiledRule{compiled}, nil)
	s.pool.SwapSnapshot(testOrgID, snap)

	event.OrgID = testOrgID

	result, evalErr := s.pool.Evaluate(ctx, event)
	if evalErr != nil {
		return nil, fmt.Errorf("rules.TestRule evaluate: %w", evalErr)
	}

	return buildTestResult(result), nil
}

// TestExistingRule fetches an existing rule by ID and delegates to TestRule.
//
// Pre-conditions: orgID and ruleID non-empty; event is a valid domain.Event.
// Post-conditions: returns TestResult using the rule's current source.
// Raises: *domain.NotFoundError if rule does not exist; errors from TestRule.
func (s *RuleService) TestExistingRule(ctx context.Context, orgID, ruleID string, event domain.Event) (*TestResult, error) {
	rule, err := s.store.GetRule(ctx, orgID, ruleID)
	if err != nil {
		return nil, err
	}
	return s.TestRule(ctx, orgID, rule.Source, event)
}

// RebuildSnapshot fetches all enabled rules and all actions for the org, compiles each
// rule (skipping rules that fail compilation with a warning log), builds a new Snapshot
// containing both the compiled rules and the action definitions, and atomically swaps
// it into the pool.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: pool snapshot for orgID reflects all currently enabled, compilable rules
// and all org actions (for action name resolution at eval time).
// Raises: error if ListEnabledRules or ListAllActionsByOrg fails.
func (s *RuleService) RebuildSnapshot(ctx context.Context, orgID string) error {
	rules, err := s.store.ListEnabledRules(ctx, orgID)
	if err != nil {
		return fmt.Errorf("rules.RebuildSnapshot list rules: %w", err)
	}

	actions, err := s.store.ListAllActionsByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("rules.RebuildSnapshot list actions: %w", err)
	}

	compiled := make([]*engine.CompiledRule, 0, len(rules))
	for _, rule := range rules {
		cr, compErr := s.compiler.CompileRule(rule.Source, rule.Name)
		if compErr != nil {
			s.logger.Warn("skipping rule with compile error during snapshot rebuild",
				"org_id", orgID, "rule_id", rule.ID, "error", compErr)
			continue
		}
		cr.ID = rule.ID
		cr.Version = rule.Version
		compiled = append(compiled, cr)
	}

	snap := engine.NewSnapshot(orgID, compiled, actions)
	s.pool.SwapSnapshot(orgID, snap)
	s.logger.Info("snapshot rebuilt",
		"org_id", orgID,
		"rule_count", len(compiled),
		"action_count", len(actions),
		"skipped", len(rules)-len(compiled))
	return nil
}

// rebuildSnapshot is a fire-and-forget wrapper around RebuildSnapshot.
// Errors are logged but not propagated. Rule CRUD succeeds even if the snapshot
// rebuild fails — the periodic maintenance job will catch up.
func (s *RuleService) rebuildSnapshot(ctx context.Context, orgID string) {
	if err := s.RebuildSnapshot(ctx, orgID); err != nil {
		s.logger.Error("failed to rebuild snapshot", "org_id", orgID, "error", err)
	}
}

// buildTestResult converts an engine.EvalResult into a TestResult.
func buildTestResult(result *engine.EvalResult) *TestResult {
	tr := &TestResult{
		Verdict:   result.Verdict.Type,
		Reason:    result.Verdict.Reason,
		RuleID:    result.Verdict.RuleID,
		Actions:   result.Verdict.Actions,
		Logs:      result.Logs,
		LatencyUs: result.LatencyUs,
	}
	if tr.Actions == nil {
		tr.Actions = []string{}
	}
	if tr.Logs == nil {
		tr.Logs = []string{}
	}
	return tr
}

// validateCreateRuleParams validates the required fields of CreateRuleParams.
func validateCreateRuleParams(params CreateRuleParams) error {
	if params.Name == "" {
		return &domain.ValidationError{
			Message: "rule name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}
	if params.Source == "" {
		return &domain.ValidationError{
			Message: "rule source is required",
			Details: map[string]string{"source": "must not be empty"},
		}
	}
	switch params.Status {
	case domain.RuleStatusLive, domain.RuleStatusBackground, domain.RuleStatusDisabled:
		// valid
	default:
		return &domain.ValidationError{
			Message: fmt.Sprintf("invalid rule status %q", params.Status),
			Details: map[string]string{"status": "must be LIVE, BACKGROUND, or DISABLED"},
		}
	}
	return nil
}
