package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/service"
)

// createAPIKeyRequest is the decoded body for POST /api/v1/api-keys.
type createAPIKeyRequest struct {
	Name string `json:"name"`
}

// handleListAPIKeys returns all API keys for the authenticated org.
//
// GET /api/v1/api-keys
func handleListAPIKeys(svc *service.APIKeyService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		keys, err := svc.List(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"api_keys": keys,
		})
	}
}

// handleCreateAPIKey creates a new API key and returns the plaintext key exactly once.
//
// POST /api/v1/api-keys
func handleCreateAPIKey(svc *service.APIKeyService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAPIKeyRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		plaintext, apiKey, err := svc.Create(r.Context(), orgID, req.Name)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, map[string]any{
			"key":     plaintext,
			"api_key": apiKey,
		})
	}
}

// handleRevokeAPIKey revokes an API key so it can no longer authenticate.
//
// DELETE /api/v1/api-keys/{id}
func handleRevokeAPIKey(svc *service.APIKeyService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		keyID := chi.URLParam(r, "id")

		if err := svc.Revoke(r.Context(), orgID, keyID); err != nil {
			mapError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
