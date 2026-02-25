package engine

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/vinaysrao1/nest/internal/domain"
)

// signalUDF returns a Starlark built-in that invokes a signal adapter by ID.
// Signature: signal(adapter_id, input_value) -> struct{score, label, metadata}
//
// Results are cached per evalContext using the key "<adapter_id>:<input_value>"
// so repeated calls within a single event evaluation do not re-invoke the adapter.
func signalUDF(w *Worker) *starlark.Builtin {
	return starlark.NewBuiltin("signal", func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var adapterID, inputValue string
		if err := starlark.UnpackPositionalArgs("signal", args, kwargs, 2, &adapterID, &inputValue); err != nil {
			return nil, err
		}

		if w.currentCtx == nil {
			return nil, fmt.Errorf("signal: no active evaluation context")
		}

		cacheKey := adapterID + ":" + inputValue
		if cached, ok := w.currentCtx.signalCache[cacheKey]; ok {
			return signalOutputToStarlark(cached), nil
		}

		adapter := w.pool.registry.Get(adapterID)
		if adapter == nil {
			return nil, fmt.Errorf("signal: adapter %q not found", adapterID)
		}

		input := domain.SignalInput{
			Type:  domain.SignalInputType(adapterID),
			Value: inputValue,
		}
		output, err := adapter.Run(w.currentCtx.ctx, input)
		if err != nil {
			return nil, fmt.Errorf("signal: adapter %q failed: %w", adapterID, err)
		}

		w.currentCtx.signalCache[cacheKey] = output
		return signalOutputToStarlark(output), nil
	})
}

// signalOutputToStarlark converts a domain.SignalOutput into a Starlark struct
// with fields: score (float), label (string), metadata (dict).
func signalOutputToStarlark(out domain.SignalOutput) starlark.Value {
	metadata := starlark.NewDict(len(out.Metadata))
	for k, v := range out.Metadata {
		// SetKey errors only on unhashable keys; string keys are always hashable.
		_ = metadata.SetKey(starlark.String(k), goValueToStarlark(v))
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"score":    starlark.Float(out.Score),
		"label":    starlark.String(out.Label),
		"metadata": metadata,
	})
}

// goValueToStarlark converts a Go any value to its closest Starlark equivalent.
// Unsupported types are rendered as their fmt.Sprintf("%v") string representation.
func goValueToStarlark(v any) starlark.Value {
	switch val := v.(type) {
	case string:
		return starlark.String(val)
	case float64:
		return starlark.Float(val)
	case float32:
		return starlark.Float(float64(val))
	case int:
		return starlark.MakeInt(val)
	case int64:
		return starlark.MakeInt64(val)
	case int32:
		return starlark.MakeInt64(int64(val))
	case bool:
		return starlark.Bool(val)
	case nil:
		return starlark.None
	case map[string]any:
		return mapToStarlarkDict(val)
	case []any:
		return sliceToStarlarkList(val)
	default:
		return starlark.String(fmt.Sprintf("%v", val))
	}
}

// mapToStarlarkDict converts a Go map[string]any into a Starlark dict.
func mapToStarlarkDict(m map[string]any) *starlark.Dict {
	d := starlark.NewDict(len(m))
	for k, v := range m {
		_ = d.SetKey(starlark.String(k), goValueToStarlark(v))
	}
	return d
}

// sliceToStarlarkList converts a Go []any into a Starlark list.
func sliceToStarlarkList(s []any) *starlark.List {
	elems := make([]starlark.Value, len(s))
	for i, v := range s {
		elems[i] = goValueToStarlark(v)
	}
	return starlark.NewList(elems)
}
