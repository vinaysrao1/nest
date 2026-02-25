package service_test

import (
	"context"
	"testing"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
)

func TestCreate_ReturnsPlaintextOnce(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-apikey-plaintext")
	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	plaintext, apiKey, err := svc.Create(ctx, orgID, "my-key")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if plaintext == "" {
		t.Error("Create: plaintext key is empty")
	}
	if apiKey == nil {
		t.Fatal("Create: returned nil apiKey")
	}
	// The stored key must not expose the plaintext in KeyHash.
	if apiKey.KeyHash == plaintext {
		t.Error("Create: KeyHash equals plaintext — hash was not applied")
	}
	// KeyHash must be non-empty.
	if apiKey.KeyHash == "" {
		t.Error("Create: KeyHash is empty")
	}
	// Prefix must be the first 8 chars of plaintext.
	if apiKey.Prefix != plaintext[:8] {
		t.Errorf("Create: Prefix want %q, got %q", plaintext[:8], apiKey.Prefix)
	}
}

func TestCreate_HashMatchesAuth(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-apikey-hash")
	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	plaintext, apiKey, err := svc.Create(ctx, orgID, "hash-check-key")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	expectedHash := auth.HashAPIKey(plaintext)
	if apiKey.KeyHash != expectedHash {
		t.Errorf("Create: KeyHash want %q, got %q", expectedHash, apiKey.KeyHash)
	}
}

func TestCreate_ValidationError(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	_, _, err := svc.Create(ctx, "org1", "")
	if err == nil {
		t.Fatal("Create with empty name: expected ValidationError, got nil")
	}
	if _, ok := err.(*domain.ValidationError); !ok {
		t.Errorf("Create with empty name: expected *domain.ValidationError, got %T: %v", err, err)
	}
}

func TestRevoke_SetsRevokedAt(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-apikey-revoke")
	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	_, apiKey, err := svc.Create(ctx, orgID, "revoke-me")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Revoke(ctx, orgID, apiKey.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// List and verify the key is revoked.
	keys, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found *domain.ApiKey
	for i := range keys {
		if keys[i].ID == apiKey.ID {
			found = &keys[i]
			break
		}
	}
	if found == nil {
		t.Fatal("Revoke: key not found in list after revocation")
	}
	if found.RevokedAt == nil {
		t.Error("Revoke: RevokedAt is nil after revocation")
	}
}

func TestRevoke_NotFound(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	err := svc.Revoke(ctx, "org1", "nonexistent-key-id")
	if err == nil {
		t.Fatal("Revoke nonexistent key: expected NotFoundError, got nil")
	}
	if _, ok := err.(*domain.NotFoundError); !ok {
		t.Errorf("Revoke nonexistent key: expected *domain.NotFoundError, got %T: %v", err, err)
	}
}

func TestList_IncludesRevoked(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-apikey-list")
	svc := service.NewAPIKeyService(q, testLogger())
	ctx := context.Background()

	// Create two keys, revoke one.
	_, k1, err := svc.Create(ctx, orgID, "active-key")
	if err != nil {
		t.Fatalf("Create k1: %v", err)
	}
	_, k2, err := svc.Create(ctx, orgID, "revoked-key")
	if err != nil {
		t.Fatalf("Create k2: %v", err)
	}
	if err := svc.Revoke(ctx, orgID, k2.ID); err != nil {
		t.Fatalf("Revoke k2: %v", err)
	}

	keys, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	foundIDs := make(map[string]bool)
	for _, k := range keys {
		foundIDs[k.ID] = true
	}
	if !foundIDs[k1.ID] {
		t.Errorf("List: active key %s not found", k1.ID)
	}
	if !foundIDs[k2.ID] {
		t.Errorf("List: revoked key %s not found (should be included)", k2.ID)
	}
}
