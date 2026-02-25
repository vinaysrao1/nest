package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

// GetUserByEmail returns a user by email address.
// NOTE: This is the ONLY query that does NOT filter by org_id.
// Used during login when the org is unknown.
//
// Pre-conditions: email must be non-empty.
// Post-conditions: returns the matching user with their org_id populated.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	const sql = `
		SELECT id, org_id, email, name, password, role, is_active, created_at, updated_at
		FROM users
		WHERE email = $1`

	row := q.dbtx.QueryRow(ctx, sql, email)
	u, err := scanUser(row)
	if err != nil {
		return nil, notFound(err, "user", email)
	}
	return u, nil
}

// GetUserByID returns a user by org and user ID.
//
// Pre-conditions: orgID, userID must be non-empty.
// Post-conditions: returns the matching user.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetUserByID(ctx context.Context, orgID, userID string) (*domain.User, error) {
	const sql = `
		SELECT id, org_id, email, name, password, role, is_active, created_at, updated_at
		FROM users
		WHERE org_id = $1 AND id = $2`

	row := q.dbtx.QueryRow(ctx, sql, orgID, userID)
	u, err := scanUser(row)
	if err != nil {
		return nil, notFound(err, "user", userID)
	}
	return u, nil
}

// GetUserByIDGlobal returns a user by ID without org scoping.
// Used only by the password reset flow where org is unknown.
//
// Pre-conditions: userID must be non-empty.
// Post-conditions: returns the user if found.
// Raises: domain.NotFoundError if not found.
func (q *Queries) GetUserByIDGlobal(ctx context.Context, userID string) (*domain.User, error) {
	const sql = `
		SELECT id, org_id, email, name, password, role, is_active, created_at, updated_at
		FROM users
		WHERE id = $1`

	row := q.dbtx.QueryRow(ctx, sql, userID)
	u, err := scanUser(row)
	if err != nil {
		return nil, notFound(err, "user", userID)
	}
	return u, nil
}

// ListUsers returns a paginated list of users for an org.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns users ordered by created_at DESC.
// Raises: error on database failure.
func (q *Queries) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error) {
	const countSQL = `SELECT COUNT(*) FROM users WHERE org_id = $1`
	const selectSQL = `
		SELECT id, org_id, email, name, password, role, is_active, created_at, updated_at
		FROM users
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	var total int
	if err := q.dbtx.QueryRow(ctx, countSQL, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	limit := paginationLimit(page)
	offset := paginationOffset(page)

	rows, err := q.dbtx.Query(ctx, selectSQL, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error listing users: %w", err)
	}

	return buildPaginatedResult(users, total, page), nil
}

// CreateUser inserts a new user.
//
// Pre-conditions: user.ID, user.OrgID, user.Email must be set.
// Post-conditions: user is persisted.
// Raises: domain.ConflictError if (org_id, email) unique constraint violated.
func (q *Queries) CreateUser(ctx context.Context, user *domain.User) error {
	const sql = `
		INSERT INTO users (id, org_id, email, name, password, role, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := q.dbtx.Exec(ctx, sql,
		user.ID,
		user.OrgID,
		user.Email,
		user.Name,
		user.Password,
		user.Role,
		user.IsActive,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return conflict(err, fmt.Sprintf("user with email %s already exists in org %s", user.Email, user.OrgID))
	}
	return nil
}

// UpdateUser updates an existing user.
//
// Pre-conditions: user.ID, user.OrgID must be set.
// Post-conditions: user is updated; updated_at set to now().
// Raises: domain.NotFoundError if not found.
func (q *Queries) UpdateUser(ctx context.Context, user *domain.User) error {
	const sql = `
		UPDATE users
		SET email = $3, name = $4, password = $5, role = $6, is_active = $7, updated_at = now()
		WHERE org_id = $1 AND id = $2`

	tag, err := q.dbtx.Exec(ctx, sql,
		user.OrgID,
		user.ID,
		user.Email,
		user.Name,
		user.Password,
		user.Role,
		user.IsActive,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("user %s not found", user.ID)}
	}
	return nil
}

// DeleteUser deletes a user.
//
// Pre-conditions: orgID, userID must be non-empty.
// Post-conditions: user is deleted.
// Raises: domain.NotFoundError if not found.
func (q *Queries) DeleteUser(ctx context.Context, orgID, userID string) error {
	const sql = `DELETE FROM users WHERE org_id = $1 AND id = $2`

	tag, err := q.dbtx.Exec(ctx, sql, orgID, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("user %s not found", userID)}
	}
	return nil
}

// scanUser scans a row into a domain.User.
// The row parameter accepts both pgx.Row and pgx.Rows via the rowScanner interface.
func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID,
		&u.OrgID,
		&u.Email,
		&u.Name,
		&u.Password,
		&u.Role,
		&u.IsActive,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
