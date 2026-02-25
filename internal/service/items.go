// Package service implements business logic orchestration for the Nest system.
// Services coordinate multi-step operations across the store and engine layers.
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

// SubmitItemParams contains the parameters for a single item submission.
type SubmitItemParams struct {
	// ItemID is the caller-provided external item identifier.
	ItemID string
	// ItemTypeID references an existing item type for this org.
	ItemTypeID string
	// OrgID is the org context (validated against the route org).
	OrgID string
	// Payload is the item content to evaluate. Must not be nil.
	Payload map[string]any
	// CreatorID is the identifier of the entity that created the item (optional).
	CreatorID string
	// CreatorTypeID categorises the creator (optional).
	CreatorTypeID string
}

// ItemService orchestrates item validation, persistence, rule evaluation, and action publishing.
//
// Dependencies: store (for persistence and item-type lookup), pool (rule evaluation),
// pipeline (post-verdict action publishing and execution logging).
type ItemService struct {
	store    *store.Queries
	pool     *engine.Pool
	pipeline *PostVerdictPipeline
	logger   *slog.Logger
}

// NewItemService creates an ItemService.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned service is ready to handle submissions.
func NewItemService(
	st *store.Queries,
	pool *engine.Pool,
	pipeline *PostVerdictPipeline,
	logger *slog.Logger,
) *ItemService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ItemService{
		store:    st,
		pool:     pool,
		pipeline: pipeline,
		logger:   logger,
	}
}

// SubmitSync validates, persists, and evaluates each item synchronously.
// For every item the rule engine is invoked and any triggered actions are published
// before the function returns. If validation fails for any item (bad item type),
// the whole batch is rejected. Evaluation errors for a single item are logged
// and a default "approve" result is returned for that item.
//
// Pre-conditions: orgID must be non-empty; items must not be nil.
// Post-conditions: returns one EvalResultResponse per input item on success.
// Raises: domain.ValidationError if any item fails validation.
// Raises: domain.NotFoundError if any item type does not exist.
func (s *ItemService) SubmitSync(
	ctx context.Context,
	orgID string,
	items []SubmitItemParams,
) ([]domain.EvalResultResponse, error) {
	// Validate and look up item types up-front so we can fail fast
	// before persisting anything.
	itemTypes := make([]*domain.ItemType, len(items))
	for i, item := range items {
		if item.ItemTypeID == "" {
			return nil, &domain.ValidationError{
				Message: fmt.Sprintf("item at index %d: ItemTypeID must not be empty", i),
			}
		}
		if item.Payload == nil {
			return nil, &domain.ValidationError{
				Message: fmt.Sprintf("item at index %d: Payload must not be nil", i),
			}
		}
		it, err := s.store.GetItemType(ctx, orgID, item.ItemTypeID)
		if err != nil {
			return nil, err
		}
		itemTypes[i] = it
	}

	results := make([]domain.EvalResultResponse, 0, len(items))
	for i, item := range items {
		resp := s.processOneItem(ctx, orgID, item, itemTypes[i])
		results = append(results, resp)
	}
	return results, nil
}

// processOneItem persists, evaluates, and publishes actions for a single item.
// It never returns an error: evaluation failures are logged and produce a default
// "approve" response, keeping the batch from being partially fulfilled.
func (s *ItemService) processOneItem(
	ctx context.Context,
	orgID string,
	item SubmitItemParams,
	itemType *domain.ItemType,
) domain.EvalResultResponse {
	now := time.Now()
	submissionID := fmt.Sprintf("sub_%d", now.UnixNano())

	domainItem := domain.Item{
		ID:            item.ItemID,
		OrgID:         orgID,
		ItemTypeID:    item.ItemTypeID,
		Data:          item.Payload,
		SubmissionID:  submissionID,
		CreatorID:     item.CreatorID,
		CreatorTypeID: item.CreatorTypeID,
		CreatedAt:     now,
	}

	if err := s.store.InsertItem(ctx, orgID, domainItem); err != nil {
		s.logger.Error("items.processOneItem: insert item failed",
			"org_id", orgID,
			"item_id", item.ItemID,
			"error", err,
		)
		// Return default approve -- persistence failure should not block the caller.
		return s.defaultApprove(item.ItemID)
	}

	event := domain.Event{
		ID:         submissionID,
		EventType:  itemType.Name,
		ItemType:   itemType.Name,
		OrgID:      orgID,
		Payload:    item.Payload,
		Timestamp:  now,
		ItemID:     item.ItemID,
		ItemTypeID: item.ItemTypeID,
	}

	result, err := s.pool.Evaluate(ctx, event)
	if err != nil {
		s.logger.Error("items.processOneItem: evaluation failed",
			"org_id", orgID,
			"item_id", item.ItemID,
			"error", err,
		)
		return s.defaultApprove(item.ItemID)
	}

	actionResults := s.pipeline.Execute(ctx, PostVerdictParams{
		ActionRequests: result.ActionRequests,
		OrgID:          orgID,
		ItemID:         item.ItemID,
		ItemTypeID:     item.ItemTypeID,
		Payload:        item.Payload,
		CorrelationID:  result.CorrelationID,
	})
	s.pipeline.LogRuleExecutions(ctx, RuleExecutionParams{
		OrgID:      orgID,
		ItemID:     item.ItemID,
		ItemTypeID: item.ItemTypeID,
		Result:     result,
	})

	return domain.EvalResultResponse{
		ItemID:         item.ItemID,
		Verdict:        result.Verdict.Type,
		TriggeredRules: result.TriggeredRules,
		Actions:        actionResults,
	}
}

// defaultApprove returns an EvalResultResponse with a default "approve" verdict.
// Used when evaluation fails so that the batch still returns a result for every item.
func (s *ItemService) defaultApprove(itemID string) domain.EvalResultResponse {
	return domain.EvalResultResponse{
		ItemID:         itemID,
		Verdict:        domain.VerdictApprove,
		TriggeredRules: []domain.TriggeredRule{},
		Actions:        []domain.ActionResult{},
	}
}

// SubmitAsync validates all items, persists them, and returns submission IDs.
// Rule evaluation and action publishing are intentionally NOT performed here;
// they happen in a river worker job (Stage 6). If any item type is invalid the
// whole batch is rejected before any items are persisted.
//
// Pre-conditions: orgID must be non-empty; items must not be nil.
// Post-conditions: all items are persisted; returned slice has one submission ID per item.
// Raises: domain.ValidationError if any item fails validation.
// Raises: domain.NotFoundError if any item type does not exist.
func (s *ItemService) SubmitAsync(
	ctx context.Context,
	orgID string,
	items []SubmitItemParams,
) ([]string, error) {
	// Validate all item types before persisting anything.
	itemTypes := make([]*domain.ItemType, len(items))
	for i, item := range items {
		if item.ItemTypeID == "" {
			return nil, &domain.ValidationError{
				Message: fmt.Sprintf("item at index %d: ItemTypeID must not be empty", i),
			}
		}
		if item.Payload == nil {
			return nil, &domain.ValidationError{
				Message: fmt.Sprintf("item at index %d: Payload must not be nil", i),
			}
		}
		it, err := s.store.GetItemType(ctx, orgID, item.ItemTypeID)
		if err != nil {
			return nil, err
		}
		itemTypes[i] = it
	}

	submissionIDs := make([]string, len(items))
	for i, item := range items {
		now := time.Now()
		submissionID := fmt.Sprintf("sub_%d", now.UnixNano())
		submissionIDs[i] = submissionID

		domainItem := domain.Item{
			ID:            item.ItemID,
			OrgID:         orgID,
			ItemTypeID:    item.ItemTypeID,
			Data:          item.Payload,
			SubmissionID:  submissionID,
			CreatorID:     item.CreatorID,
			CreatorTypeID: item.CreatorTypeID,
			CreatedAt:     now,
		}

		if err := s.store.InsertItem(ctx, orgID, domainItem); err != nil {
			return nil, fmt.Errorf("items.SubmitAsync: insert item %s: %w", item.ItemID, err)
		}

		// Suppress unused variable warning for itemType (used for validation above).
		_ = itemTypes[i]

		s.logger.Info("items.SubmitAsync: item stored",
			"org_id", orgID,
			"item_id", item.ItemID,
			"submission_id", submissionID,
		)
	}

	return submissionIDs, nil
}
