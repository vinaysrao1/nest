// Package main is the Nest load generator. It submits items to POST /api/v1/items
// at a controlled rate using API key authentication, then writes final stats to
// /tmp/nest_generator_stats.txt on exit.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Domain types that mirror the Nest API contract (no import of nest internals)
// ---------------------------------------------------------------------------

// itemPayload holds the per-item request fields for POST /api/v1/items.
type itemPayload struct {
	ItemID     string         `json:"item_id"`
	ItemTypeID string         `json:"item_type_id"`
	Payload    map[string]any `json:"payload"`
}

// submitRequest is the full JSON body for POST /api/v1/items.
type submitRequest struct {
	Items []itemPayload `json:"items"`
}

// triggeredRule mirrors domain.TriggeredRule for response decoding.
type triggeredRule struct {
	RuleID    string `json:"rule_id"`
	Version   int    `json:"version"`
	Verdict   string `json:"verdict"`
	Reason    string `json:"reason,omitempty"`
	LatencyUs int64  `json:"latency_us"`
}

// actionResult mirrors domain.ActionResult for response decoding.
type actionResult struct {
	ActionID string `json:"action_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// itemResult holds a single result from the response.
type itemResult struct {
	ItemID         string          `json:"item_id"`
	Verdict        string          `json:"verdict"`
	TriggeredRules []triggeredRule `json:"triggered_rules"`
	Actions        []actionResult  `json:"actions"`
}

// submitResponse is the parsed JSON body from a successful POST /api/v1/items.
type submitResponse struct {
	Results []itemResult `json:"results"`
}

// testdataPost matches one entry in testdata/posts.json.
type testdataPost struct {
	Text     string `json:"text"`
	EntityID string `json:"entity_id"`
}

// testdataLike matches one entry in testdata/likes.json.
type testdataLike struct {
	EntityID   string `json:"entity_id"`
	SubjectURI string `json:"subject_uri"`
}

// ---------------------------------------------------------------------------
// Atomic counters
// ---------------------------------------------------------------------------

type counters struct {
	sent     atomic.Int64
	dropped  atomic.Int64
	errors   atomic.Int64
	blocks   atomic.Int64
	reviews  atomic.Int64
	approves atomic.Int64
}

// ---------------------------------------------------------------------------
// Item generation
// ---------------------------------------------------------------------------

const (
	// shortTextChance is the fraction of posts that use short (<10 char) text.
	shortTextChance = 0.20
	// numericTextChance is the fraction of posts that contain 11+ digits.
	numericTextChance = 0.10
)

// shortWords is a pool of short words (<10 chars) used for spam-short-post triggering.
var shortWords = []string{"hi", "ok", "lol", "yo", "hey", "sup", "bye", "nope", "yep", "k"}

// normalPhrases are used for normal (non-trigger) posts.
var normalPhrases = []string{
	"Just posted a new photo album from my vacation trip last week.",
	"Really enjoyed reading that article about distributed systems today.",
	"The weather here is absolutely beautiful this time of year.",
	"Finished my morning run — feeling great and ready for the day.",
	"Has anyone tried the new restaurant downtown yet? Highly recommend it.",
	"Working on a new side project involving Go and PostgreSQL performance.",
	"Watched an amazing documentary about ocean conservation last night.",
	"Looking forward to the weekend — lots of outdoor activities planned.",
	"Just discovered a fantastic coffee shop near the library downtown.",
	"Trying to learn more about machine learning fundamentals this month.",
	"The community garden is really coming along nicely this season.",
	"Shared some thoughts on open source sustainability on my blog today.",
	"Big thunderstorm rolled through the city this afternoon — spectacular.",
	"Planning a hiking trip next month — any trail recommendations welcome.",
	"Attended a really thoughtful talk on urban planning this morning.",
}

// generatePost builds a single post item for the given sequence number.
// Text properties are chosen to exercise spam-short-post and numeric-content rules.
func generatePost(seq int64, itemTypeID string) submitRequest {
	r := rand.Float64()
	userN := (seq%50 + 1) // user-1 through user-50

	var text string
	switch {
	case r < shortTextChance:
		// Short post: < 10 chars — triggers spam-short-post after threshold.
		word := shortWords[seq%int64(len(shortWords))]
		text = word
	case r < shortTextChance+numericTextChance:
		// Numeric post: 11+ digits — triggers numeric-content rule.
		text = fmt.Sprintf("Order number 12345678901 confirmed for user %d", userN)
	default:
		// Normal post: 20-80 chars, no excessive digits.
		phrase := normalPhrases[seq%int64(len(normalPhrases))]
		text = phrase
	}

	return submitRequest{
		Items: []itemPayload{
			{
				ItemID:     fmt.Sprintf("post-%d", seq),
				ItemTypeID: itemTypeID,
				Payload: map[string]any{
					"text":       text,
					"entity_id":  fmt.Sprintf("user-%d", userN),
					"is_reply":   false,
					"created_at": time.Now().UTC().Format(time.RFC3339),
				},
			},
		},
	}
}

// generateLike builds a single like item for the given sequence number.
// Users cycle through a small pool to ensure counter thresholds are hit.
func generateLike(seq int64, itemTypeID string) submitRequest {
	userN := (seq%30 + 1) // user-1 through user-30
	subjectN := seq%100 + 1
	subjectOwner := seq%20 + 1

	return submitRequest{
		Items: []itemPayload{
			{
				ItemID:     fmt.Sprintf("like-%d", seq),
				ItemTypeID: itemTypeID,
				Payload: map[string]any{
					"entity_id":   fmt.Sprintf("user-%d", userN),
					"subject_uri": fmt.Sprintf("at://user-%d/post/%d", subjectOwner, subjectN),
					"created_at":  time.Now().UTC().Format(time.RFC3339),
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// HTTP submission with retry on 429
// ---------------------------------------------------------------------------

const (
	maxRetries        = 3
	initialBackoff    = 100 * time.Millisecond
	statsFilePath     = "/tmp/nest_generator_stats.txt"
	statsPrintInterval = 5 * time.Second
)

// submitItem POSTs req to the Nest items endpoint. On HTTP 429 it retries up to
// maxRetries times with exponential backoff. It returns an error if all retries
// are exhausted or a non-2xx response is received.
func submitItem(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	req submitRequest,
) (*submitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := nestURL + "/api/v1/items"
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		httpReq.Header.Set("X-API-Key", apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("http do: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt == maxRetries {
				return nil, fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		var result submitResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode response: %w", err)
		}
		_ = resp.Body.Close()
		return &result, nil
	}

	return nil, fmt.Errorf("exhausted retries")
}

// ---------------------------------------------------------------------------
// Stats helpers
// ---------------------------------------------------------------------------

// printStats logs the current counters to stdout with the [generator] prefix.
func printStats(c *counters) {
	fmt.Printf(
		"[generator] sent=%d dropped=%d approve=%d block=%d review=%d\n",
		c.sent.Load(),
		c.dropped.Load(),
		c.approves.Load(),
		c.blocks.Load(),
		c.reviews.Load(),
	)
}

// writeStatsFile writes final counter values to /tmp/nest_generator_stats.txt.
func writeStatsFile(c *counters) {
	content := fmt.Sprintf(
		"sent=%d\ndropped=%d\nerrors=%d\nblocks=%d\nreviews=%d\napproves=%d\n",
		c.sent.Load(),
		c.dropped.Load(),
		c.errors.Load(),
		c.blocks.Load(),
		c.reviews.Load(),
		c.approves.Load(),
	)
	if err := os.WriteFile(statsFilePath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "[generator] failed to write stats file: %v\n", err)
	}
}

// recordVerdict increments the appropriate verdict counter.
func recordVerdict(c *counters, verdict string) {
	switch verdict {
	case "block":
		c.blocks.Add(1)
	case "review":
		c.reviews.Add(1)
	default:
		c.approves.Add(1)
	}
}

// ---------------------------------------------------------------------------
// Deterministic mode: load testdata files
// ---------------------------------------------------------------------------

// loadTestdataPosts reads testdata/posts.json relative to testdataDir.
func loadTestdataPosts(testdataDir string, postItemTypeID string) ([]submitRequest, error) {
	path := filepath.Join(testdataDir, "posts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var posts []testdataPost
	if err := json.Unmarshal(data, &posts); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	reqs := make([]submitRequest, 0, len(posts))
	for i, p := range posts {
		reqs = append(reqs, submitRequest{
			Items: []itemPayload{
				{
					ItemID:     fmt.Sprintf("post-%d", i),
					ItemTypeID: postItemTypeID,
					Payload: map[string]any{
						"text":       p.Text,
						"entity_id":  p.EntityID,
						"is_reply":   false,
						"created_at": time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		})
	}
	return reqs, nil
}

// loadTestdataLikes reads testdata/likes.json relative to testdataDir.
func loadTestdataLikes(testdataDir string, likeItemTypeID string) ([]submitRequest, error) {
	path := filepath.Join(testdataDir, "likes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var likes []testdataLike
	if err := json.Unmarshal(data, &likes); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	reqs := make([]submitRequest, 0, len(likes))
	for i, l := range likes {
		reqs = append(reqs, submitRequest{
			Items: []itemPayload{
				{
					ItemID:     fmt.Sprintf("like-%d", i),
					ItemTypeID: likeItemTypeID,
					Payload: map[string]any{
						"entity_id":   l.EntityID,
						"subject_uri": l.SubjectURI,
						"created_at":  time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		})
	}
	return reqs, nil
}

// ---------------------------------------------------------------------------
// Config loading helpers
// ---------------------------------------------------------------------------

// readFileContent reads a file and returns its trimmed content.
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// parseKeyValue parses a file with lines of the form key=value.
// It returns the value for the given key or an error if not found.
func parseKeyValue(path string, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"="), nil
		}
	}
	return "", fmt.Errorf("key %q not found in %s", key, path)
}

// ---------------------------------------------------------------------------
// Concurrent dispatch helpers
// ---------------------------------------------------------------------------

// dispatchItem sends a single request concurrently, gated by the semaphore.
// It updates counters and releases the semaphore slot when done.
func dispatchItem(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	req submitRequest,
	sem chan struct{},
	c *counters,
	wg *sync.WaitGroup,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { <-sem }()

		resp, err := submitItem(ctx, client, nestURL, apiKey, req)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled — do not count as an error.
				return
			}
			if c.errors.Load() < 5 {
				fmt.Fprintf(os.Stderr, "[generator] submit error: %v\n", err)
			}
			// Distinguish rate-limit exhaustion (dropped) from other errors.
			if strings.Contains(err.Error(), "rate limited") {
				c.dropped.Add(1)
			} else {
				c.errors.Add(1)
				c.dropped.Add(1)
			}
			return
		}

		c.sent.Add(1)
		for _, result := range resp.Results {
			recordVerdict(c, result.Verdict)
		}
	}()
}

// ---------------------------------------------------------------------------
// Generation loops per mode
// ---------------------------------------------------------------------------

// runMixed sends items in random order at the configured rate until maxItems is
// reached or ctx is cancelled. postRatio controls the fraction of posts vs likes.
func runMixed(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	maxItems int64,
	rate int,
	postItemTypeID string,
	likeItemTypeID string,
	postRatio float64,
	sem chan struct{},
	c *counters,
	wg *sync.WaitGroup,
) {
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	var seq int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if maxItems > 0 && seq >= maxItems {
				return
			}

			var req submitRequest
			if rand.Float64() < postRatio {
				req = generatePost(seq, postItemTypeID)
			} else {
				req = generateLike(seq, likeItemTypeID)
			}
			seq++

			// Acquire semaphore slot (blocks until a slot is free).
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			dispatchItem(ctx, client, nestURL, apiKey, req, sem, c, wg)
		}
	}
}

// runDeterministic sends items loaded from testdata files in order.
func runDeterministic(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	maxItems int64,
	rate int,
	postItemTypeID string,
	likeItemTypeID string,
	postRatio float64,
	testdataDir string,
	sem chan struct{},
	c *counters,
	wg *sync.WaitGroup,
) {
	posts, err := loadTestdataPosts(testdataDir, postItemTypeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[generator] deterministic: %v\n", err)
		os.Exit(1)
	}
	likes, err := loadTestdataLikes(testdataDir, likeItemTypeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[generator] deterministic: %v\n", err)
		os.Exit(1)
	}

	// Interleave posts and likes according to postRatio.
	var items []submitRequest
	pi, li := 0, 0
	for pi < len(posts) || li < len(likes) {
		if pi < len(posts) && (li >= len(likes) || rand.Float64() < postRatio) {
			items = append(items, posts[pi])
			pi++
		} else if li < len(likes) {
			items = append(items, likes[li])
			li++
		}
	}

	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	for i, req := range items {
		if maxItems > 0 && int64(i) >= maxItems {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		dispatchItem(ctx, client, nestURL, apiKey, req, sem, c, wg)
	}
}

// runBurst sends all items as fast as possible with no rate limiting.
func runBurst(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	maxItems int64,
	postItemTypeID string,
	likeItemTypeID string,
	postRatio float64,
	sem chan struct{},
	c *counters,
	wg *sync.WaitGroup,
) {
	var seq int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if maxItems > 0 && seq >= maxItems {
			return
		}

		var req submitRequest
		if rand.Float64() < postRatio {
			req = generatePost(seq, postItemTypeID)
		} else {
			req = generateLike(seq, likeItemTypeID)
		}
		seq++

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		dispatchItem(ctx, client, nestURL, apiKey, req, sem, c, wg)
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	nestURL := flag.String("nest-url", "http://localhost:8080", "Nest API base URL")
	apiKeyFlag := flag.String("api-key", "", "API key (if empty, reads from /tmp/nest_test_api_key.txt)")
	maxItems := flag.Int64("max-items", 1000, "Stop after this many items (0 = unlimited)")
	rate := flag.Int("rate", 100, "Target items per second")
	concurrency := flag.Int("concurrency", 10, "Max concurrent HTTP goroutines")
	postRatio := flag.Float64("post-ratio", 0.6, "Fraction of items that are posts (rest are likes)")
	postItemTypeIDFlag := flag.String("post-item-type-id", "", "Item type ID for posts (empty: read from /tmp/nest_test_item_types.txt)")
	likeItemTypeIDFlag := flag.String("like-item-type-id", "", "Item type ID for likes (empty: read from /tmp/nest_test_item_types.txt)")
	mode := flag.String("mode", "mixed", "Generation mode: mixed, deterministic, burst")
	testdataDir := flag.String("testdata-dir", "../testdata", "Directory containing posts.json and likes.json for deterministic mode")
	flag.Parse()

	// Resolve API key.
	apiKey := *apiKeyFlag
	if apiKey == "" {
		key, err := readFileContent("/tmp/nest_test_api_key.txt")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[generator] api key: %v\n", err)
			os.Exit(1)
		}
		apiKey = key
	}

	// Resolve item type IDs.
	postItemTypeID := *postItemTypeIDFlag
	if postItemTypeID == "" {
		id, err := parseKeyValue("/tmp/nest_test_item_types.txt", "post")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[generator] post item type id: %v\n", err)
			os.Exit(1)
		}
		postItemTypeID = id
	}

	likeItemTypeID := *likeItemTypeIDFlag
	if likeItemTypeID == "" {
		id, err := parseKeyValue("/tmp/nest_test_item_types.txt", "like")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[generator] like item type id: %v\n", err)
			os.Exit(1)
		}
		likeItemTypeID = id
	}

	if *rate <= 0 {
		fmt.Fprintln(os.Stderr, "[generator] -rate must be > 0")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "[generator] -concurrency must be > 0")
		os.Exit(1)
	}

	fmt.Printf("[generator] mode=%s rate=%d concurrency=%d max-items=%d post-ratio=%.2f\n",
		*mode, *rate, *concurrency, *maxItems, *postRatio)

	c := &counters{}
	client := &http.Client{Timeout: 30 * time.Second}
	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	// Context with cancellation — driven by SIGINT/SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\n[generator] received %v, shutting down...\n", sig)
		cancel()
	}()

	// Periodic stats printer.
	statsTicker := time.NewTicker(statsPrintInterval)
	defer statsTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statsTicker.C:
				printStats(c)
			}
		}
	}()

	// Run the selected generation loop (all loops block until done or ctx cancelled).
	switch *mode {
	case "mixed":
		runMixed(ctx, client, *nestURL, apiKey, *maxItems, *rate,
			postItemTypeID, likeItemTypeID, *postRatio, sem, c, &wg)
	case "deterministic":
		runDeterministic(ctx, client, *nestURL, apiKey, *maxItems, *rate,
			postItemTypeID, likeItemTypeID, *postRatio, *testdataDir, sem, c, &wg)
	case "burst":
		runBurst(ctx, client, *nestURL, apiKey, *maxItems,
			postItemTypeID, likeItemTypeID, *postRatio, sem, c, &wg)
	default:
		fmt.Fprintf(os.Stderr, "[generator] unknown mode %q (use mixed, deterministic, burst)\n", *mode)
		os.Exit(1)
	}

	// Wait for all in-flight requests to complete.
	wg.Wait()

	fmt.Printf("[generator] DONE. Final: sent=%d dropped=%d approve=%d block=%d review=%d\n",
		c.sent.Load(), c.dropped.Load(), c.approves.Load(), c.blocks.Load(), c.reviews.Load())

	writeStatsFile(c)
}
