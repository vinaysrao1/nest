package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

// httpSignalRequest is the JSON body sent to the remote signal endpoint.
type httpSignalRequest struct {
	Type  domain.SignalInputType `json:"type"`
	Value string                `json:"value"`
}

// httpSignalResponse is the expected JSON response from the remote endpoint.
type httpSignalResponse struct {
	Score    float64        `json:"score"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// HTTPSignalAdapter delegates signal evaluation to an external HTTP service.
// It POSTs a JSON body and parses the response into a SignalOutput.
//
// The adapter is safe for concurrent use; it shares a single http.Client.
type HTTPSignalAdapter struct {
	id          string
	displayName string
	description string
	url         string
	headers     map[string]string
	httpClient  *http.Client
}

// NewHTTPSignalAdapter creates an HTTPSignalAdapter.
//
// Pre-conditions: id, displayName, url must be non-empty.
// If timeout <= 0 it defaults to 5 seconds.
//
// Post-conditions: returned adapter is ready for concurrent use.
func NewHTTPSignalAdapter(
	id, displayName, description, url string,
	headers map[string]string,
	timeout time.Duration,
) *HTTPSignalAdapter {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPSignalAdapter{
		id:          id,
		displayName: displayName,
		description: description,
		url:         url,
		headers:     headers,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

// ID returns the adapter's unique identifier.
func (a *HTTPSignalAdapter) ID() string { return a.id }

// DisplayName returns the human-readable name.
func (a *HTTPSignalAdapter) DisplayName() string { return a.displayName }

// Description returns the adapter description.
func (a *HTTPSignalAdapter) Description() string { return a.description }

// EligibleInputs returns the input types this adapter accepts.
func (a *HTTPSignalAdapter) EligibleInputs() []domain.SignalInputType {
	return []domain.SignalInputType{"text", "image_url"}
}

// Cost returns the relative processing cost.
func (a *HTTPSignalAdapter) Cost() int { return 10 }

// Run sends the input to the remote HTTP endpoint and returns the parsed score.
//
// Pre-conditions: ctx must not be nil; input.Value must be non-empty.
// Post-conditions: Score and Label are populated from the response body.
// Raises: error on marshal failure, network error, non-2xx status, or JSON decode error.
func (a *HTTPSignalAdapter) Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error) {
	reqBody, err := json.Marshal(httpSignalRequest{Type: input.Type, Value: input.Value})
	if err != nil {
		return domain.SignalOutput{}, fmt.Errorf("http-signal: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(reqBody))
	if err != nil {
		return domain.SignalOutput{}, fmt.Errorf("http-signal: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return domain.SignalOutput{}, fmt.Errorf("http-signal: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return domain.SignalOutput{}, fmt.Errorf("http-signal: non-2xx response %d: %s", resp.StatusCode, string(body))
	}

	var result httpSignalResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return domain.SignalOutput{}, fmt.Errorf("http-signal: decode response: %w", err)
	}

	return domain.SignalOutput{
		Score:    result.Score,
		Label:    result.Label,
		Metadata: result.Metadata,
	}, nil
}
