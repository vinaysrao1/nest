package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// counterTableDDL is the CREATE TABLE IF NOT EXISTS statement for the optional counter_state table.
const counterTableDDL = `
CREATE TABLE IF NOT EXISTS counter_state (
    org_id          TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    window_seconds  INTEGER NOT NULL,
    count           BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, entity_id, event_type, window_seconds)
)`

const incrementCounterSQL = `
INSERT INTO counter_state (org_id, entity_id, event_type, window_seconds, count, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (org_id, entity_id, event_type, window_seconds)
DO UPDATE SET count = counter_state.count + $5, updated_at = now()`

const getCounterSumSQL = `
SELECT COALESCE(count, 0)
FROM counter_state
WHERE org_id = $1 AND entity_id = $2 AND event_type = $3 AND window_seconds = $4`

// counterTableCreated is an atomic flag indicating whether the DDL has run successfully.
// Uses int32 via atomic operations (0=not created, 1=created).
var counterTableCreated int32

// counterTableMu guards the creation attempt so only one goroutine runs the DDL.
var counterTableMu sync.Mutex

// ensureCounterTable creates the counter_state table if it does not already exist.
// On success the table is created at most once per process; on failure the next
// call will retry, so transient errors do not permanently prevent counter use.
func (q *Queries) ensureCounterTable(ctx context.Context) error {
	if atomic.LoadInt32(&counterTableCreated) == 1 {
		return nil
	}
	counterTableMu.Lock()
	defer counterTableMu.Unlock()
	// Double-check after acquiring lock.
	if atomic.LoadInt32(&counterTableCreated) == 1 {
		return nil
	}
	if _, err := q.dbtx.Exec(ctx, counterTableDDL); err != nil {
		return fmt.Errorf("create counter_state table: %w", err)
	}
	atomic.StoreInt32(&counterTableCreated, 1)
	return nil
}

// IncrementCounter atomically increments a counter value using INSERT ON CONFLICT DO UPDATE.
// The counter_state table is created lazily on the first call.
//
// Pre-conditions: window > 0, count > 0. orgID, entityID, eventType must be non-empty.
// Post-conditions: the counter for (orgID, entityID, eventType, window) is incremented by count.
// Raises: error on database failure.
func (q *Queries) IncrementCounter(
	ctx context.Context,
	orgID, entityID, eventType string,
	window int,
	count int64,
) error {
	if err := q.ensureCounterTable(ctx); err != nil {
		return fmt.Errorf("ensure counter table: %w", err)
	}
	_, err := q.dbtx.Exec(ctx, incrementCounterSQL, orgID, entityID, eventType, window, count)
	if err != nil {
		return fmt.Errorf("increment counter: %w", err)
	}
	return nil
}

// GetCounterSum returns the current counter value for a given key and window.
// Returns 0 if no counter exists (not an error).
//
// Pre-conditions: orgID, entityID, eventType must be non-empty. window > 0.
// Post-conditions: returns the accumulated count, or 0 if the counter does not exist.
// Raises: error on database failure (not on missing row).
func (q *Queries) GetCounterSum(
	ctx context.Context,
	orgID, entityID, eventType string,
	window int,
) (int64, error) {
	if err := q.ensureCounterTable(ctx); err != nil {
		return 0, fmt.Errorf("ensure counter table: %w", err)
	}

	var sum int64
	err := q.dbtx.QueryRow(ctx, getCounterSumSQL, orgID, entityID, eventType, window).Scan(&sum)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("get counter sum: %w", err)
	}
	return sum, nil
}
