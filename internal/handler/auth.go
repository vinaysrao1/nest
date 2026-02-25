package handler

import (
	"log/slog"
	"net/http"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/service"
)

// loginRequest is the decoded body for POST /api/v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleLogin authenticates a user by email and password, creates a session,
// and returns the user object plus a CSRF token in the response body.
// A session cookie is set HttpOnly, SameSite=Strict.
//
// POST /api/v1/auth/login (public)
func handleLogin(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		user, session, csrfToken, err := svc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    session.SID,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Expires:  session.ExpiresAt,
		})

		JSON(w, http.StatusOK, map[string]any{
			"user":       user,
			"csrf_token": csrfToken,
		})
	}
}

// handleLogout deletes the session identified by the session cookie and clears it.
//
// POST /api/v1/auth/logout (session auth)
func handleLogout(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err == nil {
			if logoutErr := svc.Logout(r.Context(), cookie.Value); logoutErr != nil {
				logger.Error("logout: delete session failed", "error", logoutErr)
			}
		}

		// Clear the session cookie regardless.
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleMe returns the identity of the authenticated caller from the auth context.
//
// GET /api/v1/auth/me (session auth)
func handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ac := auth.GetAuthContext(r.Context())
		if ac == nil {
			Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"user_id": ac.UserID,
			"org_id":  ac.OrgID,
			"role":    string(ac.Role),
		})
	}
}

// resetPasswordRequest is the decoded body for POST /api/v1/auth/reset-password.
type resetPasswordRequest struct {
	Email string `json:"email"`
}

// handleRequestPasswordReset initiates the password reset flow for the given email.
// Always returns 204 to avoid leaking whether the email is registered.
//
// POST /api/v1/auth/reset-password (session auth)
func handleRequestPasswordReset(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resetPasswordRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
			logger.Error("request password reset failed", "error", err)
		}

		// Always 204 — no information about email existence should leak.
		w.WriteHeader(http.StatusNoContent)
	}
}
