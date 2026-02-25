package handler

import "net/http"

// udfDescriptor describes a built-in UDF available in Starlark rule scripts.
type udfDescriptor struct {
	Name        string `json:"name"`
	Signature   string `json:"signature"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// builtinUDFs is the hardcoded list of all built-in UDFs available to rule authors.
// This list must stay in sync with the UDFs documented in docs/NEST_DESIGN.md section 7.5.
var builtinUDFs = []udfDescriptor{
	{
		Name:        "verdict",
		Signature:   `verdict(type, reason="", actions=[])`,
		Description: "Return a verdict. Types: approve, block, review.",
		Example:     `verdict("block", reason="spam", actions=["webhook-1"])`,
	},
	{
		Name:        "signal",
		Signature:   "signal(signal_id, text)",
		Description: "Call a registered signal adapter. Returns struct with .score, .label, .metadata.",
		Example:     `signal("openai-moderation", text)`,
	},
	{
		Name:        "counter",
		Signature:   "counter(entity_id, event_type, window_seconds)",
		Description: "Cross-worker in-memory counter. Returns count in time window.",
		Example:     `counter("user:123", "post", 3600)`,
	},
	{
		Name:        "memo",
		Signature:   "memo(key, func)",
		Description: "Single-event memoization. Caches result per key within one event.",
		Example:     "memo(\"score\", lambda: expensive())",
	},
	{
		Name:        "log",
		Signature:   "log(message)",
		Description: "Structured log output from rule. Attached to evaluation result.",
		Example:     `log("score=" + str(score))`,
	},
	{
		Name:        "now",
		Signature:   "now()",
		Description: "Current Unix timestamp (float).",
		Example:     "now()",
	},
	{
		Name:        "hash",
		Signature:   "hash(value)",
		Description: "SHA-256 hash of a string.",
		Example:     "hash(email)",
	},
	{
		Name:        "regex_match",
		Signature:   "regex_match(pattern, text)",
		Description: "RE2 regex match. Returns bool.",
		Example:     `regex_match(r"spam.*", text)`,
	},
	{
		Name:        "enqueue",
		Signature:   "enqueue(queue_name, reason)",
		Description: "Enqueue current item to an MRT queue. Returns bool.",
		Example:     `enqueue("urgent", reason="needs review")`,
	},
}

// handleListUDFs returns the hardcoded list of built-in UDFs.
//
// GET /api/v1/udfs
func handleListUDFs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]any{
			"udfs": builtinUDFs,
		})
	}
}
