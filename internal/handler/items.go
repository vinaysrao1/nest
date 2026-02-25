package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/vinaysrao1/nest/internal/service"
)

// submitItemJSON is the JSON shape for a single item in submit requests.
type submitItemJSON struct {
	ItemID        string         `json:"item_id"`
	ItemTypeID    string         `json:"item_type_id"`
	Payload       map[string]any `json:"payload"`
	CreatorID     string         `json:"creator_id"`
	CreatorTypeID string         `json:"creator_type_id"`
}

// submitRequest is the decoded body for POST /api/v1/items and POST /api/v1/items/async.
type submitRequest struct {
	Items []submitItemJSON `json:"items"`
}

// EnqueueFunc is a callback that enqueues async item processing jobs.
// Defined in handler, implemented in cmd/server using the river client.
type EnqueueFunc func(ctx context.Context, args []ProcessItemJobArgs) error

// ProcessItemJobArgs mirrors worker.ProcessItemArgs for the handler layer.
// It is serialised into river job arguments by the EnqueueFunc implementation.
type ProcessItemJobArgs struct {
	OrgID      string         `json:"org_id"`
	ItemID     string         `json:"item_id"`
	ItemTypeID string         `json:"item_type_id"`
	EventType  string         `json:"event_type"`
	Payload    map[string]any `json:"payload"`
}

// handleSubmitSync evaluates items synchronously and returns results.
//
// POST /api/v1/items (API key auth)
func handleSubmitSync(svc *service.ItemService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req submitRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		params := toSubmitParams(orgID, req.Items)

		results, err := svc.SubmitSync(r.Context(), orgID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"results": results,
		})
	}
}

// handleSubmitAsync persists items and enqueues river jobs for background evaluation.
//
// POST /api/v1/items/async (API key auth)
func handleSubmitAsync(svc *service.ItemService, enqueue EnqueueFunc, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req submitRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		orgID := OrgID(r)
		params := toSubmitParams(orgID, req.Items)

		submissionIDs, err := svc.SubmitAsync(r.Context(), orgID, params)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		if enqueue != nil && len(submissionIDs) > 0 {
			jobs := buildJobArgs(orgID, req.Items)
			if enqueueErr := enqueue(r.Context(), jobs); enqueueErr != nil {
				logger.Error("failed to enqueue async items", "error", enqueueErr)
				Error(w, http.StatusInternalServerError, "failed to enqueue items")
				return
			}
		}

		JSON(w, http.StatusAccepted, map[string]any{
			"submission_ids": submissionIDs,
		})
	}
}

// toSubmitParams converts the JSON request items slice to service.SubmitItemParams.
func toSubmitParams(orgID string, items []submitItemJSON) []service.SubmitItemParams {
	params := make([]service.SubmitItemParams, len(items))
	for i, item := range items {
		params[i] = service.SubmitItemParams{
			ItemID:        item.ItemID,
			ItemTypeID:    item.ItemTypeID,
			OrgID:         orgID,
			Payload:       item.Payload,
			CreatorID:     item.CreatorID,
			CreatorTypeID: item.CreatorTypeID,
		}
	}
	return params
}

// buildJobArgs converts the JSON request items to ProcessItemJobArgs for river.
func buildJobArgs(orgID string, items []submitItemJSON) []ProcessItemJobArgs {
	jobs := make([]ProcessItemJobArgs, len(items))
	for i, item := range items {
		jobs[i] = ProcessItemJobArgs{
			OrgID:      orgID,
			ItemID:     item.ItemID,
			ItemTypeID: item.ItemTypeID,
			// EventType is resolved by the worker using the item type name.
			EventType: item.ItemTypeID,
			Payload:   item.Payload,
		}
	}
	return jobs
}
