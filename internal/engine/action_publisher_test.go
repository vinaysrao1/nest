package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
)

// mockSigner is a test implementation of the Signer interface.
type mockSigner struct {
	sig string
	err error
}

func (m *mockSigner) Sign(_ context.Context, _ string, _ []byte) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.sig, nil
}

// testTarget returns a sample ActionTarget for use in tests.
func testTarget() domain.ActionTarget {
	return domain.ActionTarget{
		ItemID:        "item-123",
		ItemTypeID:    "text",
		OrgID:         "org-test",
		Payload:       map[string]any{"text": "hello"},
		CorrelationID: "corr-abc",
	}
}

// webhookAction builds an ActionRequest of type WEBHOOK pointing at the given URL.
func webhookAction(actionID, url string) domain.ActionRequest {
	return domain.ActionRequest{
		Action: domain.Action{
			ID:         actionID,
			OrgID:      "org-test",
			Name:       "test-webhook",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{"url": url},
		},
	}
}

// TestActionPublisher_WebhookSuccess verifies a successful webhook delivery.
func TestActionPublisher_WebhookSuccess(t *testing.T) {
	t.Parallel()

	var capturedSig string
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSig = r.Header.Get("X-Nest-Signature")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	signer := &mockSigner{sig: "test-signature"}
	pub := NewActionPublisher(nil, signer, srv.Client(), nil)

	target := testTarget()
	action := webhookAction("action-1", srv.URL)
	results := pub.PublishActions(context.Background(), []domain.ActionRequest{action}, target)

	if len(results) != 1 {
		t.Fatalf("PublishActions: len(results) = %d, want 1", len(results))
	}
	if !results[0].Success {
		t.Errorf("PublishActions: Success = false, Error = %q", results[0].Error)
	}
	if capturedSig != "test-signature" {
		t.Errorf("X-Nest-Signature = %q, want %q", capturedSig, "test-signature")
	}
	if capturedBody["org_id"] != "org-test" {
		t.Errorf("payload org_id = %v, want %q", capturedBody["org_id"], "org-test")
	}
}

// TestActionPublisher_WebhookNon2xx verifies a non-2xx response produces a failure result.
func TestActionPublisher_WebhookNon2xx(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, srv.Client(), nil)

	results := pub.PublishActions(
		context.Background(),
		[]domain.ActionRequest{webhookAction("action-2", srv.URL)},
		testTarget(),
	)

	if results[0].Success {
		t.Error("PublishActions: expected failure for 500 response, got success")
	}
	if results[0].Error == "" {
		t.Error("PublishActions: expected non-empty Error for 500 response")
	}
}

// TestActionPublisher_WebhookSigningFailure verifies a signing error produces a failure result.
func TestActionPublisher_WebhookSigningFailure(t *testing.T) {
	t.Parallel()

	signer := &mockSigner{err: fmt.Errorf("no key for org")}
	pub := NewActionPublisher(nil, signer, http.DefaultClient, nil)

	results := pub.PublishActions(
		context.Background(),
		[]domain.ActionRequest{webhookAction("action-3", "http://localhost:9999")},
		testTarget(),
	)

	if results[0].Success {
		t.Error("PublishActions: expected failure on signing error, got success")
	}
	if results[0].Error == "" {
		t.Error("PublishActions: expected non-empty Error on signing failure")
	}
}

// TestActionPublisher_WebhookTimeout verifies that an HTTP timeout produces a failure result.
func TestActionPublisher_WebhookTimeout(t *testing.T) {
	t.Parallel()

	// Server that immediately closes the connection without responding,
	// which causes a client error without the server handler blocking.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close the connection abruptly to cause a client error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, srv.Client(), nil)

	results := pub.PublishActions(
		context.Background(),
		[]domain.ActionRequest{webhookAction("action-4", srv.URL)},
		testTarget(),
	)

	if results[0].Success {
		t.Error("PublishActions: expected failure on connection close, got success")
	}
}

// TestActionPublisher_MissingURL verifies a webhook action with no URL produces a failure.
func TestActionPublisher_MissingURL(t *testing.T) {
	t.Parallel()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, http.DefaultClient, nil)

	action := domain.ActionRequest{
		Action: domain.Action{
			ID:         "action-5",
			ActionType: domain.ActionTypeWebhook,
			Config:     map[string]any{}, // no url
		},
	}
	results := pub.PublishActions(context.Background(), []domain.ActionRequest{action}, testTarget())

	if results[0].Success {
		t.Error("PublishActions: expected failure for missing url, got success")
	}
}

// TestActionPublisher_UnknownActionType verifies unknown action types produce a failure.
func TestActionPublisher_UnknownActionType(t *testing.T) {
	t.Parallel()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, http.DefaultClient, nil)

	action := domain.ActionRequest{
		Action: domain.Action{
			ID:         "action-6",
			ActionType: "UNKNOWN_TYPE",
			Config:     map[string]any{},
		},
	}
	results := pub.PublishActions(context.Background(), []domain.ActionRequest{action}, testTarget())

	if results[0].Success {
		t.Error("PublishActions: expected failure for unknown action type, got success")
	}
}

// TestActionPublisher_EmptyActions verifies an empty actions slice returns empty results.
func TestActionPublisher_EmptyActions(t *testing.T) {
	t.Parallel()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, http.DefaultClient, nil)

	results := pub.PublishActions(context.Background(), nil, testTarget())
	if len(results) != 0 {
		t.Errorf("PublishActions(nil): len = %d, want 0", len(results))
	}
}

// TestActionPublisher_MultipleWebhooksParallel verifies multiple actions run concurrently.
func TestActionPublisher_MultipleWebhooksParallel(t *testing.T) {
	t.Parallel()

	callCount := make(chan struct{}, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	signer := &mockSigner{sig: "sig"}
	pub := NewActionPublisher(nil, signer, srv.Client(), nil)

	actions := []domain.ActionRequest{
		webhookAction("a1", srv.URL),
		webhookAction("a2", srv.URL),
		webhookAction("a3", srv.URL),
	}
	results := pub.PublishActions(context.Background(), actions, testTarget())

	if len(results) != 3 {
		t.Fatalf("PublishActions: len(results) = %d, want 3", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("results[%d].Success = false, Error = %q", i, r.Error)
		}
	}
	if len(callCount) != 3 {
		t.Errorf("server received %d calls, want 3", len(callCount))
	}
}
