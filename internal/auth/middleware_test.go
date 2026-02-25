package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// ---------------------------------------------------------------------------
// Mock infrastructure
// ---------------------------------------------------------------------------

// mockDBTX satisfies store.DBTX without a real database connection.
type mockDBTX struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("mockDBTX: Exec not implemented")
}

func (m *mockDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("mockDBTX: Query not implemented")
}

func (m *mockDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFn(ctx, sql, args...)
}

func (m *mockDBTX) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("mockDBTX: CopyFrom not implemented")
}

// mockRow is a pgx.Row whose Scan behavior is controlled by a function.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	return r.scanFn(dest...)
}

// ---------------------------------------------------------------------------
// Session store helpers
// ---------------------------------------------------------------------------

// newMockSessionStore returns a *store.Queries backed by a mockDBTX that
// serves sessions from the provided map, keyed by session ID.
func newMockSessionStore(sessions map[string]*domain.Session) *store.Queries {
	mock := &mockDBTX{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			sid, _ := args[0].(string)
			s, ok := sessions[sid]
			if !ok {
				return &mockRow{scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				}}
			}
			return &mockRow{scanFn: func(dest ...any) error {
				// GetSession scans: sid, user_id, data, expires_at
				*dest[0].(*string) = s.SID
				*dest[1].(*string) = s.UserID
				*dest[2].(*map[string]any) = s.Data
				*dest[3].(*time.Time) = s.ExpiresAt
				return nil
			}}
		},
	}
	return store.NewWithDBTX(mock)
}

// newMockAPIKeyStore returns a *store.Queries backed by a mockDBTX that
// serves API keys from the provided map, keyed by key hash.
func newMockAPIKeyStore(keys map[string]*domain.ApiKey) *store.Queries {
	mock := &mockDBTX{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			hash, _ := args[0].(string)
			k, ok := keys[hash]
			if !ok {
				return &mockRow{scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				}}
			}
			return &mockRow{scanFn: func(dest ...any) error {
				// GetAPIKeyByHash scans: id, org_id, name, key_hash, prefix, created_at, revoked_at
				*dest[0].(*string) = k.ID
				*dest[1].(*string) = k.OrgID
				*dest[2].(*string) = k.Name
				*dest[3].(*string) = k.KeyHash
				*dest[4].(*string) = k.Prefix
				*dest[5].(*time.Time) = k.CreatedAt
				*dest[6].(**time.Time) = k.RevokedAt
				return nil
			}}
		},
	}
	return store.NewWithDBTX(mock)
}

// ---------------------------------------------------------------------------
// Helper — ok handler
// ---------------------------------------------------------------------------

func okHandler(t *testing.T, checkFn func(r *http.Request)) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkFn != nil {
			checkFn(r)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// ---------------------------------------------------------------------------
// SessionAuth tests
// ---------------------------------------------------------------------------

func TestSessionAuth_ValidSession(t *testing.T) {
	sid := "valid-session-id"
	sessions := map[string]*domain.Session{
		sid: {
			SID:    sid,
			UserID: "user-1",
			Data: map[string]any{
				"org_id":     "org-1",
				"role":       "ADMIN",
				"csrf_token": "tok-abc",
			},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}

	var capturedAC *auth.AuthContext
	handler := okHandler(t, func(r *http.Request) {
		capturedAC = auth.GetAuthContext(r.Context())
	})

	middleware := auth.SessionAuth(newMockSessionStore(sessions))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedAC == nil {
		t.Fatal("AuthContext was not set in context")
	}
	if capturedAC.UserID != "user-1" {
		t.Errorf("UserID: want user-1, got %s", capturedAC.UserID)
	}
	if capturedAC.OrgID != "org-1" {
		t.Errorf("OrgID: want org-1, got %s", capturedAC.OrgID)
	}
	if capturedAC.Role != domain.UserRoleAdmin {
		t.Errorf("Role: want ADMIN, got %s", capturedAC.Role)
	}
}

func TestSessionAuth_MissingCookie(t *testing.T) {
	handler := okHandler(t, nil)
	middleware := auth.SessionAuth(newMockSessionStore(nil))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Errorf("body %q does not contain 'unauthorized'", rr.Body.String())
	}
}

func TestSessionAuth_SessionNotFound(t *testing.T) {
	handler := okHandler(t, nil)
	middleware := auth.SessionAuth(newMockSessionStore(nil)) // empty store

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "nonexistent-sid"})
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestSessionAuth_MissingOrgID(t *testing.T) {
	sid := "no-org-session"
	sessions := map[string]*domain.Session{
		sid: {
			SID:    sid,
			UserID: "user-2",
			Data: map[string]any{
				// org_id intentionally absent
				"role":       "ADMIN",
				"csrf_token": "tok-xyz",
			},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	handler := okHandler(t, nil)
	middleware := auth.SessionAuth(newMockSessionStore(sessions))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 when org_id missing, got %d", rr.Code)
	}
}

func TestSessionAuth_MissingRole(t *testing.T) {
	sid := "no-role-session"
	sessions := map[string]*domain.Session{
		sid: {
			SID:    sid,
			UserID: "user-3",
			Data: map[string]any{
				"org_id": "org-1",
				// role intentionally absent
				"csrf_token": "tok-xyz",
			},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	handler := okHandler(t, nil)
	middleware := auth.SessionAuth(newMockSessionStore(sessions))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 when role missing, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// APIKeyAuth tests
// ---------------------------------------------------------------------------

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	plainKey, prefix, hash := auth.GenerateAPIKey()
	keys := map[string]*domain.ApiKey{
		hash: {
			ID:        "key-id-1",
			OrgID:     "org-2",
			Name:      "test-key",
			KeyHash:   hash,
			Prefix:    prefix,
			CreatedAt: time.Now(),
		},
	}

	var capturedAC *auth.AuthContext
	handler := okHandler(t, func(r *http.Request) {
		capturedAC = auth.GetAuthContext(r.Context())
	})

	middleware := auth.APIKeyAuth(newMockAPIKeyStore(keys))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", plainKey)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedAC == nil {
		t.Fatal("AuthContext was not set in context")
	}
	if capturedAC.OrgID != "org-2" {
		t.Errorf("OrgID: want org-2, got %s", capturedAC.OrgID)
	}
	if capturedAC.Role != domain.UserRoleAdmin {
		t.Errorf("Role: want ADMIN, got %s", capturedAC.Role)
	}
	if capturedAC.UserID != "" {
		t.Errorf("UserID: want empty string for API key auth, got %s", capturedAC.UserID)
	}
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	handler := okHandler(t, nil)
	middleware := auth.APIKeyAuth(newMockAPIKeyStore(nil))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Errorf("body %q does not contain 'unauthorized'", rr.Body.String())
	}
}

func TestAPIKeyAuth_KeyNotFound(t *testing.T) {
	handler := okHandler(t, nil)
	middleware := auth.APIKeyAuth(newMockAPIKeyStore(nil)) // empty store

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// CSRFProtect tests
// ---------------------------------------------------------------------------

// buildCSRFRequest sets up a request with session data already in context
// by routing through SessionAuth with a valid session.
func buildCSRFSessionStore(csrfToken string) (*store.Queries, string) {
	sid := "csrf-session-id"
	sessions := map[string]*domain.Session{
		sid: {
			SID:    sid,
			UserID: "user-csrf",
			Data: map[string]any{
				"org_id":     "org-csrf",
				"role":       "ADMIN",
				"csrf_token": csrfToken,
			},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	return newMockSessionStore(sessions), sid
}

func TestCSRFProtect_GETPassesThrough(t *testing.T) {
	handler := okHandler(t, nil)
	wrapped := auth.CSRFProtect()(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET: want 200, got %d", rr.Code)
	}
}

func TestCSRFProtect_HEADPassesThrough(t *testing.T) {
	handler := okHandler(t, nil)
	wrapped := auth.CSRFProtect()(handler)

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HEAD: want 200, got %d", rr.Code)
	}
}

func TestCSRFProtect_OPTIONSPassesThrough(t *testing.T) {
	handler := okHandler(t, nil)
	wrapped := auth.CSRFProtect()(handler)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("OPTIONS: want 200, got %d", rr.Code)
	}
}

func TestCSRFProtect_POSTWithoutToken(t *testing.T) {
	st, sid := buildCSRFSessionStore("expected-token")
	handler := okHandler(t, nil)
	// Chain: SessionAuth -> CSRFProtect -> handler
	chain := auth.SessionAuth(st)(auth.CSRFProtect()(handler))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	// No X-CSRF-Token header
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST without token: want 403, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Errorf("body %q does not contain 'forbidden'", rr.Body.String())
	}
}

func TestCSRFProtect_POSTWithValidToken(t *testing.T) {
	st, sid := buildCSRFSessionStore("my-valid-csrf-token")
	handler := okHandler(t, nil)
	chain := auth.SessionAuth(st)(auth.CSRFProtect()(handler))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	req.Header.Set("X-CSRF-Token", "my-valid-csrf-token")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST with valid token: want 200, got %d", rr.Code)
	}
}

func TestCSRFProtect_POSTWithWrongToken(t *testing.T) {
	st, sid := buildCSRFSessionStore("correct-token")
	handler := okHandler(t, nil)
	chain := auth.SessionAuth(st)(auth.CSRFProtect()(handler))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST with wrong token: want 403, got %d", rr.Code)
	}
}

func TestCSRFProtect_PUTRequiresToken(t *testing.T) {
	st, sid := buildCSRFSessionStore("put-token")
	handler := okHandler(t, nil)
	chain := auth.SessionAuth(st)(auth.CSRFProtect()(handler))

	// Without token
	req := httptest.NewRequest(http.MethodPut, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("PUT without token: want 403, got %d", rr.Code)
	}

	// With correct token
	req2 := httptest.NewRequest(http.MethodPut, "/", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: sid})
	req2.Header.Set("X-CSRF-Token", "put-token")
	rr2 := httptest.NewRecorder()
	chain.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("PUT with token: want 200, got %d", rr2.Code)
	}
}
