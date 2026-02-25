package store

import (
	"context"
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
)

// CreateSession inserts a new session.
//
// Pre-conditions: session.SID, session.UserID must be set; session.ExpiresAt must be in the future.
// Post-conditions: session is persisted.
// Raises: error on database failure.
func (q *Queries) CreateSession(ctx context.Context, session domain.Session) error {
	const sql = `
		INSERT INTO sessions (sid, user_id, data, expires_at)
		VALUES ($1, $2, $3, $4)`

	_, err := q.dbtx.Exec(ctx, sql,
		session.SID,
		session.UserID,
		session.Data,
		session.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSession returns a session by SID. Only returns non-expired sessions.
//
// Pre-conditions: sid must be non-empty.
// Post-conditions: returns the session if found and not expired.
// Raises: domain.NotFoundError if not found or expired.
func (q *Queries) GetSession(ctx context.Context, sid string) (*domain.Session, error) {
	const sql = `
		SELECT sid, user_id, data, expires_at
		FROM sessions
		WHERE sid = $1 AND expires_at > now()`

	var s domain.Session
	err := q.dbtx.QueryRow(ctx, sql, sid).Scan(
		&s.SID,
		&s.UserID,
		&s.Data,
		&s.ExpiresAt,
	)
	if err != nil {
		return nil, notFound(err, "session", sid)
	}
	return &s, nil
}

// DeleteSession removes a session.
//
// Pre-conditions: sid must be non-empty.
// Post-conditions: session is deleted (no-op if already absent).
// Raises: error on database failure.
func (q *Queries) DeleteSession(ctx context.Context, sid string) error {
	const sql = `DELETE FROM sessions WHERE sid = $1`

	_, err := q.dbtx.Exec(ctx, sql, sid)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// CleanExpiredSessions deletes all expired sessions and returns the count of deleted rows.
//
// Pre-conditions: none.
// Post-conditions: all sessions with expires_at <= now() are deleted.
// Raises: error on database failure.
func (q *Queries) CleanExpiredSessions(ctx context.Context) (int64, error) {
	const sql = `DELETE FROM sessions WHERE expires_at <= now()`

	tag, err := q.dbtx.Exec(ctx, sql)
	if err != nil {
		return 0, fmt.Errorf("clean expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CreateAPIKey inserts a new API key record.
//
// Pre-conditions: key.ID, key.OrgID, key.KeyHash must be set.
// Post-conditions: API key metadata is persisted. Plaintext key is never stored.
// Raises: error on database failure.
func (q *Queries) CreateAPIKey(ctx context.Context, key domain.ApiKey) error {
	const sql = `
		INSERT INTO api_keys (id, org_id, name, key_hash, prefix, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := q.dbtx.Exec(ctx, sql,
		key.ID,
		key.OrgID,
		key.Name,
		key.KeyHash,
		key.Prefix,
		key.CreatedAt,
		key.RevokedAt,
	)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

// GetAPIKeyByHash looks up an API key by its SHA-256 hash.
// Returns only non-revoked keys.
//
// Pre-conditions: keyHash must be non-empty.
// Post-conditions: returns the matching non-revoked API key.
// Raises: domain.NotFoundError if not found or revoked.
func (q *Queries) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.ApiKey, error) {
	const sql = `
		SELECT id, org_id, name, key_hash, prefix, created_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL`

	var k domain.ApiKey
	err := q.dbtx.QueryRow(ctx, sql, keyHash).Scan(
		&k.ID,
		&k.OrgID,
		&k.Name,
		&k.KeyHash,
		&k.Prefix,
		&k.CreatedAt,
		&k.RevokedAt,
	)
	if err != nil {
		return nil, notFound(err, "api key", keyHash)
	}
	return &k, nil
}

// ListAPIKeys returns all API keys for an org, including revoked ones.
//
// Pre-conditions: orgID must be non-empty.
// Post-conditions: returns all API keys ordered by created_at DESC.
// Raises: error on database failure.
func (q *Queries) ListAPIKeys(ctx context.Context, orgID string) ([]domain.ApiKey, error) {
	const sql = `
		SELECT id, org_id, name, key_hash, prefix, created_at, revoked_at
		FROM api_keys
		WHERE org_id = $1
		ORDER BY created_at DESC`

	rows, err := q.dbtx.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.ApiKey, 0)
	for rows.Next() {
		var k domain.ApiKey
		if err := rows.Scan(
			&k.ID,
			&k.OrgID,
			&k.Name,
			&k.KeyHash,
			&k.Prefix,
			&k.CreatedAt,
			&k.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error listing api keys: %w", err)
	}
	return keys, nil
}

// RevokeAPIKey sets revoked_at on an API key.
//
// Pre-conditions: orgID, keyID must be non-empty.
// Post-conditions: the key's revoked_at is set to now(). No-op if already revoked.
// Raises: domain.NotFoundError if key not found in org.
func (q *Queries) RevokeAPIKey(ctx context.Context, orgID, keyID string) error {
	const sql = `
		UPDATE api_keys
		SET revoked_at = now()
		WHERE org_id = $1 AND id = $2 AND revoked_at IS NULL`

	tag, err := q.dbtx.Exec(ctx, sql, orgID, keyID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &domain.NotFoundError{Message: fmt.Sprintf("api key %s not found in org %s", keyID, orgID)}
	}
	return nil
}

// CreatePasswordResetToken inserts a password reset token.
//
// Pre-conditions: token.ID, token.UserID, token.TokenHash must be set.
// Post-conditions: token is persisted.
// Raises: error on database failure.
func (q *Queries) CreatePasswordResetToken(ctx context.Context, token domain.PasswordResetToken) error {
	const sql = `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := q.dbtx.Exec(ctx, sql,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.UsedAt,
		token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

// GetPasswordResetToken returns a token by its hash.
// Only returns unused, non-expired tokens.
//
// Pre-conditions: tokenHash must be non-empty.
// Post-conditions: returns the matching token if valid.
// Raises: domain.NotFoundError if not found, already used, or expired.
func (q *Queries) GetPasswordResetToken(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	const sql = `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`

	var t domain.PasswordResetToken
	err := q.dbtx.QueryRow(ctx, sql, tokenHash).Scan(
		&t.ID,
		&t.UserID,
		&t.TokenHash,
		&t.ExpiresAt,
		&t.UsedAt,
		&t.CreatedAt,
	)
	if err != nil {
		return nil, notFound(err, "password reset token", tokenHash)
	}
	return &t, nil
}

// MarkPasswordResetTokenUsed sets used_at on a token.
//
// Pre-conditions: tokenID must be non-empty.
// Post-conditions: token.used_at is set to now().
// Raises: error on database failure.
func (q *Queries) MarkPasswordResetTokenUsed(ctx context.Context, tokenID string) error {
	const sql = `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`

	_, err := q.dbtx.Exec(ctx, sql, tokenID)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	return nil
}
