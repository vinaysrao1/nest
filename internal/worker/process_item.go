// Package worker implements background job processing for the Nest system.
// It uses the river PostgreSQL-native job queue for async item evaluation
// and periodic maintenance tasks.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/engine"
	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/store"
)

// ProcessItemArgs are the river job arguments for async item evaluation.
// Implements river.JobArgs.
type ProcessItemArgs struct {
	OrgID      string         `json:"org_id"`
	ItemID     string         `json:"item_id"`
	ItemTypeID string         `json:"item_type_id"`
	EventType  string         `json:"event_type"`
	Payload    map[string]any `json:"payload"`
}

// Kind returns the river job kind identifier.
func (ProcessItemArgs) Kind() string { return "process_item" }

// ProcessItemWorker handles async item evaluation via river.
// It mirrors the synchronous evaluation path in ItemService.processOneItem
// but runs in a river worker goroutine.
type ProcessItemWorker struct {
	river.WorkerDefaults[ProcessItemArgs]

	pool     *engine.Pool
	pipeline *service.PostVerdictPipeline
	store    *store.Queries
	logger   *slog.Logger
}

// NewProcessItemWorker creates a ProcessItemWorker.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned worker is ready for river registration.
func NewProcessItemWorker(
	pool *engine.Pool,
	pipeline *service.PostVerdictPipeline,
	st *store.Queries,
	logger *slog.Logger,
) *ProcessItemWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProcessItemWorker{
		pool:     pool,
		pipeline: pipeline,
		store:    st,
		logger:   logger,
	}
}

// Work evaluates rules for a single item and publishes resulting actions.
// This mirrors the sync evaluation path (ItemService.processOneItem) but
// runs in a river worker goroutine.
//
// Steps:
//  1. Build domain.Event from job args
//  2. Call pool.Evaluate(ctx, event)
//  3. Call pipeline.Execute(ctx, params) to publish actions and log action executions
//  4. Call pipeline.LogRuleExecutions(ctx, params) to log rule executions
//
// Pre-conditions: job.Args must have valid OrgID, ItemID, ItemTypeID.
// Post-conditions: item is evaluated; actions are published; logs are written.
// Raises: error on evaluation failure (river will retry up to max_attempts).
func (w *ProcessItemWorker) Work(ctx context.Context, job *river.Job[ProcessItemArgs]) error {
	args := job.Args

	if args.OrgID == "" {
		return fmt.Errorf("process_item: org_id must not be empty")
	}
	if args.ItemID == "" {
		return fmt.Errorf("process_item: item_id must not be empty")
	}
	if args.ItemTypeID == "" {
		return fmt.Errorf("process_item: item_type_id must not be empty")
	}

	// Resolve item type to get the event type name (same as sync path).
	eventType := args.EventType
	if eventType == "" || eventType == args.ItemTypeID {
		itemType, err := w.store.GetItemType(ctx, args.OrgID, args.ItemTypeID)
		if err != nil {
			return fmt.Errorf("process_item: resolve item type: %w", err)
		}
		eventType = itemType.Name
	}

	now := time.Now()
	correlationID := fmt.Sprintf("async_%s_%d", args.ItemID, now.UnixNano())

	event := domain.Event{
		ID:         correlationID,
		EventType:  eventType,
		ItemType:   eventType,
		OrgID:      args.OrgID,
		Payload:    args.Payload,
		Timestamp:  now,
		ItemID:     args.ItemID,
		ItemTypeID: args.ItemTypeID,
	}

	result, err := w.pool.Evaluate(ctx, event)
	if err != nil {
		w.logger.Error("worker.process_item: evaluation failed",
			"org_id", args.OrgID,
			"item_id", args.ItemID,
			"error", err,
		)
		return fmt.Errorf("process_item: evaluate: %w", err)
	}

	w.pipeline.Execute(ctx, service.PostVerdictParams{
		ActionRequests: result.ActionRequests,
		OrgID:          args.OrgID,
		ItemID:         args.ItemID,
		ItemTypeID:     args.ItemTypeID,
		Payload:        args.Payload,
		CorrelationID:  result.CorrelationID,
	})
	w.pipeline.LogRuleExecutions(ctx, service.RuleExecutionParams{
		OrgID:      args.OrgID,
		ItemID:     args.ItemID,
		ItemTypeID: args.ItemTypeID,
		Result:     result,
	})

	w.logger.Info("worker.process_item: completed",
		"org_id", args.OrgID,
		"item_id", args.ItemID,
		"verdict", result.Verdict.Type,
		"correlation_id", result.CorrelationID,
	)
	return nil
}
