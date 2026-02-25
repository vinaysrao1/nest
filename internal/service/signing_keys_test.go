package service_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/vinaysrao1/nest/internal/service"
)

func TestRotate_DeactivatesOld(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-signing-deactivate")
	svc := service.NewSigningKeyService(q, testLogger())
	ctx := context.Background()

	// Create first key.
	first, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate first: %v", err)
	}
	if !first.IsActive {
		t.Error("Rotate first: expected IsActive=true")
	}

	// Rotate again — the first key should be deactivated.
	second, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate second: %v", err)
	}
	if !second.IsActive {
		t.Error("Rotate second: expected new key IsActive=true")
	}

	// List all keys and verify only the new one is active.
	keys, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List after rotation: %v", err)
	}
	activeCount := 0
	for _, k := range keys {
		if k.IsActive {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Errorf("after Rotate: want 1 active key, got %d", activeCount)
	}
	// Verify the old key is now inactive.
	for _, k := range keys {
		if k.ID == first.ID && k.IsActive {
			t.Errorf("after Rotate: first key %s should be inactive", first.ID)
		}
	}
}

func TestRotate_CreatesNew(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-signing-creates")
	svc := service.NewSigningKeyService(q, testLogger())
	ctx := context.Background()

	key, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if key.ID == "" {
		t.Error("Rotate: returned key has empty ID")
	}
	if key.OrgID != orgID {
		t.Errorf("Rotate: OrgID want %q, got %q", orgID, key.OrgID)
	}
	if key.PublicKey == "" {
		t.Error("Rotate: PublicKey is empty")
	}
	if key.PrivateKey == "" {
		t.Error("Rotate: PrivateKey is empty")
	}
	if !key.IsActive {
		t.Error("Rotate: expected IsActive=true")
	}
}

func TestRotate_KeyPairValid(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-signing-keypair")
	svc := service.NewSigningKeyService(q, testLogger())
	ctx := context.Background()

	key, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Parse the private key.
	privBlock, _ := pem.Decode([]byte(key.PrivateKey))
	if privBlock == nil {
		t.Fatal("PrivateKey: cannot decode PEM block")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(privBlock.Bytes)
	if err != nil {
		t.Fatalf("ParsePKCS1PrivateKey: %v", err)
	}

	// Parse the public key.
	pubBlock, _ := pem.Decode([]byte(key.PublicKey))
	if pubBlock == nil {
		t.Fatal("PublicKey: cannot decode PEM block")
	}
	publicKey, err := x509.ParsePKCS1PublicKey(pubBlock.Bytes)
	if err != nil {
		t.Fatalf("ParsePKCS1PublicKey: %v", err)
	}

	// Sign + verify round-trip.
	message := []byte("test payload for signing")
	digest := sha256.Sum256(message)
	sig, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, digest[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		t.Fatalf("SignPSS: %v", err)
	}

	if err := rsa.VerifyPSS(publicKey, crypto.SHA256, digest[:], sig, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	}); err != nil {
		t.Errorf("VerifyPSS: signature verification failed: %v", err)
	}
}

func TestList_ReturnsAll(t *testing.T) {
	q, cleanup := setupTestDB(t)
	defer cleanup()

	orgID := seedOrg(t, q, "test-org-signing-list")
	svc := service.NewSigningKeyService(q, testLogger())
	ctx := context.Background()

	// Rotate twice to create two keys.
	k1, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate k1: %v", err)
	}
	k2, err := svc.Rotate(ctx, orgID)
	if err != nil {
		t.Fatalf("Rotate k2: %v", err)
	}

	keys, err := svc.List(ctx, orgID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(keys) < 2 {
		t.Errorf("List: want at least 2 keys, got %d", len(keys))
	}
	foundIDs := make(map[string]bool)
	for _, k := range keys {
		foundIDs[k.ID] = true
	}
	if !foundIDs[k1.ID] {
		t.Errorf("List: key %s not found", k1.ID)
	}
	if !foundIDs[k2.ID] {
		t.Errorf("List: key %s not found", k2.ID)
	}
}
