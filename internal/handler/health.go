package handler

import "net/http"

// handleHealth returns a simple health-check response.
//
// GET /api/v1/health
// Response 200: {"status":"ok"}
func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
