// Package signal provides the signal adapter framework for the Nest content
// moderation rules engine. Signal adapters evaluate raw inputs and return a
// numeric score plus optional metadata consumed by the Starlark engine.
package signal

import (
	"context"

	"github.com/vinaysrao1/nest/internal/domain"
)

// Adapter is the interface all signal adapters must implement.
// Each adapter evaluates a SignalInput and returns a SignalOutput.
//
// Implementations must be safe for concurrent use from multiple goroutines.
type Adapter interface {
	// ID returns the unique, stable identifier for this adapter (e.g. "text-regex").
	ID() string

	// DisplayName returns the human-readable name shown in the UI.
	DisplayName() string

	// Description describes what this adapter does.
	Description() string

	// EligibleInputs returns the set of SignalInputTypes this adapter accepts.
	EligibleInputs() []domain.SignalInputType

	// Cost returns the relative cost of running this adapter (used for budgeting).
	Cost() int

	// Run evaluates the input and returns the resulting score and metadata.
	//
	// Pre-conditions: ctx must not be nil; input.Value must be valid for this adapter.
	// Post-conditions: returns a SignalOutput with Score in [0.0, 1.0].
	// Raises: error if the input is malformed or the evaluation fails.
	Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error)
}
