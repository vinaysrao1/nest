package signal

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/store"
)

// orgIDKey is an unexported context key for passing orgID to signal adapters.
// Using a private struct type prevents key collisions with other packages.
type orgIDKey struct{}

// WithOrgID returns a child context carrying the given orgID.
// Signal adapters that require org isolation retrieve it via OrgIDFromContext.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey{}, orgID)
}

// OrgIDFromContext retrieves the orgID stored by WithOrgID.
// Returns the empty string if no orgID is present.
func OrgIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(orgIDKey{}).(string)
	return id
}

// TextBankAdapter matches text against entries stored in a named text bank.
// Entries can be exact substrings or RE2 regex patterns.
//
// Input format: "<bankID>\n<text>" — bankID and text are separated by the first
// newline character in input.Value. The orgID is retrieved from the context.
type TextBankAdapter struct {
	store *store.Queries
}

// NewTextBankAdapter creates a TextBankAdapter backed by the given store.
//
// Pre-conditions: st must not be nil.
func NewTextBankAdapter(st *store.Queries) *TextBankAdapter {
	return &TextBankAdapter{store: st}
}

// ID returns "text-bank".
func (a *TextBankAdapter) ID() string { return "text-bank" }

// DisplayName returns the human-readable name.
func (a *TextBankAdapter) DisplayName() string { return "Text Bank" }

// Description describes the adapter.
func (a *TextBankAdapter) Description() string {
	return "Matches text against entries in a text bank (exact or regex)."
}

// EligibleInputs returns the input types this adapter accepts.
func (a *TextBankAdapter) EligibleInputs() []domain.SignalInputType {
	return []domain.SignalInputType{"text_bank"}
}

// Cost returns the relative processing cost.
func (a *TextBankAdapter) Cost() int { return 2 }

// Run evaluates text bank membership.
//
// Pre-conditions: ctx must carry an orgID (set via WithOrgID);
// input.Value must contain "<bankID>\n<text>".
// Post-conditions: Score is 1.0 when a match is found, 0.0 otherwise.
// On match, Label is set to the matching entry value.
// Raises: error if orgID is absent, separator is missing, or DB access fails.
func (a *TextBankAdapter) Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error) {
	orgID := OrgIDFromContext(ctx)
	if orgID == "" {
		return domain.SignalOutput{}, fmt.Errorf("text-bank: orgID not set in context")
	}

	idx := strings.Index(input.Value, "\n")
	if idx < 0 {
		return domain.SignalOutput{}, fmt.Errorf("text-bank: input value must contain bankID and text separated by newline")
	}
	bankID := input.Value[:idx]
	text := input.Value[idx+1:]

	entries, err := a.store.GetTextBankEntries(ctx, orgID, bankID)
	if err != nil {
		return domain.SignalOutput{}, fmt.Errorf("text-bank: get entries: %w", err)
	}

	for _, entry := range entries {
		if match, label := a.matchEntry(entry, text); match {
			return domain.SignalOutput{Score: 1.0, Label: label}, nil
		}
	}

	return domain.SignalOutput{Score: 0.0}, nil
}

// matchEntry checks a single entry against text.
// Returns (true, matchedValue) on success or (false, "") otherwise.
// Invalid regex entries are skipped.
func (a *TextBankAdapter) matchEntry(entry domain.TextBankEntry, text string) (bool, string) {
	if entry.IsRegex {
		matched, err := regexp.MatchString(entry.Value, text)
		if err != nil {
			return false, "" // skip invalid regex entries
		}
		if matched {
			return true, entry.Value
		}
		return false, ""
	}
	if strings.Contains(text, entry.Value) {
		return true, entry.Value
	}
	return false, ""
}
