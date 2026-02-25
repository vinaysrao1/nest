package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// createRuleRequest is the decoded body for POST /api/v1/rules.
type createRuleRequest struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Source    string   `json:"source"`
	Tags      []string `json:"tags"`
	PolicyIDs []string `json:"policy_ids"`
}

// updateRuleRequest is the decoded body for PUT /api/v1/rules/{id}.
type updateRuleRequest struct {
	Name      *string   `json:"name"`
	Status    *string   `json:"status"`
	Source    *string   `json:"source"`
	Tags      *[]string `json:"tags"`
	PolicyIDs *[]string `json:"policy_ids"`
}

// testRuleRequest is the decoded body for POST /api/v1/rules/test.
type testRuleRequest struct {
	Source string      `json:"source"`
	Event  eventFields `json:"event"`
}

// testExistingRuleRequest is the decoded body for POST /api/v1/rules/{id}/test.
type testExistingRuleRequest struct {
	Event eventFields `json:"event"`
}

// eventFields maps the JSON event shape to domain.Event fields.
type eventFields struct {
	EventType string         `json:"event_type"`
	ItemType  string         `json:"item_type"`
	Payload   map[string]any `json:"payload"`
}

// handleListRules returns a paginated list of rules for the authenticated org.
//
// GET /api/v1/rules
func handleListRules(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		page := PageParamsFromRequest(r)

		result, err := svc.ListRules(r.Context(), orgID, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleCreateRule compiles and persists a new rule for the authenticated org.
//
// POST /api/v1/rules
func handleCreateRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRuleRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		result, err := svc.CreateRule(r.Context(), orgID, service.CreateRuleParams{
			Name:      req.Name,
			Status:    domain.RuleStatus(req.Status),
			Source:    req.Source,
			Tags:      req.Tags,
			PolicyIDs: req.PolicyIDs,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusCreated, result)
	}
}

// handleGetRule returns a single rule by ID.
//
// GET /api/v1/rules/{id}
func handleGetRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		ruleID := chi.URLParam(r, "id")

		result, err := svc.GetRule(r.Context(), orgID, ruleID)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleUpdateRule applies a partial update to an existing rule.
//
// PUT /api/v1/rules/{id}
func handleUpdateRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateRuleRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		ruleID := chi.URLParam(r, "id")

		params := service.UpdateRuleParams{
			Name:      req.Name,
			Source:    req.Source,
			Tags:      req.Tags,
			PolicyIDs: req.PolicyIDs,
		}
		if req.Status != nil {
			s := domain.RuleStatus(*req.Status)
			params.Status = &s
		}

		result, err := svc.UpdateRule(r.Context(), orgID, ruleID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleDeleteRule removes a rule by ID.
//
// DELETE /api/v1/rules/{id}
func handleDeleteRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		ruleID := chi.URLParam(r, "id")

		if err := svc.DeleteRule(r.Context(), orgID, ruleID); err != nil {
			mapError(w, err, logger)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleTestRule compiles a rule source and evaluates it against the provided event.
// Nothing is persisted.
//
// POST /api/v1/rules/test
func handleTestRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req testRuleRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		event := domain.Event{
			EventType: req.Event.EventType,
			ItemType:  req.Event.ItemType,
			OrgID:     orgID,
			Payload:   req.Event.Payload,
		}

		result, err := svc.TestRule(r.Context(), orgID, req.Source, event)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}

// handleTestExistingRule evaluates an already-stored rule against the provided event.
//
// POST /api/v1/rules/{id}/test
func handleTestExistingRule(svc *service.RuleService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req testExistingRuleRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		ruleID := chi.URLParam(r, "id")
		event := domain.Event{
			EventType: req.Event.EventType,
			ItemType:  req.Event.ItemType,
			OrgID:     orgID,
			Payload:   req.Event.Payload,
		}

		result, err := svc.TestExistingRule(r.Context(), orgID, ruleID, event)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, result)
	}
}
