package auth_test

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
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
// Shared mock infrastructure (also used by middleware_test.go in same package)
// ---------------------------------------------------------------------------

// sharedMockDBTX satisfies store.DBTX without a real database connection.
// Defined here so auth_test.go and middleware_test.go can both use it.
// (middleware_test.go defines its own equivalent — see note below.)
// This file only uses it for the RequireRole helper.

// buildSessionStore creates a mock store with a single session for role injection.
func buildSessionStore(userID, orgID string, role domain.UserRole) (*store.Queries, string) {
	sid := "test-session-" + string(role)
	sessions := map[string]*domain.Session{
		sid: {
			SID:    sid,
			UserID: userID,
			Data: map[string]any{
				"org_id":     orgID,
				"role":       string(role),
				"csrf_token": "tok",
			},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}

	mock := &authTestMockDBTX{
		queryRowFn: func(sql string, args ...any) pgx.Row {
			id, _ := args[0].(string)
			s, ok := sessions[id]
			if !ok {
				return &authTestMockRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
			}
			return &authTestMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = s.SID
				*dest[1].(*string) = s.UserID
				*dest[2].(*map[string]any) = s.Data
				*dest[3].(*time.Time) = s.ExpiresAt
				return nil
			}}
		},
	}
	return store.NewWithDBTX(mock), sid
}

// authTestMockDBTX is a minimal DBTX implementation used only in auth_test.go.
// middleware_test.go has its own (mockDBTX) to avoid re-declaration.
type authTestMockDBTX struct {
	queryRowFn func(sql string, args ...any) pgx.Row
}

func (m *authTestMockDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *authTestMockDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *authTestMockDBTX) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFn(sql, args...)
}

func (m *authTestMockDBTX) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

// authTestMockRow is a pgx.Row implementation for use in auth_test.go.
type authTestMockRow struct {
	scanFn func(dest ...any) error
}

func (r *authTestMockRow) Scan(dest ...any) error { return r.scanFn(dest...) }

// ---------------------------------------------------------------------------
// HashPassword / CheckPassword
// ---------------------------------------------------------------------------

func TestHashPassword_RoundTrip(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword: unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword: returned empty hash")
	}
	if !auth.CheckPassword(hash, "correct-horse-battery-staple") {
		t.Error("CheckPassword: expected true for correct password, got false")
	}
}

func TestHashPassword_DifferentHashesForSameInput(t *testing.T) {
	h1, _ := auth.HashPassword("same-password")
	h2, _ := auth.HashPassword("same-password")
	if h1 == h2 {
		t.Error("HashPassword: expected different hashes due to random salt, got identical")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, _ := auth.HashPassword("right")
	if auth.CheckPassword(hash, "wrong") {
		t.Error("CheckPassword: expected false for wrong password, got true")
	}
}

// ---------------------------------------------------------------------------
// GenerateSessionID
// ---------------------------------------------------------------------------

func TestGenerateSessionID_Uniqueness(t *testing.T) {
	ids := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := auth.GenerateSessionID()
		if _, exists := ids[id]; exists {
			t.Fatalf("GenerateSessionID: collision at iteration %d", i)
		}
		ids[id] = struct{}{}
	}
}

func TestGenerateSessionID_FormatAndLength(t *testing.T) {
	id := auth.GenerateSessionID()
	if len(id) != 64 {
		t.Errorf("GenerateSessionID: want length 64, got %d", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("GenerateSessionID: not valid hex: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HashAPIKey
// ---------------------------------------------------------------------------

func TestHashAPIKey_Deterministic(t *testing.T) {
	h1 := auth.HashAPIKey("my-api-key")
	h2 := auth.HashAPIKey("my-api-key")
	if h1 != h2 {
		t.Error("HashAPIKey: same input produced different outputs")
	}
}

func TestHashAPIKey_DifferentInputs(t *testing.T) {
	if auth.HashAPIKey("key-a") == auth.HashAPIKey("key-b") {
		t.Error("HashAPIKey: different inputs produced the same hash")
	}
}

func TestHashAPIKey_IsHex(t *testing.T) {
	h := auth.HashAPIKey("any-key")
	if len(h) != 64 {
		t.Errorf("HashAPIKey: want 64 hex chars, got %d", len(h))
	}
	if _, err := hex.DecodeString(h); err != nil {
		t.Errorf("HashAPIKey: not valid hex: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GenerateAPIKey
// ---------------------------------------------------------------------------

func TestGenerateAPIKey_Consistency(t *testing.T) {
	key, prefix, hash := auth.GenerateAPIKey()
	if len(key) != 64 {
		t.Errorf("GenerateAPIKey: key want 64 chars, got %d", len(key))
	}
	if prefix != key[:8] {
		t.Errorf("GenerateAPIKey: prefix %q != key[:8] %q", prefix, key[:8])
	}
	if hash != auth.HashAPIKey(key) {
		t.Error("GenerateAPIKey: hash does not match HashAPIKey(key)")
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	k1, _, _ := auth.GenerateAPIKey()
	k2, _, _ := auth.GenerateAPIKey()
	if k1 == k2 {
		t.Error("GenerateAPIKey: two calls produced identical keys")
	}
}

// ---------------------------------------------------------------------------
// GenerateToken
// ---------------------------------------------------------------------------

func TestGenerateToken_RoundTrip(t *testing.T) {
	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: unexpected error: %v", err)
	}
	if plaintext == "" {
		t.Error("GenerateToken: empty plaintext")
	}
	if hash == "" {
		t.Error("GenerateToken: empty hash")
	}
	if auth.HashAPIKey(plaintext) != hash {
		t.Error("GenerateToken: hash does not match HashAPIKey(plaintext)")
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	p1, _, _ := auth.GenerateToken()
	p2, _, _ := auth.GenerateToken()
	if p1 == p2 {
		t.Error("GenerateToken: two calls produced the same plaintext")
	}
}

// ---------------------------------------------------------------------------
// GenerateRSAKeyPair
// ---------------------------------------------------------------------------

func TestGenerateRSAKeyPair(t *testing.T) {
	pubPEM, privPEM, err := auth.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: unexpected error: %v", err)
	}
	if pubPEM == "" {
		t.Fatal("GenerateRSAKeyPair: empty publicPEM")
	}
	if privPEM == "" {
		t.Fatal("GenerateRSAKeyPair: empty privatePEM")
	}

	// Verify public key can be parsed back.
	pubBlock, _ := pem.Decode([]byte(pubPEM))
	if pubBlock == nil {
		t.Fatal("GenerateRSAKeyPair: cannot decode publicPEM block")
	}
	parsedPub, err := x509.ParsePKCS1PublicKey(pubBlock.Bytes)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: cannot parse public key: %v", err)
	}
	if parsedPub == nil {
		t.Fatal("GenerateRSAKeyPair: parsed public key is nil")
	}

	// Verify private key can be parsed back.
	privBlock, _ := pem.Decode([]byte(privPEM))
	if privBlock == nil {
		t.Fatal("GenerateRSAKeyPair: cannot decode privatePEM block")
	}
	parsedPriv, err := x509.ParsePKCS1PrivateKey(privBlock.Bytes)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: cannot parse private key: %v", err)
	}
	if parsedPriv == nil {
		t.Fatal("GenerateRSAKeyPair: parsed private key is nil")
	}

	// Verify the parsed public key matches the public key of the parsed private key.
	if parsedPub.N.Cmp(parsedPriv.PublicKey.N) != 0 {
		t.Error("GenerateRSAKeyPair: public key does not match private key's public key")
	}
}

// ---------------------------------------------------------------------------
// AuthContext accessors with empty context
// ---------------------------------------------------------------------------

func TestAuthContext_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := req.Context()

	if auth.GetAuthContext(ctx) != nil {
		t.Error("GetAuthContext: expected nil on empty context")
	}
	if auth.UserIDFromContext(ctx) != "" {
		t.Error("UserIDFromContext: expected empty string on empty context")
	}
	if auth.OrgIDFromContext(ctx) != "" {
		t.Error("OrgIDFromContext: expected empty string on empty context")
	}
	if auth.RoleFromContext(ctx) != "" {
		t.Error("RoleFromContext: expected empty string on empty context")
	}
}

// ---------------------------------------------------------------------------
// RequireRole middleware — tested by composing with SessionAuth + mock store
// ---------------------------------------------------------------------------

// requireRoleChain builds: SessionAuth(mockStore) -> RequireRole(roles...) -> okHandler
// and fires the request with the given session cookie.
func requireRoleChain(
	t *testing.T,
	sessionStore *store.Queries,
	sid string,
	roles []domain.UserRole,
) *httptest.ResponseRecorder {
	t.Helper()

	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chain := auth.SessionAuth(sessionStore)(auth.RequireRole(roles...)(final))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if sid != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: sid})
	}
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)
	return rr
}

func TestRequireRole_NoAuthContext(t *testing.T) {
	// No cookie -> SessionAuth returns 401 before RequireRole even runs.
	st, _ := buildSessionStore("u1", "o1", domain.UserRoleAdmin)
	rr := requireRoleChain(t, st, "" /* no cookie */, []domain.UserRole{domain.UserRoleAdmin})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestRequireRole_MatchingRolePasses(t *testing.T) {
	st, sid := buildSessionStore("u1", "o1", domain.UserRoleAdmin)
	rr := requireRoleChain(t, st, sid, []domain.UserRole{domain.UserRoleAdmin})
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestRequireRole_NonMatchingRoleForbidden(t *testing.T) {
	st, sid := buildSessionStore("u1", "o1", domain.UserRoleModerator)
	rr := requireRoleChain(t, st, sid, []domain.UserRole{domain.UserRoleAdmin})
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestRequireRole_MultipleAllowedRoles(t *testing.T) {
	st, sid := buildSessionStore("u1", "o1", domain.UserRoleModerator)
	rr := requireRoleChain(t, st, sid, []domain.UserRole{domain.UserRoleAdmin, domain.UserRoleModerator})
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestRequireRole_AnalystNotInAdminList(t *testing.T) {
	st, sid := buildSessionStore("u1", "o1", domain.UserRoleAnalyst)
	rr := requireRoleChain(t, st, sid, []domain.UserRole{domain.UserRoleAdmin})
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestRequireRole_ErrorBodyContainsMessage(t *testing.T) {
	// Direct call with no context — should return 401 with JSON body.
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	auth.RequireRole(domain.UserRoleAdmin)(final).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Errorf("body %q does not contain 'unauthorized'", rr.Body.String())
	}
}
