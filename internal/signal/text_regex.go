package signal

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vinaysrao1/nest/internal/domain"
)

// TextRegexAdapter matches a text value against an RE2 regular expression.
//
// Input format: "<pattern>\n<text>" — the pattern and text are separated by the
// first newline character in input.Value.
type TextRegexAdapter struct{}

// NewTextRegexAdapter creates a TextRegexAdapter.
func NewTextRegexAdapter() *TextRegexAdapter {
	return &TextRegexAdapter{}
}

// ID returns "text-regex".
func (a *TextRegexAdapter) ID() string { return "text-regex" }

// DisplayName returns the human-readable name.
func (a *TextRegexAdapter) DisplayName() string { return "Text Regex" }

// Description describes the adapter.
func (a *TextRegexAdapter) Description() string {
	return "Matches text against RE2 regular expression patterns."
}

// EligibleInputs returns the input types this adapter accepts.
func (a *TextRegexAdapter) EligibleInputs() []domain.SignalInputType {
	return []domain.SignalInputType{"text"}
}

// Cost returns the relative processing cost.
func (a *TextRegexAdapter) Cost() int { return 1 }

// Run evaluates the regex match.
//
// Pre-conditions: input.Value must contain "<pattern>\n<text>".
// Post-conditions: Score is 1.0 on match, 0.0 on no match.
// Raises: error if the separator is missing or the pattern is invalid RE2.
func (a *TextRegexAdapter) Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error) {
	idx := strings.Index(input.Value, "\n")
	if idx < 0 {
		return domain.SignalOutput{}, fmt.Errorf("text-regex: input value must contain pattern and text separated by newline")
	}
	pattern := input.Value[:idx]
	text := input.Value[idx+1:]

	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		return domain.SignalOutput{}, fmt.Errorf("text-regex: invalid pattern: %w", err)
	}

	score := 0.0
	if matched {
		score = 1.0
	}
	return domain.SignalOutput{Score: score}, nil
}
