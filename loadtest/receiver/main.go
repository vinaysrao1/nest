// Package main implements a lightweight webhook receiver for Nest integration testing.
// It counts incoming webhook POSTs from Nest's ActionPublisher and exposes stats via HTTP.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// webhookPayload mirrors the snake_case JSON body sent by Nest's ActionPublisher.
type webhookPayload struct {
	ItemID        string         `json:"item_id"`
	ItemTypeID    string         `json:"item_type_id"`
	OrgID         string         `json:"org_id"`
	CorrelationID string         `json:"correlation_id"`
	ActionName    string         `json:"action_name"`
	Payload       map[string]any `json:"payload"`
}

// statsResponse is the JSON body returned by GET /stats.
type statsResponse struct {
	Total    int64          `json:"total"`
	ByAction map[string]int64 `json:"by_action"`
	Errors   int64          `json:"errors"`
}

// receiver holds all shared counters and state for the webhook receiver.
type receiver struct {
	total      atomic.Int64
	parseErrs  atomic.Int64
	byAction   sync.Map // string -> *atomic.Int64
}

// newReceiver allocates a ready-to-use receiver.
func newReceiver() *receiver {
	return &receiver{}
}

// incrementAction atomically increments the counter for the given action name.
func (r *receiver) incrementAction(name string) {
	actual, _ := r.byAction.LoadOrStore(name, &atomic.Int64{})
	actual.(*atomic.Int64).Add(1)
}

// handleWebhook decodes the incoming POST body, increments counters, and replies 200.
func (r *receiver) handleWebhook(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload webhookPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		r.parseErrs.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	r.total.Add(1)
	if payload.ActionName != "" {
		r.incrementAction(payload.ActionName)
	}

	w.WriteHeader(http.StatusOK)
}

// handleStats serialises current counters as JSON and writes them to the response.
func (r *receiver) handleStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	byAction := make(map[string]int64)
	r.byAction.Range(func(k, v any) bool {
		byAction[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})

	resp := statsResponse{
		Total:    r.total.Load(),
		ByAction: byAction,
		Errors:   r.parseErrs.Load(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[receiver] stats encode error: %v", err)
	}
}

// logPeriodically writes a one-line total count every interval until ctx is cancelled.
func logPeriodically(ctx context.Context, r *receiver, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[receiver] total=%d", r.total.Load())
		}
	}
}

func main() {
	addr := flag.String("addr", ":9090", "listen address for the webhook receiver")
	flag.Parse()

	rec := newReceiver()

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", rec.handleWebhook)
	mux.HandleFunc("/stats", rec.handleStats)

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go logPeriodically(ctx, rec, 10*time.Second)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[receiver] listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[receiver] server error: %v", err)
		}
	}()

	<-sigCh
	log.Printf("[receiver] shutting down (total=%d)", rec.total.Load())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[receiver] shutdown error: %v", err)
	}
}
