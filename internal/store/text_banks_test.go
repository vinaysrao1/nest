package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestTextBanks_CRUD(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "text-banks-crud-org")

	t.Run("create and get text bank", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Microsecond)
		bank := &domain.TextBank{
			ID:          "tb-001",
			OrgID:       orgID,
			Name:        "banned-words",
			Description: "A list of banned words",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := q.CreateTextBank(ctx, bank); err != nil {
			t.Fatalf("CreateTextBank: %v", err)
		}

		got, err := q.GetTextBank(ctx, orgID, bank.ID)
		if err != nil {
			t.Fatalf("GetTextBank: %v", err)
		}

		if got.ID != bank.ID {
			t.Errorf("ID: got %q, want %q", got.ID, bank.ID)
		}
		if got.OrgID != bank.OrgID {
			t.Errorf("OrgID: got %q, want %q", got.OrgID, bank.OrgID)
		}
		if got.Name != bank.Name {
			t.Errorf("Name: got %q, want %q", got.Name, bank.Name)
		}
		if got.Description != bank.Description {
			t.Errorf("Description: got %q, want %q", got.Description, bank.Description)
		}
	})

	t.Run("list text banks", func(t *testing.T) {
		orgID2 := seedOrg(t, q, "list-text-banks-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		for _, name := range []string{"zebra-words", "alpha-words", "middle-words"} {
			b := &domain.TextBank{
				ID:        "tb-list-" + name,
				OrgID:     orgID2,
				Name:      name,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := q.CreateTextBank(ctx, b); err != nil {
				t.Fatalf("CreateTextBank(%s): %v", name, err)
			}
		}

		banks, err := q.ListTextBanks(ctx, orgID2)
		if err != nil {
			t.Fatalf("ListTextBanks: %v", err)
		}
		if len(banks) != 3 {
			t.Fatalf("expected 3 text banks, got %d", len(banks))
		}
		// Verify ordered by name ASC
		if banks[0].Name != "alpha-words" {
			t.Errorf("first bank name: got %q, want %q", banks[0].Name, "alpha-words")
		}
		if banks[2].Name != "zebra-words" {
			t.Errorf("last bank name: got %q, want %q", banks[2].Name, "zebra-words")
		}
	})

	t.Run("get non-existent text bank returns NotFoundError", func(t *testing.T) {
		_, err := q.GetTextBank(ctx, orgID, "no-such-bank")
		if err == nil {
			t.Fatal("expected NotFoundError, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestTextBankEntries(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "text-bank-entries-org")

	createBank := func(t *testing.T, id, name string) *domain.TextBank {
		t.Helper()
		now := time.Now().UTC().Truncate(time.Microsecond)
		bank := &domain.TextBank{
			ID:        id,
			OrgID:     orgID,
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := q.CreateTextBank(ctx, bank); err != nil {
			t.Fatalf("CreateTextBank(%s): %v", name, err)
		}
		return bank
	}

	t.Run("add and get text bank entries", func(t *testing.T) {
		bank := createBank(t, "tbe-bank-001", "profanity-list")
		now := time.Now().UTC().Truncate(time.Microsecond)

		entries := []*domain.TextBankEntry{
			{ID: "tbe-001", TextBankID: bank.ID, Value: "badword1", IsRegex: false, CreatedAt: now},
			{ID: "tbe-002", TextBankID: bank.ID, Value: `bad.*word`, IsRegex: true, CreatedAt: now.Add(time.Millisecond)},
		}
		for _, e := range entries {
			if err := q.AddTextBankEntry(ctx, orgID, e); err != nil {
				t.Fatalf("AddTextBankEntry(%s): %v", e.ID, err)
			}
		}

		got, err := q.GetTextBankEntries(ctx, orgID, bank.ID)
		if err != nil {
			t.Fatalf("GetTextBankEntries: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		// Verify ordered by created_at ASC
		if got[0].ID != "tbe-001" {
			t.Errorf("first entry ID: got %q, want %q", got[0].ID, "tbe-001")
		}
		if got[0].Value != "badword1" {
			t.Errorf("first entry Value: got %q, want %q", got[0].Value, "badword1")
		}
		if got[0].IsRegex {
			t.Errorf("first entry IsRegex: got true, want false")
		}
		if got[1].ID != "tbe-002" {
			t.Errorf("second entry ID: got %q, want %q", got[1].ID, "tbe-002")
		}
		if !got[1].IsRegex {
			t.Errorf("second entry IsRegex: got false, want true")
		}
	})

	t.Run("delete text bank entry", func(t *testing.T) {
		bank := createBank(t, "tbe-bank-002", "spam-list")
		now := time.Now().UTC().Truncate(time.Microsecond)

		entry := &domain.TextBankEntry{
			ID:         "tbe-to-delete",
			TextBankID: bank.ID,
			Value:      "spam",
			IsRegex:    false,
			CreatedAt:  now,
		}
		if err := q.AddTextBankEntry(ctx, orgID, entry); err != nil {
			t.Fatalf("AddTextBankEntry: %v", err)
		}

		if err := q.DeleteTextBankEntry(ctx, orgID, bank.ID, entry.ID); err != nil {
			t.Fatalf("DeleteTextBankEntry: %v", err)
		}

		entries, err := q.GetTextBankEntries(ctx, orgID, bank.ID)
		if err != nil {
			t.Fatalf("GetTextBankEntries after delete: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries after delete, got %d", len(entries))
		}
	})

	t.Run("add entry with wrong org returns NotFoundError", func(t *testing.T) {
		bank := createBank(t, "tbe-bank-003", "org-isolation-bank")
		otherOrgID := seedOrg(t, q, "other-text-bank-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		entry := &domain.TextBankEntry{
			ID:         "tbe-wrong-org",
			TextBankID: bank.ID,
			Value:      "some text",
			IsRegex:    false,
			CreatedAt:  now,
		}

		// Use otherOrgID but bank belongs to orgID
		err := q.AddTextBankEntry(ctx, otherOrgID, entry)
		if err == nil {
			t.Fatal("expected NotFoundError for wrong org, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("delete entry with wrong org returns NotFoundError", func(t *testing.T) {
		bank := createBank(t, "tbe-bank-004", "delete-iso-bank")
		otherOrgID := seedOrg(t, q, "delete-iso-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		entry := &domain.TextBankEntry{
			ID:         "tbe-del-iso",
			TextBankID: bank.ID,
			Value:      "value",
			IsRegex:    false,
			CreatedAt:  now,
		}
		if err := q.AddTextBankEntry(ctx, orgID, entry); err != nil {
			t.Fatalf("AddTextBankEntry: %v", err)
		}

		// Attempt delete from wrong org
		err := q.DeleteTextBankEntry(ctx, otherOrgID, bank.ID, entry.ID)
		if err == nil {
			t.Fatal("expected NotFoundError for wrong org delete, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("get entries returns empty slice for empty bank", func(t *testing.T) {
		bank := createBank(t, "tbe-bank-005", "empty-bank")

		entries, err := q.GetTextBankEntries(ctx, orgID, bank.ID)
		if err != nil {
			t.Fatalf("GetTextBankEntries: %v", err)
		}
		if entries == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})
}
