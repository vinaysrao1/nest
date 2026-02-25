package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

func TestSigningKeys(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	orgID := seedOrg(t, q, "signing-keys-org")

	t.Run("create and list signing keys", func(t *testing.T) {
		key1 := domain.SigningKey{
			ID:         "sk-001",
			OrgID:      orgID,
			PublicKey:  "-----BEGIN PUBLIC KEY-----\npubkey1\n-----END PUBLIC KEY-----",
			PrivateKey: "-----BEGIN PRIVATE KEY-----\nprivkey1\n-----END PRIVATE KEY-----",
			IsActive:   true,
			CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateSigningKey(ctx, key1); err != nil {
			t.Fatalf("CreateSigningKey: %v", err)
		}

		keys, err := q.ListSigningKeys(ctx, orgID)
		if err != nil {
			t.Fatalf("ListSigningKeys: %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("expected 1 signing key, got %d", len(keys))
		}
		if keys[0].ID != key1.ID {
			t.Errorf("ID: got %q, want %q", keys[0].ID, key1.ID)
		}
		if keys[0].PublicKey != key1.PublicKey {
			t.Errorf("PublicKey: got %q, want %q", keys[0].PublicKey, key1.PublicKey)
		}
		if keys[0].PrivateKey != key1.PrivateKey {
			t.Errorf("PrivateKey: got %q, want %q", keys[0].PrivateKey, key1.PrivateKey)
		}
		if !keys[0].IsActive {
			t.Errorf("IsActive: got false, want true")
		}
	})

	t.Run("get active signing key", func(t *testing.T) {
		orgID2 := seedOrg(t, q, "active-key-org")
		activeKey := domain.SigningKey{
			ID:         "sk-active",
			OrgID:      orgID2,
			PublicKey:  "pubkey-active",
			PrivateKey: "privkey-active",
			IsActive:   true,
			CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateSigningKey(ctx, activeKey); err != nil {
			t.Fatalf("CreateSigningKey: %v", err)
		}

		got, err := q.GetActiveSigningKey(ctx, orgID2)
		if err != nil {
			t.Fatalf("GetActiveSigningKey: %v", err)
		}
		if got.ID != activeKey.ID {
			t.Errorf("ID: got %q, want %q", got.ID, activeKey.ID)
		}
		if !got.IsActive {
			t.Errorf("IsActive: got false, want true")
		}
	})

	t.Run("deactivate all keys then get active returns NotFoundError", func(t *testing.T) {
		orgID3 := seedOrg(t, q, "deactivate-keys-org")
		key := domain.SigningKey{
			ID:         "sk-to-deactivate",
			OrgID:      orgID3,
			PublicKey:  "pubkey-deact",
			PrivateKey: "privkey-deact",
			IsActive:   true,
			CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateSigningKey(ctx, key); err != nil {
			t.Fatalf("CreateSigningKey: %v", err)
		}

		if err := q.DeactivateSigningKeys(ctx, orgID3); err != nil {
			t.Fatalf("DeactivateSigningKeys: %v", err)
		}

		_, err := q.GetActiveSigningKey(ctx, orgID3)
		if err == nil {
			t.Fatal("expected NotFoundError after deactivate all, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("key rotation flow", func(t *testing.T) {
		orgID4 := seedOrg(t, q, "rotation-org")
		now := time.Now().UTC().Truncate(time.Microsecond)

		// Create first active key
		key1 := domain.SigningKey{
			ID:         "sk-rotation-1",
			OrgID:      orgID4,
			PublicKey:  "pubkey-rotation-1",
			PrivateKey: "privkey-rotation-1",
			IsActive:   true,
			CreatedAt:  now,
		}
		if err := q.CreateSigningKey(ctx, key1); err != nil {
			t.Fatalf("CreateSigningKey(key1): %v", err)
		}

		// Deactivate all before creating new active key
		if err := q.DeactivateSigningKeys(ctx, orgID4); err != nil {
			t.Fatalf("DeactivateSigningKeys: %v", err)
		}

		// Create second active key
		key2 := domain.SigningKey{
			ID:         "sk-rotation-2",
			OrgID:      orgID4,
			PublicKey:  "pubkey-rotation-2",
			PrivateKey: "privkey-rotation-2",
			IsActive:   true,
			CreatedAt:  now.Add(time.Second),
		}
		if err := q.CreateSigningKey(ctx, key2); err != nil {
			t.Fatalf("CreateSigningKey(key2): %v", err)
		}

		// Verify key2 is now the active key
		got, err := q.GetActiveSigningKey(ctx, orgID4)
		if err != nil {
			t.Fatalf("GetActiveSigningKey: %v", err)
		}
		if got.ID != key2.ID {
			t.Errorf("active key ID: got %q, want %q", got.ID, key2.ID)
		}

		// Verify both keys are listed
		keys, err := q.ListSigningKeys(ctx, orgID4)
		if err != nil {
			t.Fatalf("ListSigningKeys: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("expected 2 signing keys after rotation, got %d", len(keys))
		}
	})

	t.Run("get active key for org with no keys returns NotFoundError", func(t *testing.T) {
		orgID5 := seedOrg(t, q, "no-keys-org")
		_, err := q.GetActiveSigningKey(ctx, orgID5)
		if err == nil {
			t.Fatal("expected NotFoundError for org with no keys, got nil")
		}
		var nfe *domain.NotFoundError
		if !isNotFound(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}
