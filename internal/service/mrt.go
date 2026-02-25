package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// CacheInvalidator allows MRTService to invalidate cached queue IDs in the
// engine's action cache without importing the engine package.
//
// The concrete implementation wraps engine.Cache.Delete with the correct
// cache key format ("queue:{orgID}:{queueName}").
type CacheInvalidator interface {
	// InvalidateMRTQueue removes the cached queue ID for the given org and queue name.
	// If the entry is not cached, this is a no-op.
	InvalidateMRTQueue(orgID, queueName string)
}

// CacheInvalidatorFunc is a function adapter for CacheInvalidator.
// This allows the composition root (cmd/server) to pass a closure that
// calls engine.Cache.Delete with the correct key format.
type CacheInvalidatorFunc func(orgID, queueName string)

// InvalidateMRTQueue implements CacheInvalidator.
func (f CacheInvalidatorFunc) InvalidateMRTQueue(orgID, queueName string) {
	f(orgID, queueName)
}

// CreateQueueParams holds inputs for creating a new MRT queue.
type CreateQueueParams struct {
	Name        string
	Description string
	IsDefault   bool
}

// EnqueueParams contains the parameters for enqueuing an item to an MRT queue.
type EnqueueParams struct {
	// OrgID is the tenant context.
	OrgID string
	// QueueName is the name of the target MRT queue (must exist for this org).
	QueueName string
	// ItemID is the external item identifier.
	ItemID string
	// ItemTypeID references the item type of the item being reviewed.
	ItemTypeID string
	// Payload is the item content to surface to the moderator.
	Payload map[string]any
	// EnqueueSource describes where this enqueue originated (e.g. "rule", "api").
	EnqueueSource string
	// SourceInfo provides additional context about the enqueue source.
	SourceInfo map[string]any
	// PolicyIDs lists the policies applicable to this review job.
	PolicyIDs []string
}

// DecisionParams contains the parameters for recording a moderator decision.
type DecisionParams struct {
	// OrgID is the tenant context.
	OrgID string
	// JobID is the MRT job being decided.
	JobID string
	// UserID is the moderator recording the decision.
	UserID string
	// Verdict is one of APPROVE, BLOCK, SKIP, ROUTE.
	Verdict string
	// Reason is the moderator's rationale (optional).
	Reason string
	// ActionIDs lists the actions the moderator wants to execute.
	ActionIDs []string
	// PolicyIDs lists the policies cited for this decision.
	PolicyIDs []string
	// TargetQueueID is the destination queue ID. Required when Verdict == ROUTE.
	TargetQueueID *string
}

// DecisionResult is the return value of RecordDecision.
// The caller (handler) is responsible for executing ActionRequests via PostVerdictPipeline.
// MRTService does NOT call ActionPublisher — this is invariant 8.
type DecisionResult struct {
	Decision       domain.MRTDecision
	ActionRequests []domain.ActionRequest
	// WebhookRequired is true only for APPROVE and BLOCK verdicts. The handler
	// must check this flag before calling ActionPublisher.PublishActions.
	WebhookRequired bool
	// ItemID is from the MRT job, for post-verdict pipeline logging.
	ItemID string
	// ItemTypeID is from the MRT job, for post-verdict pipeline logging.
	ItemTypeID string
	// Payload is from the MRT job, for ActionTarget construction in the pipeline.
	Payload map[string]any
}

// MRTService manages Manual Review Tool workflows: enqueueing items for human
// review, assigning jobs to moderators, and recording decisions.
//
// CRITICAL INVARIANT: MRTService does NOT depend on engine.ActionPublisher.
// RecordDecision returns ActionRequests for the handler to execute. This avoids
// a circular dependency and preserves invariant 8 from the design document.
//
// Dependencies: store, optional CacheInvalidator.
type MRTService struct {
	store    *store.Queries
	logger   *slog.Logger
	cacheInv CacheInvalidator
}

// NewMRTService creates an MRTService.
// cacheInv may be nil (cache invalidation is skipped, suitable for tests).
//
// Pre-conditions: st must be non-nil.
// Post-conditions: returned service is ready to handle MRT operations.
func NewMRTService(st *store.Queries, logger *slog.Logger, cacheInv CacheInvalidator) *MRTService {
	if logger == nil {
		logger = slog.Default()
	}
	return &MRTService{
		store:    st,
		logger:   logger,
		cacheInv: cacheInv,
	}
}

// Enqueue creates a new MRT job in the named queue.
// The queue must already exist for this org.
//
// Pre-conditions: params.OrgID, params.QueueName, params.ItemID, params.ItemTypeID must be non-empty.
// Post-conditions: a new MRT job with PENDING status is persisted and its ID returned.
// Raises: domain.NotFoundError if the queue does not exist.
// Raises: error on database failure.
func (s *MRTService) Enqueue(ctx context.Context, params EnqueueParams) (string, error) {
	queue, err := s.store.GetMRTQueueByName(ctx, params.OrgID, params.QueueName)
	if err != nil {
		return "", err
	}

	now := time.Now()
	jobID := fmt.Sprintf("mrt_%d", now.UnixNano())

	policyIDs := params.PolicyIDs
	if policyIDs == nil {
		policyIDs = []string{}
	}
	sourceInfo := params.SourceInfo
	if sourceInfo == nil {
		sourceInfo = map[string]any{}
	}
	payload := params.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	job := domain.MRTJob{
		ID:            jobID,
		OrgID:         params.OrgID,
		QueueID:       queue.ID,
		ItemID:        params.ItemID,
		ItemTypeID:    params.ItemTypeID,
		Payload:       payload,
		Status:        domain.MRTJobStatusPending,
		PolicyIDs:     policyIDs,
		EnqueueSource: params.EnqueueSource,
		SourceInfo:    sourceInfo,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.InsertMRTJob(ctx, &job); err != nil {
		return "", fmt.Errorf("mrt.Enqueue: %w", err)
	}

	s.logger.Info("mrt.Enqueue: job created",
		"org_id", params.OrgID,
		"job_id", jobID,
		"queue_id", queue.ID,
		"item_id", params.ItemID,
	)
	return job.ID, nil
}

// AssignNext atomically claims the oldest PENDING job in the queue for the given user.
// Returns nil, nil when the queue has no pending jobs — this is not an error.
//
// Pre-conditions: orgID, queueID, and userID must be non-empty.
// Post-conditions: the returned job (if non-nil) has status ASSIGNED and assigned_to set.
// Raises: error on unexpected database failure (NotFoundError is converted to nil, nil).
func (s *MRTService) AssignNext(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error) {
	job, err := s.store.AssignNextMRTJob(ctx, orgID, queueID, userID)
	if err != nil {
		var nfErr *domain.NotFoundError
		if errors.As(err, &nfErr) {
			// Empty queue is not an application error.
			return nil, nil
		}
		return nil, fmt.Errorf("mrt.AssignNext: %w", err)
	}
	return job, nil
}

// ClaimJob claims a specific MRT job for the given user.
//
// If the atomic claim succeeds (job was PENDING), returns the now-ASSIGNED job.
// If the claim fails (0 rows updated), performs a follow-up GET to classify:
//   - Job not found at all: returns domain.NotFoundError.
//   - Job already ASSIGNED to this user: returns the job as-is (idempotent).
//   - Job in any other state: returns domain.ConflictError.
//
// Note: the follow-up GET has a TOCTOU window -- the job's state could change
// between the failed UPDATE and this GET. This is acceptable because the GET
// result is used only to produce an informational error message; no state
// mutation depends on it.
func (s *MRTService) ClaimJob(ctx context.Context, orgID, jobID, userID string) (*domain.MRTJob, error) {
	job, err := s.store.ClaimMRTJob(ctx, orgID, jobID, userID)
	if err == nil {
		return job, nil
	}

	// Claim failed (0 rows). Classify the reason via a follow-up GET.
	// TOCTOU note: the job's state may change between the failed UPDATE above
	// and this GET. This is acceptable -- the result is used only for the error
	// message, not for any state mutation.
	existing, getErr := s.store.GetMRTJob(ctx, orgID, jobID)
	if getErr != nil {
		// Job does not exist at all.
		return nil, getErr
	}

	// Already assigned to this user -- idempotent success.
	if existing.Status == domain.MRTJobStatusAssigned && existing.AssignedTo != nil && *existing.AssignedTo == userID {
		return existing, nil
	}

	// Job exists but is not claimable (assigned to someone else, or decided).
	return nil, &domain.ConflictError{
		Message: fmt.Sprintf("mrt job %s is in %s status and cannot be claimed", jobID, existing.Status),
	}
}

// RecordDecision records a moderator's decision on an assigned MRT job.
//
// All verdict paths use store.WithTx to ensure atomicity of the decision insert
// and subsequent job status/queue update.
//
// Behavior varies by verdict:
//
//	APPROVE: Insert decision, set job DECIDED, resolve actions. WebhookRequired=true.
//	BLOCK:   Insert decision, set job DECIDED, resolve actions. WebhookRequired=true.
//	SKIP:    Insert decision (with optional reason), call UpdateMRTJobStatus to set
//	         job back to PENDING with assigned_to=NULL. WebhookRequired=false.
//	ROUTE:   Validate target queue exists and is active. Insert decision with target_queue_id.
//	         Call RouteMRTJob to move job to target queue (PENDING, no assignee).
//	         WebhookRequired=false.
//
// CRITICAL INVARIANT: This method does NOT call ActionPublisher. The caller
// (the HTTP handler) receives ActionRequests and executes them separately.
//
// Pre-conditions: params.OrgID, params.JobID, params.UserID, params.Verdict must be non-empty.
// Pre-conditions: params.Verdict must be one of APPROVE, BLOCK, SKIP, ROUTE.
// Pre-conditions: params.TargetQueueID must be set when Verdict == ROUTE.
// Post-conditions: decision is always persisted in mrt_decisions.
// Post-conditions: job transitions per verdict as described above.
// Raises: domain.ValidationError if verdict is invalid or ROUTE without target_queue_id.
// Raises: domain.ValidationError if the job is not in ASSIGNED status.
// Raises: domain.ForbiddenError if the job is not assigned to params.UserID.
// Raises: domain.NotFoundError if the job or target queue does not exist.
// Raises: error on database failure.
func (s *MRTService) RecordDecision(ctx context.Context, params DecisionParams) (*DecisionResult, error) {
	switch params.Verdict {
	case domain.MRTDecisionApprove, domain.MRTDecisionBlock, domain.MRTDecisionSkip, domain.MRTDecisionRoute:
		// valid
	default:
		return nil, &domain.ValidationError{
			Message: fmt.Sprintf("mrt.RecordDecision: invalid verdict %q; must be one of APPROVE, BLOCK, SKIP, ROUTE", params.Verdict),
		}
	}

	if params.Verdict == domain.MRTDecisionRoute && params.TargetQueueID == nil {
		return nil, &domain.ValidationError{
			Message: "mrt.RecordDecision: target_queue_id is required when verdict is ROUTE",
		}
	}

	job, err := s.store.GetMRTJob(ctx, params.OrgID, params.JobID)
	if err != nil {
		return nil, err
	}

	if job.Status != domain.MRTJobStatusAssigned {
		return nil, &domain.ValidationError{
			Message: fmt.Sprintf("mrt.RecordDecision: job %s is not in ASSIGNED status (current: %s)",
				params.JobID, job.Status),
		}
	}

	if job.AssignedTo == nil || *job.AssignedTo != params.UserID {
		return nil, &domain.ForbiddenError{
			Message: fmt.Sprintf("mrt.RecordDecision: job %s is not assigned to user %s",
				params.JobID, params.UserID),
		}
	}

	now := time.Now()
	decisionID := fmt.Sprintf("dec_%d", now.UnixNano())

	actionIDs := params.ActionIDs
	if actionIDs == nil {
		actionIDs = []string{}
	}
	policyIDs := params.PolicyIDs
	if policyIDs == nil {
		policyIDs = []string{}
	}

	decision := domain.MRTDecision{
		ID:            decisionID,
		OrgID:         params.OrgID,
		JobID:         params.JobID,
		UserID:        params.UserID,
		Verdict:       params.Verdict,
		ActionIDs:     actionIDs,
		PolicyIDs:     policyIDs,
		Reason:        params.Reason,
		TargetQueueID: params.TargetQueueID,
		CreatedAt:     now,
	}

	var webhookRequired bool

	switch params.Verdict {
	case domain.MRTDecisionApprove, domain.MRTDecisionBlock:
		err = s.recordDecisionDecided(ctx, &decision, job.AssignedTo)
		if err != nil {
			return nil, fmt.Errorf("mrt.RecordDecision: %w", err)
		}
		webhookRequired = true

	case domain.MRTDecisionSkip:
		err = s.recordDecisionSkip(ctx, &decision, params.OrgID, params.JobID)
		if err != nil {
			return nil, fmt.Errorf("mrt.RecordDecision: %w", err)
		}

	case domain.MRTDecisionRoute:
		err = s.recordDecisionRoute(ctx, &decision, params.OrgID, params.JobID, *params.TargetQueueID)
		if err != nil {
			return nil, fmt.Errorf("mrt.RecordDecision: %w", err)
		}
	}

	// Resolve action IDs to Action definitions outside the transaction.
	// Individual GetAction failures are logged but do not invalidate the decision.
	var actionRequests []domain.ActionRequest
	if webhookRequired {
		actionRequests = s.resolveActionRequests(ctx, params.OrgID, params.ActionIDs, job, decisionID)
	}

	s.logger.Info("mrt.RecordDecision: decision recorded",
		"org_id", params.OrgID,
		"job_id", params.JobID,
		"decision_id", decisionID,
		"verdict", params.Verdict,
		"webhook_required", webhookRequired,
	)

	return &DecisionResult{
		Decision:        decision,
		ActionRequests:  actionRequests,
		WebhookRequired: webhookRequired,
		ItemID:          job.ItemID,
		ItemTypeID:      job.ItemTypeID,
		Payload:         job.Payload,
	}, nil
}

// recordDecisionDecided handles APPROVE and BLOCK verdicts: inserts the decision
// and transitions the job to DECIDED within a single transaction.
func (s *MRTService) recordDecisionDecided(
	ctx context.Context,
	decision *domain.MRTDecision,
	assignedTo *string,
) error {
	return s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.InsertMRTDecision(ctx, decision); err != nil {
			return fmt.Errorf("insert mrt decision: %w", err)
		}
		if err := tx.UpdateMRTJobStatus(ctx, decision.OrgID, decision.JobID, domain.MRTJobStatusDecided, assignedTo); err != nil {
			return fmt.Errorf("update mrt job status: %w", err)
		}
		return nil
	})
}

// recordDecisionSkip handles SKIP verdict: inserts the decision and resets the
// job back to PENDING with assigned_to=NULL within a single transaction.
func (s *MRTService) recordDecisionSkip(
	ctx context.Context,
	decision *domain.MRTDecision,
	orgID, jobID string,
) error {
	return s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.InsertMRTDecision(ctx, decision); err != nil {
			return fmt.Errorf("insert mrt decision: %w", err)
		}
		if err := tx.UpdateMRTJobStatus(ctx, orgID, jobID, domain.MRTJobStatusPending, nil); err != nil {
			return fmt.Errorf("update mrt job status to pending: %w", err)
		}
		return nil
	})
}

// recordDecisionRoute handles ROUTE verdict: validates the target queue, inserts
// the decision, and moves the job to the target queue as PENDING within a single transaction.
func (s *MRTService) recordDecisionRoute(
	ctx context.Context,
	decision *domain.MRTDecision,
	orgID, jobID, targetQueueID string,
) error {
	// Validate target queue exists and is not archived (outside the transaction
	// to keep the transaction short; the store check is the final guard).
	queue, err := s.store.GetMRTQueue(ctx, orgID, targetQueueID)
	if err != nil {
		return fmt.Errorf("get target queue: %w", err)
	}
	if queue.ArchivedAt != nil {
		return &domain.NotFoundError{
			Message: fmt.Sprintf("target queue %s is archived and cannot accept new jobs", targetQueueID),
		}
	}

	return s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.InsertMRTDecision(ctx, decision); err != nil {
			return fmt.Errorf("insert mrt decision: %w", err)
		}
		if err := tx.RouteMRTJob(ctx, orgID, jobID, targetQueueID); err != nil {
			return fmt.Errorf("route mrt job: %w", err)
		}
		return nil
	})
}

// resolveActionRequests converts action IDs to domain.ActionRequest values.
// Actions that cannot be found are skipped with a warning log.
func (s *MRTService) resolveActionRequests(
	ctx context.Context,
	orgID string,
	actionIDs []string,
	job *domain.MRTJob,
	correlationID string,
) []domain.ActionRequest {
	requests := make([]domain.ActionRequest, 0, len(actionIDs))
	for _, actionID := range actionIDs {
		action, err := s.store.GetAction(ctx, orgID, actionID)
		if err != nil {
			s.logger.Warn("mrt.resolveActionRequests: action not found, skipping",
				"org_id", orgID,
				"action_id", actionID,
				"error", err,
			)
			continue
		}
		requests = append(requests, domain.ActionRequest{
			Action:        *action,
			ItemID:        job.ItemID,
			Payload:       job.Payload,
			CorrelationID: correlationID,
		})
	}
	return requests
}

// ListQueues returns all MRT queues for an org.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all queues ordered by name (empty slice if none).
// Raises: error on database failure.
func (s *MRTService) ListQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error) {
	queues, err := s.store.ListMRTQueues(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("mrt.ListQueues: %w", err)
	}
	return queues, nil
}

// ListJobs returns a paginated list of MRT jobs for a queue, optionally filtered by status.
//
// Pre-conditions: orgID and queueID must be non-empty. status may be nil.
// Post-conditions: returns paginated jobs ordered by created_at DESC.
// Raises: error on database failure.
func (s *MRTService) ListJobs(
	ctx context.Context,
	orgID, queueID string,
	status *string,
	page domain.PageParams,
) (*domain.PaginatedResult[domain.MRTJob], error) {
	result, err := s.store.ListMRTJobs(ctx, orgID, queueID, status, page)
	if err != nil {
		return nil, fmt.Errorf("mrt.ListJobs: %w", err)
	}
	return result, nil
}

// GetJob returns a single MRT job by org and job ID.
//
// Pre-conditions: orgID and jobID must be non-empty.
// Post-conditions: returns the job if found.
// Raises: domain.NotFoundError if not found.
// Raises: error on database failure.
func (s *MRTService) GetJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error) {
	job, err := s.store.GetMRTJob(ctx, orgID, jobID)
	if err != nil {
		return nil, fmt.Errorf("mrt.GetJob: %w", err)
	}
	return job, nil
}

// CreateQueue validates and persists a new MRT queue.
//
// Pre-conditions: orgID non-empty; params.Name non-empty.
// Post-conditions: queue persisted; returned with generated ID.
// Raises: *domain.ValidationError if name is empty.
// Raises: *domain.ConflictError if (org_id, name) already exists (among active queues).
func (s *MRTService) CreateQueue(ctx context.Context, orgID string, params CreateQueueParams) (*domain.MRTQueue, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, &domain.ValidationError{
			Message: "queue name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}

	now := time.Now()
	queue := &domain.MRTQueue{
		ID:          fmt.Sprintf("q_%d", now.UnixNano()),
		OrgID:       orgID,
		Name:        strings.TrimSpace(params.Name),
		Description: params.Description,
		IsDefault:   params.IsDefault,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateMRTQueue(ctx, queue); err != nil {
		return nil, err
	}

	s.logger.Info("mrt.CreateQueue: queue created",
		"org_id", orgID,
		"queue_id", queue.ID,
		"name", queue.Name,
	)
	return queue, nil
}

// ArchiveQueue soft-deletes an MRT queue and invalidates the UDF cache entry.
//
// Steps:
//  1. Fetch queue by ID to learn the queue name.
//  2. Archive the queue in the database.
//  3. Invalidate the action cache entry for this queue (if cacheInvalidator is set).
//
// Pre-conditions: orgID and queueID non-empty.
// Post-conditions: queue's archived_at is set; queue no longer appears in ListQueues;
//
//	cached queue ID (if any) is evicted from the engine action cache.
//
// Raises: *domain.NotFoundError if queue does not exist or is already archived.
func (s *MRTService) ArchiveQueue(ctx context.Context, orgID, queueID string) error {
	// Step 1: Fetch queue to learn its name (needed for cache key).
	queue, err := s.store.GetMRTQueue(ctx, orgID, queueID)
	if err != nil {
		return err
	}

	// Step 2: Archive in the database.
	if err := s.store.ArchiveMRTQueue(ctx, orgID, queueID); err != nil {
		return err
	}

	// Step 3: Invalidate the UDF cache entry (nil-safe).
	if s.cacheInv != nil {
		s.cacheInv.InvalidateMRTQueue(orgID, queue.Name)
	}

	s.logger.Info("mrt.ArchiveQueue: queue archived",
		"org_id", orgID,
		"queue_id", queueID,
		"name", queue.Name,
	)
	return nil
}
