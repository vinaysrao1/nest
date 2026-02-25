package worker_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/vinaysrao1/nest/internal/worker"
)

// TestSnapshotRebuildArgs_Kind verifies that SnapshotRebuildArgs implements
// river.JobArgs and Kind returns "snapshot_rebuild".
func TestSnapshotRebuildArgs_Kind(t *testing.T) {
	args := worker.SnapshotRebuildArgs{}

	// Compile-time interface check.
	var _ river.JobArgs = args

	if got := args.Kind(); got != "snapshot_rebuild" {
		t.Errorf("SnapshotRebuildArgs.Kind() = %q, want %q", got, "snapshot_rebuild")
	}
}

// TestPartitionManagerArgs_Kind verifies that PartitionManagerArgs implements
// river.JobArgs and Kind returns "partition_manager".
func TestPartitionManagerArgs_Kind(t *testing.T) {
	args := worker.PartitionManagerArgs{}

	// Compile-time interface check.
	var _ river.JobArgs = args

	if got := args.Kind(); got != "partition_manager" {
		t.Errorf("PartitionManagerArgs.Kind() = %q, want %q", got, "partition_manager")
	}
}

// TestSessionCleanupArgs_Kind verifies that SessionCleanupArgs implements
// river.JobArgs and Kind returns "session_cleanup".
func TestSessionCleanupArgs_Kind(t *testing.T) {
	args := worker.SessionCleanupArgs{}

	// Compile-time interface check.
	var _ river.JobArgs = args

	if got := args.Kind(); got != "session_cleanup" {
		t.Errorf("SessionCleanupArgs.Kind() = %q, want %q", got, "session_cleanup")
	}
}

// TestNewSnapshotRebuildWorker_NilLogger verifies that NewSnapshotRebuildWorker
// handles a nil logger without panicking.
func TestNewSnapshotRebuildWorker_NilLogger(t *testing.T) {
	w := worker.NewSnapshotRebuildWorker(nil, nil, nil)
	if w == nil {
		t.Fatal("NewSnapshotRebuildWorker returned nil")
	}
}

// TestNewPartitionManagerWorker_NilLogger verifies that NewPartitionManagerWorker
// handles a nil logger without panicking.
func TestNewPartitionManagerWorker_NilLogger(t *testing.T) {
	w := worker.NewPartitionManagerWorker(nil, nil)
	if w == nil {
		t.Fatal("NewPartitionManagerWorker returned nil")
	}
}

// TestNewSessionCleanupWorker_NilLogger verifies that NewSessionCleanupWorker
// handles a nil logger without panicking.
func TestNewSessionCleanupWorker_NilLogger(t *testing.T) {
	w := worker.NewSessionCleanupWorker(nil, nil)
	if w == nil {
		t.Fatal("NewSessionCleanupWorker returned nil")
	}
}

// TestMaintenanceArgs_UniqueKinds verifies that each maintenance args type has a
// distinct Kind string (preventing river registration conflicts).
func TestMaintenanceArgs_UniqueKinds(t *testing.T) {
	kinds := []string{
		worker.SnapshotRebuildArgs{}.Kind(),
		worker.PartitionManagerArgs{}.Kind(),
		worker.SessionCleanupArgs{}.Kind(),
	}

	seen := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate job kind: %q", k)
		}
		seen[k] = true
	}
}

// TestAllArgs_DifferentFromProcessItem verifies that none of the maintenance
// args types use the same Kind as ProcessItemArgs (which would cause conflicts
// if all workers are registered with the same river client).
func TestAllArgs_DifferentFromProcessItem(t *testing.T) {
	processItemKind := worker.ProcessItemArgs{}.Kind()

	maintenanceKinds := []string{
		worker.SnapshotRebuildArgs{}.Kind(),
		worker.PartitionManagerArgs{}.Kind(),
		worker.SessionCleanupArgs{}.Kind(),
	}

	for _, k := range maintenanceKinds {
		if k == processItemKind {
			t.Errorf("maintenance kind %q conflicts with ProcessItemArgs kind %q", k, processItemKind)
		}
	}
}

// makeJobRow is a test helper that builds a minimal *rivertype.JobRow for use
// in constructing river.Job values without a real database.
func makeJobRow(kind string) *rivertype.JobRow {
	return &rivertype.JobRow{
		ID:        1,
		Attempt:   1,
		CreatedAt: time.Now(),
		Kind:      kind,
		State:     rivertype.JobStateRunning,
	}
}

// TestSnapshotRebuildJob_ArgsRoundTrip verifies that a river.Job constructed
// with SnapshotRebuildArgs carries the correct Kind through its JobRow, matching
// what SnapshotRebuildWorker.Work would receive from the river runtime.
func TestSnapshotRebuildJob_ArgsRoundTrip(t *testing.T) {
	t.Parallel()

	args := worker.SnapshotRebuildArgs{}
	job := &river.Job[worker.SnapshotRebuildArgs]{
		JobRow: makeJobRow(args.Kind()),
		Args:   args,
	}

	if job.Kind != "snapshot_rebuild" {
		t.Errorf("JobRow.Kind = %q, want %q", job.Kind, "snapshot_rebuild")
	}
	if job.Args.Kind() != "snapshot_rebuild" {
		t.Errorf("job.Args.Kind() = %q, want %q", job.Args.Kind(), "snapshot_rebuild")
	}
}

// TestPartitionManagerJob_ArgsRoundTrip verifies that a river.Job constructed
// with PartitionManagerArgs carries the correct Kind through its JobRow.
func TestPartitionManagerJob_ArgsRoundTrip(t *testing.T) {
	t.Parallel()

	args := worker.PartitionManagerArgs{}
	job := &river.Job[worker.PartitionManagerArgs]{
		JobRow: makeJobRow(args.Kind()),
		Args:   args,
	}

	if job.Kind != "partition_manager" {
		t.Errorf("JobRow.Kind = %q, want %q", job.Kind, "partition_manager")
	}
	if job.Args.Kind() != "partition_manager" {
		t.Errorf("job.Args.Kind() = %q, want %q", job.Args.Kind(), "partition_manager")
	}
}

// TestSessionCleanupJob_ArgsRoundTrip verifies that a river.Job constructed
// with SessionCleanupArgs carries the correct Kind through its JobRow.
func TestSessionCleanupJob_ArgsRoundTrip(t *testing.T) {
	t.Parallel()

	args := worker.SessionCleanupArgs{}
	job := &river.Job[worker.SessionCleanupArgs]{
		JobRow: makeJobRow(args.Kind()),
		Args:   args,
	}

	if job.Kind != "session_cleanup" {
		t.Errorf("JobRow.Kind = %q, want %q", job.Kind, "session_cleanup")
	}
	if job.Args.Kind() != "session_cleanup" {
		t.Errorf("job.Args.Kind() = %q, want %q", job.Args.Kind(), "session_cleanup")
	}
}

// TestSnapshotRebuildWorker_WorkerDefaults verifies that SnapshotRebuildWorker
// embeds river.WorkerDefaults, giving it default timeout and max attempts
// behaviour without a manual implementation.
func TestSnapshotRebuildWorker_WorkerDefaults(t *testing.T) {
	t.Parallel()

	// river.WorkerDefaults embeds correctly if the worker satisfies the
	// river.Worker interface for the correct args type. This is a compile-time
	// guarantee checked here at test time via a type assertion.
	w := worker.NewSnapshotRebuildWorker(nil, nil, nil)
	var _ river.Worker[worker.SnapshotRebuildArgs] = w
}

// TestPartitionManagerWorker_WorkerDefaults verifies that PartitionManagerWorker
// satisfies the river.Worker interface for PartitionManagerArgs.
func TestPartitionManagerWorker_WorkerDefaults(t *testing.T) {
	t.Parallel()

	w := worker.NewPartitionManagerWorker(nil, nil)
	var _ river.Worker[worker.PartitionManagerArgs] = w
}

// TestSessionCleanupWorker_WorkerDefaults verifies that SessionCleanupWorker
// satisfies the river.Worker interface for SessionCleanupArgs.
func TestSessionCleanupWorker_WorkerDefaults(t *testing.T) {
	t.Parallel()

	w := worker.NewSessionCleanupWorker(nil, nil)
	var _ river.Worker[worker.SessionCleanupArgs] = w
}
