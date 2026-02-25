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

// APIKeyService manages the lifecycle of API keys: creation, listing, and revocation.
type APIKeyService struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewAPIKeyService constructs an APIKeyService with the required dependencies.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned APIKeyService is ready for use.
func NewAPIKeyService(st *store.Queries, logger *slog.Logger) *APIKeyService {
	return &APIKeyService{store: st, logger: logger}
}

// Create generates a new API key for the org, stores only its hash, and returns
// the plaintext key exactly once. The caller must communicate the plaintext to
// the end user; it cannot be recovered after this call.
//
// Pre-conditions: orgID and name non-empty.
// Post-conditions: API key metadata persisted with hash; plaintext returned once.
// Raises: *domain.ValidationError if name is empty; error on store failure.
func (s *APIKeyService) Create(ctx context.Context, orgID, name string) (key string, apiKey *domain.ApiKey, err error) {
	if name == "" {
		return "", nil, &domain.ValidationError{
			Message: "api key name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}

	plaintext, prefix, hash := auth.GenerateAPIKey()

	now := time.Now().UTC()
	ak := domain.ApiKey{
		ID:        fmt.Sprintf("apk_%d", now.UnixNano()),
		OrgID:     orgID,
		Name:      name,
		KeyHash:   hash,
		Prefix:    prefix,
		CreatedAt: now,
	}

	if err := s.store.CreateAPIKey(ctx, ak); err != nil {
		return "", nil, fmt.Errorf("api_keys.Create: %w", err)
	}

	s.logger.Info("api key created", "org_id", orgID, "key_id", ak.ID, "name", name)
	return plaintext, &ak, nil
}

// List returns all API keys for the org, including revoked ones.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns all API keys ordered by created_at DESC.
// Raises: error on database failure.
func (s *APIKeyService) List(ctx context.Context, orgID string) ([]domain.ApiKey, error) {
	return s.store.ListAPIKeys(ctx, orgID)
}

// Revoke marks an API key as revoked. Revoked keys cannot be used for authentication.
//
// Pre-conditions: orgID and keyID non-empty.
// Post-conditions: key's revoked_at is set.
// Raises: *domain.NotFoundError if key not found in org.
func (s *APIKeyService) Revoke(ctx context.Context, orgID, keyID string) error {
	if err := s.store.RevokeAPIKey(ctx, orgID, keyID); err != nil {
		return err
	}
	s.logger.Info("api key revoked", "org_id", orgID, "key_id", keyID)
	return nil
}
