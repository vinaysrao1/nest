# Integration Guide

This guide covers connecting your application to Nest: submitting items for moderation, configuring signal adapters, setting up webhooks, and receiving verdicts.

## End-to-End Flow

```
Your App                     Nest                          Your Webhook
   |                          |                                |
   |-- POST /api/v1/items -->|                                |
   |                          |-- evaluate rules              |
   |                          |-- resolve verdict             |
   |                          |-- publish actions ----------->|
   |<-- verdict response ----|                                |
```

1. Your application submits items via the REST API (with an API key).
2. Nest evaluates all matching rules against each item.
3. A final verdict is resolved (approve, block, review).
4. If the verdict triggers actions, they are executed (webhooks, MRT routing).
5. The verdict is returned to your application.

## Prerequisites

Before submitting items, set up:

1. **An API key** (see [Admin Guide](ADMIN_GUIDE.md))
2. **Item types** that describe your content schema
3. **Rules** that define moderation logic (see [Rules Engine](RULES_ENGINE.md))
4. **Actions** (optional) for webhooks or MRT routing

## Setting Up Item Types

Item types define the schema for content you submit. Create one for each content category:

```bash
curl -X POST http://localhost:8080/api/v1/item-types \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "post",
    "kind": "CONTENT",
    "schema": {
      "type": "object",
      "properties": {
        "text": {"type": "string"},
        "user_id": {"type": "string"},
        "image_url": {"type": "string"}
      }
    },
    "field_roles": {
      "text": "content",
      "user_id": "creator_id",
      "image_url": "media"
    }
  }'
```

Valid `kind` values: `CONTENT`, `USER`, `THREAD`.

## Submitting Items

### Synchronous Submission

Items are evaluated immediately and verdicts are returned in the response.

```bash
curl -X POST http://localhost:8080/api/v1/items \
  -H "X-API-Key: <your_api_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {
        "item_id": "post-12345",
        "item_type_id": "<item_type_id>",
        "payload": {
          "text": "Check out this great deal!",
          "user_id": "user-789"
        },
        "creator_id": "user-789",
        "creator_type_id": "user"
      }
    ]
  }'
```

Response:

```json
{
  "results": [
    {
      "item_id": "post-12345",
      "verdict": "approve",
      "triggered_rules": [
        {
          "rule_id": "keyword-filter",
          "version": 3,
          "verdict": "approve",
          "latency_us": 450
        }
      ],
      "actions": []
    }
  ]
}
```

Each entry in `triggered_rules` may include a `reason` field containing the string passed to `verdict()` in the rule source. The `reason` field is omitted when empty.

You can submit multiple items in a single request. Each item is evaluated independently.

### Asynchronous Submission

Items are persisted and queued for background evaluation via river jobs. Use this for high-throughput scenarios where you do not need the verdict immediately.

```bash
curl -X POST http://localhost:8080/api/v1/items/async \
  -H "X-API-Key: <your_api_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {
        "item_id": "post-12345",
        "item_type_id": "<item_type_id>",
        "payload": {
          "text": "Hello world"
        }
      }
    ]
  }'
```

Response (HTTP 202 Accepted):

```json
{
  "submission_ids": ["sub-abc123"]
}
```

With async submission, verdicts are delivered via webhooks (if configured).

### Item Request Fields

| Field | Required | Description |
|-------|----------|-------------|
| `item_id` | Yes | Your application's unique ID for this item |
| `item_type_id` | Yes | The item type ID (from item type creation) |
| `payload` | Yes | Key-value data accessible in rules via `event["payload"]` |
| `creator_id` | No | ID of the user who created this content |
| `creator_type_id` | No | Type of the creator entity |

## Configuring Actions

Actions are triggered by verdicts. Create them before referencing them in rules.

### Webhook Action

```bash
curl -X POST http://localhost:8080/api/v1/actions \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "notify-admin",
    "action_type": "WEBHOOK",
    "config": {
      "url": "https://your-app.example.com/webhooks/moderation"
    }
  }'
```

### ENQUEUE_TO_MRT Action

```bash
curl -X POST http://localhost:8080/api/v1/actions \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "route-to-review",
    "action_type": "ENQUEUE_TO_MRT",
    "config": {
      "queue_name": "default"
    }
  }'
```

## Webhook Delivery

When a rule triggers a webhook action, Nest sends an HTTP POST to the configured URL.

### Webhook Payload

```json
{
  "item_id": "post-12345",
  "item_type_id": "item-type-abc",
  "org_id": "org-default",
  "correlation_id": "corr-xyz",
  "action_name": "notify-admin",
  "payload": {
    "text": "the original item content",
    "user_id": "user-789"
  }
}
```

### Webhook Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `X-Nest-Signature` | RSA-PSS signature of the JSON payload body |

### Verifying Webhook Signatures

Nest signs webhook payloads using RSA-PSS with the org's active signing key. The `X-Nest-Signature` header contains a base64-encoded (standard encoding) RSA-PSS signature over the raw request body. To verify:

1. Retrieve the org's public key via `GET /api/v1/signing-keys`.
2. Base64-decode the value of the `X-Nest-Signature` header to obtain the raw signature bytes.
3. Compute the SHA-256 hash of the raw request body.
4. Verify the hash against the decoded signature using RSA-PSS with SHA-256 and a salt length equal to the hash length (32 bytes).

Rotate signing keys periodically via `POST /api/v1/signing-keys/rotate`.

### Webhook Error Handling

- Non-2xx responses from your endpoint are treated as failures and logged.
- The webhook HTTP client has a default 10-second timeout.
- Webhook delivery does not block the verdict response (for sync submissions, actions run after the response; for async, actions run during background processing).

## Signal Adapter Setup

### Built-In Adapters

Nest ships with two signal adapters:

**text-regex**: Matches text against RE2 regular expressions.
- Adapter ID: `text-regex`
- Input: `"<pattern>\n<text>"`
- Score: 1.0 on match, 0.0 on no match.

**text-bank**: Matches text against entries in a named text bank.
- Adapter ID: `text-bank`
- Input: `"<bank_id>\n<text>"`
- Score: 1.0 on match (exact substring or regex), 0.0 on no match.
- Requires text banks to be configured (see below).

### Configuring Text Banks

Text banks are named collections of patterns used by the `text-bank` signal adapter.

Create a text bank:

```bash
curl -X POST http://localhost:8080/api/v1/text-banks \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "banned-words",
    "description": "Words that should be blocked"
  }'
```

Add entries:

```bash
# Exact match entry
curl -X POST http://localhost:8080/api/v1/text-banks/<bank_id>/entries \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{"value": "badword", "is_regex": false}'

# Regex entry
curl -X POST http://localhost:8080/api/v1/text-banks/<bank_id>/entries \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{"value": "sp[a@]m", "is_regex": true}'
```

List entries:

```bash
curl http://localhost:8080/api/v1/text-banks/<bank_id>/entries \
  -b "session=<session_id>"
```

Delete an entry:

```bash
curl -X DELETE http://localhost:8080/api/v1/text-banks/<bank_id>/entries/<entry_id> \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>"
```

### Custom HTTP Signal Adapters

You can register external signal adapters at server startup by creating an `HTTPSignalAdapter` in the composition root. The adapter POSTs a JSON body to your endpoint and expects a response with `score`, `label`, and optional `metadata`:

Request (sent by Nest):

```json
{"type": "text", "value": "content to evaluate"}
```

Expected response (from your service):

```json
{
  "score": 0.95,
  "label": "spam",
  "metadata": {"confidence": 0.95, "model": "v2"}
}
```

### Testing Signals

Test a signal adapter without submitting an item:

```bash
curl -X POST http://localhost:8080/api/v1/signals/test \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "signal_id": "text-regex",
    "input": {
      "type": "text",
      "value": "spam.*offer\nCheck out this spam offer now!"
    }
  }'
```

List all registered signal adapters:

```bash
curl http://localhost:8080/api/v1/signals \
  -b "session=<session_id>"
```

## Creating Rules

See [Rules Engine](RULES_ENGINE.md) for full syntax. Quick example:

```bash
curl -X POST http://localhost:8080/api/v1/rules \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Basic Spam Filter",
    "status": "LIVE",
    "source": "rule_id = \"spam-filter\"\nevent_types = [\"post.create\"]\npriority = 100\n\ndef evaluate(event):\n    text = event[\"payload\"].get(\"text\", \"\")\n    if \"buy now\" in text.lower():\n        return verdict(\"block\", reason=\"spam\")\n    return verdict(\"approve\")",
    "tags": ["spam", "content"],
    "policy_ids": []
  }'
```

## Configuring Policies

Policies are content moderation policy categories that can be linked to rules and MRT decisions.

```bash
# Create a policy
curl -X POST http://localhost:8080/api/v1/policies \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Spam Policy",
    "description": "Rules governing spam content",
    "strike_penalty": 1
  }'

# Create a child policy
curl -X POST http://localhost:8080/api/v1/policies \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Promotional Spam",
    "description": "Unsolicited promotional content",
    "parent_id": "<parent_policy_id>",
    "strike_penalty": 2
  }'
```

## Complete Integration Example

Here is a full setup from scratch:

```bash
# 1. Log in and get CSRF token
LOGIN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@nest.local", "password": "admin123"}')
CSRF=$(echo $LOGIN | jq -r '.csrf_token')
# Extract session cookie from response headers

# 2. Create an API key for your application
curl -X POST http://localhost:8080/api/v1/api-keys \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-app"}'
# Save the returned key

# 3. Create an item type
curl -X POST http://localhost:8080/api/v1/item-types \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{"name":"post","kind":"CONTENT","schema":{},"field_roles":{}}'

# 4. Create a text bank with banned words
BANK=$(curl -s -X POST http://localhost:8080/api/v1/text-banks \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{"name":"banned-words","description":"Blocked terms"}')
BANK_ID=$(echo $BANK | jq -r '.id')

curl -X POST "http://localhost:8080/api/v1/text-banks/$BANK_ID/entries" \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{"value":"spam","is_regex":false}'

# 5. Create a webhook action
curl -X POST http://localhost:8080/api/v1/actions \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{"name":"notify","action_type":"WEBHOOK","config":{"url":"https://example.com/hook"}}'

# 6. Create a rule
curl -X POST http://localhost:8080/api/v1/rules \
  -b "session=<sid>" -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Banned Words Filter",
    "status": "LIVE",
    "source": "rule_id = \"banned-words\"\nevent_types = [\"post.create\"]\npriority = 100\n\ndef evaluate(event):\n    text = event[\"payload\"].get(\"text\", \"\")\n    result = signal(\"text-bank\", \"banned-words\\n\" + text)\n    if result.score > 0:\n        return verdict(\"block\", reason=\"banned: \" + result.label, actions=[\"notify\"])\n    return verdict(\"approve\")"
  }'

# 7. Submit items using your API key
curl -X POST http://localhost:8080/api/v1/items \
  -H "X-API-Key: <your_api_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "items": [{
      "item_id": "post-001",
      "item_type_id": "<item_type_id>",
      "payload": {"text": "This contains spam content"}
    }]
  }'
```

## API Endpoint Reference

### Public (no auth)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/login` | Authenticate and create session |
| GET | `/api/v1/health` | Health check |

### External (API key auth via `X-API-Key` header)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/items` | Submit items synchronously |
| POST | `/api/v1/items/async` | Submit items asynchronously |
| GET | `/api/v1/policies` | List policies |

### Internal (session auth + CSRF)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/logout` | End session |
| GET | `/api/v1/auth/me` | Current user identity |
| GET/POST | `/api/v1/rules` | List/create rules |
| GET/PUT/DELETE | `/api/v1/rules/{id}` | Get/update/delete a rule |
| POST | `/api/v1/rules/test` | Test rule source |
| POST | `/api/v1/rules/{id}/test` | Test existing rule |
| GET/POST | `/api/v1/actions` | List/create actions |
| GET/PUT/DELETE | `/api/v1/actions/{id}` | Get/update/delete an action |
| GET/POST | `/api/v1/policies` | List/create policies |
| GET/PUT/DELETE | `/api/v1/policies/{id}` | Get/update/delete a policy |
| GET/POST | `/api/v1/item-types` | List/create item types |
| GET/PUT/DELETE | `/api/v1/item-types/{id}` | Get/update/delete an item type |
| GET | `/api/v1/mrt/queues` | List MRT queues |
| POST | `/api/v1/mrt/queues` | Create MRT queue (ADMIN only) |
| DELETE | `/api/v1/mrt/queues/{id}` | Archive MRT queue (ADMIN only) |
| GET | `/api/v1/mrt/queues/{id}/jobs` | List jobs in queue |
| POST | `/api/v1/mrt/queues/{id}/assign` | Assign next job |
| POST | `/api/v1/mrt/jobs/claim` | Claim a specific job |
| POST | `/api/v1/mrt/decisions` | Record a decision |
| GET | `/api/v1/mrt/jobs/{id}` | Get a specific job (see note below) |
| GET | `/api/v1/users` | List users |
| POST | `/api/v1/users/invite` | Invite user |
| PUT/DELETE | `/api/v1/users/{id}` | Update/deactivate user |
| GET/POST | `/api/v1/api-keys` | List/create API keys |
| DELETE | `/api/v1/api-keys/{id}` | Revoke API key |
| GET/POST | `/api/v1/text-banks` | List/create text banks |
| GET | `/api/v1/text-banks/{id}` | Get text bank |
| GET/POST | `/api/v1/text-banks/{id}/entries` | List/add entries |
| DELETE | `/api/v1/text-banks/{id}/entries/{entryId}` | Delete entry |
| GET | `/api/v1/signals` | List signal adapters |
| POST | `/api/v1/signals/test` | Test a signal adapter |
| GET | `/api/v1/signing-keys` | List signing keys |
| POST | `/api/v1/signing-keys/rotate` | Rotate signing keys |
| GET | `/api/v1/udfs` | List built-in UDFs |
| GET | `/api/v1/orgs/settings` | Get org settings |

**Note on `GET /api/v1/mrt/jobs/{id}`:** This route uses a wildcard path parameter — the job ID can contain slashes (for example, AT Protocol URIs such as `at://did:plc:abc/app.bsky.feed.post/123`). URL-encode the full ID when constructing the request path (e.g., replace `/` with `%2F`).
