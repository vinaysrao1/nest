package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// BuildUDFs constructs the predeclared Starlark StringDict for a worker.
// The returned dict is set on the worker's thread as predeclared globals so
// that every rule script can call verdict(), signal(), counter(), memo(), etc.
//
// Pre-conditions: w must not be nil.
// Post-conditions: returned StringDict contains all built-in UDFs bound to w.
func BuildUDFs(w *Worker) starlark.StringDict {
	return starlark.StringDict{
		"verdict":     verdictUDF(w),
		"signal":      signalUDF(w),
		"counter":     counterUDF(w),
		"memo":        memoUDF(w),
		"log":         logUDF(w),
		"now":         nowUDF(),
		"hash":        hashUDF(),
		"regex_match": regexMatchUDF(w),
		"enqueue":     enqueueUDF(w),
	}
}

// verdictUDF returns a Starlark built-in that constructs a verdict struct.
// Signature: verdict(type, reason="", actions=[])
// type must be one of "approve", "block", "review".
// actions is an optional list of action name strings declared in the org.
//
// The UDF records action names on the current evalContext so the pool can
// resolve Action IDs after the Starlark script finishes.
func verdictUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("verdict", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var verdictType string
		var reason string
		var actionsList *starlark.List

		if err := starlark.UnpackArgs("verdict", args, kwargs,
			"type", &verdictType,
			"reason?", &reason,
			"actions?", &actionsList,
		); err != nil {
			return nil, err
		}

		if err := validateVerdictType(verdictType); err != nil {
			return nil, err
		}

		actions, err := extractActionNames(w, actionsList)
		if err != nil {
			return nil, err
		}

		return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"type":    starlark.String(verdictType),
			"reason":  starlark.String(reason),
			"actions": actions,
		}), nil
	})
}

// validateVerdictType returns an error if vt is not one of the three valid types.
func validateVerdictType(vt string) error {
	switch vt {
	case "approve", "block", "review":
		return nil
	default:
		return fmt.Errorf("verdict: type must be 'approve', 'block', or 'review', got %q", vt)
	}
}

// extractActionNames converts an optional Starlark list of strings into a
// *starlark.List, recording each action name on the evalContext.
func extractActionNames(w *Worker, actionsList *starlark.List) (*starlark.List, error) {
	out := starlark.NewList(nil)
	if actionsList == nil {
		return out, nil
	}
	for i := 0; i < actionsList.Len(); i++ {
		s, ok := starlark.AsString(actionsList.Index(i))
		if !ok {
			return nil, fmt.Errorf("verdict: actions must be a list of strings")
		}
		if err := out.Append(starlark.String(s)); err != nil {
			return nil, fmt.Errorf("verdict: append action: %w", err)
		}
		if w.currentCtx != nil {
			w.currentCtx.actionNames = append(w.currentCtx.actionNames, s)
		}
	}
	return out, nil
}

// logUDF returns a Starlark built-in that appends a message to the evalContext log.
// Signature: log(message)
// Logged messages are included in EvalResult.Logs for debugging.
func logUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("log", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var message string
		if err := starlark.UnpackPositionalArgs("log", args, kwargs, 1, &message); err != nil {
			return nil, err
		}
		if w.currentCtx != nil {
			w.currentCtx.logs = append(w.currentCtx.logs, message)
		}
		return starlark.None, nil
	})
}

// nowUDF returns a Starlark built-in that returns the current Unix timestamp as
// an integer. Signature: now() -> int
// Pure function with no side effects; not bound to the Worker.
func nowUDF() *starlark.Builtin {
	return starlark.NewBuiltin("now", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		if err := starlark.UnpackPositionalArgs("now", args, kwargs, 0); err != nil {
			return nil, err
		}
		return starlark.MakeInt64(time.Now().Unix()), nil
	})
}

// hashUDF returns a Starlark built-in that computes the SHA-256 hex digest of
// the input string. Signature: hash(value) -> string
// Pure function with no side effects; not bound to the Worker.
func hashUDF() *starlark.Builtin {
	return starlark.NewBuiltin("hash", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var value string
		if err := starlark.UnpackPositionalArgs("hash", args, kwargs, 1, &value); err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(value))
		return starlark.String(hex.EncodeToString(sum[:])), nil
	})
}

// regexMatchUDF returns a Starlark built-in that tests whether a regex pattern
// matches a text string. Signature: regex_match(pattern, text) -> bool
//
// Compiled regexes are cached in the worker's regexCache map to avoid
// recompilation on every call (ReDoS mitigation). The cache is per-worker and
// only accessed from that worker's goroutine, so no locking is needed here.
func regexMatchUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("regex_match", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var pattern, text string
		if err := starlark.UnpackPositionalArgs("regex_match", args, kwargs, 2, &pattern, &text); err != nil {
			return nil, err
		}
		re, ok := w.regexCache[pattern]
		if !ok {
			var err error
			re, err = regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("regex_match: invalid pattern %q: %w", pattern, err)
			}
			w.regexCache[pattern] = re
		}
		return starlark.Bool(re.MatchString(text)), nil
	})
}

// memoUDF returns a Starlark built-in that memoizes the result of a callable
// within a single event evaluation. Signature: memo(key, fn) -> value
// If key is already in the worker's memo map, fn is not called. The memo map
// is reset between events by the worker's eval loop.
func memoUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("memo", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var key string
		var computeFn starlark.Callable
		if err := starlark.UnpackPositionalArgs("memo", args, kwargs, 2, &key, &computeFn); err != nil {
			return nil, err
		}
		if val, ok := w.memo[key]; ok {
			return val, nil
		}
		result, err := starlark.Call(thread, computeFn, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("memo: compute fn for key %q: %w", key, err)
		}
		w.memo[key] = result
		return result, nil
	})
}
