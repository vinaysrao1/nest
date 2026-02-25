# Admin Guide

This guide covers organization, user, RBAC, API key, and session management in Nest.

## Authentication

Nest has two authentication mechanisms:

1. **Session cookies** -- For human users accessing the API or admin UI. Created via login, stored server-side in PostgreSQL.
2. **API keys** -- For programmatic access. Sent in the `X-API-Key` header. Only the SHA-256 hash is stored; the plaintext key is shown once at creation.

### Login

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@nest.local", "password": "admin123"}'
```

Response:

```json
{
  "user": {
    "id": "usr-admin-default",
    "org_id": "org-default",
    "email": "admin@nest.local",
    "name": "Admin",
    "role": "ADMIN",
    "is_active": true
  },
  "csrf_token": "abc123..."
}
```

(Additional fields like `created_at` and `updated_at` are present on the user object but omitted above for brevity.)

The response sets a `session` cookie (HttpOnly, SameSite=Strict). Save the `csrf_token` for mutating requests.

### Using Session Auth

All session-authenticated endpoints require:

- The `session` cookie (set automatically by the browser or passed with `-b`).
- The `X-CSRF-Token` header for POST, PUT, PATCH, DELETE requests.

```bash
curl http://localhost:8080/api/v1/auth/me \
  -b "session=<session_id>"

curl -X POST http://localhost:8080/api/v1/rules \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

### Logout

```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>"
```

Returns `204 No Content` and clears the session cookie.

### Password Reset

```bash
curl -X POST http://localhost:8080/api/v1/auth/reset-password \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

Always returns `204 No Content` regardless of whether the email exists (prevents enumeration).

## RBAC: Roles and Permissions

Nest has three roles:

| Role | Description |
|------|-------------|
| `ADMIN` | Full access. Can manage users, orgs, rules, API keys, signing keys. Required to create or archive MRT queues. |
| `MODERATOR` | Intended for reviewers: review MRT queues, record decisions, and view rules. |
| `ANALYST` | Intended for read-only access to rules, analytics, and configuration. |

API key requests are granted `ADMIN` role within the key's org.

**Role enforcement note:** Role-based access is enforced at the API level only for MRT queue management: creating a queue (`POST /api/v1/mrt/queues`) and archiving a queue (`DELETE /api/v1/mrt/queues/{id}`) require the `ADMIN` role. All other session-authenticated API endpoints do not enforce role restrictions at the API level — any authenticated user can call them. The UI enforces role-based navigation visibility so that MODERATOR and ANALYST users only see the pages appropriate for their role.

## User Management

### List Users

```bash
curl "http://localhost:8080/api/v1/users?page=1&page_size=20" \
  -b "session=<session_id>"
```

### Invite a User

Creates a new user in the org with a generated password:

```bash
curl -X POST http://localhost:8080/api/v1/users/invite \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "moderator@example.com",
    "name": "Jane Doe",
    "role": "MODERATOR"
  }'
```

Response includes the new user object. Valid roles: `ADMIN`, `MODERATOR`, `ANALYST`.

### Update a User

```bash
curl -X PUT http://localhost:8080/api/v1/users/<user_id> \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Jane Smith",
    "role": "ADMIN",
    "is_active": true
  }'
```

All fields are optional. Only provided fields are updated.

### Deactivate a User

```bash
curl -X DELETE http://localhost:8080/api/v1/users/<user_id> \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>"
```

Returns `204 No Content`. Deactivated users cannot log in.

## API Key Management

API keys authenticate programmatic access (item submission, policy reads). Keys use the `X-API-Key` header.

### Create an API Key

```bash
curl -X POST http://localhost:8080/api/v1/api-keys \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "production-ingestion"}'
```

Response:

```json
{
  "key": "nk_abc123...full_plaintext_key",
  "api_key": {
    "id": "key-id",
    "org_id": "org-default",
    "name": "production-ingestion",
    "prefix": "nk_abc1",
    "created_at": "2026-02-21T...",
    "revoked_at": null
  }
}
```

The `revoked_at` field is included in all API key objects — `null` for active keys, and an ISO 8601 timestamp when the key has been revoked.

**The plaintext key is shown only once.** Store it securely. Only the hash is persisted.

### List API Keys

```bash
curl http://localhost:8080/api/v1/api-keys \
  -b "session=<session_id>"
```

Returns key metadata (id, name, prefix, created_at, revoked_at). Never returns the full key.

### Revoke an API Key

```bash
curl -X DELETE http://localhost:8080/api/v1/api-keys/<key_id> \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>"
```

Returns `204 No Content`. The key is immediately unusable.

### Using an API Key

```bash
curl -X POST http://localhost:8080/api/v1/items \
  -H "X-API-Key: nk_abc123...full_plaintext_key" \
  -H "Content-Type: application/json" \
  -d '{"items": [...]}'
```

API key-authenticated routes:

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/items` | Submit items (synchronous) |
| POST | `/api/v1/items/async` | Submit items (asynchronous) |
| GET | `/api/v1/policies` | List policies |

## Signing Key Management

Signing keys are RSA key pairs used to sign webhook payloads (RSA-PSS). Recipients verify the `X-Nest-Signature` header to confirm the payload was sent by Nest.

### List Signing Keys

```bash
curl http://localhost:8080/api/v1/signing-keys \
  -b "session=<session_id>"
```

Returns key metadata with public keys. Private keys are never exposed in API responses.

### Rotate Signing Keys

Generates a new RSA key pair, deactivates all existing keys, and activates the new one:

```bash
curl -X POST http://localhost:8080/api/v1/signing-keys/rotate \
  -b "session=<session_id>" \
  -H "X-CSRF-Token: <csrf_token>"
```

After rotation, all subsequent webhook deliveries use the new key. Consumers should update their verification logic with the new public key.

## Organization Settings

### Get Org Settings

```bash
curl http://localhost:8080/api/v1/orgs/settings \
  -b "session=<session_id>"
```

Returns the org's JSONB settings map. Settings are org-specific configuration data stored as key-value pairs.

## Pagination

All list endpoints support pagination:

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| `page` | 1 | - | Page number (1-indexed) |
| `page_size` | 20 | 100 | Items per page |

Example:

```bash
curl "http://localhost:8080/api/v1/rules?page=2&page_size=50" \
  -b "session=<session_id>"
```

## Error Responses

All errors return a consistent JSON shape:

```json
{
  "error": "human-readable message",
  "details": {"field": "validation detail"}
}
```

The `details` field is omitted when empty (the field uses `omitempty`). Most error responses contain only `{"error": "message"}`; `details` appears only for validation errors with field-level information.

HTTP status code mapping:

| Status | Meaning |
|--------|---------|
| 400 | Validation error (bad input) |
| 401 | Not authenticated |
| 403 | Forbidden (insufficient role or invalid CSRF token) |
| 404 | Resource not found |
| 409 | Conflict (duplicate resource) |
| 422 | Unprocessable entity (Starlark compile error) |
| 500 | Internal server error |

## Seed Data Reference

The `cmd/seed` tool creates:

| Entity | ID | Details |
|--------|----|---------|
| Org | `org-default` | "Default Org" |
| User | `usr-admin-default` | `admin@nest.local` / `admin123`, role ADMIN |
| MRT Queue | `mrtq-default` | "default" |
| MRT Queue | `mrtq-urgent` | "urgent" |
| MRT Queue | `mrtq-escalation` | "escalation" |

All inserts are idempotent (ON CONFLICT DO NOTHING). Running seed multiple times is safe.
