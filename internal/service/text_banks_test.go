package service_test

import (
	"context"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

func TestTextBankCreate_Success(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-create")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "bad-words", "A bank of bad words")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if bank.ID == "" {
		t.Error("Create: returned bank has empty ID")
	}
	if bank.Name != "bad-words" {
		t.Errorf("Create: Name want %q, got %q", "bad-words", bank.Name)
	}
	if bank.OrgID != orgID {
		t.Errorf("Create: OrgID want %q, got %q", orgID, bank.OrgID)
	}
	if bank.Description != "A bank of bad words" {
		t.Errorf("Create: Description want %q, got %q", "A bank of bad words", bank.Description)
	}
}

func TestTextBankCreate_EmptyName(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	_, err := svc.Create(ctx, "org1", "", "description")
	if err == nil {
		t.Fatal("Create with empty name: expected ValidationError, got nil")
	}
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Errorf("Create with empty name: expected *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestTextBankAddEntry_Success(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-add-entry")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "words", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry, err := svc.AddEntry(ctx, orgID, bank.ID, "badword", false)
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if entry.ID == "" {
		t.Error("AddEntry: returned entry has empty ID")
	}
	if entry.TextBankID != bank.ID {
		t.Errorf("AddEntry: TextBankID want %q, got %q", bank.ID, entry.TextBankID)
	}
	if entry.Value != "badword" {
		t.Errorf("AddEntry: Value want %q, got %q", "badword", entry.Value)
	}
	if entry.IsRegex {
		t.Error("AddEntry: IsRegex should be false")
	}
}

func TestTextBankAddEntry_ValidRegex(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-regex-valid")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "patterns", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry, err := svc.AddEntry(ctx, orgID, bank.ID, `\b(foo|bar)\b`, true)
	if err != nil {
		t.Fatalf("AddEntry valid regex: %v", err)
	}
	if !entry.IsRegex {
		t.Error("AddEntry: IsRegex should be true")
	}
}

func TestTextBankAddEntry_InvalidRegex(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-regex-invalid")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "bad-patterns", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// "[unclosed" is an invalid regex.
	_, err = svc.AddEntry(ctx, orgID, bank.ID, "[unclosed", true)
	if err == nil {
		t.Fatal("AddEntry with invalid regex: expected ValidationError, got nil")
	}
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Errorf("AddEntry with invalid regex: expected *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestTextBankDeleteEntry_Success(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-delete-entry")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "delete-me-bank", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry, err := svc.AddEntry(ctx, orgID, bank.ID, "delete-me", false)
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	if err := svc.DeleteEntry(ctx, orgID, bank.ID, entry.ID); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	// Verify deletion: attempting to delete again should give NotFoundError.
	err = svc.DeleteEntry(ctx, orgID, bank.ID, entry.ID)
	if err == nil {
		t.Fatal("DeleteEntry second time: expected NotFoundError, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("DeleteEntry second time: expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

func TestTextBankDeleteEntry_WrongOrg(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-delete-wrong-org")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	bank, err := svc.Create(ctx, orgID, "my-bank", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry, err := svc.AddEntry(ctx, orgID, bank.ID, "value", false)
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	// Attempt to delete from a different org.
	err = svc.DeleteEntry(ctx, "different-org", bank.ID, entry.ID)
	if err == nil {
		t.Fatal("DeleteEntry wrong org: expected NotFoundError, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("DeleteEntry wrong org: expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

func TestTextBankGet_Success(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-tb-get")
	svc := service.NewTextBankService(q, testLogger())
	ctx := context.Background()

	created, err := svc.Create(ctx, orgID, "fetchable", "some description")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	fetched, err := svc.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("Get: ID mismatch: want %q, got %q", created.ID, fetched.ID)
	}
	if fetched.Name != created.Name {
		t.Errorf("Get: Name mismatch: want %q, got %q", created.Name, fetched.Name)
	}
}
