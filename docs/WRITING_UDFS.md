# Writing UDFs

UDFs (User-Defined Functions) are built-in functions available to every Starlark rule in Nest. They provide the bridge between rule logic and the system's capabilities: signal evaluation, rate limiting, manual review routing, and more.

You do not register or define UDFs yourself. They are provided by the engine and are available as global functions in every rule script.

## Available UDFs

### verdict(type, reason="", actions=[])

Returns a verdict struct. This is the primary output of a rule.

**Parameters:**
- `type` (string, required): One of `"approve"`, `"block"`, or `"review"`.
- `reason` (string, optional): Human-readable explanation.
- `actions` (list of strings, optional): Names of actions to trigger (must match action names configured in the org).

**Returns:** A verdict struct with `.type`, `.reason`, and `.actions` fields.

**Examples:**

```python
# Simple block
verdict("block", reason="contains prohibited content")

# Block with webhook action
verdict("block", reason="spam detected", actions=["notify-webhook"])

# Route to manual review
verdict("review", reason="borderline content")

# Explicit approve
verdict("approve")
```

### signal(signal_id, text)

Invokes a registered signal adapter and returns its output. Results are cached within a single event evaluation, so calling the same signal with the same input twice does not re-run the adapter.

**Parameters:**
- `signal_id` (string, required): The adapter ID (e.g., `"text-regex"`, `"text-bank"`).
- `text` (string, required): The input value to evaluate.

**Returns:** A struct with:
- `.score` (float): Numeric score, typically in [0.0, 1.0].
- `.label` (string): Classification label (if applicable).
- `.metadata` (dict): Additional adapter-specific data.

**Built-in signal adapters:**

| Adapter ID | Input Format | Description |
|-----------|-------------|-------------|
| `text-regex` | `"<pattern>\n<text>"` | RE2 regex match. Score 1.0 on match, 0.0 otherwise. |
| `text-bank` | `"<bank_id>\n<text>"` | Matches text against a text bank's entries (exact or regex). |

Custom HTTP signal adapters can be registered at server startup for external services (e.g., ML classifiers).

**Examples:**

```python
# Regex check
result = signal("text-regex", r"spam.*offer\n" + event["payload"]["text"])
if result.score > 0.5:
    return verdict("block", reason="regex match: spam pattern")

# Text bank check
result = signal("text-bank", "banned-words\n" + event["payload"]["text"])
if result.score > 0:
    return verdict("block", reason="banned word: " + result.label)
```

### counter(entity_id, event_type, window_seconds)

Increments and reads a time-bucketed rate counter. Counters are distributed across all worker goroutines and aggregated on read. They are in-memory and reset when the server restarts.

**Parameters:**
- `entity_id` (string, required): The entity being counted (e.g., a user ID).
- `event_type` (string, required): The event category to count.
- `window_seconds` (int, required): The time window in seconds. Must be positive.

**Returns:** Integer count of events in the current time window across all workers.

**Examples:**

```python
# Rate limit: block if user posts more than 10 times in 1 hour
user_id = event["payload"].get("user_id", "")
count = counter("user:" + user_id, "post", 3600)
if count > 10:
    return verdict("block", reason="rate limit exceeded: " + str(count) + " posts/hour")

# Short burst detection: 5 posts in 60 seconds
burst = counter("user:" + user_id, "post", 60)
if burst > 5:
    return verdict("review", reason="burst activity detected")
```

### memo(key, func)

Caches the result of an expensive computation within a single event evaluation. If the same key is used again during the same event, the cached value is returned without calling the function.

**Parameters:**
- `key` (string, required): Cache key.
- `func` (callable, required): Zero-argument function to compute the value.

**Returns:** The cached or computed value.

**Example:**

```python
def get_text_score():
    return signal("text-regex", r"spam\n" + event["payload"]["text"])

# Both calls use the same cached result
score = memo("text_score", get_text_score)
score_again = memo("text_score", get_text_score)  # no re-computation
```

### log(message)

Appends a message to the evaluation log. Logged messages are included in the evaluation result for debugging. They do not go to the server log.

**Parameters:**
- `message` (string, required): The message to log.

**Returns:** None.

**Example:**

```python
text = event["payload"].get("text", "")
log("evaluating text of length " + str(len(text)))
score = signal("text-regex", r"bad\n" + text)
log("score=" + str(score.score))
```

### now()

Returns the current Unix timestamp as an integer.

**Parameters:** None.

**Returns:** Integer Unix timestamp (seconds since epoch).

**Example:**

```python
ts = now()
log("current time: " + str(ts))
```

### hash(value)

Computes the SHA-256 hex digest of a string. Useful for pseudonymizing user identifiers in logs or counters.

**Parameters:**
- `value` (string, required): The input string.

**Returns:** String hex digest (64 characters).

**Example:**

```python
email = event["payload"].get("email", "")
hashed = hash(email)
log("processing item from user " + hashed[:8])

# Use hashed email as counter key for privacy
count = counter(hashed, "post", 3600)
```

### regex_match(pattern, text)

Tests whether an RE2 regular expression matches the given text. Compiled regex patterns are cached per-worker to avoid recompilation overhead.

**Parameters:**
- `pattern` (string, required): RE2 regex pattern.
- `text` (string, required): Text to match against.

**Returns:** Boolean.

**Example:**

```python
text = event["payload"].get("text", "")
if regex_match(r"https?://bit\.ly/", text):
    return verdict("review", reason="contains shortened URL")
```

### enqueue(queue_name, reason="")

Inserts the current item into a named MRT (Manual Review Tool) queue for human review. Queue names must match queues configured in the org. Returns True on success, False if the queue is not found.

**Parameters:**
- `queue_name` (string, required): Name of the MRT queue.
- `reason` (string, optional): Why this item needs review.

**Returns:** Boolean (True on success, False on failure). Failures are logged but do not abort rule evaluation.

**Example:**

```python
text = event["payload"].get("text", "")
result = signal("text-regex", r"(kill|threat)\n" + text)
if result.score > 0:
    enqueue("urgent", reason="potential threat detected")
    return verdict("review", reason="flagged for urgent review")
```

## The Event Dict

Every rule's `evaluate(event)` function receives a Starlark dict with these keys:

| Key | Type | Description |
|-----|------|-------------|
| `event_id` | string | Unique event identifier |
| `event_type` | string | Event type (e.g., `"post.create"`) |
| `item_type` | string | Item type ID |
| `org_id` | string | Organization ID |
| `timestamp` | int | Unix timestamp |
| `payload` | dict | The submitted item data |

Access payload fields with `event["payload"]["field_name"]`.

## Execution Limits

- **Per-rule timeout:** 1 second (hardcoded).
- **Per-event timeout:** 5 seconds (hardcoded).
- **Step limit:** 10 million Starlark execution steps per rule (prevents infinite loops).
- **Panic recovery:** If a rule panics, it is caught and treated as an error (defaults to approve).
