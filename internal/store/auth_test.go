package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestSessions(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "sessions-org")
	user := seedUser(t, q, orgID, "session-user@example.com")

	t.Run("create and get session", func(t *testing.T) {
		session := domain.Session{
			SID:       "sid-001",
			UserID:    user.ID,
			Data:      map[string]any{"key": "value", "count": float64(42)},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		if err := q.CreateSession(ctx, session); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		got, err := q.GetSession(ctx, session.SID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}

		if got.SID != session.SID {
			t.Errorf("SID: got %q, want %q", got.SID, session.SID)
		}
		if got.UserID != session.UserID {
			t.Errorf("UserID: got %q, want %q", got.UserID, session.UserID)
		}
		// Verify JSONB round-trip
		if got.Data["key"] != "value" {
			t.Errorf("Data[key]: got %v, want %q", got.Data["key"], "value")
		}
		if got.Data["count"] != float64(42) {
			t.Errorf("Data[count]: got %v, want 42", got.Data["count"])
		}
	})

	t.Run("get expired session returns NotFoundError", func(t *testing.T) {
		expired := domain.Session{
			SID:       "sid-expired",
			UserID:    user.ID,
			Data:      map[string]any{},
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}
		if err := q.CreateSession(ctx, expired); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		_, err := q.GetSession(ctx, expired.SID)
		if err == nil {
			t.Fatal("expected NotFoundError for expired session, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("delete session", func(t *testing.T) {
		session := domain.Session{
			SID:       "sid-to-delete",
			UserID:    user.ID,
			Data:      map[string]any{},
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := q.CreateSession(ctx, session); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		if err := q.DeleteSession(ctx, session.SID); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}

		_, err := q.GetSession(ctx, session.SID)
		if err == nil {
			t.Fatal("expected NotFoundError after delete, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("clean expired sessions", func(t *testing.T) {
		// Insert 2 expired + 1 valid session
		orgID2 := seedOrg(t, q, "clean-sessions-org")
		user2 := seedUser(t, q, orgID2, "clean-session-user@example.com")

		for _, sid := range []string{"clean-expired-1", "clean-expired-2"} {
			s := domain.Session{
				SID:       sid,
				UserID:    user2.ID,
				Data:      map[string]any{},
				ExpiresAt: time.Now().Add(-time.Hour),
			}
			if err := q.CreateSession(ctx, s); err != nil {
				t.Fatalf("CreateSession(%s): %v", sid, err)
			}
		}
		valid := domain.Session{
			SID:       "clean-valid",
			UserID:    user2.ID,
			Data:      map[string]any{},
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := q.CreateSession(ctx, valid); err != nil {
			t.Fatalf("CreateSession(valid): %v", err)
		}

		n, err := q.CleanExpiredSessions(ctx)
		if err != nil {
			t.Fatalf("CleanExpiredSessions: %v", err)
		}
		// At least 2 deleted (there may be more from earlier sub-tests)
		if n < 2 {
			t.Errorf("CleanExpiredSessions: expected at least 2 deleted, got %d", n)
		}

		// Valid session must still exist
		if _, err := q.GetSession(ctx, valid.SID); err != nil {
			t.Errorf("valid session should still exist after clean: %v", err)
		}
	})
}

func TestAPIKeys(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "api-keys-org")

	t.Run("create and get by hash", func(t *testing.T) {
		key := domain.ApiKey{
			ID:        "key-001",
			OrgID:     orgID,
			Name:      "test-key",
			KeyHash:   "sha256hash-abc123",
			Prefix:    "nst_",
			CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.CreateAPIKey(ctx, key); err != nil {
			t.Fatalf("CreateAPIKey: %v", err)
		}

		got, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
		if err != nil {
			t.Fatalf("GetAPIKeyByHash: %v", err)
		}

		if got.ID != key.ID {
			t.Errorf("ID: got %q, want %q", got.ID, key.ID)
		}
		if got.OrgID != key.OrgID {
			t.Errorf("OrgID: got %q, want %q", got.OrgID, key.OrgID)
		}
		if got.Name != key.Name {
			t.Errorf("Name: got %q, want %q", got.Name, key.Name)
		}
		if got.KeyHash != key.KeyHash {
			t.Errorf("KeyHash: got %q, want %q", got.KeyHash, key.KeyHash)
		}
		if got.Prefix != key.Prefix {
			t.Errorf("Prefix: got %q, want %q", got.Prefix, key.Prefix)
		}
		if got.RevokedAt != nil {
			t.Errorf("RevokedAt: expected nil, got %v", got.RevokedAt)
		}
	})

	t.Run("get revoked key returns NotFoundError", func(t *testing.T) {
		key := domain.ApiKey{
			ID:        "key-revoked",
			OrgID:     orgID,
			Name:      "revoked-key",
			KeyHash:   "sha256hash-revoked",
			Prefix:    "nst_",
			CreatedAt: time.Now().UTC(),
		}
		if err := q.CreateAPIKey(ctx, key); err != nil {
			t.Fatalf("CreateAPIKey: %v", err)
		}
		if err := q.RevokeAPIKey(ctx, orgID, key.ID); err != nil {
			t.Fatalf("RevokeAPIKey: %v", err)
		}

		_, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
		if err == nil {
			t.Fatal("expected NotFoundError for revoked key, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("list api keys", func(t *testing.T) {
		orgID2 := seedOrg(t, q, "list-keys-org")
		for i, name := range []string{"key-alpha", "key-beta"} {
			k := domain.ApiKey{
				ID:        "list-key-" + name,
				OrgID:     orgID2,
				Name:      name,
				KeyHash:   "hash-list-" + name + "-" + string(rune('0'+i)),
				Prefix:    "nst_",
				CreatedAt: time.Now().UTC(),
			}
			if err := q.CreateAPIKey(ctx, k); err != nil {
				t.Fatalf("CreateAPIKey(%s): %v", name, err)
			}
		}

		keys, err := q.ListAPIKeys(ctx, orgID2)
		if err != nil {
			t.Fatalf("ListAPIKeys: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("revoke api key", func(t *testing.T) {
		key := domain.ApiKey{
			ID:        "key-to-revoke",
			OrgID:     orgID,
			Name:      "key-to-revoke",
			KeyHash:   "sha256hash-to-revoke",
			Prefix:    "nst_",
			CreatedAt: time.Now().UTC(),
		}
		if err := q.CreateAPIKey(ctx, key); err != nil {
			t.Fatalf("CreateAPIKey: %v", err)
		}

		if err := q.RevokeAPIKey(ctx, orgID, key.ID); err != nil {
			t.Fatalf("RevokeAPIKey: %v", err)
		}

		_, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
		if err == nil {
			t.Fatal("expected NotFoundError after revoke, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("revoke non-existent key returns NotFoundError", func(t *testing.T) {
		err := q.RevokeAPIKey(ctx, orgID, "nonexistent-key-id")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestPasswordResetTokens(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "prt-org")
	user := seedUser(t, q, orgID, "prt-user@example.com")

	t.Run("create and get token", func(t *testing.T) {
		token := domain.PasswordResetToken{
			ID:        "prt-001",
			UserID:    user.ID,
			TokenHash: "tokenhash-abc123",
			ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond),
			CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		}

		if err := q.CreatePasswordResetToken(ctx, token); err != nil {
			t.Fatalf("CreatePasswordResetToken: %v", err)
		}

		got, err := q.GetPasswordResetToken(ctx, token.TokenHash)
		if err != nil {
			t.Fatalf("GetPasswordResetToken: %v", err)
		}

		if got.ID != token.ID {
			t.Errorf("ID: got %q, want %q", got.ID, token.ID)
		}
		if got.UserID != token.UserID {
			t.Errorf("UserID: got %q, want %q", got.UserID, token.UserID)
		}
		if got.TokenHash != token.TokenHash {
			t.Errorf("TokenHash: got %q, want %q", got.TokenHash, token.TokenHash)
		}
		if got.UsedAt != nil {
			t.Errorf("UsedAt: expected nil, got %v", got.UsedAt)
		}
	})

	t.Run("get expired token returns NotFoundError", func(t *testing.T) {
		token := domain.PasswordResetToken{
			ID:        "prt-expired",
			UserID:    user.ID,
			TokenHash: "tokenhash-expired",
			ExpiresAt: time.Now().Add(-time.Hour).UTC(),
			CreatedAt: time.Now().UTC(),
		}
		if err := q.CreatePasswordResetToken(ctx, token); err != nil {
			t.Fatalf("CreatePasswordResetToken: %v", err)
		}

		_, err := q.GetPasswordResetToken(ctx, token.TokenHash)
		if err == nil {
			t.Fatal("expected NotFoundError for expired token, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("mark token used then get returns NotFoundError", func(t *testing.T) {
		token := domain.PasswordResetToken{
			ID:        "prt-to-use",
			UserID:    user.ID,
			TokenHash: "tokenhash-to-use",
			ExpiresAt: time.Now().Add(time.Hour).UTC(),
			CreatedAt: time.Now().UTC(),
		}
		if err := q.CreatePasswordResetToken(ctx, token); err != nil {
			t.Fatalf("CreatePasswordResetToken: %v", err)
		}

		if err := q.MarkPasswordResetTokenUsed(ctx, token.ID); err != nil {
			t.Fatalf("MarkPasswordResetTokenUsed: %v", err)
		}

		_, err := q.GetPasswordResetToken(ctx, token.TokenHash)
		if err == nil {
			t.Fatal("expected NotFoundError for used token, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// isNotFound checks if err is a *domain.NotFoundError and assigns it to nfe.
func isNotFound(err error, nfe **domain.NotFoundError) bool {
	if e, ok := err.(*domain.NotFoundError); ok {
		*nfe = e
		return true
	}
	return false
}
