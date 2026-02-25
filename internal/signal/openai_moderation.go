package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

const (
	_openaiModerationURL         = "https://api.openai.com/v1/moderations"
	_openaiModerationDefaultMax  = 102400 // 100 KB
	_openaiModerationDefaultTO   = 5 * time.Second
	_openaiModerationRespLimit   = 1 << 20 // 1 MB
	_openaiModerationRetryCapSec = 3
)

// openaiModerationRequest is the JSON body sent to the OpenAI Moderations API.
type openaiModerationRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

// openaiModerationResult holds a single entry in the "results" array.
type openaiModerationResult struct {
	Flagged        bool               `json:"flagged"`
	Categories     map[string]bool    `json:"categories"`
	CategoryScores map[string]float64 `json:"category_scores"`
}

// openaiModerationResponse is the top-level response from the OpenAI Moderations API.
type openaiModerationResponse struct {
	Results []openaiModerationResult `json:"results"`
}

// OpenAIModerationAdapter calls the OpenAI Moderations API and surfaces the
// per-category scores as a SignalOutput.
//
// The adapter is safe for concurrent use; it shares a single http.Client.
type OpenAIModerationAdapter struct {
	apiKey     string
	model      string
	maxInput   int
	httpClient *http.Client
}

// NewOpenAIModerationAdapter creates an OpenAIModerationAdapter.
//
// Pre-conditions: apiKey must be non-empty.
// If timeout <= 0 it defaults to 5 seconds.
// If maxInput <= 0 it defaults to 102400 (100 KB).
//
// Post-conditions: returned adapter is ready for concurrent use.
func NewOpenAIModerationAdapter(apiKey, model string, timeout time.Duration, maxInput int) *OpenAIModerationAdapter {
	if timeout <= 0 {
		timeout = _openaiModerationDefaultTO
	}
	if maxInput <= 0 {
		maxInput = _openaiModerationDefaultMax
	}
	return &OpenAIModerationAdapter{
		apiKey:     apiKey,
		model:      model,
		maxInput:   maxInput,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// NewOpenAIModerationAdapterWithClient creates an OpenAIModerationAdapter using
// a caller-provided http.Client. This is intended for testing only.
//
// If maxInput <= 0 it defaults to 102400 (100 KB).
//
// Post-conditions: returned adapter is ready for concurrent use.
func NewOpenAIModerationAdapterWithClient(apiKey, model string, maxInput int, client *http.Client) *OpenAIModerationAdapter {
	if maxInput <= 0 {
		maxInput = _openaiModerationDefaultMax
	}
	return &OpenAIModerationAdapter{
		apiKey:     apiKey,
		model:      model,
		maxInput:   maxInput,
		httpClient: client,
	}
}

// ID returns "openai-moderation".
func (a *OpenAIModerationAdapter) ID() string { return "openai-moderation" }

// DisplayName returns the human-readable name.
func (a *OpenAIModerationAdapter) DisplayName() string { return "OpenAI Moderation" }

// Description describes the adapter.
func (a *OpenAIModerationAdapter) Description() string {
	return "Evaluates text content using the OpenAI Moderations API, returning per-category scores for hate, harassment, self-harm, sexual, and violence categories."
}

// EligibleInputs returns the input types this adapter accepts.
func (a *OpenAIModerationAdapter) EligibleInputs() []domain.SignalInputType {
	return []domain.SignalInputType{"text"}
}

// Cost returns the relative processing cost.
func (a *OpenAIModerationAdapter) Cost() int { return 15 }

// Run calls the OpenAI Moderations API and returns a SignalOutput.
//
// Pre-conditions: ctx must not be nil; input.Value must be non-empty and within maxInput bytes.
// Post-conditions: Score is the maximum category score; Label is the highest-scoring category.
//
// Raises: error if input.Value is empty, exceeds maxInput, the request fails, the API
// returns a non-2xx status (after one retry on 429), or the response is malformed.
func (a *OpenAIModerationAdapter) Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error) {
	if input.Value == "" {
		return domain.SignalOutput{}, fmt.Errorf("openai-moderation: input value must not be empty")
	}
	if len(input.Value) > a.maxInput {
		return domain.SignalOutput{}, fmt.Errorf("openai-moderation: input value exceeds maximum length of %d bytes", a.maxInput)
	}

	out, err := a.doRequest(ctx, input.Value)
	if err != nil {
		return domain.SignalOutput{}, err
	}
	return out, nil
}

// doRequest executes the HTTP call with one retry on 429.
func (a *OpenAIModerationAdapter) doRequest(ctx context.Context, text string) (domain.SignalOutput, error) {
	resp, err := a.sendRequest(ctx, text)
	if err != nil {
		return domain.SignalOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		wait := a.parseRetryAfter(resp.Header.Get("Retry-After"))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return domain.SignalOutput{}, ctx.Err()
		}

		// One retry after 429.
		resp2, err2 := a.sendRequest(ctx, text)
		if err2 != nil {
			return domain.SignalOutput{}, err2
		}
		defer resp2.Body.Close()
		return a.parseResponse(resp2)
	}

	return a.parseResponse(resp)
}

// sendRequest builds and executes the POST request to the OpenAI Moderations API.
func (a *OpenAIModerationAdapter) sendRequest(ctx context.Context, text string) (*http.Response, error) {
	body, err := json.Marshal(openaiModerationRequest{Input: text, Model: a.model})
	if err != nil {
		return nil, fmt.Errorf("openai-moderation: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, _openaiModerationURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai-moderation: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-moderation: request failed: %w", err)
	}
	return resp, nil
}

// parseResponse decodes the OpenAI response body into a SignalOutput.
func (a *OpenAIModerationAdapter) parseResponse(resp *http.Response) (domain.SignalOutput, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, _openaiModerationRespLimit))
		return domain.SignalOutput{}, fmt.Errorf("openai-moderation: non-2xx response %d: %s", resp.StatusCode, string(body))
	}

	limited := io.LimitReader(resp.Body, _openaiModerationRespLimit)
	var parsed openaiModerationResponse
	if err := json.NewDecoder(limited).Decode(&parsed); err != nil {
		return domain.SignalOutput{}, fmt.Errorf("openai-moderation: decode response: %w", err)
	}

	if len(parsed.Results) == 0 {
		return domain.SignalOutput{}, fmt.Errorf("openai-moderation: response contained no results")
	}

	return a.buildOutput(parsed.Results[0]), nil
}

// buildOutput converts a moderation result into a SignalOutput.
func (a *OpenAIModerationAdapter) buildOutput(result openaiModerationResult) domain.SignalOutput {
	metadata := make(map[string]any, len(result.CategoryScores)+len(result.Categories)+2)

	var maxScore float64
	var maxLabel string
	for category, score := range result.CategoryScores {
		metadata[category] = score
		if score > maxScore {
			maxScore = score
			maxLabel = category
		}
	}

	for category, flagged := range result.Categories {
		metadata[category+"_flagged"] = flagged
	}
	metadata["flagged"] = result.Flagged
	metadata["model"] = a.model

	return domain.SignalOutput{
		Score:    maxScore,
		Label:    maxLabel,
		Metadata: metadata,
	}
}

// parseRetryAfter parses the Retry-After header value into a wait duration.
// Defaults to 1s if missing or unparseable; caps at 3s.
func (a *OpenAIModerationAdapter) parseRetryAfter(header string) time.Duration {
	const defaultWait = 1 * time.Second
	const maxWait = time.Duration(_openaiModerationRetryCapSec) * time.Second

	if header == "" {
		return defaultWait
	}
	secs, err := strconv.Atoi(header)
	if err != nil {
		return defaultWait
	}
	wait := time.Duration(secs) * time.Second
	if wait > maxWait {
		return maxWait
	}
	return wait
}
