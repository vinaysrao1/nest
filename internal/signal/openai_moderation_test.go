package signal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/signal"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// allCategories is the canonical list of 13 OpenAI moderation categories.
var allCategories = []string{
	"hate",
	"hate/threatening",
	"harassment",
	"harassment/threatening",
	"illicit",
	"illicit/violent",
	"self-harm",
	"self-harm/intent",
	"self-harm/instructions",
	"sexual",
	"sexual/minors",
	"violence",
	"violence/graphic",
}

// realisticModerationResponse builds a full OpenAI moderation response body with
// realistic per-category scores. The category named by highCategory receives the
// highest score (0.95); all others receive 0.01.
func realisticModerationResponse(highCategory string, flagged bool) []byte {
	scores := make(map[string]float64, len(allCategories))
	categories := make(map[string]bool, len(allCategories))
	for _, c := range allCategories {
		scores[c] = 0.01
		categories[c] = false
	}
	scores[highCategory] = 0.95
	if flagged {
		categories[highCategory] = true
	}

	resp := map[string]any{
		"results": []map[string]any{
			{
				"flagged":         flagged,
				"categories":      categories,
				"category_scores": scores,
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// ---------------------------------------------------------------------------
// Identity / metadata tests
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_ID(t *testing.T) {
	a := signal.NewOpenAIModerationAdapter("key", "model", 0, 0)
	if a.ID() != "openai-moderation" {
		t.Errorf("expected ID 'openai-moderation', got %q", a.ID())
	}
}

func TestOpenAIModerationAdapter_EligibleInputs(t *testing.T) {
	a := signal.NewOpenAIModerationAdapter("key", "model", 0, 0)
	inputs := a.EligibleInputs()
	if len(inputs) != 1 {
		t.Fatalf("expected 1 eligible input, got %d", len(inputs))
	}
	if inputs[0] != domain.SignalInputType("text") {
		t.Errorf("expected 'text', got %q", inputs[0])
	}
}

func TestOpenAIModerationAdapter_Cost(t *testing.T) {
	a := signal.NewOpenAIModerationAdapter("key", "model", 0, 0)
	if a.Cost() != 15 {
		t.Errorf("expected cost 15, got %d", a.Cost())
	}
}

// ---------------------------------------------------------------------------
// Run — happy path
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_TextInput(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = readAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("violence", true))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "text-moderation-stable")
	out, err := a.Run(context.Background(), domain.SignalInput{Type: "openai-moderation", Value: "violent content here"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request body contains input and model.
	var decoded map[string]any
	if err2 := json.Unmarshal(capturedBody, &decoded); err2 != nil {
		t.Fatalf("captured body is not valid JSON: %v", err2)
	}
	if decoded["input"] != "violent content here" {
		t.Errorf("unexpected 'input' field: %v", decoded["input"])
	}
	if decoded["model"] != "text-moderation-stable" {
		t.Errorf("unexpected 'model' field: %v", decoded["model"])
	}

	// Score should be the max of all category scores (0.95 for violence).
	if out.Score != 0.95 {
		t.Errorf("expected score 0.95, got %v", out.Score)
	}
	// Label should be the highest-scoring category.
	if out.Label != "violence" {
		t.Errorf("expected label 'violence', got %q", out.Label)
	}

	// Metadata should contain all 13 category scores.
	if out.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	for _, c := range allCategories {
		if _, ok := out.Metadata[c]; !ok {
			t.Errorf("metadata missing category score %q", c)
		}
		flagKey := c + "_flagged"
		if _, ok := out.Metadata[flagKey]; !ok {
			t.Errorf("metadata missing category flag %q", flagKey)
		}
	}
	if _, ok := out.Metadata["flagged"]; !ok {
		t.Error("metadata missing 'flagged' key")
	}
	if out.Metadata["model"] != "text-moderation-stable" {
		t.Errorf("metadata 'model' wrong: %v", out.Metadata["model"])
	}
}

// ---------------------------------------------------------------------------
// Run — does NOT validate input.Type
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_DoesNotValidateInputType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("hate", false))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "text-moderation-stable")

	// Pass a Type value that is not "text" — adapter must not reject it.
	_, err := a.Run(context.Background(), domain.SignalInput{
		Type:  "some-other-type",
		Value: "some text",
	})
	if err != nil {
		t.Fatalf("expected no error for arbitrary input.Type, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Run — validation errors (no HTTP calls)
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_EmptyInput(t *testing.T) {
	a := signal.NewOpenAIModerationAdapter("key", "model", 5*time.Second, 0)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: ""})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestOpenAIModerationAdapter_InputTooLarge(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// maxInput of 10 bytes.
	a := signal.NewOpenAIModerationAdapter("key", "model", 5*time.Second, 10)

	largeInput := strings.Repeat("a", 11)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: largeInput})
	if err == nil {
		t.Fatal("expected error for oversized input, got nil")
	}
	if callCount != 0 {
		t.Errorf("expected zero HTTP calls for oversized input, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// Run — non-2xx response
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_Non2xxResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code 500, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Run — 429 retry tests
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_429Retry(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: 429 with a 0-second Retry-After to keep the test fast.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// Second call: success.
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("hate", false))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount.Load())
	}
}

func TestOpenAIModerationAdapter_429RetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error after retry exhausted, got nil")
	}
}

func TestOpenAIModerationAdapter_429RetryAfterMissing(t *testing.T) {
	var firstCall atomic.Bool
	var elapsed time.Duration
	start := time.Now()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCall.Swap(true) {
			// First call: 429 with no Retry-After header.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		elapsed = time.Since(start)
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("hate", false))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have waited at least 1 second (default backoff).
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected at least ~1s backoff for missing Retry-After, got %v", elapsed)
	}
}

func TestOpenAIModerationAdapter_429RetryAfterCapped(t *testing.T) {
	var firstCall atomic.Bool
	start := time.Now()
	var elapsed time.Duration

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstCall.Swap(true) {
			// First call: 429 with Retry-After: 100 (should be capped at 3s).
			w.Header().Set("Retry-After", "100")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		elapsed = time.Since(start)
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("hate", false))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.Run(ctx, domain.SignalInput{Type: "text", Value: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Wait should be capped at 3s, so elapsed should be < 5s.
	if elapsed > 5*time.Second {
		t.Errorf("expected capped backoff (max 3s), got %v", elapsed)
	}
	// And it should have waited at least 2.5s (within tolerance of the 3s cap).
	if elapsed < 2500*time.Millisecond {
		t.Errorf("expected at least ~3s backoff for capped Retry-After, got %v", elapsed)
	}
}

func TestOpenAIModerationAdapter_429RetryContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always 429 with a long Retry-After.
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	a := adapterForServer(srv.URL, "model")

	// Cancel the context shortly after the request is issued.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := a.Run(ctx, domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error when context cancelled during retry wait, got nil")
	}
	// The error should be the context error.
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Run — timeout
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Redirect to test server with very short timeout.
	a := adapterWithTimeout(srv.URL, "model", 50*time.Millisecond)
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — malformed JSON
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("this is not json{{{"))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — large response body (>1MB)
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_LargeResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write slightly over 1MB of garbage then a closing brace so the body
		// exists but won't decode into a valid OpenAI response.
		w.Write([]byte(`{"results":[{"flagged":false,"categories":{`))
		// Pad with > 1MB of data inside the categories object.
		pad := strings.Repeat(`"x":false,`, 110000) // ~110 000 * 10 = 1.1MB
		// Remove trailing comma to keep it valid-ish JSON, but the total body
		// will exceed 1MB causing the LimitReader to truncate it, producing a
		// JSON decode error.
		w.Write([]byte(pad))
		w.Write([]byte(`"z":false},"category_scores":{}}`))
		w.Write([]byte(`]}`))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for response body exceeding 1MB, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — empty results array
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err == nil {
		t.Fatal("expected error for empty results array, got nil")
	}
}

// ---------------------------------------------------------------------------
// Run — authorization header
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_AuthorizationHeader(t *testing.T) {
	var capturedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("hate", false))
	}))
	defer srv.Close()

	a := adapterForServerWithKey(srv.URL, "model", "my-secret-api-key")
	_, err := a.Run(context.Background(), domain.SignalInput{Type: "text", Value: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Bearer my-secret-api-key"
	if capturedAuth != expected {
		t.Errorf("expected Authorization header %q, got %q", expected, capturedAuth)
	}
}

// ---------------------------------------------------------------------------
// Run — concurrent safety
// ---------------------------------------------------------------------------

func TestOpenAIModerationAdapter_ConcurrentSafety(t *testing.T) {
	const goroutines = 50

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(realisticModerationResponse("harassment", false))
	}))
	defer srv.Close()

	a := adapterForServer(srv.URL, "model")

	var wg sync.WaitGroup
	errors := make([]error, goroutines)
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			_, err := a.Run(context.Background(), domain.SignalInput{
				Type:  "text",
				Value: fmt.Sprintf("concurrent text %d", idx),
			})
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Internal helpers for test server redirection
//
// Because OpenAIModerationAdapter uses a hardcoded URL internally, tests must
// intercept outgoing requests. We achieve this by injecting a custom
// http.Transport that rewrites the target host to the test server host.
// ---------------------------------------------------------------------------

// redirectTransport rewrites all requests to target a specific test server URL.
type redirectTransport struct {
	target  string
	wrapped http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse the target to extract host.
	targetURL := req.URL
	targetURL.Host = strings.TrimPrefix(t.target, "http://")
	targetURL.Scheme = "http"
	req2 := req.Clone(req.Context())
	req2.URL = targetURL
	return t.wrapped.RoundTrip(req2)
}

// adapterForServer creates an OpenAIModerationAdapter that sends requests to
// the given test server URL instead of the real OpenAI API.
func adapterForServer(serverURL, model string) *signal.OpenAIModerationAdapter {
	return adapterForServerWithKey(serverURL, model, "test-api-key")
}

func adapterForServerWithKey(serverURL, model, apiKey string) *signal.OpenAIModerationAdapter {
	transport := &redirectTransport{
		target:  serverURL,
		wrapped: http.DefaultTransport,
	}
	return signal.NewOpenAIModerationAdapterWithClient(
		apiKey, model, 0,
		&http.Client{Transport: transport, Timeout: 5 * time.Second},
	)
}

func adapterWithTimeout(serverURL, model string, timeout time.Duration) *signal.OpenAIModerationAdapter {
	transport := &redirectTransport{
		target:  serverURL,
		wrapped: http.DefaultTransport,
	}
	return signal.NewOpenAIModerationAdapterWithClient(
		"test-api-key", model, 0,
		&http.Client{Transport: transport, Timeout: timeout},
	)
}

// readAll reads all bytes from a reader (helper for tests).
func readAll(r io.Reader) ([]byte, error) {
	var buf []byte
	b := make([]byte, 512)
	for {
		n, err := r.Read(b)
		buf = append(buf, b[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
