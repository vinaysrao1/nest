package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

// TestBuildMRTJob_UsesCorrectItemFields verifies that buildMRTJob maps
// event.ItemID and event.ItemTypeID to the job fields — not event.ID (the
// correlation ID) or event.ItemType (the type name string).
func TestBuildMRTJob_UsesCorrectItemFields(t *testing.T) {
	t.Parallel()

	event := domain.Event{
		ID:         "sub_12345",
		ItemType:   "post",
		ItemID:     "item-uuid-123",
		ItemTypeID: "type-uuid-456",
		OrgID:      "org-1",
		EventType:  "content",
		Timestamp:  time.Now(),
		Payload:    map[string]any{"text": "hello"},
	}

	job := buildMRTJob(event, "test-queue", "test-queue-id", "test reason")

	if job.ItemID != "item-uuid-123" {
		t.Errorf("ItemID = %q, want %q (must not be correlation ID %q)",
			job.ItemID, "item-uuid-123", event.ID)
	}

	if job.ItemTypeID != "type-uuid-456" {
		t.Errorf("ItemTypeID = %q, want %q (must not be type name %q)",
			job.ItemTypeID, "type-uuid-456", event.ItemType)
	}

	if job.QueueID != "test-queue-id" {
		t.Errorf("QueueID = %q, want %q", job.QueueID, "test-queue-id")
	}

	if !strings.Contains(job.EnqueueSource, "test reason") {
		t.Errorf("EnqueueSource = %q, want it to contain %q", job.EnqueueSource, "test reason")
	}

	if job.OrgID != "org-1" {
		t.Errorf("OrgID = %q, want %q", job.OrgID, "org-1")
	}

	if got, ok := job.Payload["text"]; !ok || got != "hello" {
		t.Errorf("Payload[\"text\"] = %v (ok=%v), want \"hello\"", got, ok)
	}
}
