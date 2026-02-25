// Package store provides all PostgreSQL data access for the Nest system.
// All query methods are on *Queries. No other package executes SQL.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vinaysrao1/nest/internal/domain"
)

// DBTX is the interface satisfied by both *pgxpool.Pool and pgx.Tx.
// All query methods use this interface so they work inside or outside transactions.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

// Queries wraps a DBTX (pool or transaction) and provides all database operations.
// Create via New(pool). Use WithTx for transactional operations.
type Queries struct {
	dbtx DBTX
	pool *pgxpool.Pool // retained for WithTx (need pool.Begin)
}

// New creates a Queries instance backed by a connection pool.
//
// Pre-conditions: pool must be non-nil and connected.
// Post-conditions: returned Queries is ready for use.
func New(pool *pgxpool.Pool) *Queries {
	return &Queries{dbtx: pool, pool: pool}
}

// NewWithDBTX creates a Queries instance backed by a raw DBTX.
// Primarily used in tests where a real connection pool is not available.
// WithTx will panic if called on a Queries created this way.
func NewWithDBTX(dbtx DBTX) *Queries {
	return &Queries{dbtx: dbtx}
}

// Pool returns the underlying connection pool.
// Used by callers that need direct pool access (e.g., river job client).
//
// Post-conditions: returns the pool passed to New.
func (q *Queries) Pool() *pgxpool.Pool {
	return q.pool
}

// WithTx executes fn within a database transaction.
// If fn returns nil, the transaction is committed.
// If fn returns an error or panics, the transaction is rolled back.
//
// Pre-conditions: ctx must not be cancelled.
// Post-conditions: transaction is either committed or rolled back.
// Raises: any error from fn, or transaction begin/commit/rollback errors.
func (q *Queries) WithTx(ctx context.Context, fn func(tx *Queries) error) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is best-effort after commit

	txq := &Queries{dbtx: tx, pool: q.pool}
	if err := fn(txq); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// notFound wraps pgx.ErrNoRows into domain.NotFoundError.
// If err is not pgx.ErrNoRows, it is returned unchanged.
func notFound(err error, entity, id string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.NotFoundError{Message: fmt.Sprintf("%s %s not found", entity, id)}
	}
	return err
}

// isUniqueViolation checks if a pgx error is a unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// conflict wraps unique constraint violations into domain.ConflictError.
// If err is not a unique violation, it is returned unchanged.
func conflict(err error, msg string) error {
	if isUniqueViolation(err) {
		return &domain.ConflictError{Message: msg}
	}
	return err
}

// paginationOffset computes the SQL OFFSET value from PageParams.
// Page numbers below 1 are clamped to 1.
func paginationOffset(p domain.PageParams) int {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	return (p.Page - 1) * p.PageSize
}

// paginationLimit returns the sanitized page size.
// Defaults to 20 if PageSize < 1. Caps at 100 if PageSize > 100.
func paginationLimit(p domain.PageParams) int {
	if p.PageSize < 1 {
		return 20
	}
	if p.PageSize > 100 {
		return 100
	}
	return p.PageSize
}

// buildPaginatedResult constructs a PaginatedResult from a total item count,
// the current page of items, and the page parameters.
// Items is always a non-nil slice to ensure JSON serializes as [] not null.
func buildPaginatedResult[T any](items []T, total int, page domain.PageParams) *domain.PaginatedResult[T] {
	ps := paginationLimit(page)
	p := page.Page
	if p < 1 {
		p = 1
	}
	totalPages := (total + ps - 1) / ps
	if totalPages < 1 {
		totalPages = 1
	}
	if items == nil {
		items = []T{}
	}
	return &domain.PaginatedResult[T]{
		Items:      items,
		Total:      total,
		Page:       p,
		PageSize:   ps,
		TotalPages: totalPages,
	}
}
