package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/store"
)

// SnapshotRebuildArgs are the river periodic job arguments for snapshot rebuild.
// Implements river.JobArgs.
type SnapshotRebuildArgs struct{}

// Kind returns the river job kind identifier.
func (SnapshotRebuildArgs) Kind() string { return "snapshot_rebuild" }

// PartitionManagerArgs are the river periodic job arguments for partition management.
// Implements river.JobArgs.
type PartitionManagerArgs struct{}

// Kind returns the river job kind identifier.
func (PartitionManagerArgs) Kind() string { return "partition_manager" }

// SessionCleanupArgs are the river periodic job arguments for session cleanup.
// Implements river.JobArgs.
type SessionCleanupArgs struct{}

// Kind returns the river job kind identifier.
func (SessionCleanupArgs) Kind() string { return "session_cleanup" }

// SnapshotRebuildWorker rebuilds engine snapshots for all orgs.
// It lists all distinct org IDs from the rules table and calls
// ruleService.RebuildSnapshot for each one.
type SnapshotRebuildWorker struct {
	river.WorkerDefaults[SnapshotRebuildArgs]

	ruleService *service.RuleService
	store       *store.Queries
	logger      *slog.Logger
}

// NewSnapshotRebuildWorker creates a SnapshotRebuildWorker.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned worker is ready for river registration.
func NewSnapshotRebuildWorker(
	ruleService *service.RuleService,
	st *store.Queries,
	logger *slog.Logger,
) *SnapshotRebuildWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SnapshotRebuildWorker{
		ruleService: ruleService,
		store:       st,
		logger:      logger,
	}
}

// Work rebuilds snapshots for all orgs by listing distinct org IDs from the
// rules table and calling ruleService.RebuildSnapshot for each.
//
// Pre-conditions: none.
// Post-conditions: snapshots are rebuilt for all orgs with at least one rule.
// Raises: error if ListDistinctOrgIDs fails (river will retry).
func (w *SnapshotRebuildWorker) Work(ctx context.Context, job *river.Job[SnapshotRebuildArgs]) error {
	orgIDs, err := w.store.ListDistinctOrgIDs(ctx)
	if err != nil {
		return fmt.Errorf("snapshot_rebuild: list org IDs: %w", err)
	}

	w.logger.Info("worker.snapshot_rebuild: rebuilding snapshots",
		"org_count", len(orgIDs),
	)

	var rebuildErrors int
	for _, orgID := range orgIDs {
		if err := w.ruleService.RebuildSnapshot(ctx, orgID); err != nil {
			w.logger.Error("worker.snapshot_rebuild: failed to rebuild snapshot",
				"org_id", orgID,
				"error", err,
			)
			rebuildErrors++
		}
	}

	if rebuildErrors > 0 {
		w.logger.Warn("worker.snapshot_rebuild: some snapshots failed to rebuild",
			"failed", rebuildErrors,
			"total", len(orgIDs),
		)
	}

	return nil
}

// PartitionManagerWorker creates future execution log partitions.
// It creates the next month's partitions for rule_executions and action_executions.
type PartitionManagerWorker struct {
	river.WorkerDefaults[PartitionManagerArgs]

	store  *store.Queries
	logger *slog.Logger
}

// NewPartitionManagerWorker creates a PartitionManagerWorker.
//
// Pre-conditions: st must be non-nil.
// Post-conditions: returned worker is ready for river registration.
func NewPartitionManagerWorker(
	st *store.Queries,
	logger *slog.Logger,
) *PartitionManagerWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &PartitionManagerWorker{
		store:  st,
		logger: logger,
	}
}

// Work creates next month's partitions for rule_executions and action_executions.
//
// Pre-conditions: none.
// Post-conditions: partitions exist for next month.
// Raises: error on DDL failure (river will retry).
func (w *PartitionManagerWorker) Work(ctx context.Context, job *river.Job[PartitionManagerArgs]) error {
	next := time.Now().AddDate(0, 1, 0)
	year, month, _ := next.Date()

	w.logger.Info("worker.partition_manager: creating partitions",
		"year", year,
		"month", int(month),
	)

	if err := w.store.CreatePartitionsForMonth(ctx, year, int(month)); err != nil {
		return fmt.Errorf("partition_manager: create partitions for %d-%02d: %w", year, int(month), err)
	}

	w.logger.Info("worker.partition_manager: partitions created",
		"year", year,
		"month", int(month),
	)
	return nil
}

// SessionCleanupWorker deletes expired sessions.
type SessionCleanupWorker struct {
	river.WorkerDefaults[SessionCleanupArgs]

	store  *store.Queries
	logger *slog.Logger
}

// NewSessionCleanupWorker creates a SessionCleanupWorker.
//
// Pre-conditions: st must be non-nil.
// Post-conditions: returned worker is ready for river registration.
func NewSessionCleanupWorker(
	st *store.Queries,
	logger *slog.Logger,
) *SessionCleanupWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionCleanupWorker{
		store:  st,
		logger: logger,
	}
}

// Work calls store.CleanExpiredSessions and logs the count.
//
// Pre-conditions: none.
// Post-conditions: expired sessions are removed from the database.
// Raises: error on database failure (river will retry).
func (w *SessionCleanupWorker) Work(ctx context.Context, job *river.Job[SessionCleanupArgs]) error {
	count, err := w.store.CleanExpiredSessions(ctx)
	if err != nil {
		return fmt.Errorf("session_cleanup: %w", err)
	}

	w.logger.Info("worker.session_cleanup: cleaned expired sessions",
		"count", count,
	)
	return nil
}

// RegisterMaintenanceJobs registers all periodic maintenance workers with the
// river client. It also registers the worker implementations so the client
// knows how to execute each job kind.
//
// Schedules:
//   - Snapshot rebuild: every 5 minutes
//   - Partition manager: every 24 hours
//   - Session cleanup: every 1 hour
//
// Pre-conditions: client must be non-nil and started.
// Post-conditions: all periodic jobs are registered and will fire on schedule.
func RegisterMaintenanceJobs(
	client *river.Client[pgx.Tx],
	ruleService *service.RuleService,
	st *store.Queries,
	logger *slog.Logger,
) {
	periodicJobs := client.PeriodicJobs()

	periodicJobs.Add(river.NewPeriodicJob(
		river.PeriodicInterval(5*time.Minute),
		func() (river.JobArgs, *river.InsertOpts) {
			return SnapshotRebuildArgs{}, nil
		},
		&river.PeriodicJobOpts{
			ID:         "snapshot_rebuild",
			RunOnStart: true,
		},
	))

	periodicJobs.Add(river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return PartitionManagerArgs{}, nil
		},
		&river.PeriodicJobOpts{
			ID:         "partition_manager",
			RunOnStart: true,
		},
	))

	periodicJobs.Add(river.NewPeriodicJob(
		river.PeriodicInterval(1*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return SessionCleanupArgs{}, nil
		},
		&river.PeriodicJobOpts{
			ID:         "session_cleanup",
			RunOnStart: false,
		},
	))

	if logger != nil {
		logger.Info("worker: registered maintenance periodic jobs",
			"jobs", []string{"snapshot_rebuild", "partition_manager", "session_cleanup"},
		)
	}
}
