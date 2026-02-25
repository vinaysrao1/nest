package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vinaysrao1/nest/internal/domain"
)

// ---- MRT Queue SQL constants ----

const listMRTQueuesSQL = `
SELECT id, org_id, name, COALESCE(description, ''), is_default, archived_at, created_at, updated_at
FROM mrt_queues
WHERE org_id = $1 AND archived_at IS NULL
ORDER BY name ASC`

const getMRTQueueSQL = `
SELECT id, org_id, name, COALESCE(description, ''), is_default, archived_at, created_at, updated_at
FROM mrt_queues
WHERE org_id = $1 AND id = $2`

const getMRTQueueByNameSQL = `
SELECT id, org_id, name, COALESCE(description, ''), is_default, archived_at, created_at, updated_at
FROM mrt_queues
WHERE org_id = $1 AND name = $2 AND archived_at IS NULL`

const createMRTQueueSQL = `
INSERT INTO mrt_queues (id, org_id, name, description, is_default, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`

const archiveMRTQueueSQL = `
UPDATE mrt_queues SET archived_at = now(), updated_at = now()
WHERE org_id = $1 AND id = $2 AND archived_at IS NULL`

// ---- MRT Job SQL constants ----

const getMRTJobSQL = `
SELECT id, org_id, queue_id, item_id, item_type_id, payload, status, assigned_to,
       policy_ids, enqueue_source, source_info, created_at, updated_at
FROM mrt_jobs
WHERE org_id = $1 AND id = $2`

const insertMRTJobSQL = `
INSERT INTO mrt_jobs (id, org_id, queue_id, item_id, item_type_id, payload, status,
       assigned_to, policy_ids, enqueue_source, source_info, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

// assignNextMRTJobSQL uses FOR UPDATE SKIP LOCKED to atomically claim the oldest PENDING job.
const assignNextMRTJobSQL = `
UPDATE mrt_jobs
SET status = 'ASSIGNED', assigned_to = $3, updated_at = now()
WHERE id = (
    SELECT id FROM mrt_jobs
    WHERE org_id = $1 AND queue_id = $2 AND status = 'PENDING'
    ORDER BY created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, org_id, queue_id, item_id, item_type_id, payload, status, assigned_to,
          policy_ids, enqueue_source, source_info, created_at, updated_at`

// claimMRTJobSQL atomically claims a specific PENDING job for a user.
// Standard UPDATE row-locking handles concurrency: a second concurrent UPDATE
// on the same row blocks until the first commits, then re-evaluates the WHERE
// clause (status = 'PENDING' will be false), returning 0 rows.
// Note: unlike assignNextMRTJobSQL, no FOR UPDATE SKIP LOCKED subquery is needed
// because we target a single row by primary key, not selecting from a set.
const claimMRTJobSQL = `
UPDATE mrt_jobs
SET status = 'ASSIGNED', assigned_to = $3, updated_at = now()
WHERE org_id = $1 AND id = $2 AND status = 'PENDING'
RETURNING id, org_id, queue_id, item_id, item_type_id, payload, status, assigned_to,
          policy_ids, enqueue_source, source_info, created_at, updated_at`

const insertMRTDecisionSQL = `
INSERT INTO mrt_decisions (id, org_id, job_id, user_id, verdict, action_ids, policy_ids, reason, target_queue_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

const routeMRTJobSQL = `
UPDATE mrt_jobs
SET queue_id = $3, status = 'PENDING', assigned_to = NULL, updated_at = now()
WHERE org_id = $1 AND id = $2 AND status = 'ASSIGNED'`

const updateMRTJobStatusSQL = `
UPDATE mrt_jobs
SET status = $3, assigned_to = $4, updated_at = now()
WHERE org_id = $1 AND id = $2`

// ---- MRT Queue methods ----

// ListMRTQueues returns all MRT queues for an org, ordered by name.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all queues for the org (empty slice if none).
// Raises: error on database failure.
func (q *Queries) ListMRTQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error) {
	rows, err := q.dbtx.Query(ctx, listMRTQueuesSQL, orgID)
	if err != nil {
		return nil, fmt.Errorf("list mrt queues: %w", err)
	}
	defer rows.Close()

	queues := make([]domain.MRTQueue, 0)
	for rows.Next() {
		queue, scanErr := scanMRTQueue(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan mrt queue: %w", scanErr)
		}
		queues = append(queues, *queue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list mrt queues rows: %w", err)
	}
	return queues, nil
}

// GetMRTQueue returns a single MRT queue by org and queue ID.
//
// Pre-conditions: orgID and queueID must be non-empty.
// Post-conditions: returns the queue if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetMRTQueue(ctx context.Context, orgID, queueID string) (*domain.MRTQueue, error) {
	row := q.dbtx.QueryRow(ctx, getMRTQueueSQL, orgID, queueID)
	queue, err := scanMRTQueue(row)
	if err != nil {
		return nil, notFound(err, "mrt_queue", queueID)
	}
	return queue, nil
}

// GetMRTQueueByName returns an MRT queue by its unique (org_id, name).
//
// Pre-conditions: orgID and name must be non-empty.
// Post-conditions: returns the queue if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetMRTQueueByName(ctx context.Context, orgID, name string) (*domain.MRTQueue, error) {
	row := q.dbtx.QueryRow(ctx, getMRTQueueByNameSQL, orgID, name)
	queue, err := scanMRTQueue(row)
	if err != nil {
		return nil, notFound(err, "mrt_queue", name)
	}
	return queue, nil
}

// CreateMRTQueue inserts a new MRT queue.
//
// Pre-conditions: queue.ID, queue.OrgID, queue.Name must be set.
// Post-conditions: queue is persisted.
// Raises: domain.ConflictError if (org_id, name) unique constraint violated.
func (q *Queries) CreateMRTQueue(ctx context.Context, queue *domain.MRTQueue) error {
	_, err := q.dbtx.Exec(ctx, createMRTQueueSQL,
		queue.ID,
		queue.OrgID,
		queue.Name,
		queue.Description,
		queue.IsDefault,
		queue.CreatedAt,
		queue.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return &domain.ConflictError{
				Message: fmt.Sprintf("mrt queue with name %q already exists in org %s", queue.Name, queue.OrgID),
			}
		}
		return fmt.Errorf("create mrt queue: %w", err)
	}
	return nil
}

// ArchiveMRTQueue soft-deletes an MRT queue by setting archived_at to now().
//
// Pre-conditions: orgID and queueID must be non-empty.
// Post-conditions: queue's archived_at is set to now() and updated_at is refreshed.
// Raises: domain.NotFoundError if queue not found or already archived.
func (q *Queries) ArchiveMRTQueue(ctx context.Context, orgID, queueID string) error {
	tag, err := q.dbtx.Exec(ctx, archiveMRTQueueSQL, orgID, queueID)
	if err != nil {
		return fmt.Errorf("archive mrt queue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("mrt_queue %s not found or already archived", queueID)}
	}
	return nil
}

// ---- MRT Job methods ----

// ListMRTJobs returns a paginated list of MRT jobs for a queue, optionally filtered by status.
//
// Pre-conditions: orgID and queueID must be non-empty. status may be nil (returns all).
// Post-conditions: returns paginated jobs ordered by created_at DESC.
// Raises: error on database failure.
func (q *Queries) ListMRTJobs(
	ctx context.Context,
	orgID, queueID string,
	status *string,
	page domain.PageParams,
) (*domain.PaginatedResult[domain.MRTJob], error) {
	limit := paginationLimit(page)
	offset := paginationOffset(page)

	if status != nil {
		return q.listMRTJobsWithStatus(ctx, orgID, queueID, *status, limit, offset, page)
	}
	return q.listMRTJobsAll(ctx, orgID, queueID, limit, offset, page)
}

func (q *Queries) listMRTJobsWithStatus(
	ctx context.Context,
	orgID, queueID, status string,
	limit, offset int,
	page domain.PageParams,
) (*domain.PaginatedResult[domain.MRTJob], error) {
	countSQL := `SELECT COUNT(*) FROM mrt_jobs WHERE org_id = $1 AND queue_id = $2 AND status = $3`
	selectSQL := `
SELECT id, org_id, queue_id, item_id, item_type_id, payload, status, assigned_to,
       policy_ids, enqueue_source, source_info, created_at, updated_at
FROM mrt_jobs
WHERE org_id = $1 AND queue_id = $2 AND status = $3
ORDER BY created_at DESC
LIMIT $4 OFFSET $5`

	var total int
	if err := q.dbtx.QueryRow(ctx, countSQL, orgID, queueID, status).Scan(&total); err != nil {
		return nil, fmt.Errorf("count mrt jobs: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, selectSQL, orgID, queueID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list mrt jobs with status: %w", err)
	}
	return collectMRTJobRows(rows, total, page)
}

func (q *Queries) listMRTJobsAll(
	ctx context.Context,
	orgID, queueID string,
	limit, offset int,
	page domain.PageParams,
) (*domain.PaginatedResult[domain.MRTJob], error) {
	countSQL := `SELECT COUNT(*) FROM mrt_jobs WHERE org_id = $1 AND queue_id = $2`
	selectSQL := `
SELECT id, org_id, queue_id, item_id, item_type_id, payload, status, assigned_to,
       policy_ids, enqueue_source, source_info, created_at, updated_at
FROM mrt_jobs
WHERE org_id = $1 AND queue_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`

	var total int
	if err := q.dbtx.QueryRow(ctx, countSQL, orgID, queueID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count mrt jobs: %w", err)
	}

	rows, err := q.dbtx.Query(ctx, selectSQL, orgID, queueID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list mrt jobs: %w", err)
	}
	return collectMRTJobRows(rows, total, page)
}

// GetMRTJob returns a single MRT job by org and job ID.
//
// Pre-conditions: orgID and jobID must be non-empty.
// Post-conditions: returns the job if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetMRTJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error) {
	row := q.dbtx.QueryRow(ctx, getMRTJobSQL, orgID, jobID)
	job, err := scanMRTJob(row)
	if err != nil {
		return nil, notFound(err, "mrt_job", jobID)
	}
	return job, nil
}

// InsertMRTJob inserts a new MRT job.
//
// Pre-conditions: job.ID, job.OrgID, job.QueueID, job.ItemID, job.ItemTypeID must be set.
// Post-conditions: job is persisted.
// Raises: error on database failure.
func (q *Queries) InsertMRTJob(ctx context.Context, job *domain.MRTJob) error {
	_, err := q.dbtx.Exec(ctx, insertMRTJobSQL,
		job.ID,
		job.OrgID,
		job.QueueID,
		job.ItemID,
		job.ItemTypeID,
		job.Payload,
		string(job.Status),
		job.AssignedTo,
		job.PolicyIDs,
		job.EnqueueSource,
		job.SourceInfo,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert mrt job: %w", err)
	}
	return nil
}

// AssignNextMRTJob atomically assigns the oldest PENDING job in a queue to a user.
// Uses SELECT ... FOR UPDATE SKIP LOCKED to prevent double-assignment under concurrency.
//
// Pre-conditions: orgID, queueID, and userID must be non-empty.
// Post-conditions: the oldest PENDING job is updated to ASSIGNED with assigned_to=userID and returned.
// Raises: domain.NotFoundError if no pending jobs exist in the queue.
func (q *Queries) AssignNextMRTJob(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error) {
	row := q.dbtx.QueryRow(ctx, assignNextMRTJobSQL, orgID, queueID, userID)
	job, err := scanMRTJob(row)
	if err != nil {
		return nil, notFound(err, "mrt_job (pending)", queueID)
	}
	return job, nil
}

// ClaimMRTJob atomically claims a specific PENDING job for a user.
//
// Pre-conditions: orgID, jobID, and userID must be non-empty.
// Post-conditions: if successful, the job has status ASSIGNED with assigned_to = userID.
// Raises: domain.NotFoundError if no PENDING job matches (not found, already assigned, or decided).
func (q *Queries) ClaimMRTJob(ctx context.Context, orgID, jobID, userID string) (*domain.MRTJob, error) {
	row := q.dbtx.QueryRow(ctx, claimMRTJobSQL, orgID, jobID, userID)
	job, err := scanMRTJob(row)
	if err != nil {
		return nil, notFound(err, "mrt_job (pending)", jobID)
	}
	return job, nil
}

// InsertMRTDecision inserts a decision for an MRT job.
//
// Pre-conditions: decision.ID, decision.OrgID, decision.JobID, decision.UserID must be set.
// Post-conditions: decision is persisted.
// Raises: error on database failure.
func (q *Queries) InsertMRTDecision(ctx context.Context, decision *domain.MRTDecision) error {
	_, err := q.dbtx.Exec(ctx, insertMRTDecisionSQL,
		decision.ID,
		decision.OrgID,
		decision.JobID,
		decision.UserID,
		decision.Verdict,
		decision.ActionIDs,
		decision.PolicyIDs,
		decision.Reason,
		decision.TargetQueueID,
		decision.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert mrt decision: %w", err)
	}
	return nil
}

// RouteMRTJob moves an ASSIGNED MRT job to a different queue, resetting it to PENDING
// with assigned_to = NULL. Used by the Route flow.
//
// Pre-conditions: orgID, jobID, targetQueueID must be non-empty. Job must be in ASSIGNED status.
// Post-conditions: job queue_id is updated, status is PENDING, assigned_to is NULL.
// Raises: domain.NotFoundError if job not found or not in ASSIGNED status.
func (q *Queries) RouteMRTJob(ctx context.Context, orgID, jobID, targetQueueID string) error {
	tag, err := q.dbtx.Exec(ctx, routeMRTJobSQL, orgID, jobID, targetQueueID)
	if err != nil {
		return fmt.Errorf("route mrt job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("mrt_job %s not found or not in ASSIGNED status", jobID)}
	}
	return nil
}

// UpdateMRTJobStatus updates the status and optionally the assigned_to field of an MRT job.
//
// Pre-conditions: orgID and jobID must be non-empty. status must be a valid MRTJobStatus.
// Post-conditions: job status and assigned_to are updated, updated_at is set to now().
// Raises: domain.NotFoundError if not found.
func (q *Queries) UpdateMRTJobStatus(
	ctx context.Context,
	orgID, jobID string,
	status domain.MRTJobStatus,
	assignedTo *string,
) error {
	tag, err := q.dbtx.Exec(ctx, updateMRTJobStatusSQL, orgID, jobID, string(status), assignedTo)
	if err != nil {
		return fmt.Errorf("update mrt job status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("mrt_job %s not found", jobID)}
	}
	return nil
}

// ---- Scan helpers ----

// scanMRTQueue scans a single MRT queue from a rowScanner (pgx.Row or pgx.Rows).
func scanMRTQueue(row rowScanner) (*domain.MRTQueue, error) {
	var mq domain.MRTQueue
	err := row.Scan(
		&mq.ID,
		&mq.OrgID,
		&mq.Name,
		&mq.Description,
		&mq.IsDefault,
		&mq.ArchivedAt,
		&mq.CreatedAt,
		&mq.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &mq, nil
}

// scanMRTJob scans a single MRT job from a rowScanner (pgx.Row or pgx.Rows).
// JSONB fields (Payload, SourceInfo) are scanned into map[string]any.
// TEXT[] field (PolicyIDs) is scanned into []string.
func scanMRTJob(row rowScanner) (*domain.MRTJob, error) {
	var j domain.MRTJob
	var statusStr string
	err := row.Scan(
		&j.ID,
		&j.OrgID,
		&j.QueueID,
		&j.ItemID,
		&j.ItemTypeID,
		&j.Payload,
		&statusStr,
		&j.AssignedTo,
		&j.PolicyIDs,
		&j.EnqueueSource,
		&j.SourceInfo,
		&j.CreatedAt,
		&j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	j.Status = domain.MRTJobStatus(statusStr)
	if j.PolicyIDs == nil {
		j.PolicyIDs = []string{}
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	if j.SourceInfo == nil {
		j.SourceInfo = map[string]any{}
	}
	return &j, nil
}

// collectMRTJobRows iterates pgx.Rows and builds a PaginatedResult.
func collectMRTJobRows(rows pgx.Rows, total int, page domain.PageParams) (*domain.PaginatedResult[domain.MRTJob], error) {
	defer rows.Close()
	jobs := make([]domain.MRTJob, 0)
	for rows.Next() {
		job, err := scanMRTJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan mrt job row: %w", err)
		}
		jobs = append(jobs, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mrt job rows iteration: %w", err)
	}
	return buildPaginatedResult(jobs, total, page), nil
}
