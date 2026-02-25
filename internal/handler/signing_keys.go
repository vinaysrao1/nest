package handler

import (
	"log/slog"
	"net/http"

	"github.com/vinaysrao1/nest/internal/service"
)

// handleListSigningKeys returns all signing keys for the authenticated org.
//
// GET /api/v1/signing-keys
func handleListSigningKeys(svc *service.SigningKeyService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		keys, err := svc.List(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"signing_keys": keys,
		})
	}
}

// handleRotateSigningKey generates a new RSA key pair, deactivates all existing
// keys, and activates the new key.
//
// POST /api/v1/signing-keys/rotate
func handleRotateSigningKey(svc *service.SigningKeyService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		key, err := svc.Rotate(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, key)
	}
}
