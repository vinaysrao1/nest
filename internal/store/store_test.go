// Package store_test contains integration test infrastructure shared across
// all store test files. Tests require either TEST_DATABASE_URL env var or
// Docker to be available (for testcontainers).
package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// setupTestDB creates a test PostgreSQL database, runs all migrations, and
// returns a *store.Queries instance and a cleanup function.
//
// If the TEST_DATABASE_URL environment variable is set, it uses that connection
// string directly (suitable for CI). Otherwise, it attempts to use testcontainers
// with Docker. If Docker is unavailable, the test is skipped.
func setupTestDB(t *testing.T) (*store.Queries, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		// Attempt testcontainers. If Docker is not available, skip.
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

	// Run migrations.
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

// startTestContainer launches a PostgreSQL testcontainer and returns a DSN.
// If Docker is unavailable, it calls t.Skip.
func startTestContainer(t *testing.T) string {
	t.Helper()

	// Try to detect Docker availability cheaply.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !isDockerAvailable(ctx) {
		t.Skip("requires Docker for PostgreSQL testcontainer (set TEST_DATABASE_URL to skip)")
	}

	dsn, err := startPostgresContainer(t)
	if err != nil {
		t.Skipf("start postgres container: %v", err)
	}
	return dsn
}

// runMigrations applies all SQL migration files from the migrations/ directory.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot determine caller path")
	}
	// Navigate from internal/store/store_test.go to migrations/
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	migrations := []string{
		filepath.Join(migrationsDir, "001_initial.sql"),
		filepath.Join(migrationsDir, "002_partitions.sql"),
		filepath.Join(migrationsDir, "003_mrt_queue_archive.sql"),
	}

	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			// Skip if objects already exist (idempotent re-runs in shared DBs).
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
		ID:        generateTestID(),
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

// seedUser creates a test user in an org and returns the user.
func seedUser(t *testing.T, q *store.Queries, orgID, email string) *domain.User {
	t.Helper()
	user := &domain.User{
		ID:        generateTestID(),
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
		ID:         generateTestID(),
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

// generateTestID generates a unique test ID using timestamp.
func generateTestID() string {
	return fmt.Sprintf("test-%d", time.Now().UnixNano())
}
