# Rules Engine

Rules in Nest are written in [Starlark](https://github.com/google/starlark-go), a Python-like configuration language. Each rule is a standalone Starlark script that evaluates incoming content and returns one of three verdict types: **approve**, **block**, or **review** (enqueue for human review).

---

## Rule Structure

Every rule must define three globals and one function:

```python
rule_id = "my-spam-filter"
event_types = ["post.create", "comment.create"]
priority = 100

def evaluate(event):
    text = event["payload"].get("text", "")
    if "buy now" in text:
        return verdict("block", reason="spam phrase detected")
    return verdict("approve")
```

| Global | Type | Description |
|--------|------|-------------|
| `rule_id` | string | Unique identifier for this rule. |
| `event_types` | list of strings | Which event types trigger this rule. Use `["*"]` for all events. |
| `priority` | int | Evaluation order. Higher numbers run first. |

The `evaluate` function receives a single event dict and must return a `verdict()` call or `None` (treated as approve).

---

## The Three Verdict Types

Every rule returns one of three verdicts:

### 1. Auto-Action: Approve or Block

The rule makes a final automated decision on the item. No human is involved.

```python
# Auto-approve
return verdict("approve")

# Auto-block with reason
return verdict("block", reason="violates policy")

# Auto-block with reason and webhook notification
return verdict("block", reason="spam", actions=["notify-admin-webhook"])
```

### 2. Enqueue: Route to a Human Review Queue

The rule flags the item for human judgment by sending it to a named MRT (Manual Review Tool) queue. A moderator later picks it up and decides.

There are two ways to enqueue:

**Direct enqueue via `enqueue()` UDF:**

```python
def evaluate(event):
    text = event["payload"].get("text", "")
    result = signal("text-bank", "ambiguous-terms\n" + text)
    if result.score > 0:
        enqueue("default", reason="ambiguous term: " + result.label)
        return verdict("review", reason="routed to manual review")
    return verdict("approve")
```

`enqueue(queue_name, reason="...")` inserts an MRT job into the named queue. It returns `True` on success, `False` if the queue is not found. Failures are logged but do not abort rule evaluation.

**Enqueue via an ENQUEUE_TO_MRT action:**

First, create an action of type `ENQUEUE_TO_MRT` via the API (or in the Actions page):

```json
{
  "name": "route-to-escalation",
  "action_type": "ENQUEUE_TO_MRT",
  "config": { "queue_name": "escalation" }
}
```

Then reference the action name in a verdict:

```python
def evaluate(event):
    result = signal("text-bank", "severe-violations\n" + text)
    if result.score > 0:
        return verdict("review",
            reason="severe violation detected",
            actions=["route-to-escalation"])
    return verdict("approve")
```

The engine resolves the action name and the ActionPublisher inserts the MRT job.

**When to use which:**
- Use `enqueue()` when the queue name is known at rule-write time and you want inline control.
- Use `ENQUEUE_TO_MRT` actions when you want to decouple queue routing from rule logic (e.g., an admin can change the target queue without editing the rule source).

---

## Verdict Types

| Verdict | Effect |
|---------|--------|
| `"approve"` | Item passes moderation automatically. |
| `"block"` | Item is blocked automatically. |
| `"review"` | Item is flagged for human review. Combine with `enqueue()` or an `ENQUEUE_TO_MRT` action to route it to a specific queue. |

The `actions` parameter on any verdict triggers named actions (webhooks or MRT enqueue) after the verdict is resolved.

```python
verdict("block", reason="spam", actions=["notify-webhook", "log-to-siem"])
verdict("review", reason="borderline", actions=["route-to-escalation"])
verdict("approve")
```

---

## Priority and Verdict Resolution

Rules are evaluated in descending priority order. When multiple rules at the same priority produce verdicts, the heaviest verdict wins:

1. `block` (weight 3) -- highest
2. `review` (weight 2)
3. `approve` (weight 1) -- lowest

All matching rules are evaluated. During verdict resolution, the highest-priority rule's verdict wins. Lower-priority rule verdicts are discarded, but their side effects (`enqueue`, `counter`, `signal`, `log` calls) still execute.

```python
priority = 1000  # Runs first
priority = 100   # Runs after 1000
priority = 0     # Runs last
```

---

## Rule Status

Rules have three operational statuses, set via the API or UI (not in the Starlark source):

| Status | Behavior |
|--------|----------|
| `LIVE` | Fully active. Verdicts are applied. |
| `BACKGROUND` | Evaluated but verdicts are logged only, not applied. Use this to test rules in production. |
| `DISABLED` | Not evaluated. |

---

## Event Types

```python
event_types = ["post.create"]                           # One event type
event_types = ["post.create", "post.update"]            # Multiple
event_types = ["*"]                                     # All events (wildcard)
```

The wildcard `"*"` cannot be mixed with specific types.

---

## Available UDFs

Every rule has access to these built-in functions. See [Writing UDFs](WRITING_UDFS.md) for full parameter documentation, return value details, and more examples.

| UDF | Signature | Purpose |
|-----|-----------|---------|
| `verdict` | `verdict(type, reason="", actions=[])` | Return a moderation decision. |
| `signal` | `signal(signal_id, text)` | Invoke a signal adapter (text-regex, text-bank, HTTP). Returns `.score`, `.label`, `.metadata`. |
| `counter` | `counter(entity_id, event_type, window_seconds)` | Increment and read a rate counter. Returns int count. |
| `enqueue` | `enqueue(queue_name, reason="")` | Insert item into an MRT queue. Returns bool. |
| `memo` | `memo(key, func)` | Cache an expensive computation within one evaluation. |
| `log` | `log(message)` | Append to the evaluation debug log. |
| `now` | `now()` | Current Unix timestamp (int). |
| `hash` | `hash(value)` | SHA-256 hex digest of a string. |
| `regex_match` | `regex_match(pattern, text)` | RE2 regex match. Returns bool. |

---

## Built-in Signal Adapters

| Adapter ID | Input Format | Description |
|-----------|-------------|-------------|
| `text-regex` | `"<pattern>\n<text>"` | RE2 regex match. Score 1.0 on match, 0.0 otherwise. |
| `text-bank` | `"<bank_name>\n<text>"` | Match text against a text bank's entries (exact or regex). |

Custom HTTP signal adapters (e.g., ML classifiers) can be registered at server startup.

---

## MRT Queue Configuration

Queues are created via the API or the MRT Queues page. The seed tool creates three defaults:

| Queue | Purpose |
|-------|---------|
| `default` | General-purpose review |
| `urgent` | High-priority, fast turnaround |
| `escalation` | Senior moderator attention |

---

## Complete Examples

### Auto-Block: Keyword Filter

```python
rule_id = "keyword-filter"
event_types = ["post.create", "comment.create"]
priority = 100

BLOCKED_WORDS = ["spam", "scam", "phishing"]

def evaluate(event):
    text = event["payload"].get("text", "").lower()
    for word in BLOCKED_WORDS:
        if word in text:
            return verdict("block", reason="blocked keyword: " + word)
    return verdict("approve")
```

### Auto-Block: Rate Limiter

```python
rule_id = "rate-limiter"
event_types = ["*"]
priority = 500

def evaluate(event):
    user_id = event["payload"].get("user_id", "")
    if user_id == "":
        return verdict("approve")

    hourly = counter(user_id, event["event_type"], 3600)
    if hourly > 20:
        return verdict("block", reason="rate limit: " + str(hourly) + "/hr")

    burst = counter(user_id, event["event_type"], 60)
    if burst > 5:
        return verdict("review", reason="burst: " + str(burst) + " in 60s")

    return verdict("approve")
```

### Auto-Block with Webhook Action

```python
rule_id = "severe-violation"
event_types = ["*"]
priority = 1000

def evaluate(event):
    text = event["payload"].get("text", "")
    result = signal("text-bank", "severe-violations\n" + text)
    if result.score > 0:
        return verdict("block",
            reason="severe policy violation: " + result.label,
            actions=["notify-trust-safety", "log-to-siem"])
    return verdict("approve")
```

### Enqueue: Direct Routing to Review

```python
rule_id = "needs-human-review"
event_types = ["post.create"]
priority = 200

def evaluate(event):
    text = event["payload"].get("text", "")
    result = signal("text-bank", "ambiguous-terms\n" + text)
    if result.score > 0:
        enqueue("default", reason="ambiguous term: " + result.label)
        return verdict("review", reason="routed to manual review")
    return verdict("approve")
```

### Enqueue: Tiered Routing by Severity

```python
rule_id = "tiered-router"
event_types = ["post.create", "comment.create"]
priority = 300

def evaluate(event):
    text = event["payload"].get("text", "")

    # Tier 1: Severe -> block + escalation queue + webhook
    severe = signal("text-bank", "severe-violations\n" + text)
    if severe.score > 0:
        enqueue("escalation", reason="severe: " + severe.label)
        return verdict("block",
            reason="severe violation",
            actions=["notify-trust-safety"])

    # Tier 2: Moderate -> review + urgent queue
    moderate = signal("text-bank", "moderate-violations\n" + text)
    if moderate.score > 0:
        enqueue("urgent", reason="moderate: " + moderate.label)
        return verdict("review", reason="moderate violation")

    # Tier 3: Suspicious -> review + default queue
    suspicious = signal("text-regex", r"(buy|free|offer|click)\n" + text)
    if suspicious.score > 0:
        enqueue("default", reason="suspicious content")
        return verdict("review", reason="suspicious content flagged")

    return verdict("approve")
```

### Multi-Signal with Memoization

```python
rule_id = "multi-signal"
event_types = ["post.create"]
priority = 400

def evaluate(event):
    text = event["payload"].get("text", "")

    def check_regex():
        return signal("text-regex", r"(buy|sell|offer)\n" + text)

    def check_bank():
        return signal("text-bank", "spam-phrases\n" + text)

    regex_result = memo("regex_check", check_regex)
    bank_result = memo("bank_check", check_bank)

    log("regex_score=" + str(regex_result.score) + " bank_score=" + str(bank_result.score))

    if regex_result.score > 0 and bank_result.score > 0:
        return verdict("block", reason="spam: both signals triggered")
    elif regex_result.score > 0 or bank_result.score > 0:
        return verdict("review", reason="single signal triggered")
    return verdict("approve")
```

---

## Testing Rules

Test a rule without persisting it:

```bash
curl -X POST http://localhost:8080/api/v1/rules/test \
  -H "Cookie: session=<sid>" \
  -H "X-CSRF-Token: <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "rule_id = \"test\"\nevent_types = [\"post.create\"]\npriority = 1\ndef evaluate(event):\n    return verdict(\"block\")",
    "event": {
      "event_type": "post.create",
      "item_type": "post",
      "payload": {"text": "hello world"}
    }
  }'
```

Test an existing rule against a custom event:

```bash
curl -X POST http://localhost:8080/api/v1/rules/<rule_id>/test \
  -H "Cookie: session=<sid>" \
  -H "X-CSRF-Token: <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "event": {
      "event_type": "post.create",
      "item_type": "post",
      "payload": {"text": "test content"}
    }
  }'
```

The UI also provides a test panel on the rule editor page (see [NESTUI_GUIDE.md](NESTUI_GUIDE.md)).

---

## MRT Job Lifecycle

When an item is enqueued, the MRT job follows this lifecycle:

```
PENDING  -->  ASSIGNED  -->  DECIDED
```

1. **PENDING**: Waiting in the queue for a moderator.
2. **ASSIGNED**: A moderator has claimed the job (atomic assignment -- no two moderators get the same job).
3. **DECIDED**: The moderator has recorded a verdict (approve, block, skip, or route to another queue).

Decisions can optionally trigger further actions (webhooks or additional MRT routing).

---

## Key Design Points

- **Starlark source is the single source of truth.** Database columns like `event_types` and `priority` are derived at compile time.
- **`enqueue()` is fire-and-forget.** A failure to enqueue does not change the verdict or abort evaluation.
- **`ENQUEUE_TO_MRT` actions run after verdict resolution.** They execute via the ActionPublisher concurrently with webhook actions.
- **MRT jobs carry the full item payload.** Moderators see the complete content without a separate lookup.
- **Job assignment is atomic.** Two moderators calling assign on the same queue never receive the same job.

---

## Starlark Language Notes

Starlark is intentionally restricted compared to Python:

- No `import` statements. All functionality is via built-in UDFs.
- No `try`/`except`. Errors propagate and are caught by the engine.
- No `class` definitions. Use dicts and functions.
- No `while` loops (use `for` with `range()`).
- No mutable global state between evaluations.
- Strings, lists, dicts, tuples, ints, floats, bools, and None are supported.

## Execution Limits

- **Per-rule timeout:** 1 second (hardcoded).
- **Per-event timeout:** 5 seconds (hardcoded).
- **Step limit:** 10 million Starlark execution steps per rule.
