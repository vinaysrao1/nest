package engine

import (
	"fmt"

	"github.com/vinaysrao1/nest/internal/domain"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Compiler compiles Starlark source into reusable CompiledRule values.
type Compiler struct{}

// CompiledRule holds the compiled program and metadata extracted from Starlark globals.
// Version is 0 by default from the compiler; the service layer sets it when building
// snapshots from the database rule version.
type CompiledRule struct {
	ID         string
	Version    int
	EventTypes []string
	Priority   int
	Program    *starlark.Program
	Source     string
}

// predeclaredNames lists all built-in names available to rules at compile time.
var predeclaredNames = starlark.StringDict{
	"verdict":     starlark.NewBuiltin("verdict", stubVerdict),
	"signal":      starlark.NewBuiltin("signal", stubNoop),
	"counter":     starlark.NewBuiltin("counter", stubNoop),
	"memo":        starlark.NewBuiltin("memo", stubNoop),
	"log":         starlark.NewBuiltin("log", stubNoop),
	"now":         starlark.NewBuiltin("now", stubNoop),
	"hash":        starlark.NewBuiltin("hash", stubNoop),
	"regex_match": starlark.NewBuiltin("regex_match", stubNoop),
	"enqueue":     starlark.NewBuiltin("enqueue", stubNoop),
}

// CompileRule parses, resolves, and validates a Starlark rule from source.
//
// It extracts the rule_id, event_types, and priority globals and validates
// that an evaluate function is present. The returned CompiledRule's Program
// can be Init'd repeatedly to evaluate events.
//
// Returns *domain.CompileError on any parse, resolution, or validation failure.
func (c *Compiler) CompileRule(source string, filename string) (*CompiledRule, error) {
	f, err := syntax.Parse(filename, source, 0)
	if err != nil {
		return nil, syntaxToCompileError(err, filename)
	}

	program, err := starlark.FileProgram(f, predeclaredNames.Has)
	if err != nil {
		return nil, resolveToCompileError(err, filename)
	}

	globals, err := initProgram(program, filename)
	if err != nil {
		return nil, err
	}

	ruleID, err := extractRuleID(globals, filename)
	if err != nil {
		return nil, err
	}

	eventTypes, err := extractEventTypes(globals, filename)
	if err != nil {
		return nil, err
	}

	priority, err := extractPriority(globals, filename)
	if err != nil {
		return nil, err
	}

	if err := validateEvaluateFunc(globals, filename); err != nil {
		return nil, err
	}

	return &CompiledRule{
		ID:         ruleID,
		EventTypes: eventTypes,
		Priority:   priority,
		Program:    program,
		Source:     source,
	}, nil
}

// initProgram executes the program once to extract its globals.
func initProgram(program *starlark.Program, filename string) (starlark.StringDict, error) {
	thread := &starlark.Thread{Name: "compile:" + filename}
	globals, err := program.Init(thread, predeclaredNames)
	if err != nil {
		return nil, &domain.CompileError{
			Message:  fmt.Sprintf("execution error: %v", err),
			Filename: filename,
		}
	}
	return globals, nil
}

// extractRuleID validates and returns the rule_id global.
func extractRuleID(globals starlark.StringDict, filename string) (string, error) {
	val, ok := globals["rule_id"]
	if !ok {
		return "", &domain.CompileError{
			Message:  "missing required global: rule_id",
			Filename: filename,
		}
	}
	ruleID, ok := starlark.AsString(val)
	if !ok {
		return "", &domain.CompileError{
			Message:  "rule_id must be a string",
			Filename: filename,
		}
	}
	return ruleID, nil
}

// extractEventTypes validates and returns the event_types global.
func extractEventTypes(globals starlark.StringDict, filename string) ([]string, error) {
	val, ok := globals["event_types"]
	if !ok {
		return nil, &domain.CompileError{
			Message:  "missing required global: event_types",
			Filename: filename,
		}
	}
	list, ok := val.(*starlark.List)
	if !ok {
		return nil, &domain.CompileError{
			Message:  "event_types must be a list",
			Filename: filename,
		}
	}
	if list.Len() == 0 {
		return nil, &domain.CompileError{
			Message:  "event_types must not be empty",
			Filename: filename,
		}
	}

	eventTypes := make([]string, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, &domain.CompileError{
				Message:  "event_types must contain only strings",
				Filename: filename,
			}
		}
		eventTypes[i] = s
	}

	return validateEventTypes(eventTypes, filename)
}

// validateEventTypes enforces the wildcard constraint.
func validateEventTypes(eventTypes []string, filename string) ([]string, error) {
	hasWildcard := false
	for _, et := range eventTypes {
		if et == "*" {
			hasWildcard = true
			break
		}
	}
	if hasWildcard && len(eventTypes) > 1 {
		return nil, &domain.CompileError{
			Message:  `event_types cannot mix "*" with specific types`,
			Filename: filename,
		}
	}
	return eventTypes, nil
}

// extractPriority validates and returns the priority global.
func extractPriority(globals starlark.StringDict, filename string) (int, error) {
	val, ok := globals["priority"]
	if !ok {
		return 0, &domain.CompileError{
			Message:  "missing required global: priority",
			Filename: filename,
		}
	}
	priorityInt, ok := val.(starlark.Int)
	if !ok {
		return 0, &domain.CompileError{
			Message:  "priority must be an integer",
			Filename: filename,
		}
	}
	p, ok := priorityInt.Int64()
	if !ok {
		return 0, &domain.CompileError{
			Message:  "priority value out of range",
			Filename: filename,
		}
	}
	return int(p), nil
}

// validateEvaluateFunc confirms the evaluate callable is present.
func validateEvaluateFunc(globals starlark.StringDict, filename string) error {
	val, ok := globals["evaluate"]
	if !ok {
		return &domain.CompileError{
			Message:  "missing required function: evaluate",
			Filename: filename,
		}
	}
	if _, ok := val.(starlark.Callable); !ok {
		return &domain.CompileError{
			Message:  "evaluate must be a function",
			Filename: filename,
		}
	}
	return nil
}

// stubVerdict is a no-op stub for verdict() used during compilation.
func stubVerdict(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	_ starlark.Tuple,
	_ []starlark.Tuple,
) (starlark.Value, error) {
	return starlark.None, nil
}

// stubNoop is a no-op stub for UDFs used during compilation.
func stubNoop(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	_ starlark.Tuple,
	_ []starlark.Tuple,
) (starlark.Value, error) {
	return starlark.None, nil
}

// syntaxToCompileError converts a syntax.Error to a domain.CompileError.
func syntaxToCompileError(err error, filename string) *domain.CompileError {
	var synErr syntax.Error
	if asErr, ok := err.(syntax.Error); ok {
		synErr = asErr
		return &domain.CompileError{
			Message:  synErr.Msg,
			Line:     int(synErr.Pos.Line),
			Column:   int(synErr.Pos.Col),
			Filename: filename,
		}
	}
	return &domain.CompileError{Message: err.Error(), Filename: filename}
}

// resolveToCompileError wraps a resolution error into a domain.CompileError.
func resolveToCompileError(err error, filename string) *domain.CompileError {
	return &domain.CompileError{Message: err.Error(), Filename: filename}
}
