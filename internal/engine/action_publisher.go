package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// Signer signs byte payloads for authenticated webhook delivery.
// Implementations are expected to use an org-specific key.
type Signer interface {
	// Sign returns a base64-encoded or hex-encoded signature of payload for the given org.
	// Raises: error if no signing key is available for the org.
	Sign(ctx context.Context, orgID string, payload []byte) (string, error)
}

// ActionPublisher executes action requests produced by rule evaluation.
// It supports webhook delivery and MRT queue enqueuing.
type ActionPublisher struct {
	store      *store.Queries
	signer     Signer
	httpClient *http.Client
	logger     *slog.Logger
}

// NewActionPublisher creates an ActionPublisher.
//
// Pre-conditions: st and signer must not be nil.
// Post-conditions: returned publisher is ready to publish actions.
func NewActionPublisher(
	st *store.Queries,
	signer Signer,
	httpClient *http.Client,
	logger *slog.Logger,
) *ActionPublisher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ActionPublisher{
		store:      st,
		signer:     signer,
		httpClient: httpClient,
		logger:     logger,
	}
}

// PublishActions executes each ActionRequest concurrently and returns one
// ActionResult per action. The results slice is in the same order as actions.
//
// Pre-conditions: target.OrgID must be non-empty.
// Post-conditions: len(results) == len(actions); never returns nil.
func (p *ActionPublisher) PublishActions(
	ctx context.Context,
	actions []domain.ActionRequest,
	target domain.ActionTarget,
) []domain.ActionResult {
	if len(actions) == 0 {
		return []domain.ActionResult{}
	}

	results := make([]domain.ActionResult, len(actions))
	var wg sync.WaitGroup
	for i, action := range actions {
		wg.Add(1)
		go func(idx int, ar domain.ActionRequest) {
			defer wg.Done()
			switch ar.Action.ActionType {
			case domain.ActionTypeWebhook:
				results[idx] = p.publishWebhook(ctx, ar, target)
			case domain.ActionTypeEnqueueToMRT:
				results[idx] = p.publishMRTEnqueue(ctx, ar, target)
			default:
				results[idx] = domain.ActionResult{
					ActionID: ar.Action.ID,
					Success:  false,
					Error:    fmt.Sprintf("unknown action type: %s", ar.Action.ActionType),
				}
			}
		}(i, action)
	}
	wg.Wait()
	return results
}

// publishWebhook signs and delivers a JSON payload to the action's URL via HTTP POST.
func (p *ActionPublisher) publishWebhook(
	ctx context.Context,
	ar domain.ActionRequest,
	target domain.ActionTarget,
) domain.ActionResult {
	payload := map[string]any{
		"item_id":        target.ItemID,
		"item_type_id":   target.ItemTypeID,
		"org_id":         target.OrgID,
		"correlation_id": target.CorrelationID,
		"action_name":    ar.Action.Name,
		"payload":        target.Payload,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return domain.ActionResult{ActionID: ar.Action.ID, Success: false, Error: err.Error()}
	}

	sig, err := p.signer.Sign(ctx, target.OrgID, jsonPayload)
	if err != nil {
		return domain.ActionResult{
			ActionID: ar.Action.ID,
			Success:  false,
			Error:    fmt.Sprintf("signing failed: %v", err),
		}
	}

	url, ok := ar.Action.Config["url"].(string)
	if !ok || url == "" {
		return domain.ActionResult{
			ActionID: ar.Action.ID,
			Success:  false,
			Error:    "webhook action missing 'url' config",
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return domain.ActionResult{ActionID: ar.Action.ID, Success: false, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nest-Signature", sig)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return domain.ActionResult{ActionID: ar.Action.ID, Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain body to enable connection reuse

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.ActionResult{
			ActionID: ar.Action.ID,
			Success:  false,
			Error:    fmt.Sprintf("webhook returned status %d", resp.StatusCode),
		}
	}

	return domain.ActionResult{ActionID: ar.Action.ID, Success: true}
}

// publishMRTEnqueue inserts an MRT job into the named queue for manual review.
func (p *ActionPublisher) publishMRTEnqueue(
	ctx context.Context,
	ar domain.ActionRequest,
	target domain.ActionTarget,
) domain.ActionResult {
	queueName, _ := ar.Action.Config["queue_name"].(string)
	if queueName == "" {
		return domain.ActionResult{
			ActionID: ar.Action.ID,
			Success:  false,
			Error:    "MRT action missing 'queue_name' config",
		}
	}

	queue, err := p.store.GetMRTQueueByName(ctx, target.OrgID, queueName)
	if err != nil {
		return domain.ActionResult{
			ActionID: ar.Action.ID,
			Success:  false,
			Error:    fmt.Sprintf("queue %q not found: %v", queueName, err),
		}
	}

	now := time.Now()
	job := domain.MRTJob{
		ID:            fmt.Sprintf("mrt-action-%s-%d", ar.Action.ID, now.UnixNano()),
		OrgID:         target.OrgID,
		QueueID:       queue.ID,
		ItemID:        target.ItemID,
		ItemTypeID:    target.ItemTypeID,
		Payload:       target.Payload,
		Status:        domain.MRTJobStatusPending,
		PolicyIDs:     []string{},
		EnqueueSource: "action",
		SourceInfo:    map[string]any{"action_id": ar.Action.ID, "action_name": ar.Action.Name},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := p.store.InsertMRTJob(ctx, &job); err != nil {
		return domain.ActionResult{ActionID: ar.Action.ID, Success: false, Error: err.Error()}
	}

	return domain.ActionResult{ActionID: ar.Action.ID, Success: true}
}
