package engine

import (
	"fmt"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/vinaysrao1/nest/internal/domain"
)

// enqueueUDF returns a Starlark built-in that inserts an MRT job for the
// current event into the named queue.
//
// Signature: enqueue(queue_name, reason="") -> bool
//
// Returns True on success, False if the queue does not exist or the insert
// fails. Errors are logged at WARN level rather than propagated so that a
// failing enqueue does not abort the rule evaluation.
//
// Queue IDs are resolved by name from the database and cached in the Pool's
// actionCache to avoid a database round-trip on every invocation.
func enqueueUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("enqueue", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var queueName string
		var reason string
		if err := starlark.UnpackArgs("enqueue", args, kwargs,
			"queue_name", &queueName,
			"reason?", &reason,
		); err != nil {
			return nil, err
		}

		if w.currentCtx == nil {
			return starlark.False, nil
		}

		queueID, ok := resolveQueueID(w, queueName)
		if !ok {
			return starlark.False, nil
		}

		job := buildMRTJob(w.currentCtx.event, queueName, queueID, reason)
		if err := w.pool.store.InsertMRTJob(w.currentCtx.ctx, &job); err != nil {
			w.logger.Warn("enqueue: insert mrt job failed",
				"queue", queueName,
				"org", w.currentCtx.event.OrgID,
				"error", err,
			)
			return starlark.False, nil
		}

		w.currentCtx.enqueuedJobs = append(w.currentCtx.enqueuedJobs, job)
		return starlark.True, nil
	})
}

// resolveQueueID looks up the MRT queue ID for queueName within the org.
// It first checks the Pool's actionCache, then queries the store.
// Returns the queue ID and true on success; false if not found.
func resolveQueueID(w *Worker, queueName string) (string, bool) {
	orgID := w.currentCtx.event.OrgID
	cacheKey := "queue:" + orgID + ":" + queueName

	if cached, ok := w.pool.actionCache.Get(cacheKey); ok {
		if queueID, ok := cached.(string); ok {
			return queueID, true
		}
	}

	queue, err := w.pool.store.GetMRTQueueByName(w.currentCtx.ctx, orgID, queueName)
	if err != nil {
		w.logger.Warn("enqueue: queue not found",
			"queue", queueName,
			"org", orgID,
			"error", err,
		)
		return "", false
	}

	w.pool.actionCache.Set(cacheKey, queue.ID)
	return queue.ID, true
}

// buildMRTJob constructs an MRT job for the given event targeting queueID.
// queueName is included in SourceInfo for traceability. reason is appended to
// the EnqueueSource so callers can identify the triggering rule condition.
func buildMRTJob(event domain.Event, queueName, queueID, reason string) domain.MRTJob {
	now := time.Now()
	enqueueSource := "rule"
	if reason != "" {
		enqueueSource = "rule:" + reason
	}
	// Sanitize item ID: AT Protocol URIs contain "://" and "/" which break URL routing.
	sanitized := strings.ReplaceAll(strings.ReplaceAll(event.ItemID, "://", "-"), "/", "-")
	return domain.MRTJob{
		ID:            fmt.Sprintf("mrt-%s-%d", sanitized, now.UnixNano()),
		OrgID:         event.OrgID,
		QueueID:       queueID,
		ItemID:        event.ItemID,
		ItemTypeID:    event.ItemTypeID,
		Payload:       event.Payload,
		Status:        domain.MRTJobStatusPending,
		PolicyIDs:     []string{},
		EnqueueSource: enqueueSource,
		SourceInfo:    map[string]any{"queue_name": queueName, "reason": reason},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
