package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// SigningKeyService manages RSA signing key lifecycle: listing and rotation.
type SigningKeyService struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewSigningKeyService constructs a SigningKeyService with the required dependencies.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned SigningKeyService is ready for use.
func NewSigningKeyService(st *store.Queries, logger *slog.Logger) *SigningKeyService {
	return &SigningKeyService{store: st, logger: logger}
}

// List returns all signing keys for the org, ordered by created_at DESC.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns all signing keys (active and inactive).
// Raises: error on database failure.
func (s *SigningKeyService) List(ctx context.Context, orgID string) ([]domain.SigningKey, error) {
	return s.store.ListSigningKeys(ctx, orgID)
}

// Rotate generates a new RSA-2048 key pair, deactivates all existing signing keys
// for the org, and creates the new key as the active key — all within a single
// transaction.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: all previous keys are inactive; one new active key exists.
// Raises: error if RSA key generation fails or store operations fail.
func (s *SigningKeyService) Rotate(ctx context.Context, orgID string) (*domain.SigningKey, error) {
	pubPEM, privPEM, err := auth.GenerateRSAKeyPair()
	if err != nil {
		return nil, fmt.Errorf("signing_keys.Rotate: generate key pair: %w", err)
	}

	now := time.Now().UTC()
	key := domain.SigningKey{
		ID:        fmt.Sprintf("sgk_%d", now.UnixNano()),
		OrgID:     orgID,
		PublicKey: pubPEM,
		PrivateKey: privPEM,
		IsActive:  true,
		CreatedAt: now,
	}

	if err := s.store.WithTx(ctx, func(tx *store.Queries) error {
		if err := tx.DeactivateSigningKeys(ctx, orgID); err != nil {
			return err
		}
		return tx.CreateSigningKey(ctx, key)
	}); err != nil {
		return nil, fmt.Errorf("signing_keys.Rotate: %w", err)
	}

	s.logger.Info("signing key rotated", "org_id", orgID, "key_id", key.ID)
	return &key, nil
}
