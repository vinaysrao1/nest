// Package auth provides authentication, session management, password hashing,
// API key generation, RBAC middleware, and webhook signing for the Nest system.
package auth

import (
	"context"

	"github.com/vinaysrao1/nest/internal/domain"
)

type contextKey int

const (
	authContextKey contextKey = iota
	sessionDataKey
)

// AuthContext holds the authenticated identity for the current request.
type AuthContext struct {
	UserID string
	OrgID  string
	Role   domain.UserRole
}

// withAuthContext stores ac in ctx under authContextKey.
func withAuthContext(ctx context.Context, ac *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey, ac)
}

// SetAuthContext stores ac in ctx and returns the updated context.
// Intended for use in tests and middleware that need to inject an identity
// without going through session or API-key lookup.
func SetAuthContext(ctx context.Context, ac *AuthContext) context.Context {
	return withAuthContext(ctx, ac)
}

// GetAuthContext retrieves the AuthContext stored in ctx, or nil if absent.
func GetAuthContext(ctx context.Context) *AuthContext {
	ac, _ := ctx.Value(authContextKey).(*AuthContext)
	return ac
}

// UserIDFromContext returns the user ID from ctx, or the empty string if none.
func UserIDFromContext(ctx context.Context) string {
	if ac := GetAuthContext(ctx); ac != nil {
		return ac.UserID
	}
	return ""
}

// OrgIDFromContext returns the org ID from ctx, or the empty string if none.
func OrgIDFromContext(ctx context.Context) string {
	if ac := GetAuthContext(ctx); ac != nil {
		return ac.OrgID
	}
	return ""
}

// RoleFromContext returns the user role from ctx, or the empty string if none.
func RoleFromContext(ctx context.Context) domain.UserRole {
	if ac := GetAuthContext(ctx); ac != nil {
		return ac.Role
	}
	return ""
}

// withSessionData stores the session data map in ctx under sessionDataKey.
// Called by SessionAuth so CSRFProtect can read the csrf_token.
func withSessionData(ctx context.Context, data map[string]any) context.Context {
	return context.WithValue(ctx, sessionDataKey, data)
}

// sessionDataFromContext retrieves session data from ctx, or nil if absent.
func sessionDataFromContext(ctx context.Context) map[string]any {
	data, _ := ctx.Value(sessionDataKey).(map[string]any)
	return data
}
