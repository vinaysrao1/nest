package auth

import (
	"encoding/json"
	"net/http"

	"github.com/vinaysrao1/nest/internal/domain"
)

// RequireRole returns a middleware that allows only requests whose AuthContext
// carries one of the listed roles. An absent AuthContext yields 401;
// a mismatched role yields 403.
//
// Pre-conditions: at least one role must be supplied.
// Post-conditions: next is called only when the role check passes.
func RequireRole(roles ...domain.UserRole) func(http.Handler) http.Handler {
	allowed := make(map[domain.UserRole]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ac := GetAuthContext(r.Context())
			if ac == nil {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !allowed[ac.Role] {
				writeJSON(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeJSON writes a JSON error body. Kept private; shared with middleware.go
// via the same package.
func writeJSON(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
