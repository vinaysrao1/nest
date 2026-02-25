// Package service implements business logic orchestration for the Nest system.
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

// PostVerdictParams contains the inputs for the shared post-verdict pipeline.
// The Execute method constructs domain.ActionTarget internally from these fields.
type PostVerdictParams struct {
	// ActionRequests are the actions to publish (from rule engine or MRT decision).
	ActionRequests []domain.ActionRequest
	// OrgID is the tenant context.
	OrgID string
	// ItemID is the item identifier.
	ItemID string
	// ItemTypeID is the item type.
	ItemTypeID string
	// Payload is the item content (included in ActionTarget for webhook delivery).
	Payload map[string]any
	// CorrelationID ties the action executions to the verdict (rule eval or MRT decision).
	CorrelationID string
}

// RuleExecutionParams contains the inputs for logging rule executions.
// MRT decisions do NOT call this (no rule ran).
type RuleExecutionParams struct {
	OrgID      string
	ItemID     string
	ItemTypeID string
	Result     *engine.EvalResult
}

// PostVerdictPipeline executes the post-verdict steps shared by automated
// rule evaluation and MRT manual decisions: action publishing and execution logging.
//
// This struct does NOT evaluate rules. It only handles the steps AFTER a verdict
// has been determined.
type PostVerdictPipeline struct {
	publisher *engine.ActionPublisher
	store     *store.Queries
	logger    *slog.Logger
}

// NewPostVerdictPipeline creates a PostVerdictPipeline.
//
// Pre-conditions: publisher and st must be non-nil.
// Post-conditions: returned pipeline is ready for use.
func NewPostVerdictPipeline(
	publisher *engine.ActionPublisher,
	st *store.Queries,
	logger *slog.Logger,
) *PostVerdictPipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostVerdictPipeline{
		publisher: publisher,
		store:     st,
		logger:    logger,
	}
}

// Execute publishes actions and logs action executions.
//
// Steps:
//  1. Construct domain.ActionTarget from params (ItemID, ItemTypeID, OrgID, Payload, CorrelationID)
//  2. Call publisher.PublishActions(ctx, params.ActionRequests, target)
//  3. Convert results to domain.ActionExecution records
//  4. Call store.LogActionExecutions(ctx, executions)
//
// Pre-conditions: params.OrgID must be non-empty.
// Post-conditions: all actions are published; action_executions are logged.
// Returns the ActionResults (never nil, len == len(params.ActionRequests)).
// Logging failures are logged but do not cause Execute to fail.
func (p *PostVerdictPipeline) Execute(
	ctx context.Context,
	params PostVerdictParams,
) []domain.ActionResult {
	target := domain.ActionTarget{
		ItemID:        params.ItemID,
		ItemTypeID:    params.ItemTypeID,
		OrgID:         params.OrgID,
		Payload:       params.Payload,
		CorrelationID: params.CorrelationID,
	}
	actionResults := p.publisher.PublishActions(ctx, params.ActionRequests, target)
	p.logActionExecutions(ctx, params.OrgID, params.ItemID, params.ItemTypeID, params.CorrelationID, actionResults)
	return actionResults
}

// LogRuleExecutions converts TriggeredRules from an EvalResult into
// domain.RuleExecution records and persists them.
//
// Pre-conditions: params.Result may have zero TriggeredRules (no-op).
// Post-conditions: rule executions are persisted. Errors are logged, not returned.
func (p *PostVerdictPipeline) LogRuleExecutions(
	ctx context.Context,
	params RuleExecutionParams,
) {
	if len(params.Result.TriggeredRules) == 0 {
		return
	}
	now := time.Now()
	execs := make([]domain.RuleExecution, len(params.Result.TriggeredRules))
	for i, tr := range params.Result.TriggeredRules {
		execs[i] = domain.RuleExecution{
			ID:            fmt.Sprintf("rex_%d_%d", now.UnixNano(), i),
			OrgID:         params.OrgID,
			RuleID:        tr.RuleID,
			RuleVersion:   tr.Version,
			ItemID:        params.ItemID,
			ItemTypeID:    params.ItemTypeID,
			Verdict:       string(tr.Verdict),
			Reason:        tr.Reason,
			LatencyUs:     tr.LatencyUs,
			CorrelationID: params.Result.CorrelationID,
			ExecutedAt:    now,
		}
	}
	if err := p.store.LogRuleExecutions(ctx, execs); err != nil {
		p.logger.Error("verdict.LogRuleExecutions: failed to log rule executions",
			"org_id", params.OrgID,
			"item_id", params.ItemID,
			"error", err,
		)
	}
}

// logActionExecutions converts ActionResults into domain.ActionExecution records
// and persists them. Errors are logged but not propagated.
func (p *PostVerdictPipeline) logActionExecutions(
	ctx context.Context,
	orgID, itemID, itemTypeID, correlationID string,
	actionResults []domain.ActionResult,
) {
	if len(actionResults) == 0 {
		return
	}
	now := time.Now()
	execs := make([]domain.ActionExecution, len(actionResults))
	for i, ar := range actionResults {
		execs[i] = domain.ActionExecution{
			ID:            fmt.Sprintf("aex_%d_%d", now.UnixNano(), i),
			OrgID:         orgID,
			ActionID:      ar.ActionID,
			ItemID:        itemID,
			ItemTypeID:    itemTypeID,
			Success:       ar.Success,
			CorrelationID: correlationID,
			ExecutedAt:    now,
		}
	}
	if err := p.store.LogActionExecutions(ctx, execs); err != nil {
		p.logger.Error("verdict.logActionExecutions: failed to log action executions",
			"org_id", orgID,
			"item_id", itemID,
			"error", err,
		)
	}
}
