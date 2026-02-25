package auth

import (
	"net/http"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// SessionAuth returns a middleware that authenticates requests via a "session"
// cookie. It looks up the session in the store, extracts org_id and role from
// the session Data map, and populates AuthContext. The raw session data is also
// stored in context so CSRFProtect can read the csrf_token.
//
// Returns 401 if the cookie is missing, the session is not found/expired, or
// the session data lacks org_id or role.
func SessionAuth(st *store.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			session, err := st.GetSession(r.Context(), cookie.Value)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			orgID, _ := session.Data["org_id"].(string)
			roleStr, _ := session.Data["role"].(string)
			if orgID == "" || roleStr == "" {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			ctx := withAuthContext(r.Context(), &AuthContext{
				UserID: session.UserID,
				OrgID:  orgID,
				Role:   domain.UserRole(roleStr),
			})
			ctx = withSessionData(ctx, session.Data)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth returns a middleware that authenticates requests via the
// "X-API-Key" header. It hashes the provided key and looks it up in the store.
// API key requests are granted the ADMIN role within the key's org.
//
// Returns 401 if the header is missing or the key is not found/revoked.
func APIKeyAuth(st *store.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			hash := HashAPIKey(key)
			apiKey, err := st.GetAPIKeyByHash(r.Context(), hash)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			ctx := withAuthContext(r.Context(), &AuthContext{
				UserID: "",
				OrgID:  apiKey.OrgID,
				Role:   domain.UserRoleAdmin,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CSRFProtect returns a middleware that enforces CSRF token validation for
// state-mutating methods (POST, PUT, PATCH, DELETE). Safe methods (GET, HEAD,
// OPTIONS) pass through unconditionally.
//
// The CSRF token is read from the "X-CSRF-Token" header and compared with the
// csrf_token value stored in the session data (placed by SessionAuth). Returns
// 403 on a mismatch or absent token.
//
// CSRFProtect must be placed after SessionAuth in the middleware chain.
func CSRFProtect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			token := r.Header.Get("X-CSRF-Token")
			data := sessionDataFromContext(r.Context())
			csrfToken, _ := data["csrf_token"].(string)

			if token == "" || token != csrfToken {
				writeJSON(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
