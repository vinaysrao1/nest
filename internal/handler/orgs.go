package handler

import (
	"log/slog"
	"net/http"

	"github.com/vinaysrao1/nest/internal/service"
)

// handleGetOrgSettings returns the settings map for the authenticated org.
//
// GET /api/v1/orgs/settings
func handleGetOrgSettings(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		settings, err := svc.GetOrgSettings(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, settings)
	}
}
