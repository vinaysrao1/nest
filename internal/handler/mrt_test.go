package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/engine"
	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/signal"
	"github.com/vinaysrao1/nest/internal/store"
)

// ---- MRT handler integration test setup ------------------------------------

// setupMRTHandlerDB opens a test database, runs migrations, and returns
// a *store.Queries and a cleanup function.
//
// Tests are skipped if TEST_DATABASE_URL is not set and Docker is unavailable.
func setupMRTHandlerDB(t *testing.T) (*store.Queries, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires TEST_DATABASE_URL to run MRT handler integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("create pgxpool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}

	if err := runMRTHandlerMigrations(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("run migrations: %v", err)
	}

	q := store.New(pool)
	return q, func() { pool.Close() }
}

// runMRTHandlerMigrations applies all SQL migrations for the handler test suite.
func runMRTHandlerMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot determine caller path")
	}
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	migrations := []string{
		filepath.Join(migrationsDir, "001_initial.sql"),
		filepath.Join(migrationsDir, "002_partitions.sql"),
		filepath.Join(migrationsDir, "003_mrt_queue_archive.sql"),
		filepath.Join(migrationsDir, "005_mrt_decision_target_queue.sql"),
	}

	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("execute migration %s: %w", path, err)
			}
		}
	}
	return nil
}

// mrtHandlerTestID generates a unique ID for test isolation.
func mrtHandlerTestID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// seedMRTHandlerOrg inserts a test org and returns its ID.
func seedMRTHandlerOrg(t *testing.T, q *store.Queries) string {
	t.Helper()
	org := &domain.Org{
		ID:        mrtHandlerTestID("org"),
		Name:      "handler-test-org",
		Settings:  map[string]any{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := q.CreateOrg(context.Background(), org); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	return org.ID
}

// seedMRTHandlerUser inserts a test user and returns it.
func seedMRTHandlerUser(t *testing.T, q *store.Queries, orgID, email string) *domain.User {
	t.Helper()
	user := &domain.User{
		ID:        mrtHandlerTestID("usr"),
		OrgID:     orgID,
		Email:     email,
		Name:      "Test Mod",
		Password:  "hashed",
		Role:      domain.UserRoleAdmin,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := q.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return user
}

// seedMRTHandlerItemType inserts a test item type and returns it.
func seedMRTHandlerItemType(t *testing.T, q *store.Queries, orgID string) *domain.ItemType {
	t.Helper()
	it := &domain.ItemType{
		ID:         mrtHandlerTestID("ity"),
		OrgID:      orgID,
		Name:       "content",
		Kind:       domain.ItemTypeKindContent,
		Schema:     map[string]any{"type": "object"},
		FieldRoles: map[string]any{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := q.CreateItemType(context.Background(), it); err != nil {
		t.Fatalf("seed item type: %v", err)
	}
	return it
}

// seedMRTHandlerQueue inserts an MRT queue and returns it.
func seedMRTHandlerQueue(t *testing.T, q *store.Queries, orgID, name string) *domain.MRTQueue {
	t.Helper()
	queue := &domain.MRTQueue{
		ID:          mrtHandlerTestID("q"),
		OrgID:       orgID,
		Name:        name,
		Description: "handler test queue",
		IsDefault:   false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := q.CreateMRTQueue(context.Background(), queue); err != nil {
		t.Fatalf("seed queue %s: %v", name, err)
	}
	return queue
}

// seedMRTHandlerAction inserts a webhook action pointing to the given URL.
func seedMRTHandlerAction(t *testing.T, q *store.Queries, orgID, webhookURL string) *domain.Action {
	t.Helper()
	action := &domain.Action{
		ID:         mrtHandlerTestID("act"),
		OrgID:      orgID,
		Name:       "handler-test-webhook",
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{"url": webhookURL},
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := q.CreateAction(context.Background(), action); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	return action
}

// noopHandlerSigner is a Signer stub for the ActionPublisher in handler tests.
type noopHandlerSigner struct{}

func (n *noopHandlerSigner) Sign(_ context.Context, _ string, _ []byte) (string, error) {
	return "test-sig", nil
}

// buildDecisionRequest serializes a recordDecisionRequest to a JSON bytes buffer.
func buildDecisionRequest(t *testing.T, req recordDecisionRequest) *bytes.Buffer {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal decision request: %v", err)
	}
	return bytes.NewBuffer(body)
}

// withTestAuthCtx wraps an *http.Request with an injected auth context.
func withTestAuthCtx(r *http.Request, orgID, userID string) *http.Request {
	ctx := auth.SetAuthContext(r.Context(), &auth.AuthContext{
		OrgID:  orgID,
		UserID: userID,
		Role:   domain.UserRoleAdmin,
	})
	return r.WithContext(ctx)
}

// ---- TestHandleClaimJob tests -----------------------------------------------

// TestHandleClaimJob_200 verifies that handleClaimJob returns 200 with job JSON
// when a PENDING job is successfully claimed.
func TestHandleClaimJob_200(t *testing.T) {
	q, cleanup := setupMRTHandlerDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedMRTHandlerOrg(t, q)
	it := seedMRTHandlerItemType(t, q, orgID)
	queue := seedMRTHandlerQueue(t, q, orgID, "claim-200-queue")
	user := seedMRTHandlerUser(t, q, orgID, "claim-200@example.com")

	mrtSvc := service.NewMRTService(q, newNopLogger(), nil)

	// Enqueue a PENDING job.
	jobID, err := mrtSvc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        mrtHandlerTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "claim me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	body, err := json.Marshal(claimJobRequest{JobID: jobID})
	if err != nil {
		t.Fatalf("marshal claim request: %v", err)
	}

	handler := handleClaimJob(mrtSvc, newNopLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mrt/jobs/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withTestAuthCtx(req, orgID, user.ID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp domain.MRTJob
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ID != jobID {
		t.Errorf("job.ID: got %q, want %q", resp.ID, jobID)
	}
	if string(resp.Status) != "ASSIGNED" {
		t.Errorf("job.Status: got %q, want ASSIGNED", resp.Status)
	}
}

// TestHandleClaimJob_409 verifies that handleClaimJob returns 409 when the job
// is already assigned to a different user.
func TestHandleClaimJob_409(t *testing.T) {
	q, cleanup := setupMRTHandlerDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedMRTHandlerOrg(t, q)
	it := seedMRTHandlerItemType(t, q, orgID)
	queue := seedMRTHandlerQueue(t, q, orgID, "claim-409-queue")
	mod1 := seedMRTHandlerUser(t, q, orgID, "claim-409-mod1@example.com")
	mod2 := seedMRTHandlerUser(t, q, orgID, "claim-409-mod2@example.com")

	mrtSvc := service.NewMRTService(q, newNopLogger(), nil)

	// Enqueue a PENDING job.
	jobID, err := mrtSvc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        mrtHandlerTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "conflict"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// mod1 claims the job first.
	_, err = mrtSvc.ClaimJob(ctx, orgID, jobID, mod1.ID)
	if err != nil {
		t.Fatalf("ClaimJob (mod1): %v", err)
	}

	// mod2 tries to claim the same job via the handler.
	body, err := json.Marshal(claimJobRequest{JobID: jobID})
	if err != nil {
		t.Fatalf("marshal claim request: %v", err)
	}

	handler := handleClaimJob(mrtSvc, newNopLogger())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mrt/jobs/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withTestAuthCtx(req, orgID, mod2.ID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status: got %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

// TestHandleClaimJob_400 verifies that handleClaimJob returns 400 when job_id is missing.
func TestHandleClaimJob_400(t *testing.T) {
	t.Parallel()

	// No DB needed: the handler rejects the request before calling the service.
	// Pass a nil service — the handler must not call it.
	handler := handleClaimJob(nil, newNopLogger())

	body, err := json.Marshal(claimJobRequest{JobID: ""})
	if err != nil {
		t.Fatalf("marshal claim request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mrt/jobs/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withTestAuthCtx(req, "org-123", "usr-123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// ---- TestHandleRecordDecision_WebhookConditional ----------------------------

// TestHandleRecordDecision_WebhookConditional verifies that handleRecordDecision
// calls ActionPublisher.PublishActions (fires webhook) for APPROVE, and does NOT
// fire the webhook for SKIP. This validates invariant 8: only WebhookRequired=true
// verdicts trigger webhook delivery.
func TestHandleRecordDecision_WebhookConditional(t *testing.T) {
	q, cleanup := setupMRTHandlerDB(t)
	defer cleanup()

	// Track how many times the webhook target is called.
	var webhookCallCount atomic.Int32
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCallCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	ctx := context.Background()
	orgID := seedMRTHandlerOrg(t, q)
	it := seedMRTHandlerItemType(t, q, orgID)
	queue := seedMRTHandlerQueue(t, q, orgID, "handler-test-queue")
	user := seedMRTHandlerUser(t, q, orgID, "mod@example.com")
	action := seedMRTHandlerAction(t, q, orgID, webhookServer.URL)

	signer := &noopHandlerSigner{}
	publisher := engine.NewActionPublisher(q, signer, webhookServer.Client(), newNopLogger())

	registry := signal.NewRegistry()
	pool := engine.NewPool(1, registry, q, newNopLogger())
	defer pool.Stop()
	mrtSvc := service.NewMRTService(q, newNopLogger(), nil)

	pipeline := service.NewPostVerdictPipeline(publisher, q, newNopLogger())
	handler := handleRecordDecision(mrtSvc, pipeline, newNopLogger())

	// --- Scenario 1: APPROVE should fire webhook ---

	time.Sleep(time.Millisecond)
	jobID1, err := mrtSvc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        mrtHandlerTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "approve me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue job1: %v", err)
	}

	job1, err := mrtSvc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job1 == nil {
		t.Fatalf("AssignNext job1: err=%v, job=%v", err, job1)
	}

	approveBody := buildDecisionRequest(t, recordDecisionRequest{
		JobID:     jobID1,
		Verdict:   domain.MRTDecisionApprove,
		ActionIDs: []string{action.ID},
	})
	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/mrt/decisions", approveBody)
	approveReq.Header.Set("Content-Type", "application/json")
	approveReq = withTestAuthCtx(approveReq, orgID, user.ID)

	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)

	if approveRec.Code != http.StatusOK {
		t.Errorf("APPROVE: status=%d, want 200; body=%s", approveRec.Code, approveRec.Body.String())
	}
	if webhookCallCount.Load() != 1 {
		t.Errorf("APPROVE: webhook calls=%d, want 1", webhookCallCount.Load())
	}

	// --- Scenario 2: SKIP should NOT fire webhook ---

	time.Sleep(time.Millisecond)
	jobID2, err := mrtSvc.Enqueue(ctx, service.EnqueueParams{
		OrgID:         orgID,
		QueueName:     queue.Name,
		ItemID:        mrtHandlerTestID("item"),
		ItemTypeID:    it.ID,
		Payload:       map[string]any{"text": "skip me"},
		EnqueueSource: "test",
	})
	if err != nil {
		t.Fatalf("Enqueue job2: %v", err)
	}

	job2, err := mrtSvc.AssignNext(ctx, orgID, queue.ID, user.ID)
	if err != nil || job2 == nil {
		t.Fatalf("AssignNext job2: err=%v, job=%v", err, job2)
	}

	skipBody := buildDecisionRequest(t, recordDecisionRequest{
		JobID:     jobID2,
		Verdict:   domain.MRTDecisionSkip,
		ActionIDs: []string{action.ID}, // action IDs present but should not trigger publish
	})
	skipReq := httptest.NewRequest(http.MethodPost, "/api/v1/mrt/decisions", skipBody)
	skipReq.Header.Set("Content-Type", "application/json")
	skipReq = withTestAuthCtx(skipReq, orgID, user.ID)

	skipRec := httptest.NewRecorder()
	handler.ServeHTTP(skipRec, skipReq)

	if skipRec.Code != http.StatusOK {
		t.Errorf("SKIP: status=%d, want 200; body=%s", skipRec.Code, skipRec.Body.String())
	}
	// Webhook count must remain at 1 (no new webhook calls for SKIP).
	if webhookCallCount.Load() != 1 {
		t.Errorf("SKIP: webhook calls=%d after SKIP, want 1 (no new calls)", webhookCallCount.Load())
	}
}
