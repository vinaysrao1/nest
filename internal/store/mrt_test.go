package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// seedMRTQueue creates a test MRT queue and returns it.
func seedMRTQueue(t *testing.T, q *store.Queries, orgID, name string) *domain.MRTQueue {
	t.Helper()
	queue := &domain.MRTQueue{
		ID:          generateTestID(),
		OrgID:       orgID,
		Name:        name,
		Description: "test queue",
		IsDefault:   false,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := q.CreateMRTQueue(context.Background(), queue); err != nil {
		t.Fatalf("seedMRTQueue %s: %v", name, err)
	}
	return queue
}

// seedMRTJob creates a PENDING MRT job in the given queue.
func seedMRTJob(t *testing.T, q *store.Queries, orgID, queueID, itemID, itemTypeID string) *domain.MRTJob {
	t.Helper()
	job := &domain.MRTJob{
		ID:            generateTestID(),
		OrgID:         orgID,
		QueueID:       queueID,
		ItemID:        itemID,
		ItemTypeID:    itemTypeID,
		Payload:       map[string]any{"key": "value"},
		Status:        domain.MRTJobStatusPending,
		PolicyIDs:     []string{},
		EnqueueSource: "test",
		SourceInfo:    map[string]any{"src": "unit-test"},
		CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := q.InsertMRTJob(context.Background(), job); err != nil {
		t.Fatalf("seedMRTJob: %v", err)
	}
	return job
}

// ---- MRT Queue tests ----

func TestMRTQueues(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-queues-org")

	t.Run("create and get queue", func(t *testing.T) {
		queue := &domain.MRTQueue{
			ID:          generateTestID(),
			OrgID:       orgID,
			Name:        "queue-create-get",
			Description: "a test queue",
			IsDefault:   true,
			CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:   time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.CreateMRTQueue(ctx, queue); err != nil {
			t.Fatalf("CreateMRTQueue: %v", err)
		}

		got, err := q.GetMRTQueue(ctx, orgID, queue.ID)
		if err != nil {
			t.Fatalf("GetMRTQueue: %v", err)
		}

		if got.ID != queue.ID {
			t.Errorf("ID: got %q, want %q", got.ID, queue.ID)
		}
		if got.Name != queue.Name {
			t.Errorf("Name: got %q, want %q", got.Name, queue.Name)
		}
		if got.Description != queue.Description {
			t.Errorf("Description: got %q, want %q", got.Description, queue.Description)
		}
		if got.IsDefault != queue.IsDefault {
			t.Errorf("IsDefault: got %v, want %v", got.IsDefault, queue.IsDefault)
		}
	})

	t.Run("list queues ordered by name", func(t *testing.T) {
		listOrgID := seedOrg(t, q, "mrt-list-queues-org")
		seedMRTQueue(t, q, listOrgID, "list-q-alpha")
		seedMRTQueue(t, q, listOrgID, "list-q-beta")

		queues, err := q.ListMRTQueues(ctx, listOrgID)
		if err != nil {
			t.Fatalf("ListMRTQueues: %v", err)
		}
		if len(queues) != 2 {
			t.Errorf("expected 2 queues, got %d", len(queues))
		}
		if len(queues) == 2 && queues[0].Name >= queues[1].Name {
			t.Errorf("expected ascending name order, got %q then %q", queues[0].Name, queues[1].Name)
		}
	})

	t.Run("get queue by name", func(t *testing.T) {
		nameOrgID := seedOrg(t, q, "mrt-name-lookup-org")
		queue := seedMRTQueue(t, q, nameOrgID, "named-queue")

		got, err := q.GetMRTQueueByName(ctx, nameOrgID, "named-queue")
		if err != nil {
			t.Fatalf("GetMRTQueueByName: %v", err)
		}
		if got.ID != queue.ID {
			t.Errorf("ID: got %q, want %q", got.ID, queue.ID)
		}
	})

	t.Run("duplicate name returns ConflictError", func(t *testing.T) {
		dupOrgID := seedOrg(t, q, "mrt-dup-queue-org")
		seedMRTQueue(t, q, dupOrgID, "dup-queue")

		dup := &domain.MRTQueue{
			ID:        generateTestID(),
			OrgID:     dupOrgID,
			Name:      "dup-queue",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := q.CreateMRTQueue(ctx, dup)
		if err == nil {
			t.Fatal("expected ConflictError, got nil")
		}
		var ce *domain.ConflictError
		if !errors.As(err, &ce) {
			t.Errorf("expected *domain.ConflictError, got %T: %v", err, err)
		}
	})

	t.Run("get non-existent queue returns NotFoundError", func(t *testing.T) {
		_, err := q.GetMRTQueue(ctx, orgID, "no-such-queue-id")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

// ---- MRT Job tests ----

func TestMRTJobs(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-jobs-org")
	user := seedUser(t, q, orgID, "mrt-jobs-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-jobs-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-jobs-queue")

	t.Run("insert and get job with JSONB and array fields", func(t *testing.T) {
		job := &domain.MRTJob{
			ID:            generateTestID(),
			OrgID:         orgID,
			QueueID:       queue.ID,
			ItemID:        generateTestID(),
			ItemTypeID:    it.ID,
			Payload:       map[string]any{"content": "test content", "score": float64(0.95)},
			Status:        domain.MRTJobStatusPending,
			PolicyIDs:     []string{"pol-1", "pol-2"},
			EnqueueSource: "rule-engine",
			SourceInfo:    map[string]any{"rule_id": "rule-abc", "verdict": "flag"},
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertMRTJob(ctx, job); err != nil {
			t.Fatalf("InsertMRTJob: %v", err)
		}

		got, err := q.GetMRTJob(ctx, orgID, job.ID)
		if err != nil {
			t.Fatalf("GetMRTJob: %v", err)
		}

		if got.ID != job.ID {
			t.Errorf("ID: got %q, want %q", got.ID, job.ID)
		}
		if got.Status != domain.MRTJobStatusPending {
			t.Errorf("Status: got %q, want PENDING", got.Status)
		}
		if len(got.PolicyIDs) != 2 {
			t.Errorf("PolicyIDs: got %v, want 2 entries", got.PolicyIDs)
		}
		if got.Payload["content"] != "test content" {
			t.Errorf("Payload[content]: got %v, want %q", got.Payload["content"], "test content")
		}
		if got.SourceInfo["rule_id"] != "rule-abc" {
			t.Errorf("SourceInfo[rule_id]: got %v, want %q", got.SourceInfo["rule_id"], "rule-abc")
		}
	})

	t.Run("list jobs with status filter", func(t *testing.T) {
		listOrgID := seedOrg(t, q, "mrt-list-jobs-org")
		listUser := seedUser(t, q, listOrgID, "mrt-list-jobs-user@example.com")
		listIT := seedItemType(t, q, listOrgID, "mrt-list-jobs-type")
		listQ := seedMRTQueue(t, q, listOrgID, "mrt-list-jobs-queue")

		for i := 0; i < 2; i++ {
			seedMRTJob(t, q, listOrgID, listQ.ID, generateTestID(), listIT.ID)
		}
		assignedJob := seedMRTJob(t, q, listOrgID, listQ.ID, generateTestID(), listIT.ID)
		assignedTo := listUser.ID
		if err := q.UpdateMRTJobStatus(ctx, listOrgID, assignedJob.ID, domain.MRTJobStatusAssigned, &assignedTo); err != nil {
			t.Fatalf("UpdateMRTJobStatus: %v", err)
		}

		pendingStatus := "PENDING"
		result, err := q.ListMRTJobs(ctx, listOrgID, listQ.ID, &pendingStatus, domain.PageParams{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("ListMRTJobs (status=PENDING): %v", err)
		}
		if result.Total != 2 {
			t.Errorf("Total: got %d, want 2", result.Total)
		}
		for _, j := range result.Items {
			if j.Status != domain.MRTJobStatusPending {
				t.Errorf("expected PENDING job, got status %q", j.Status)
			}
		}
	})

	t.Run("list jobs without filter returns all", func(t *testing.T) {
		allOrgID := seedOrg(t, q, "mrt-list-all-jobs-org")
		allIT := seedItemType(t, q, allOrgID, "mrt-list-all-type")
		allQ := seedMRTQueue(t, q, allOrgID, "mrt-list-all-queue")

		for i := 0; i < 3; i++ {
			seedMRTJob(t, q, allOrgID, allQ.ID, generateTestID(), allIT.ID)
		}

		result, err := q.ListMRTJobs(ctx, allOrgID, allQ.ID, nil, domain.PageParams{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("ListMRTJobs (no status filter): %v", err)
		}
		if result.Total != 3 {
			t.Errorf("Total: got %d, want 3", result.Total)
		}
	})

	t.Run("list jobs pagination", func(t *testing.T) {
		pageOrgID := seedOrg(t, q, "mrt-page-jobs-org")
		pageIT := seedItemType(t, q, pageOrgID, "mrt-page-type")
		pageQ := seedMRTQueue(t, q, pageOrgID, "mrt-page-queue")

		for i := 0; i < 5; i++ {
			seedMRTJob(t, q, pageOrgID, pageQ.ID, generateTestID(), pageIT.ID)
		}

		result, err := q.ListMRTJobs(ctx, pageOrgID, pageQ.ID, nil, domain.PageParams{Page: 1, PageSize: 3})
		if err != nil {
			t.Fatalf("ListMRTJobs page 1: %v", err)
		}
		if result.Total != 5 {
			t.Errorf("Total: got %d, want 5", result.Total)
		}
		if len(result.Items) != 3 {
			t.Errorf("Items count page 1: got %d, want 3", len(result.Items))
		}
		if result.TotalPages != 2 {
			t.Errorf("TotalPages: got %d, want 2", result.TotalPages)
		}
	})

	t.Run("update job status", func(t *testing.T) {
		job := seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)
		assignedTo := user.ID

		if err := q.UpdateMRTJobStatus(ctx, orgID, job.ID, domain.MRTJobStatusAssigned, &assignedTo); err != nil {
			t.Fatalf("UpdateMRTJobStatus: %v", err)
		}

		got, err := q.GetMRTJob(ctx, orgID, job.ID)
		if err != nil {
			t.Fatalf("GetMRTJob after update: %v", err)
		}
		if got.Status != domain.MRTJobStatusAssigned {
			t.Errorf("Status: got %q, want ASSIGNED", got.Status)
		}
		if got.AssignedTo == nil || *got.AssignedTo != user.ID {
			t.Errorf("AssignedTo: got %v, want %q", got.AssignedTo, user.ID)
		}
	})

	t.Run("update non-existent job returns NotFoundError", func(t *testing.T) {
		err := q.UpdateMRTJobStatus(ctx, orgID, "no-such-job", domain.MRTJobStatusDecided, nil)
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("get non-existent job returns NotFoundError", func(t *testing.T) {
		_, err := q.GetMRTJob(ctx, orgID, "no-such-job-id")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

// ---- AssignNextMRTJob tests ----

func TestMRTJobs_AssignNext(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-assign-org")
	user := seedUser(t, q, orgID, "mrt-assign-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-assign-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-assign-queue")

	t.Run("returns oldest pending job in FIFO order", func(t *testing.T) {
		// Insert 3 jobs with explicit created_at offsets to ensure deterministic ordering.
		job1 := &domain.MRTJob{
			ID:            generateTestID(),
			OrgID:         orgID,
			QueueID:       queue.ID,
			ItemID:        generateTestID(),
			ItemTypeID:    it.ID,
			Payload:       map[string]any{},
			Status:        domain.MRTJobStatusPending,
			PolicyIDs:     []string{},
			EnqueueSource: "test",
			SourceInfo:    map[string]any{},
			CreatedAt:     time.Now().UTC().Add(-2 * time.Second).Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Add(-2 * time.Second).Truncate(time.Microsecond),
		}
		job2 := &domain.MRTJob{
			ID:            generateTestID(),
			OrgID:         orgID,
			QueueID:       queue.ID,
			ItemID:        generateTestID(),
			ItemTypeID:    it.ID,
			Payload:       map[string]any{},
			Status:        domain.MRTJobStatusPending,
			PolicyIDs:     []string{},
			EnqueueSource: "test",
			SourceInfo:    map[string]any{},
			CreatedAt:     time.Now().UTC().Add(-1 * time.Second).Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Add(-1 * time.Second).Truncate(time.Microsecond),
		}
		job3 := &domain.MRTJob{
			ID:            generateTestID(),
			OrgID:         orgID,
			QueueID:       queue.ID,
			ItemID:        generateTestID(),
			ItemTypeID:    it.ID,
			Payload:       map[string]any{},
			Status:        domain.MRTJobStatusPending,
			PolicyIDs:     []string{},
			EnqueueSource: "test",
			SourceInfo:    map[string]any{},
			CreatedAt:     time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt:     time.Now().UTC().Truncate(time.Microsecond),
		}
		for _, j := range []*domain.MRTJob{job1, job2, job3} {
			if err := q.InsertMRTJob(ctx, j); err != nil {
				t.Fatalf("InsertMRTJob: %v", err)
			}
		}

		assigned, err := q.AssignNextMRTJob(ctx, orgID, queue.ID, user.ID)
		if err != nil {
			t.Fatalf("AssignNextMRTJob (1st): %v", err)
		}
		if assigned.ID != job1.ID {
			t.Errorf("first assign: expected job1 (%q), got %q", job1.ID, assigned.ID)
		}
		if assigned.Status != domain.MRTJobStatusAssigned {
			t.Errorf("Status: got %q, want ASSIGNED", assigned.Status)
		}
		if assigned.AssignedTo == nil || *assigned.AssignedTo != user.ID {
			t.Errorf("AssignedTo: got %v, want %q", assigned.AssignedTo, user.ID)
		}

		assigned2, err := q.AssignNextMRTJob(ctx, orgID, queue.ID, user.ID)
		if err != nil {
			t.Fatalf("AssignNextMRTJob (2nd): %v", err)
		}
		if assigned2.ID != job2.ID {
			t.Errorf("second assign: expected job2 (%q), got %q", job2.ID, assigned2.ID)
		}
	})

	t.Run("no pending jobs returns NotFoundError", func(t *testing.T) {
		emptyOrgID := seedOrg(t, q, "mrt-assign-empty-org")
		emptyIT := seedItemType(t, q, emptyOrgID, "mrt-assign-empty-type")
		emptyQ := seedMRTQueue(t, q, emptyOrgID, "mrt-assign-empty-queue")
		emptyUser := seedUser(t, q, emptyOrgID, "mrt-assign-empty@example.com")

		// Assign the one job so queue becomes empty.
		seedMRTJob(t, q, emptyOrgID, emptyQ.ID, generateTestID(), emptyIT.ID)
		if _, err := q.AssignNextMRTJob(ctx, emptyOrgID, emptyQ.ID, emptyUser.ID); err != nil {
			t.Fatalf("first assign failed unexpectedly: %v", err)
		}

		// Now the queue is empty — expect NotFoundError.
		_, err := q.AssignNextMRTJob(ctx, emptyOrgID, emptyQ.ID, emptyUser.ID)
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

// TestMRTJobs_ConcurrentAssign verifies FOR UPDATE SKIP LOCKED prevents double-assignment.
func TestMRTJobs_ConcurrentAssign(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-concurrent-org")
	user := seedUser(t, q, orgID, "mrt-concurrent-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-concurrent-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-concurrent-queue")

	// Insert exactly 1 PENDING job.
	seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)

	// Launch 2 goroutines concurrently; only 1 should succeed.
	var (
		successCount  int
		notFoundCount int
		mu            sync.Mutex
		wg            sync.WaitGroup
	)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := q.AssignNextMRTJob(ctx, orgID, queue.ID, user.ID)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else {
				var nfe *domain.NotFoundError
				if errors.As(err, &nfe) {
					notFoundCount++
				}
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful assignment, got %d", successCount)
	}
	if notFoundCount != 1 {
		t.Errorf("expected exactly 1 NotFoundError, got %d", notFoundCount)
	}
}

// ---- ClaimMRTJob tests ----

func TestClaimMRTJob_Success(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-claim-success-org")
	user := seedUser(t, q, orgID, "mrt-claim-success-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-claim-success-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-claim-success-queue")

	job := seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)

	claimed, err := q.ClaimMRTJob(ctx, orgID, job.ID, user.ID)
	if err != nil {
		t.Fatalf("ClaimMRTJob: %v", err)
	}
	if claimed.Status != domain.MRTJobStatusAssigned {
		t.Errorf("Status: got %q, want ASSIGNED", claimed.Status)
	}
	if claimed.AssignedTo == nil || *claimed.AssignedTo != user.ID {
		t.Errorf("AssignedTo: got %v, want %q", claimed.AssignedTo, user.ID)
	}
	if claimed.ID != job.ID {
		t.Errorf("ID: got %q, want %q", claimed.ID, job.ID)
	}
}

func TestClaimMRTJob_NotFound(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-claim-notfound-org")
	user := seedUser(t, q, orgID, "mrt-claim-notfound-user@example.com")

	_, err := q.ClaimMRTJob(ctx, orgID, "no-such-job-id", user.ID)
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfe *domain.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

func TestClaimMRTJob_AlreadyAssigned(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-claim-assigned-org")
	user := seedUser(t, q, orgID, "mrt-claim-assigned-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-claim-assigned-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-claim-assigned-queue")

	// Seed a PENDING job, then atomically assign it via AssignNextMRTJob.
	seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)
	assigned, err := q.AssignNextMRTJob(ctx, orgID, queue.ID, user.ID)
	if err != nil {
		t.Fatalf("AssignNextMRTJob: %v", err)
	}

	// Attempt to claim the now-ASSIGNED job -- the WHERE status='PENDING' clause
	// returns 0 rows, so ClaimMRTJob must return NotFoundError.
	_, err = q.ClaimMRTJob(ctx, orgID, assigned.ID, user.ID)
	if err == nil {
		t.Fatal("expected NotFoundError for already-assigned job, got nil")
	}
	var nfe *domain.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

// ---- MRT Queue archive tests ----

func TestMRTQueues_Archive(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-archive-org")

	t.Run("archive sets archived_at", func(t *testing.T) {
		queue := seedMRTQueue(t, q, orgID, "archive-basic")

		if err := q.ArchiveMRTQueue(ctx, orgID, queue.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		got, err := q.GetMRTQueue(ctx, orgID, queue.ID)
		if err != nil {
			t.Fatalf("GetMRTQueue after archive: %v", err)
		}
		if got.ArchivedAt == nil {
			t.Error("ArchivedAt is nil after archive, want non-nil")
		}
	})

	t.Run("archive already-archived returns NotFoundError", func(t *testing.T) {
		queue := seedMRTQueue(t, q, orgID, "archive-twice")
		if err := q.ArchiveMRTQueue(ctx, orgID, queue.ID); err != nil {
			t.Fatalf("first ArchiveMRTQueue: %v", err)
		}

		err := q.ArchiveMRTQueue(ctx, orgID, queue.ID)
		if err == nil {
			t.Fatal("expected NotFoundError on second archive, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("archive non-existent returns NotFoundError", func(t *testing.T) {
		err := q.ArchiveMRTQueue(ctx, orgID, "no-such-queue")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("ListMRTQueues excludes archived", func(t *testing.T) {
		listOrgID := seedOrg(t, q, "mrt-archive-list-org")
		active := seedMRTQueue(t, q, listOrgID, "active-queue")
		archived := seedMRTQueue(t, q, listOrgID, "archived-queue")

		if err := q.ArchiveMRTQueue(ctx, listOrgID, archived.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		queues, err := q.ListMRTQueues(ctx, listOrgID)
		if err != nil {
			t.Fatalf("ListMRTQueues: %v", err)
		}
		if len(queues) != 1 {
			t.Fatalf("expected 1 queue, got %d", len(queues))
		}
		if queues[0].ID != active.ID {
			t.Errorf("expected active queue %q, got %q", active.ID, queues[0].ID)
		}
	})

	t.Run("GetMRTQueueByName excludes archived", func(t *testing.T) {
		nameOrgID := seedOrg(t, q, "mrt-archive-name-org")
		queue := seedMRTQueue(t, q, nameOrgID, "name-lookup-archived")

		if err := q.ArchiveMRTQueue(ctx, nameOrgID, queue.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		_, err := q.GetMRTQueueByName(ctx, nameOrgID, "name-lookup-archived")
		if err == nil {
			t.Fatal("expected NotFoundError for archived queue name, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("GetMRTQueue includes archived", func(t *testing.T) {
		incOrgID := seedOrg(t, q, "mrt-archive-inc-org")
		queue := seedMRTQueue(t, q, incOrgID, "get-includes-archived")

		if err := q.ArchiveMRTQueue(ctx, incOrgID, queue.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		got, err := q.GetMRTQueue(ctx, incOrgID, queue.ID)
		if err != nil {
			t.Fatalf("GetMRTQueue: %v", err)
		}
		if got.ID != queue.ID {
			t.Errorf("ID: got %q, want %q", got.ID, queue.ID)
		}
		if got.ArchivedAt == nil {
			t.Error("ArchivedAt should be non-nil for archived queue")
		}
	})

	t.Run("create duplicate name after archive succeeds", func(t *testing.T) {
		dupOrgID := seedOrg(t, q, "mrt-archive-dup-org")
		queue := seedMRTQueue(t, q, dupOrgID, "reuse-name")

		if err := q.ArchiveMRTQueue(ctx, dupOrgID, queue.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		// Create a new queue with the same name -- should succeed due to partial index.
		newQueue := &domain.MRTQueue{
			ID:        generateTestID(),
			OrgID:     dupOrgID,
			Name:      "reuse-name",
			CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
			UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateMRTQueue(ctx, newQueue); err != nil {
			t.Fatalf("CreateMRTQueue after archive: %v", err)
		}
	})

	t.Run("AssignNext works on archived queue", func(t *testing.T) {
		assignOrgID := seedOrg(t, q, "mrt-archive-assign-org")
		user := seedUser(t, q, assignOrgID, "assign-archived@example.com")
		it := seedItemType(t, q, assignOrgID, "assign-archived-type")
		aQueue := seedMRTQueue(t, q, assignOrgID, "assign-archived-queue")

		// Insert a pending job, then archive the queue.
		job := seedMRTJob(t, q, assignOrgID, aQueue.ID, generateTestID(), it.ID)

		if err := q.ArchiveMRTQueue(ctx, assignOrgID, aQueue.ID); err != nil {
			t.Fatalf("ArchiveMRTQueue: %v", err)
		}

		// AssignNext should still work -- drain behavior.
		assigned, err := q.AssignNextMRTJob(ctx, assignOrgID, aQueue.ID, user.ID)
		if err != nil {
			t.Fatalf("AssignNextMRTJob on archived queue: %v", err)
		}
		if assigned.ID != job.ID {
			t.Errorf("expected job %q, got %q", job.ID, assigned.ID)
		}
		if assigned.Status != domain.MRTJobStatusAssigned {
			t.Errorf("expected ASSIGNED status, got %q", assigned.Status)
		}
	})
}

// ---- RouteMRTJob tests ----

func TestRouteMRTJob(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-route-org")
	user := seedUser(t, q, orgID, "mrt-route-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-route-type")
	srcQueue := seedMRTQueue(t, q, orgID, "mrt-route-src-queue")
	dstQueue := seedMRTQueue(t, q, orgID, "mrt-route-dst-queue")

	t.Run("success: ASSIGNED job moves to target queue as PENDING with no assignee", func(t *testing.T) {
		job := seedMRTJob(t, q, orgID, srcQueue.ID, generateTestID(), it.ID)

		// First assign the job so it is in ASSIGNED status.
		if err := q.UpdateMRTJobStatus(ctx, orgID, job.ID, domain.MRTJobStatusAssigned, &user.ID); err != nil {
			t.Fatalf("UpdateMRTJobStatus to ASSIGNED: %v", err)
		}

		if err := q.RouteMRTJob(ctx, orgID, job.ID, dstQueue.ID); err != nil {
			t.Fatalf("RouteMRTJob: %v", err)
		}

		got, err := q.GetMRTJob(ctx, orgID, job.ID)
		if err != nil {
			t.Fatalf("GetMRTJob after route: %v", err)
		}
		if got.QueueID != dstQueue.ID {
			t.Errorf("QueueID: got %q, want %q", got.QueueID, dstQueue.ID)
		}
		if got.Status != domain.MRTJobStatusPending {
			t.Errorf("Status: got %q, want PENDING", got.Status)
		}
		if got.AssignedTo != nil {
			t.Errorf("AssignedTo: got %v, want nil", got.AssignedTo)
		}
	})

	t.Run("not-assigned: PENDING job returns NotFoundError", func(t *testing.T) {
		job := seedMRTJob(t, q, orgID, srcQueue.ID, generateTestID(), it.ID)

		// Job is PENDING, not ASSIGNED — RouteMRTJob must reject it.
		err := q.RouteMRTJob(ctx, orgID, job.ID, dstQueue.ID)
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected *domain.NotFoundError, got %T: %v", err, err)
		}
	})
}

// ---- MRT Decision tests ----

func TestMRTDecisions(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "mrt-decisions-org")
	user := seedUser(t, q, orgID, "mrt-decisions-user@example.com")
	it := seedItemType(t, q, orgID, "mrt-decisions-type")
	queue := seedMRTQueue(t, q, orgID, "mrt-decisions-queue")
	job := seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)

	t.Run("insert decision with action and policy arrays", func(t *testing.T) {
		decision := &domain.MRTDecision{
			ID:        generateTestID(),
			OrgID:     orgID,
			JobID:     job.ID,
			UserID:    user.ID,
			Verdict:   "block",
			ActionIDs: []string{"action-1", "action-2"},
			PolicyIDs: []string{"policy-a"},
			Reason:    "test decision",
			CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertMRTDecision(ctx, decision); err != nil {
			t.Fatalf("InsertMRTDecision: %v", err)
		}
	})

	t.Run("insert decision with empty arrays", func(t *testing.T) {
		job2 := seedMRTJob(t, q, orgID, queue.ID, generateTestID(), it.ID)
		decision := &domain.MRTDecision{
			ID:        generateTestID(),
			OrgID:     orgID,
			JobID:     job2.ID,
			UserID:    user.ID,
			Verdict:   "approve",
			ActionIDs: []string{},
			PolicyIDs: []string{},
			Reason:    "",
			CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.InsertMRTDecision(ctx, decision); err != nil {
			t.Fatalf("InsertMRTDecision with empty arrays: %v", err)
		}
	})
}
