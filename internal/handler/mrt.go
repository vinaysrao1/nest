package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/service"
)

// recordDecisionRequest is the decoded body for POST /api/v1/mrt/decisions.
type recordDecisionRequest struct {
	JobID         string   `json:"job_id"`
	Verdict       string   `json:"verdict"`
	Reason        string   `json:"reason"`
	ActionIDs     []string `json:"action_ids"`
	PolicyIDs     []string `json:"policy_ids"`
	TargetQueueID *string  `json:"target_queue_id,omitempty"`
}

// handleListQueues returns all MRT queues for the authenticated org.
//
// GET /api/v1/mrt/queues
func handleListQueues(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)

		queues, err := svc.ListQueues(r.Context(), orgID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"queues": queues,
		})
	}
}

// handleListJobs returns a paginated list of jobs in a queue, optionally filtered by status.
//
// GET /api/v1/mrt/queues/{id}/jobs
func handleListJobs(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		queueID := chi.URLParam(r, "id")
		page := PageParamsFromRequest(r)

		var status *string
		if s := r.URL.Query().Get("status"); s != "" {
			status = &s
		}

		result, err := svc.ListJobs(r.Context(), orgID, queueID, status, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleAssignJob atomically claims the next pending job in the queue for the caller.
// Returns 204 when no pending jobs are available.
//
// POST /api/v1/mrt/queues/{id}/assign
func handleAssignJob(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		userID := UserID(r)
		queueID := chi.URLParam(r, "id")

		job, err := svc.AssignNext(r.Context(), orgID, queueID, userID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		if job == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		JSON(w, http.StatusOK, job)
	}
}

// claimJobRequest is the decoded body for POST /api/v1/mrt/jobs/claim.
type claimJobRequest struct {
	JobID string `json:"job_id"`
}

// handleClaimJob atomically claims a specific job for the caller.
// Returns the claimed job (200), or 409 if already claimed by another user.
//
// POST /api/v1/mrt/jobs/claim
func handleClaimJob(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req claimJobRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.JobID == "" {
			Error(w, http.StatusBadRequest, "job_id is required")
			return
		}

		orgID := OrgID(r)
		userID := UserID(r)

		job, err := svc.ClaimJob(r.Context(), orgID, req.JobID, userID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, job)
	}
}

// handleRecordDecision records a moderator decision and publishes resulting actions.
//
// CRITICAL: MRTService.RecordDecision returns ActionRequests. The handler then
// calls pipeline.Execute — MRTService never calls ActionPublisher directly
// (invariant 8 of the design document).
//
// POST /api/v1/mrt/decisions
func handleRecordDecision(svc *service.MRTService, pipeline *service.PostVerdictPipeline, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recordDecisionRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		userID := UserID(r)

		result, err := svc.RecordDecision(r.Context(), service.DecisionParams{
			OrgID:         orgID,
			JobID:         req.JobID,
			UserID:        userID,
			Verdict:       req.Verdict,
			ActionIDs:     req.ActionIDs,
			PolicyIDs:     req.PolicyIDs,
			Reason:        req.Reason,
			TargetQueueID: req.TargetQueueID,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}

		// Handler orchestrates action publishing (invariant 8): MRTService
		// returns ActionRequests and the handler executes them via PostVerdictPipeline.
		// Only APPROVE and BLOCK verdicts require webhook delivery.
		if result.WebhookRequired && len(result.ActionRequests) > 0 {
			pipeline.Execute(r.Context(), service.PostVerdictParams{
				ActionRequests: result.ActionRequests,
				OrgID:          orgID,
				ItemID:         result.ItemID,
				ItemTypeID:     result.ItemTypeID,
				Payload:        result.Payload,
				CorrelationID:  result.Decision.ID,
			})
		}

		JSON(w, http.StatusOK, result.Decision)
	}
}

// handleGetJob returns a single MRT job by ID.
//
// GET /api/v1/mrt/jobs/* (wildcard to support IDs containing forward slashes)
func handleGetJob(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		jobID := chi.URLParam(r, "*")

		job, err := svc.GetJob(r.Context(), orgID, jobID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, job)
	}
}

// createQueueRequest is the decoded body for POST /api/v1/mrt/queues.
type createQueueRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// handleCreateQueue creates a new MRT queue.
// Requires ADMIN role (enforced at router level via RequireRole middleware).
//
// POST /api/v1/mrt/queues
// Request: {"name": "...", "description": "...", "is_default": false}
// Response: 201 Created with MRTQueue JSON body
// Errors: 400 (validation), 409 (duplicate name)
func handleCreateQueue(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createQueueRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)

		queue, err := svc.CreateQueue(r.Context(), orgID, service.CreateQueueParams{
			Name:        req.Name,
			Description: req.Description,
			IsDefault:   req.IsDefault,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, queue)
	}
}

// handleArchiveQueue soft-deletes an MRT queue.
// Requires ADMIN role (enforced at router level via RequireRole middleware).
//
// DELETE /api/v1/mrt/queues/{id}
// Response: 204 No Content
// Errors: 404 (not found or already archived)
func handleArchiveQueue(svc *service.MRTService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		queueID := chi.URLParam(r, "id")

		if err := svc.ArchiveQueue(r.Context(), orgID, queueID); err != nil {
			mapError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
