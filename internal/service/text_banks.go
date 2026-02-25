package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// TextBankService manages text banks and their entries.
type TextBankService struct {
	store  *store.Queries
	logger *slog.Logger
}

// NewTextBankService constructs a TextBankService with the required dependencies.
//
// Pre-conditions: all parameters must be non-nil.
// Post-conditions: returned TextBankService is ready for use.
func NewTextBankService(st *store.Queries, logger *slog.Logger) *TextBankService {
	return &TextBankService{store: st, logger: logger}
}

// List returns all text banks for the org, ordered by name ASC.
//
// Pre-conditions: orgID non-empty.
// Post-conditions: returns all text banks.
// Raises: error on database failure.
func (s *TextBankService) List(ctx context.Context, orgID string) ([]domain.TextBank, error) {
	return s.store.ListTextBanks(ctx, orgID)
}

// Get returns a single text bank by org and bank ID.
//
// Pre-conditions: orgID and bankID non-empty.
// Post-conditions: returns the matching text bank.
// Raises: *domain.NotFoundError if not found.
func (s *TextBankService) Get(ctx context.Context, orgID, bankID string) (*domain.TextBank, error) {
	return s.store.GetTextBank(ctx, orgID, bankID)
}

// Create creates a new text bank in the org.
//
// Pre-conditions: orgID and name non-empty.
// Post-conditions: text bank is persisted.
// Raises: *domain.ValidationError if name is empty; error on store failure.
func (s *TextBankService) Create(ctx context.Context, orgID, name, description string) (*domain.TextBank, error) {
	if name == "" {
		return nil, &domain.ValidationError{
			Message: "text bank name is required",
			Details: map[string]string{"name": "must not be empty"},
		}
	}

	now := time.Now().UTC()
	bank := domain.TextBank{
		ID:          fmt.Sprintf("tbk_%d", now.UnixNano()),
		OrgID:       orgID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateTextBank(ctx, &bank); err != nil {
		return nil, fmt.Errorf("text_banks.Create: %w", err)
	}

	s.logger.Info("text bank created", "org_id", orgID, "bank_id", bank.ID, "name", name)
	return &bank, nil
}

// ListEntries returns all entries for a text bank.
//
// Pre-conditions: orgID and bankID non-empty.
// Post-conditions: returns all entries for the bank.
// Raises: error on database failure.
func (s *TextBankService) ListEntries(ctx context.Context, orgID, bankID string) ([]domain.TextBankEntry, error) {
	return s.store.GetTextBankEntries(ctx, orgID, bankID)
}

// AddEntry adds a text entry to the specified text bank.
// If isRegex is true, the value is validated as a valid Go regular expression.
//
// Pre-conditions: orgID, bankID, and value non-empty.
// Post-conditions: entry is persisted.
// Raises: *domain.ValidationError if value is empty or isRegex=true and regex is invalid;
//
//	*domain.NotFoundError if bank not found in org.
func (s *TextBankService) AddEntry(ctx context.Context, orgID, bankID, value string, isRegex bool) (*domain.TextBankEntry, error) {
	if value == "" {
		return nil, &domain.ValidationError{
			Message: "entry value is required",
			Details: map[string]string{"value": "must not be empty"},
		}
	}

	if isRegex {
		if _, err := regexp.Compile(value); err != nil {
			return nil, &domain.ValidationError{
				Message: fmt.Sprintf("invalid regular expression: %v", err),
				Details: map[string]string{"value": "must be a valid Go regular expression"},
			}
		}
	}

	now := time.Now().UTC()
	entry := domain.TextBankEntry{
		ID:         fmt.Sprintf("tbe_%d", now.UnixNano()),
		TextBankID: bankID,
		Value:      value,
		IsRegex:    isRegex,
		CreatedAt:  now,
	}

	if err := s.store.AddTextBankEntry(ctx, orgID, &entry); err != nil {
		return nil, err
	}

	s.logger.Info("text bank entry added", "org_id", orgID, "bank_id", bankID, "entry_id", entry.ID)
	return &entry, nil
}

// DeleteEntry removes an entry from a text bank.
//
// Pre-conditions: orgID, bankID, and entryID non-empty.
// Post-conditions: entry is deleted.
// Raises: *domain.NotFoundError if entry or bank not found in org.
func (s *TextBankService) DeleteEntry(ctx context.Context, orgID, bankID, entryID string) error {
	if err := s.store.DeleteTextBankEntry(ctx, orgID, bankID, entryID); err != nil {
		return err
	}
	s.logger.Info("text bank entry deleted", "org_id", orgID, "bank_id", bankID, "entry_id", entryID)
	return nil
}
