// Package main implements a Jetstream WebSocket consumer that reads Bluesky
// posts and likes from the AT Protocol firehose and submits them to the Nest
// POST /api/v1/items endpoint.
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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// ---------------------------------------------------------------------------
// Jetstream event types (mirrors the AT Protocol Jetstream JSON schema)
// ---------------------------------------------------------------------------

// jetstreamEvent represents a single event from the Jetstream WebSocket.
type jetstreamEvent struct {
	DID    string           `json:"did"`
	TimeUS int64            `json:"time_us"`
	Kind   string           `json:"kind"`
	Commit *jetstreamCommit `json:"commit,omitempty"`
}

// jetstreamCommit holds the commit payload of a Jetstream event.
type jetstreamCommit struct {
	Rev        string          `json:"rev"`
	Operation  string          `json:"operation"`
	Collection string          `json:"collection"`
	Rkey       string          `json:"rkey"`
	Record     json.RawMessage `json:"record"`
	CID        string          `json:"cid"`
}

// postRecord holds the relevant fields from an app.bsky.feed.post record.
type postRecord struct {
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

// likeRecord holds the relevant fields from an app.bsky.feed.like record.
type likeRecord struct {
	Subject   likeSubject `json:"subject"`
	CreatedAt string      `json:"createdAt"`
}

// likeSubject is the subject reference inside a like record.
type likeSubject struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// ---------------------------------------------------------------------------
// Nest API types (no import of Nest internals)
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

// ---------------------------------------------------------------------------
// Atomic counters
// ---------------------------------------------------------------------------

// counters holds all runtime metrics for the consumer, each updated atomically.
type counters struct {
	received  atomic.Int64 // total events received from WebSocket
	posts     atomic.Int64 // post events parsed
	likes     atomic.Int64 // like events parsed
	sent      atomic.Int64 // items successfully submitted to Nest
	dropped   atomic.Int64 // items that failed submission
	errors    atomic.Int64 // HTTP errors
	parseErrs atomic.Int64 // JSON parse errors on WebSocket messages
	reconnects atomic.Int64 // WebSocket reconnection count
}

// ---------------------------------------------------------------------------
// Item mapping
// ---------------------------------------------------------------------------

// mapPostToItem converts a parsed post record into a Nest itemPayload.
func mapPostToItem(did, rkey string, record postRecord, postItemTypeID string) itemPayload {
	return itemPayload{
		ItemID:     fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey),
		ItemTypeID: postItemTypeID,
		Payload: map[string]any{
			"text":       record.Text,
			"entity_id":  did,
			"created_at": record.CreatedAt,
		},
	}
}

// mapLikeToItem converts a parsed like record into a Nest itemPayload.
func mapLikeToItem(did, rkey string, record likeRecord, likeItemTypeID string) itemPayload {
	return itemPayload{
		ItemID:     fmt.Sprintf("at://%s/app.bsky.feed.like/%s", did, rkey),
		ItemTypeID: likeItemTypeID,
		Payload: map[string]any{
			"entity_id":   did,
			"subject_uri": record.Subject.URI,
			"created_at":  record.CreatedAt,
		},
	}
}

// ---------------------------------------------------------------------------
// WebSocket read loop
// ---------------------------------------------------------------------------

// readLoop reads events from the WebSocket connection, parses them, and sends
// matching items to itemsCh. It updates cursor atomically on each event and
// returns a non-nil error when the connection is lost.
func readLoop(
	ctx context.Context,
	conn *websocket.Conn,
	postItemTypeID string,
	likeItemTypeID string,
	itemsCh chan<- itemPayload,
	cursor *atomic.Int64,
	stats *counters,
	maxItems int64,
) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if maxItems > 0 && stats.sent.Load()+stats.dropped.Load() >= maxItems {
			return nil
		}

		_, msg, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("websocket read: %w", err)
		}

		stats.received.Add(1)

		var event jetstreamEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			stats.parseErrs.Add(1)
			fmt.Fprintf(os.Stderr, "[consumer] parse event: %v\n", err)
			continue
		}

		// Update cursor for reconnection after gaps.
		if event.TimeUS > 0 {
			cursor.Store(event.TimeUS)
		}

		if event.Kind != "commit" || event.Commit == nil {
			continue
		}
		if event.Commit.Operation != "create" {
			continue
		}

		item, ok := parseCommit(event, postItemTypeID, likeItemTypeID, stats)
		if !ok {
			continue
		}

		select {
		case itemsCh <- item:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// parseCommit routes a commit event by collection and returns the mapped item.
// Returns (item, true) on success or (zero, false) if the collection is
// unrecognised or the record cannot be parsed.
func parseCommit(
	event jetstreamEvent,
	postItemTypeID string,
	likeItemTypeID string,
	stats *counters,
) (itemPayload, bool) {
	commit := event.Commit
	switch commit.Collection {
	case "app.bsky.feed.post":
		var rec postRecord
		if err := json.Unmarshal(commit.Record, &rec); err != nil {
			stats.parseErrs.Add(1)
			fmt.Fprintf(os.Stderr, "[consumer] parse post record: %v\n", err)
			return itemPayload{}, false
		}
		stats.posts.Add(1)
		return mapPostToItem(event.DID, commit.Rkey, rec, postItemTypeID), true

	case "app.bsky.feed.like":
		var rec likeRecord
		if err := json.Unmarshal(commit.Record, &rec); err != nil {
			stats.parseErrs.Add(1)
			fmt.Fprintf(os.Stderr, "[consumer] parse like record: %v\n", err)
			return itemPayload{}, false
		}
		stats.likes.Add(1)
		return mapLikeToItem(event.DID, commit.Rkey, rec, likeItemTypeID), true

	default:
		return itemPayload{}, false
	}
}

// ---------------------------------------------------------------------------
// Batch submission loop
// ---------------------------------------------------------------------------

// batchLoop accumulates items from itemsCh and flushes them to Nest either
// when the batch reaches batchSize or when flushInterval elapses. It exits
// when ctx is cancelled.
func batchLoop(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	itemsCh <-chan itemPayload,
	batchSize int,
	flushInterval time.Duration,
	stats *counters,
) {
	batch := make([]itemPayload, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flushWith := func(fctx context.Context) {
		if len(batch) == 0 {
			return
		}
		toSend := batch
		batch = make([]itemPayload, 0, batchSize)

		if err := submitBatch(fctx, client, nestURL, apiKey, toSend); err != nil {
			if fctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "[consumer] submit batch: %v\n", err)
			}
			stats.dropped.Add(int64(len(toSend)))
			stats.errors.Add(1)
			return
		}
		stats.sent.Add(int64(len(toSend)))
	}

	flush := func() { flushWith(ctx) }

	for {
		select {
		case <-ctx.Done():
			// Use a fresh context with timeout for the final drain so
			// the remaining items are actually submitted, not dropped.
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 10*time.Second)
		drain:
			for {
				select {
				case item := <-itemsCh:
					batch = append(batch, item)
					if len(batch) >= batchSize {
						flushWith(drainCtx)
					}
				default:
					break drain
				}
			}
			flushWith(drainCtx)
			drainCancel()
			return

		case item := <-itemsCh:
			batch = append(batch, item)
			if len(batch) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// ---------------------------------------------------------------------------
// HTTP submission with 429 retry
// ---------------------------------------------------------------------------

const (
	maxRetries     = 3
	initialBackoff = 100 * time.Millisecond
	statsFilePath  = "/tmp/nest_consumer_stats.txt"
	statsPrintInterval = 5 * time.Second
)

// submitBatch POSTs items to the Nest items endpoint. On HTTP 429 it retries
// up to maxRetries times with exponential backoff. Returns an error if all
// retries are exhausted or a non-2xx response is received.
func submitBatch(
	ctx context.Context,
	client *http.Client,
	nestURL string,
	apiKey string,
	items []itemPayload,
) error {
	body, err := json.Marshal(submitRequest{Items: items})
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	endpoint := nestURL + "/api/v1/items"
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("http do: %w", err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == maxRetries {
				return fmt.Errorf("rate limited after %d retries", maxRetries)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		return nil
	}

	return fmt.Errorf("exhausted retries")
}

// ---------------------------------------------------------------------------
// WebSocket connection with exponential backoff
// ---------------------------------------------------------------------------

const (
	wsInitialBackoff = time.Second
	wsMaxBackoff     = 30 * time.Second
)

// connectWithBackoff dials the Jetstream WebSocket URL, appending collection
// filters and an optional cursor. It retries with jittered exponential backoff
// until a connection is established or ctx is cancelled.
func connectWithBackoff(
	ctx context.Context,
	jetstreamURL string,
	collections []string,
	cursor *atomic.Int64,
) (*websocket.Conn, error) {
	url := buildJetstreamURL(jetstreamURL, collections, cursor.Load())
	backoff := wsInitialBackoff

	for {
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err == nil {
			// Jetstream messages can be large; raise the default 32KB read limit.
			conn.SetReadLimit(1 << 20) // 1 MB
			return conn, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		sleep := backoff + jitter
		fmt.Fprintf(os.Stderr, "[consumer] dial error: %v — retrying in %v\n", err, sleep)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleep):
		}

		backoff *= 2
		if backoff > wsMaxBackoff {
			backoff = wsMaxBackoff
		}
		// Rebuild URL in case cursor advanced since last attempt.
		url = buildJetstreamURL(jetstreamURL, collections, cursor.Load())
	}
}

// buildJetstreamURL constructs the subscription URL with collection filters
// and an optional cursor parameter.
func buildJetstreamURL(base string, collections []string, cursorVal int64) string {
	url := base + "?"
	for i, c := range collections {
		if i > 0 {
			url += "&"
		}
		url += "wantedCollections=" + c
	}
	if cursorVal > 0 {
		url += fmt.Sprintf("&cursor=%d", cursorVal)
	}
	return url
}

// ---------------------------------------------------------------------------
// Config loading helpers (same pattern as generator)
// ---------------------------------------------------------------------------

// readFileContent reads a file and returns its trimmed content.
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// parseKeyValue parses a file with lines of the form key=value and returns
// the value for the given key, or an error if the key is not found.
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
// Stats helpers
// ---------------------------------------------------------------------------

// printStats logs current counters to stdout with the [consumer] prefix.
func printStats(c *counters) {
	fmt.Printf(
		"[consumer] received=%d sent=%d dropped=%d posts=%d likes=%d reconnects=%d\n",
		c.received.Load(),
		c.sent.Load(),
		c.dropped.Load(),
		c.posts.Load(),
		c.likes.Load(),
		c.reconnects.Load(),
	)
}

// writeStatsFile writes final counter values to /tmp/nest_consumer_stats.txt.
func writeStatsFile(c *counters) {
	content := fmt.Sprintf(
		"sent=%d\ndropped=%d\nerrors=%d\nposts=%d\nlikes=%d\nreceived=%d\nparse_errors=%d\nreconnects=%d\n",
		c.sent.Load(),
		c.dropped.Load(),
		c.errors.Load(),
		c.posts.Load(),
		c.likes.Load(),
		c.received.Load(),
		c.parseErrs.Load(),
		c.reconnects.Load(),
	)
	if err := os.WriteFile(statsFilePath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "[consumer] failed to write stats file: %v\n", err)
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	jetstreamURL := flag.String("jetstream-url", "wss://jetstream2.us-east.bsky.network/subscribe",
		"Jetstream WebSocket base URL")
	nestURL := flag.String("nest-url", "http://localhost:8080",
		"Nest API base URL")
	apiKeyFlag := flag.String("api-key", "",
		"API key (if empty, reads from /tmp/nest_test_api_key.txt)")
	batchSize := flag.Int("batch-size", 10,
		"Number of items per batch submitted to Nest")
	flushInterval := flag.Duration("flush-interval", time.Second,
		"Maximum time between batch submissions")
	maxItems := flag.Int64("max-items", 0,
		"Stop after this many items have been processed (0 = unlimited)")
	postItemTypeIDFlag := flag.String("post-item-type-id", "",
		"Item type ID for posts (empty: read from /tmp/nest_test_item_types.txt)")
	likeItemTypeIDFlag := flag.String("like-item-type-id", "",
		"Item type ID for likes (empty: read from /tmp/nest_test_item_types.txt)")
	collectionsFlag := flag.String("collections", "app.bsky.feed.post,app.bsky.feed.like",
		"Comma-separated list of AT Protocol collections to subscribe to")
	flag.Parse()

	collections := strings.Split(*collectionsFlag, ",")

	// Resolve API key.
	apiKey := *apiKeyFlag
	if apiKey == "" {
		key, err := readFileContent("/tmp/nest_test_api_key.txt")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[consumer] api key: %v\n", err)
			os.Exit(1)
		}
		apiKey = key
	}

	// Resolve item type IDs.
	postItemTypeID := *postItemTypeIDFlag
	if postItemTypeID == "" {
		id, err := parseKeyValue("/tmp/nest_test_item_types.txt", "post")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[consumer] post item type id: %v\n", err)
			os.Exit(1)
		}
		postItemTypeID = id
	}

	likeItemTypeID := *likeItemTypeIDFlag
	if likeItemTypeID == "" {
		id, err := parseKeyValue("/tmp/nest_test_item_types.txt", "like")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[consumer] like item type id: %v\n", err)
			os.Exit(1)
		}
		likeItemTypeID = id
	}

	fmt.Printf("[consumer] jetstream=%s nest=%s batch-size=%d flush-interval=%v max-items=%d\n",
		*jetstreamURL, *nestURL, *batchSize, *flushInterval, *maxItems)

	// Context driven by SIGINT / SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\n[consumer] received %v, shutting down...\n", sig)
		cancel()
	}()

	stats := &counters{}
	itemsCh := make(chan itemPayload, 1000)
	client := &http.Client{Timeout: 30 * time.Second}

	// Cursor is stored atomically so the reconnection loop can resume from
	// the last processed event without a mutex.
	var cursor atomic.Int64

	// Periodic stats printer.
	statsTicker := time.NewTicker(statsPrintInterval)
	defer statsTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statsTicker.C:
				printStats(stats)
			}
		}
	}()

	// batchLoop runs until ctx is cancelled, then flushes remaining items.
	batchDone := make(chan struct{})
	go func() {
		defer close(batchDone)
		batchLoop(ctx, client, *nestURL, apiKey, itemsCh, *batchSize, *flushInterval, stats)
	}()

	// Outer reconnection loop — exits when ctx is cancelled.
	for ctx.Err() == nil {
		conn, err := connectWithBackoff(ctx, *jetstreamURL, collections, &cursor)
		if err != nil {
			// ctx was cancelled during dial.
			break
		}

		fmt.Fprintf(os.Stderr, "[consumer] connected to %s (cursor=%d)\n", *jetstreamURL, cursor.Load())

		err = readLoop(ctx, conn, postItemTypeID, likeItemTypeID, itemsCh, &cursor, stats, *maxItems)
		conn.Close(websocket.StatusNormalClosure, "reconnecting")

		if ctx.Err() != nil {
			break
		}

		if err == nil {
			// readLoop exited cleanly (e.g. maxItems reached).
			break
		}

		fmt.Fprintf(os.Stderr, "[consumer] read loop error: %v — reconnecting...\n", err)
		stats.reconnects.Add(1)
	}

	// Wait for the batch goroutine to flush all pending items.
	<-batchDone

	fmt.Printf("[consumer] DONE. Final: sent=%d dropped=%d posts=%d likes=%d received=%d reconnects=%d\n",
		stats.sent.Load(), stats.dropped.Load(), stats.posts.Load(), stats.likes.Load(),
		stats.received.Load(), stats.reconnects.Load())

	writeStatsFile(stats)
}
