package service_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// TestEnqueue_CreatesJob verifies that Enqueue persists a PENDING job in the correct queue.
func TestEnqueue_CreatesJob(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestEnqueue_CreatesJob")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "default-queue")

	params := service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "review me"},
		EnqueueSource: "test",
		SourceInfo:    map[string]any{"test": true},
		PolicyIDs:     []string{},
	}

	jobID, err := svc.Enqueue(ctx, params)
	if err != nil {
		t.Fatalf("Enqueue returned unexpected error: %v", err)
	}
	if jobID == "" {
		t.Fatal("Enqueue returned empty job ID")
	}

	// Verify the job exists and has PENDING status.
	job, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob returned unexpected error: %v", err)
	}
	if job.Status != domain.MRTJobStatusPending {
		t.Errorf("expected status %q, got %q", domain.MRTJobStatusPending, job.Status)
	}
	if job.QueueID != queue.ID {
		t.Errorf("expected queue_id %q, got %q", queue.ID, job.QueueID)
	}
	if job.ItemID != params.ItemID {
		t.Errorf("expected item_id %q, got %q", params.ItemID, job.ItemID)
	}
}

// TestEnqueue_QueueNotFound verifies that Enqueue returns a NotFoundError when
// the named queue does not exist.
func TestEnqueue_QueueNotFound(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestEnqueue_QueueNotFound")
	it := seedItemType(t, q, orgID, "content")

	params := service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     "nonexistent-queue",
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "hello"},
		EnqueueSource: "test",
	}

	_, err := svc.Enqueue(ctx, params)
	if err == nil {
		t.Fatal("expected error for nonexistent queue, got nil")
	}
	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestAssignNext_ReturnsPendingJob verifies that AssignNext claims the oldest PENDING job
// and returns it with ASSIGNED status.
func TestAssignNext_ReturnsPendingJob(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestAssignNext_ReturnsPendingJob")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "review-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	// Enqueue a job.
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "review"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Assign the next job.
	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil {
		t.Fatalf("AssignNext returned unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("AssignNext returned nil job, expected a job")
	}
	if job.ID != jobID {
		t.Errorf("expected job ID %q, got %q", jobID, job.ID)
	}
	if job.Status != domain.MRTJobStatusAssigned {
		t.Errorf("expected status %q, got %q", domain.MRTJobStatusAssigned, job.Status)
	}
	if job.AssignedTo == nil || *job.AssignedTo != user.ID {
		t.Errorf("expected assigned_to %q, got %v", user.ID, job.AssignedTo)
	}
}

// TestAssignNext_EmptyQueue verifies that AssignNext returns nil, nil when no pending
// jobs are available — an empty queue is not an error.
func TestAssignNext_EmptyQueue(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestAssignNext_EmptyQueue")
	queue := seedMRTQueue(t, q, orgID, "empty-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil {
		t.Fatalf("AssignNext returned unexpected error for empty queue: %v", err)
	}
	if job != nil {
		t.Errorf("expected nil job for empty queue, got job ID %q", job.ID)
	}
}

// TestRecordDecision_Success verifies that RecordDecision:
//   - transitions the job to DECIDED
//   - persists the decision
//   - returns ActionRequests (not executed)
//   - does NOT call ActionPublisher
func TestRecordDecision_Success(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Success")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "decision-queue")
	user := seedUser(t, q, orgID, "mod@example.com")
	action := seedAction(t, q, orgID, "notify-webhook")

	// Enqueue and assign a job.
	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "decide me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	// Record a decision with the seeded action.
	result, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:     orgID,
		JobID:     jobID,
		UserID:    user.ID,
		Verdict:   domain.MRTDecisionApprove,
		Reason:    "looks fine",
		ActionIDs: []string{action.ID},
		PolicyIDs: []string{},
	})
	if err != nil {
		t.Fatalf("RecordDecision returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("RecordDecision returned nil result")
	}

	// Decision fields should be correct.
	if result.Decision.Verdict != domain.MRTDecisionApprove {
		t.Errorf("decision.Verdict = %q, want %q", result.Decision.Verdict, domain.MRTDecisionApprove)
	}
	if result.Decision.UserID != user.ID {
		t.Errorf("decision.UserID = %q, want %q", result.Decision.UserID, user.ID)
	}
	if result.Decision.JobID != jobID {
		t.Errorf("decision.JobID = %q, want %q", result.Decision.JobID, jobID)
	}

	// ActionRequests are returned but NOT executed by the service (invariant 8).
	if len(result.ActionRequests) != 1 {
		t.Errorf("expected 1 ActionRequest, got %d", len(result.ActionRequests))
	} else {
		ar := result.ActionRequests[0]
		if ar.Action.ID != action.ID {
			t.Errorf("ActionRequest.Action.ID = %q, want %q", ar.Action.ID, action.ID)
		}
	}

	// Job should now be in DECIDED status.
	updatedJob, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob after decision: %v", err)
	}
	if updatedJob.Status != domain.MRTJobStatusDecided {
		t.Errorf("expected job status %q after decision, got %q",
			domain.MRTJobStatusDecided, updatedJob.Status)
	}
}

// TestRecordDecision_WrongUser verifies that RecordDecision returns ForbiddenError
// when the requesting user is not the assigned moderator.
func TestRecordDecision_WrongUser(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_WrongUser")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "decision-queue")
	mod := seedUser(t, q, orgID, "mod@example.com")
	time.Sleep(time.Millisecond)
	otherMod := seedUser(t, q, orgID, "other@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "review"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Assign to mod.
	if _, err := svc.AssignNext(ctx, orgID, queue.ID, mod.ID); err != nil {
		t.Fatalf("AssignNext: %v", err)
	}

	// Try to record decision as otherMod.
	_, err = svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  otherMod.ID, // wrong user
		Verdict: domain.MRTDecisionApprove,
	})
	if err == nil {
		t.Fatal("expected ForbiddenError, got nil")
	}
	var forbErr *domain.ForbiddenError
	if !isForbiddenError(err, &forbErr) {
		t.Errorf("expected *domain.ForbiddenError, got %T: %v", err, err)
	}
}

// TestRecordDecision_WrongStatus verifies that RecordDecision returns ValidationError
// when the job is not in ASSIGNED status.
func TestRecordDecision_WrongStatus(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_WrongStatus")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "decision-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "review"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// The job is PENDING, not ASSIGNED — decision should fail.
	_, err = svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  user.ID,
		Verdict: domain.MRTDecisionApprove,
	})
	if err == nil {
		t.Fatal("expected ValidationError for PENDING job, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestListQueues verifies that ListQueues returns all queues for an org.
func TestListQueues(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestListQueues")
	time.Sleep(time.Millisecond)
	queue1 := seedMRTQueue(t, q, orgID, "queue-alpha")
	time.Sleep(time.Millisecond)
	queue2 := seedMRTQueue(t, q, orgID, "queue-beta")

	queues, err := svc.ListQueues(ctx, orgID)
	if err != nil {
		t.Fatalf("ListQueues returned unexpected error: %v", err)
	}
	if len(queues) < 2 {
		t.Fatalf("expected at least 2 queues, got %d", len(queues))
	}

	// Verify both seeded queues are present.
	found := map[string]bool{queue1.ID: false, queue2.ID: false}
	for _, q := range queues {
		if _, ok := found[q.ID]; ok {
			found[q.ID] = true
		}
	}
	for id, seen := range found {
		if !seen {
			t.Errorf("queue %q not found in ListQueues result", id)
		}
	}
}

// TestListJobs_WithStatusFilter verifies that ListJobs correctly filters by status.
func TestListJobs_WithStatusFilter(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestListJobs_WithStatusFilter")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "filter-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	// Enqueue two jobs.
	for i := range 2 {
		time.Sleep(time.Millisecond)
		_, err := svc.Enqueue(ctx, service.EnqueueParams{
			OrgID:         orgID,
			QueueName:     queue.Name,
			ItemID:        generateTestID("item"),
			ItemTypeID:    it.ID,
			Payload:       map[string]any{"index": i},
			EnqueueSource: "test",
		})
		if err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Assign one job.
	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	// Filter by PENDING — should return exactly one job.
	pendingStatus := "PENDING"
	page := domain.PageParams{Page: 1, PageSize: 20}
	result, err := svc.ListJobs(ctx, orgID, queue.ID, &pendingStatus, page)
	if err != nil {
		t.Fatalf("ListJobs(PENDING): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 PENDING job, got %d", result.Total)
	}

	// Filter by ASSIGNED — should return exactly one job.
	assignedStatus := "ASSIGNED"
	result, err = svc.ListJobs(ctx, orgID, queue.ID, &assignedStatus, page)
	if err != nil {
		t.Fatalf("ListJobs(ASSIGNED): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 ASSIGNED job, got %d", result.Total)
	}
}

// isForbiddenError checks whether err is a *domain.ForbiddenError (direct or wrapped).
func isForbiddenError(err error, target **domain.ForbiddenError) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if asErr, ok := err.(*domain.ForbiddenError); ok {
			*target = asErr
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}

// isConflictError checks whether err is a *domain.ConflictError (direct or wrapped).
func isConflictError(err error, target **domain.ConflictError) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if asErr, ok := err.(*domain.ConflictError); ok {
			*target = asErr
			return true
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}

// TestCreateQueue_Success verifies that CreateQueue persists a new queue.
func TestCreateQueue_Success(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestCreateQueue_Success")

	queue, err := svc.CreateQueue(ctx, orgID, service.CreateQueueParams{
		Name:        "new-queue",
		Description: "A new test queue",
		IsDefault:   false,
	})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	if queue == nil {
		t.Fatal("CreateQueue returned nil queue")
	}
	if queue.Name != "new-queue" {
		t.Errorf("Name: got %q, want %q", queue.Name, "new-queue")
	}
	if queue.ID == "" {
		t.Error("ID is empty")
	}

	// Verify it appears in ListQueues.
	queues, err := svc.ListQueues(ctx, orgID)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	found := false
	for _, qu := range queues {
		if qu.ID == queue.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created queue not found in ListQueues")
	}
}

// TestCreateQueue_EmptyName verifies that CreateQueue returns ValidationError for empty name.
func TestCreateQueue_EmptyName(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestCreateQueue_EmptyName")

	_, err := svc.CreateQueue(ctx, orgID, service.CreateQueueParams{
		Name: "",
	})
	if err == nil {
		t.Fatal("expected ValidationError, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestCreateQueue_DuplicateName verifies that CreateQueue returns ConflictError
// when a queue with the same name already exists in the org.
func TestCreateQueue_DuplicateName(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestCreateQueue_DuplicateName")

	_, err := svc.CreateQueue(ctx, orgID, service.CreateQueueParams{
		Name: "dup-queue",
	})
	if err != nil {
		t.Fatalf("first CreateQueue: %v", err)
	}

	_, err = svc.CreateQueue(ctx, orgID, service.CreateQueueParams{
		Name: "dup-queue",
	})
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	var confErr *domain.ConflictError
	if !isConflictError(err, &confErr) {
		t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
	}
}

// TestArchiveQueue_Success verifies that ArchiveQueue removes queue from ListQueues.
func TestArchiveQueue_Success(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestArchiveQueue_Success")
	queue := seedMRTQueue(t, q, orgID, "to-archive")

	if err := svc.ArchiveQueue(ctx, orgID, queue.ID); err != nil {
		t.Fatalf("ArchiveQueue: %v", err)
	}

	queues, err := svc.ListQueues(ctx, orgID)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	for _, qu := range queues {
		if qu.ID == queue.ID {
			t.Error("archived queue still appears in ListQueues")
		}
	}
}

// TestArchiveQueue_NotFound verifies that ArchiveQueue returns NotFoundError.
func TestArchiveQueue_NotFound(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestArchiveQueue_NotFound")

	err := svc.ArchiveQueue(ctx, orgID, "no-such-queue-id")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestArchiveQueue_InvalidatesCache verifies that ArchiveQueue calls CacheInvalidator.
func TestArchiveQueue_InvalidatesCache(t *testing.T) {
	q, dbCleanup := setupTestDB(t)
	defer dbCleanup()
	logger := slog.Default()

	var calledOrgID, calledQueueName string
	mockInv := service.CacheInvalidatorFunc(func(orgID, queueName string) {
		calledOrgID = orgID
		calledQueueName = queueName
	})

	svc := service.NewMRTService(q, logger, mockInv)

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestArchiveQueue_InvalidatesCache")
	queue := seedMRTQueue(t, q, orgID, "cached-queue")

	if err := svc.ArchiveQueue(ctx, orgID, queue.ID); err != nil {
		t.Fatalf("ArchiveQueue: %v", err)
	}

	if calledOrgID != orgID {
		t.Errorf("InvalidateMRTQueue orgID: got %q, want %q", calledOrgID, orgID)
	}
	if calledQueueName != queue.Name {
		t.Errorf("InvalidateMRTQueue queueName: got %q, want %q", calledQueueName, queue.Name)
	}
}

// TestArchiveQueue_NilCacheInvalidator verifies no panic when cacheInvalidator is nil.
func TestArchiveQueue_NilCacheInvalidator(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestArchiveQueue_NilCacheInvalidator")
	queue := seedMRTQueue(t, q, orgID, "nil-inv-queue")

	// Should not panic -- setupMRTService passes nil CacheInvalidator.
	if err := svc.ArchiveQueue(ctx, orgID, queue.ID); err != nil {
		t.Fatalf("ArchiveQueue with nil CacheInvalidator: %v", err)
	}
}

// TestRecordDecision_Approve verifies that an APPROVE verdict transitions the job to DECIDED
// and sets WebhookRequired=true.
func TestRecordDecision_Approve(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Approve")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "approve-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "approve me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	result, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  user.ID,
		Verdict: domain.MRTDecisionApprove,
		Reason:  "looks good",
	})
	if err != nil {
		t.Fatalf("RecordDecision(APPROVE): unexpected error: %v", err)
	}
	if result.Decision.Verdict != domain.MRTDecisionApprove {
		t.Errorf("verdict: got %q, want %q", result.Decision.Verdict, domain.MRTDecisionApprove)
	}
	if !result.WebhookRequired {
		t.Error("WebhookRequired: got false, want true for APPROVE")
	}

	updatedJob, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if updatedJob.Status != domain.MRTJobStatusDecided {
		t.Errorf("job status: got %q, want %q", updatedJob.Status, domain.MRTJobStatusDecided)
	}
}

// TestRecordDecision_Block verifies that a BLOCK verdict transitions the job to DECIDED
// and sets WebhookRequired=true.
func TestRecordDecision_Block(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Block")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "block-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "block me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	result, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  user.ID,
		Verdict: domain.MRTDecisionBlock,
		Reason:  "policy violation",
	})
	if err != nil {
		t.Fatalf("RecordDecision(BLOCK): unexpected error: %v", err)
	}
	if result.Decision.Verdict != domain.MRTDecisionBlock {
		t.Errorf("verdict: got %q, want %q", result.Decision.Verdict, domain.MRTDecisionBlock)
	}
	if !result.WebhookRequired {
		t.Error("WebhookRequired: got false, want true for BLOCK")
	}

	updatedJob, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if updatedJob.Status != domain.MRTJobStatusDecided {
		t.Errorf("job status: got %q, want %q", updatedJob.Status, domain.MRTJobStatusDecided)
	}
}

// TestRecordDecision_Skip verifies that a SKIP verdict resets the job to PENDING
// with assigned_to=NULL and sets WebhookRequired=false.
func TestRecordDecision_Skip(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Skip")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "skip-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "skip me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	result, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  user.ID,
		Verdict: domain.MRTDecisionSkip,
		Reason:  "need more info",
	})
	if err != nil {
		t.Fatalf("RecordDecision(SKIP): unexpected error: %v", err)
	}
	if result.Decision.Verdict != domain.MRTDecisionSkip {
		t.Errorf("verdict: got %q, want %q", result.Decision.Verdict, domain.MRTDecisionSkip)
	}
	if result.WebhookRequired {
		t.Error("WebhookRequired: got true, want false for SKIP")
	}

	// Job should be PENDING again with no assignee.
	updatedJob, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if updatedJob.Status != domain.MRTJobStatusPending {
		t.Errorf("job status: got %q, want %q", updatedJob.Status, domain.MRTJobStatusPending)
	}
	if updatedJob.AssignedTo != nil {
		t.Errorf("assigned_to: got %q, want nil", *updatedJob.AssignedTo)
	}
}

// TestRecordDecision_Route verifies that a ROUTE verdict moves the job to the target queue
// as PENDING with assigned_to=NULL, records target_queue_id on the decision, and
// sets WebhookRequired=false.
func TestRecordDecision_Route(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Route")
	it := seedItemType(t, q, orgID, "content")
	srcQueue := seedMRTQueue(t, q, orgID, "source-queue")
	dstQueue := seedMRTQueue(t, q, orgID, "target-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     srcQueue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "route me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.AssignNext(ctx, orgID, srcQueue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	result, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:         orgID,
		JobID:         jobID,
		UserID:        user.ID,
		Verdict:       domain.MRTDecisionRoute,
		Reason:        "escalate to specialist queue",
		TargetQueueID: &dstQueue.ID,
	})
	if err != nil {
		t.Fatalf("RecordDecision(ROUTE): unexpected error: %v", err)
	}
	if result.Decision.Verdict != domain.MRTDecisionRoute {
		t.Errorf("verdict: got %q, want %q", result.Decision.Verdict, domain.MRTDecisionRoute)
	}
	if result.Decision.TargetQueueID == nil || *result.Decision.TargetQueueID != dstQueue.ID {
		t.Errorf("target_queue_id: got %v, want %q", result.Decision.TargetQueueID, dstQueue.ID)
	}
	if result.WebhookRequired {
		t.Error("WebhookRequired: got true, want false for ROUTE")
	}

	// Job should now belong to the target queue and be PENDING with no assignee.
	updatedJob, err := svc.GetJob(ctx, orgID, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if updatedJob.Status != domain.MRTJobStatusPending {
		t.Errorf("job status: got %q, want %q", updatedJob.Status, domain.MRTJobStatusPending)
	}
	if updatedJob.AssignedTo != nil {
		t.Errorf("assigned_to: got %q, want nil", *updatedJob.AssignedTo)
	}
	if updatedJob.QueueID != dstQueue.ID {
		t.Errorf("queue_id: got %q, want %q (target queue)", updatedJob.QueueID, dstQueue.ID)
	}
}

// TestRecordDecision_Route_ArchivedQueue verifies that RecordDecision returns
// NotFoundError when the ROUTE target queue has been archived.
func TestRecordDecision_Route_ArchivedQueue(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Route_ArchivedQueue")
	it := seedItemType(t, q, orgID, "content")
	srcQueue := seedMRTQueue(t, q, orgID, "source-queue")
	dstQueue := seedMRTQueue(t, q, orgID, "archived-target-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	// Enqueue a job in the source queue.
	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     srcQueue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "route me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Assign the job to the user.
	job, err := svc.AssignNext(ctx, orgID, srcQueue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}

	// Archive the target queue before routing.
	if err := svc.ArchiveQueue(ctx, orgID, dstQueue.ID); err != nil {
		t.Fatalf("ArchiveQueue: %v", err)
	}

	// RecordDecision with ROUTE pointing to the archived target queue must return NotFoundError.
	_, err = svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:         orgID,
		JobID:         jobID,
		UserID:        user.ID,
		Verdict:       domain.MRTDecisionRoute,
		Reason:        "escalate",
		TargetQueueID: &dstQueue.ID,
	})
	if err == nil {
		t.Fatal("expected NotFoundError for archived target queue, got nil")
	}
	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestRecordDecision_Route_MissingTargetQueueID verifies that RecordDecision returns
// ValidationError when ROUTE verdict is used without a target_queue_id.
func TestRecordDecision_Route_MissingTargetQueueID(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_Route_MissingTargetQueueID")

	_, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:         orgID,
		JobID:         generateTestID("job"),
		UserID:        generateTestID("usr"),
		Verdict:       domain.MRTDecisionRoute,
		TargetQueueID: nil,
	})
	if err == nil {
		t.Fatal("expected ValidationError, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestRecordDecision_InvalidVerdict verifies that RecordDecision returns ValidationError
// for an unrecognized verdict string.
func TestRecordDecision_InvalidVerdict(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestRecordDecision_InvalidVerdict")

	_, err := svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   generateTestID("job"),
		UserID:  generateTestID("usr"),
		Verdict: "REJECT", // not a valid verdict
	})
	if err == nil {
		t.Fatal("expected ValidationError for invalid verdict, got nil")
	}
	var valErr *domain.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected *domain.ValidationError, got %T: %v", err, err)
	}
}

// TestClaimJob_Success verifies that ClaimJob claims a PENDING job and returns it as ASSIGNED.
func TestClaimJob_Success(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestClaimJob_Success")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "claim-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "claim me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	job, err := svc.ClaimJob(ctx, orgID, jobID, user.ID)
	if err != nil {
		t.Fatalf("ClaimJob returned unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("ClaimJob returned nil job")
	}
	if job.ID != jobID {
		t.Errorf("job.ID: got %q, want %q", job.ID, jobID)
	}
	if job.Status != domain.MRTJobStatusAssigned {
		t.Errorf("job.Status: got %q, want ASSIGNED", job.Status)
	}
	if job.AssignedTo == nil || *job.AssignedTo != user.ID {
		t.Errorf("job.AssignedTo: got %v, want %q", job.AssignedTo, user.ID)
	}
}

// TestClaimJob_Idempotent verifies that ClaimJob returns the job without error when the
// job is already ASSIGNED to the same user (idempotent re-claim).
func TestClaimJob_Idempotent(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestClaimJob_Idempotent")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "idempotent-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "idempotent"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// First claim succeeds.
	_, err = svc.ClaimJob(ctx, orgID, jobID, user.ID)
	if err != nil {
		t.Fatalf("first ClaimJob: %v", err)
	}

	// Second claim by the same user returns the job without error.
	job, err := svc.ClaimJob(ctx, orgID, jobID, user.ID)
	if err != nil {
		t.Fatalf("idempotent ClaimJob returned unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("idempotent ClaimJob returned nil job")
	}
	if job.Status != domain.MRTJobStatusAssigned {
		t.Errorf("job.Status: got %q, want ASSIGNED", job.Status)
	}
	if job.AssignedTo == nil || *job.AssignedTo != user.ID {
		t.Errorf("job.AssignedTo: got %v, want %q", job.AssignedTo, user.ID)
	}
}

// TestClaimJob_ConflictOtherUser verifies that ClaimJob returns ConflictError when the
// job is already ASSIGNED to a different user.
func TestClaimJob_ConflictOtherUser(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestClaimJob_ConflictOtherUser")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "conflict-user-queue")
	mod1 := seedUser(t, q, orgID, "mod1@example.com")
	time.Sleep(time.Millisecond)
	mod2 := seedUser(t, q, orgID, "mod2@example.com")

	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "conflict"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// mod1 claims the job first.
	_, err = svc.ClaimJob(ctx, orgID, jobID, mod1.ID)
	if err != nil {
		t.Fatalf("first ClaimJob (mod1): %v", err)
	}

	// mod2 tries to claim the same job.
	_, err = svc.ClaimJob(ctx, orgID, jobID, mod2.ID)
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	var confErr *domain.ConflictError
	if !isConflictError(err, &confErr) {
		t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
	}
}

// TestClaimJob_ConflictDecided verifies that ClaimJob returns ConflictError when the
// job is in DECIDED status.
func TestClaimJob_ConflictDecided(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestClaimJob_ConflictDecided")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "decided-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "decide then claim"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Assign then record a decision to put the job in DECIDED status.
	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job == nil {
		t.Fatalf("AssignNext: err=%v, job=%v", err, job)
	}
	_, err = svc.RecordDecision(ctx, service.DecisionParams{
		OrgID:   orgID,
		JobID:   jobID,
		UserID:  user.ID,
		Verdict: domain.MRTDecisionApprove,
	})
	if err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	// Now try to claim the DECIDED job.
	_, err = svc.ClaimJob(ctx, orgID, jobID, user.ID)
	if err == nil {
		t.Fatal("expected ConflictError for DECIDED job, got nil")
	}
	var confErr *domain.ConflictError
	if !isConflictError(err, &confErr) {
		t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
	}
}

// TestClaimJob_NotFound verifies that ClaimJob returns NotFoundError when the job does not exist.
func TestClaimJob_NotFound(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestClaimJob_NotFound")
	user := seedUser(t, q, orgID, "mod@example.com")

	_, err := svc.ClaimJob(ctx, orgID, "no-such-job-id", user.ID)
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfErr *domain.NotFoundError
	if !isNotFoundError(err, &nfErr) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// TestAssignNext_ArchivedQueue verifies that AssignNext still works on archived queues (drain behavior).
func TestAssignNext_ArchivedQueue(t *testing.T) {
	svc, q, cleanup := setupMRTService(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "TestAssignNext_ArchivedQueue")
	it := seedItemType(t, q, orgID, "content")
	queue := seedMRTQueue(t, q, orgID, "drain-queue")
	user := seedUser(t, q, orgID, "mod@example.com")

	// Enqueue a job.
	time.Sleep(time.Millisecond)
	jobID, err := svc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        generateTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "drain test"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Archive the queue.
	if err := svc.ArchiveQueue(ctx, orgID, queue.ID); err != nil {
		t.Fatalf("ArchiveQueue: %v", err)
	}

	// AssignNext should still return the pending job.
	job, err := svc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil {
		t.Fatalf("AssignNext on archived queue: %v", err)
	}
	if job == nil {
		t.Fatal("AssignNext returned nil, expected a job")
	}
	if job.ID != jobID {
		t.Errorf("expected job %q, got %q", jobID, job.ID)
	}
}
