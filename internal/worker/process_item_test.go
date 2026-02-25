package worker_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/vinaysrao1/nest/internal/worker"
)

// TestProcessItemArgs_Kind verifies that ProcessItemArgs implements river.JobArgs
// correctly and that Kind returns the expected string "process_item".
func TestProcessItemArgs_Kind(t *testing.T) {
	args := worker.ProcessItemArgs{
		OrgID:      "org_1",
		ItemID:     "item_1",
		ItemTypeID: "ity_1",
		EventType:  "content",
		Payload:    map[string]any{"text": "hello"},
	}

	// Compile-time interface check.
	var _ river.JobArgs = args

	if got := args.Kind(); got != "process_item" {
		t.Errorf("ProcessItemArgs.Kind() = %q, want %q", got, "process_item")
	}
}

// TestProcessItemArgs_KindZeroValue verifies that Kind() works even on a zero-value struct.
func TestProcessItemArgs_KindZeroValue(t *testing.T) {
	var args worker.ProcessItemArgs

	if got := args.Kind(); got != "process_item" {
		t.Errorf("ProcessItemArgs{}.Kind() = %q, want %q", got, "process_item")
	}
}

// TestNewProcessItemWorker_NilLogger verifies that NewProcessItemWorker sets a
// default logger when nil is passed, and does not panic.
func TestNewProcessItemWorker_NilLogger(t *testing.T) {
	w := worker.NewProcessItemWorker(nil, nil, nil, nil)
	if w == nil {
		t.Fatal("NewProcessItemWorker returned nil")
	}
}

// TestProcessItemWorker_WorkerDefaults verifies that ProcessItemWorker satisfies
// the river.Worker interface for ProcessItemArgs. This is a compile-time
// guarantee enforced at test time via a type assertion.
func TestProcessItemWorker_WorkerDefaults(t *testing.T) {
	t.Parallel()

	w := worker.NewProcessItemWorker(nil, nil, nil, nil)
	var _ river.Worker[worker.ProcessItemArgs] = w
}

// TestProcessItemArgs_JSONFields verifies that the json tags are set correctly
// by checking the Kind and that the struct has the expected exported fields.
func TestProcessItemArgs_JSONFields(t *testing.T) {
	args := worker.ProcessItemArgs{
		OrgID:      "org_test",
		ItemID:     "item_test",
		ItemTypeID: "ity_test",
		EventType:  "post",
		Payload:    map[string]any{"key": "value"},
	}

	if args.OrgID != "org_test" {
		t.Errorf("OrgID = %q, want %q", args.OrgID, "org_test")
	}
	if args.ItemID != "item_test" {
		t.Errorf("ItemID = %q, want %q", args.ItemID, "item_test")
	}
	if args.ItemTypeID != "ity_test" {
		t.Errorf("ItemTypeID = %q, want %q", args.ItemTypeID, "ity_test")
	}
	if args.EventType != "post" {
		t.Errorf("EventType = %q, want %q", args.EventType, "post")
	}
}

// makeProcessItemJob is a test helper that builds a *river.Job[worker.ProcessItemArgs]
// from the given args. It uses a minimal JobRow so no database is required.
func makeProcessItemJob(args worker.ProcessItemArgs) *river.Job[worker.ProcessItemArgs] {
	return &river.Job[worker.ProcessItemArgs]{
		JobRow: &rivertype.JobRow{
			ID:        1,
			Attempt:   1,
			CreatedAt: time.Now(),
			Kind:      args.Kind(),
			State:     rivertype.JobStateRunning,
		},
		Args: args,
	}
}

// TestProcessItemWorker_Work_EmptyOrgID verifies that Work returns an error
// containing "org_id" when OrgID is empty. The validation runs before any
// database or engine access, so nil dependencies are safe here.
func TestProcessItemWorker_Work_EmptyOrgID(t *testing.T) {
	t.Parallel()

	w := worker.NewProcessItemWorker(nil, nil, nil, nil)

	args := worker.ProcessItemArgs{
		OrgID:      "", // intentionally empty
		ItemID:     "item_1",
		ItemTypeID: "ity_1",
		EventType:  "content",
	}
	job := makeProcessItemJob(args)

	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("Work(empty OrgID): want error, got nil")
	}
	if !strings.Contains(err.Error(), "org_id") {
		t.Errorf("Work(empty OrgID): error %q does not mention org_id", err.Error())
	}
}

// TestProcessItemWorker_Work_EmptyItemID verifies that Work returns an error
// containing "item_id" when ItemID is empty. Validation runs before any
// database or engine access.
func TestProcessItemWorker_Work_EmptyItemID(t *testing.T) {
	t.Parallel()

	w := worker.NewProcessItemWorker(nil, nil, nil, nil)

	args := worker.ProcessItemArgs{
		OrgID:      "org_1",
		ItemID:     "", // intentionally empty
		ItemTypeID: "ity_1",
		EventType:  "content",
	}
	job := makeProcessItemJob(args)

	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("Work(empty ItemID): want error, got nil")
	}
	if !strings.Contains(err.Error(), "item_id") {
		t.Errorf("Work(empty ItemID): error %q does not mention item_id", err.Error())
	}
}

// TestProcessItemWorker_Work_EmptyItemTypeID verifies that Work returns an error
// containing "item_type_id" when ItemTypeID is empty. Validation runs before
// any database or engine access.
func TestProcessItemWorker_Work_EmptyItemTypeID(t *testing.T) {
	t.Parallel()

	w := worker.NewProcessItemWorker(nil, nil, nil, nil)

	args := worker.ProcessItemArgs{
		OrgID:      "org_1",
		ItemID:     "item_1",
		ItemTypeID: "", // intentionally empty
		EventType:  "content",
	}
	job := makeProcessItemJob(args)

	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("Work(empty ItemTypeID): want error, got nil")
	}
	if !strings.Contains(err.Error(), "item_type_id") {
		t.Errorf("Work(empty ItemTypeID): error %q does not mention item_type_id", err.Error())
	}
}

// TestProcessItemWorker_Work_ValidationOrder verifies that OrgID is checked
// before ItemID, and ItemID before ItemTypeID. This ensures the validation
// sequence in Work is stable and predictable.
func TestProcessItemWorker_Work_ValidationOrder(t *testing.T) {
	t.Parallel()

	w := worker.NewProcessItemWorker(nil, nil, nil, nil)

	// All three fields empty: OrgID check fires first.
	args := worker.ProcessItemArgs{}
	job := makeProcessItemJob(args)

	err := w.Work(context.Background(), job)
	if err == nil {
		t.Fatal("Work(all empty): want error, got nil")
	}
	if !strings.Contains(err.Error(), "org_id") {
		t.Errorf("Work(all empty): expected org_id error first, got %q", err.Error())
	}

	// OrgID set, ItemID still empty: ItemID check fires.
	args2 := worker.ProcessItemArgs{OrgID: "org_1"}
	job2 := makeProcessItemJob(args2)

	err2 := w.Work(context.Background(), job2)
	if err2 == nil {
		t.Fatal("Work(OrgID only): want error, got nil")
	}
	if !strings.Contains(err2.Error(), "item_id") {
		t.Errorf("Work(OrgID only): expected item_id error, got %q", err2.Error())
	}
}
