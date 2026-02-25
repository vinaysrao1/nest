package handler

import (
	"log/slog"
	"net/http"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
)

// signalSummary is the JSON shape returned in GET /api/v1/signals.
type signalSummary struct {
	ID             string                  `json:"id"`
	DisplayName    string                  `json:"display_name"`
	Description    string                  `json:"description"`
	EligibleInputs []domain.SignalInputType `json:"eligible_inputs"`
	Cost           int                     `json:"cost"`
}

// testSignalRequest is the decoded body for POST /api/v1/signals/test.
type testSignalRequest struct {
	SignalID string           `json:"signal_id"`
	Input    testSignalInput  `json:"input"`
}

// testSignalInput maps the JSON input shape to domain.SignalInput.
type testSignalInput struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// handleListSignals returns all registered signal adapters with their metadata.
//
// GET /api/v1/signals
func handleListSignals(reg *signal.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapters := reg.All()
		summaries := make([]signalSummary, len(adapters))
		for i, a := range adapters {
			summaries[i] = signalSummary{
				ID:             a.ID(),
				DisplayName:    a.DisplayName(),
				Description:    a.Description(),
				EligibleInputs: a.EligibleInputs(),
				Cost:           a.Cost(),
			}
		}

		JSON(w, http.StatusOK, map[string]any{
			"signals": summaries,
		})
	}
}

// handleTestSignal runs a single signal adapter against a test input and returns the output.
//
// POST /api/v1/signals/test
func handleTestSignal(reg *signal.Registry, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req testSignalRequest
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}

		adapter := reg.Get(req.SignalID)
		if adapter == nil {
			Error(w, http.StatusNotFound, "signal adapter not found: "+req.SignalID)
			return
		}

		input := domain.SignalInput{
			Type:  domain.SignalInputType(req.Input.Type),
			Value: req.Input.Value,
		}

		output, err := adapter.Run(r.Context(), input)
		if err != nil {
			mapError(w, err, logger)
			return
		}

		JSON(w, http.StatusOK, output)
	}
}
