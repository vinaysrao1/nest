package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

// ---- Action handlers --------------------------------------------------------

// createActionRequest is the decoded body for POST /api/v1/actions.
type createActionRequest struct {
	Name        string         `json:"name"`
	ActionType  string         `json:"action_type"`
	Config      map[string]any `json:"config"`
	ItemTypeIDs []string       `json:"item_type_ids"`
}

// updateActionRequest is the decoded body for PUT /api/v1/actions/{id}.
type updateActionRequest struct {
	Name        *string         `json:"name"`
	ActionType  *string         `json:"action_type"`
	Config      *map[string]any `json:"config"`
	ItemTypeIDs *[]string       `json:"item_type_ids"`
}

// handleListActions returns a paginated list of actions.
//
// GET /api/v1/actions
func handleListActions(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		page := PageParamsFromRequest(r)

		result, err := svc.ListActions(r.Context(), orgID, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleCreateAction creates a new action.
//
// POST /api/v1/actions
func handleCreateAction(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createActionRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		result, err := svc.CreateAction(r.Context(), orgID, service.CreateActionParams{
			Name:        req.Name,
			ActionType:  domain.ActionType(req.ActionType),
			Config:      req.Config,
			ItemTypeIDs: req.ItemTypeIDs,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusCreated, result)
	}
}

// handleGetAction returns a single action by ID.
//
// GET /api/v1/actions/{id}
func handleGetAction(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		actionID := chi.URLParam(r, "id")

		result, err := svc.GetAction(r.Context(), orgID, actionID)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleUpdateAction applies a partial update to an existing action.
//
// PUT /api/v1/actions/{id}
func handleUpdateAction(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateActionRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		actionID := chi.URLParam(r, "id")

		params := service.UpdateActionParams{
			Name:        req.Name,
			Config:      req.Config,
			ItemTypeIDs: req.ItemTypeIDs,
		}
		if req.ActionType != nil {
			at := domain.ActionType(*req.ActionType)
			params.ActionType = &at
		}

		result, err := svc.UpdateAction(r.Context(), orgID, actionID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleDeleteAction removes an action by ID.
//
// DELETE /api/v1/actions/{id}
func handleDeleteAction(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		actionID := chi.URLParam(r, "id")

		if err := svc.DeleteAction(r.Context(), orgID, actionID); err != nil {
			mapError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- Policy handlers --------------------------------------------------------

// createPolicyRequest is the decoded body for POST /api/v1/policies.
type createPolicyRequest struct {
	Name          string  `json:"name"`
	Description   *string `json:"description"`
	ParentID      *string `json:"parent_id"`
	StrikePenalty int     `json:"strike_penalty"`
}

// updatePolicyRequest is the decoded body for PUT /api/v1/policies/{id}.
type updatePolicyRequest struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	ParentID      *string `json:"parent_id"`
	StrikePenalty *int    `json:"strike_penalty"`
}

// handleListPolicies returns a paginated list of policies.
//
// GET /api/v1/policies
func handleListPolicies(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		page := PageParamsFromRequest(r)

		result, err := svc.ListPolicies(r.Context(), orgID, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleCreatePolicy creates a new policy.
//
// POST /api/v1/policies
func handleCreatePolicy(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPolicyRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		result, err := svc.CreatePolicy(r.Context(), orgID, service.CreatePolicyParams{
			Name:          req.Name,
			Description:   req.Description,
			ParentID:      req.ParentID,
			StrikePenalty: req.StrikePenalty,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusCreated, result)
	}
}

// handleGetPolicy returns a single policy by ID.
//
// GET /api/v1/policies/{id}
func handleGetPolicy(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		policyID := chi.URLParam(r, "id")

		result, err := svc.GetPolicy(r.Context(), orgID, policyID)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleUpdatePolicy applies a partial update to an existing policy.
//
// PUT /api/v1/policies/{id}
func handleUpdatePolicy(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updatePolicyRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		policyID := chi.URLParam(r, "id")

		result, err := svc.UpdatePolicy(r.Context(), orgID, policyID, service.UpdatePolicyParams{
			Name:          req.Name,
			Description:   req.Description,
			ParentID:      req.ParentID,
			StrikePenalty: req.StrikePenalty,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleDeletePolicy removes a policy by ID.
//
// DELETE /api/v1/policies/{id}
func handleDeletePolicy(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		policyID := chi.URLParam(r, "id")

		if err := svc.DeletePolicy(r.Context(), orgID, policyID); err != nil {
			mapError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- Item Type handlers -----------------------------------------------------

// createItemTypeRequest is the decoded body for POST /api/v1/item-types.
type createItemTypeRequest struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Schema     map[string]any `json:"schema"`
	FieldRoles map[string]any `json:"field_roles"`
}

// updateItemTypeRequest is the decoded body for PUT /api/v1/item-types/{id}.
type updateItemTypeRequest struct {
	Name       *string         `json:"name"`
	Kind       *string         `json:"kind"`
	Schema     *map[string]any `json:"schema"`
	FieldRoles *map[string]any `json:"field_roles"`
}

// handleListItemTypes returns a paginated list of item types.
//
// GET /api/v1/item-types
func handleListItemTypes(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		page := PageParamsFromRequest(r)

		result, err := svc.ListItemTypes(r.Context(), orgID, page)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleCreateItemType creates a new item type.
//
// POST /api/v1/item-types
func handleCreateItemType(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createItemTypeRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		result, err := svc.CreateItemType(r.Context(), orgID, service.CreateItemTypeParams{
			Name:       req.Name,
			Kind:       domain.ItemTypeKind(req.Kind),
			Schema:     req.Schema,
			FieldRoles: req.FieldRoles,
		})
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusCreated, result)
	}
}

// handleGetItemType returns a single item type by ID.
//
// GET /api/v1/item-types/{id}
func handleGetItemType(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		itemTypeID := chi.URLParam(r, "id")

		result, err := svc.GetItemType(r.Context(), orgID, itemTypeID)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleUpdateItemType applies a partial update to an existing item type.
//
// PUT /api/v1/item-types/{id}
func handleUpdateItemType(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateItemTypeRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		itemTypeID := chi.URLParam(r, "id")

		params := service.UpdateItemTypeParams{
			Name:       req.Name,
			Schema:     req.Schema,
			FieldRoles: req.FieldRoles,
		}
		if req.Kind != nil {
			k := domain.ItemTypeKind(*req.Kind)
			params.Kind = &k
		}

		result, err := svc.UpdateItemType(r.Context(), orgID, itemTypeID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}
		JSON(w, http.StatusOK, result)
	}
}

// handleDeleteItemType removes an item type by ID.
//
// DELETE /api/v1/item-types/{id}
func handleDeleteItemType(svc *service.ConfigService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := OrgID(r)
		itemTypeID := chi.URLParam(r, "id")

		if err := svc.DeleteItemType(r.Context(), orgID, itemTypeID); err != nil {
			mapError(w, err, logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
