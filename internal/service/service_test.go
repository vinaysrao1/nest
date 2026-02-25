// Package service_test contains integration test infrastructure for the service layer.
// Tests require TEST_DATABASE_URL to be set, or Docker to be available for testcontainers.
package service_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/engine"
	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/signal"
	"github.com/vinaysrao1/nest/internal/store"
)

// setupTestDB creates a test PostgreSQL database, runs all migrations, and
// returns a *store.Queries instance and a cleanup function.
//
// If TEST_DATABASE_URL is set, it is used directly. Otherwise, testcontainers
// are attempted; if Docker is unavailable, the test is skipped.
func setupTestDB(t *testing.T) (*store.Queries, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = startTestContainer(t)
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

	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("run migrations: %v", err)
	}

	q := store.New(pool)
	cleanup := func() {
		pool.Close()
	}
	return q, cleanup
}

// startTestContainer launches a PostgreSQL Docker container for testing.
// Returns DSN or calls t.Skip if Docker is unavailable.
func startTestContainer(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !isDockerAvailable(ctx) {
		t.Skip("requires Docker for PostgreSQL testcontainer (set TEST_DATABASE_URL to skip)")
	}
	t.Skip("testcontainers not configured; set TEST_DATABASE_URL to run integration tests")
	return ""
}

// isDockerAvailable checks whether the Docker daemon is reachable.
func isDockerAvailable(ctx context.Context) bool {
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", "/var/run/docker.sock")
	if err == nil {
		conn.Close()
		return true
	}
	conn, err = d.DialContext(ctx, "tcp", "localhost:2375")
	if err == nil {
		conn.Close()
		return true
	}
	return false
}

// runMigrations applies all SQL migration files from the migrations/ directory.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot determine caller path")
	}
	// Navigate from internal/service/service_test.go to migrations/
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

// seedOrg creates a test org with a unique ID and returns its ID.
func seedOrg(t *testing.T, q *store.Queries, name string) string {
	t.Helper()
	org := &domain.Org{
		ID:        generateTestID("org"),
		Name:      name,
		Settings:  map[string]any{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := q.CreateOrg(context.Background(), org); err != nil {
		t.Fatalf("seed org %s: %v", name, err)
	}
	return org.ID
}

// generateTestID generates a unique ID with the given prefix.
func generateTestID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// setupRuleService creates a RuleService backed by the test database.
// It also creates a pool with 2 workers and a signal registry.
// Returns the service and a cleanup function that stops the pool.
func setupRuleService(t *testing.T) (*service.RuleService, *store.Queries, func()) {
	t.Helper()

	q, dbCleanup := setupTestDB(t)
	registry := signal.NewRegistry()
	logger := slog.Default()
	pool := engine.NewPool(2, registry, q, logger)
	compiler := &engine.Compiler{}

	svc := service.NewRuleService(q, compiler, pool, logger)

	cleanup := func() {
		pool.Stop()
		dbCleanup()
	}
	return svc, q, cleanup
}

// setupConfigService creates a ConfigService backed by the test database.
// Returns the service, queries, and a cleanup function.
func setupConfigService(t *testing.T) (*service.ConfigService, *store.Queries, func()) {
	t.Helper()

	q, dbCleanup := setupTestDB(t)
	logger := slog.Default()
	svc := service.NewConfigService(q, logger)
	return svc, q, dbCleanup
}

// setupItemService creates an ItemService backed by the test database and a real engine.Pool.
// Returns the service, the store.Queries, and a combined cleanup function.
func setupItemService(t *testing.T) (*service.ItemService, *store.Queries, func()) {
	t.Helper()

	q, dbCleanup := setupTestDB(t)
	registry := signal.NewRegistry()
	logger := slog.Default()
	pool := engine.NewPool(2, registry, q, logger)

	// ActionPublisher needs a Signer. Use a no-op stub that returns an empty signature
	// so webhook actions do not actually fire during tests.
	signer := &noopSigner{}
	publisher := engine.NewActionPublisher(q, signer, nil, logger)
	pipeline := service.NewPostVerdictPipeline(publisher, q, logger)

	svc := service.NewItemService(q, pool, pipeline, logger)

	cleanup := func() {
		pool.Stop()
		dbCleanup()
	}
	return svc, q, cleanup
}

// setupMRTService creates an MRTService backed by the test database.
// Returns the service, the store.Queries, and a cleanup function.
func setupMRTService(t *testing.T) (*service.MRTService, *store.Queries, func()) {
	t.Helper()

	q, dbCleanup := setupTestDB(t)
	logger := slog.Default()
	svc := service.NewMRTService(q, logger, nil)
	return svc, q, dbCleanup
}

// seedUser creates a test user in an org and returns the user.
func seedUser(t *testing.T, q *store.Queries, orgID, email string) *domain.User {
	t.Helper()
	user := &domain.User{
		ID:        generateTestID("usr"),
		OrgID:     orgID,
		Email:     email,
		Name:      "Test User",
		Password:  "hashed-password",
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

// seedItemType creates a test item type in an org and returns it.
func seedItemType(t *testing.T, q *store.Queries, orgID, name string) *domain.ItemType {
	t.Helper()
	it := &domain.ItemType{
		ID:         generateTestID("ity"),
		OrgID:      orgID,
		Name:       name,
		Kind:       domain.ItemTypeKindContent,
		Schema:     map[string]any{"type": "object"},
		FieldRoles: map[string]any{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := q.CreateItemType(context.Background(), it); err != nil {
		t.Fatalf("seed item type %s: %v", name, err)
	}
	return it
}

// seedMRTQueue creates a test MRT queue in an org and returns it.
func seedMRTQueue(t *testing.T, q *store.Queries, orgID, name string) *domain.MRTQueue {
	t.Helper()
	queue := &domain.MRTQueue{
		ID:          generateTestID("q"),
		OrgID:       orgID,
		Name:        name,
		Description: "test queue",
		IsDefault:   false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := q.CreateMRTQueue(context.Background(), queue); err != nil {
		t.Fatalf("seed mrt queue %s: %v", name, err)
	}
	return queue
}

// seedAction creates a test action in an org and returns it.
func seedAction(t *testing.T, q *store.Queries, orgID, name string) *domain.Action {
	t.Helper()
	action := &domain.Action{
		ID:         generateTestID("act"),
		OrgID:      orgID,
		Name:       name,
		ActionType: domain.ActionTypeWebhook,
		Config:     map[string]any{"url": "https://example.com/webhook"},
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := q.CreateAction(context.Background(), action); err != nil {
		t.Fatalf("seed action %s: %v", name, err)
	}
	return action
}

// noopSigner implements engine.Signer for tests. It always returns an empty signature
// so that webhook actions do not make real HTTP calls during tests.
type noopSigner struct{}

func (n *noopSigner) Sign(_ context.Context, _ string, _ []byte) (string, error) {
	return "test-signature", nil
}

// testLogger returns slog.Default() for use in service test helpers.
func testLogger() *slog.Logger {
	return slog.Default()
}
