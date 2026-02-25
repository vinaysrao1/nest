package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// inviteUserRequest is the decoded body for POST /api/v1/users/invite.
type inviteUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// updateUserRequest is the decoded body for PUT /api/v1/users/{id}.
type updateUserRequest struct {
	Name     *string `json:"name"`
	Role     *string `json:"role"`
	IsActive *bool   `json:"is_active"`
}

// handleListUsers returns a paginated list of users for the authenticated org.
//
// GET /api/v1/users
func handleListUsers(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		page := PageParamsFromRequest(r)

		result, err := svc.ListUsers(r.Context(), orgID, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleInviteUser creates a new user in the org.
//
// POST /api/v1/users/invite
func handleInviteUser(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req inviteUserRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		user, err := svc.InviteUser(r.Context(), orgID, req.Email, req.Name, domain.UserRole(req.Role))
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, user)
	}
}

// handleUpdateUser applies a partial update to an existing user.
//
// PUT /api/v1/users/{id}
func handleUpdateUser(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateUserRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		userID := chi.URLParam(r, "id")

		params := service.UserUpdateParams{
			Name:     req.Name,
			IsActive: req.IsActive,
		}
		if req.Role != nil {
			role := domain.UserRole(*req.Role)
			params.Role = &role
		}

		user, err := svc.UpdateUser(r.Context(), orgID, userID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, user)
	}
}

// handleDeleteUser deactivates a user.
//
// DELETE /api/v1/users/{id}
func handleDeleteUser(svc *service.UserService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		userID := chi.URLParam(r, "id")

		if err := svc.DeactivateUser(r.Context(), orgID, userID); err != nil {
			mapError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
