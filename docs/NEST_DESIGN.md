# Nest -- Architecture Design Document

A lightweight, expressive rules engine that merges fruitfly's power with coop-lite-go's convenience. One binary. One database. Zero compromise.

---

## 1. Problem Statement

Three systems exist in the lineage. Each solves part of the problem:

- **Fruitfly** is a blazing-fast, zero-dependency rules engine. It evaluates Starlark rules against JSON events in real time. It has no external dependencies (embedded DuckDB, in-process counters, atomic rule reload). But it is single-purpose: it processes events, evaluates rules, and fires webhooks. It has no management UI, no multi-tenancy, no human review workflow, and no CRUD API for rules. Rules are files on disk.

- **Coop-lite-go** is a full-featured content moderation platform. It has multi-tenancy, RBAC, a rule engine with nested AND/OR conditions and pluggable signals, a Manual Review Tool, API key management, webhook signing, temporal versioning, analytics, and a complete REST API. But its rule evaluation is rigid: condition trees with threshold comparisons against signal scores. There is no general-purpose scripting, no UDFs, no counters, and no way to express complex business logic that does not fit the signal-threshold model.

- **Coop (original)** is the 350+ dependency monolith that both lite versions simplify. It has the same rigid condition model plus massive accidental complexity.

**Nest merges the best of fruitfly and coop-lite-go.** It takes fruitfly's Starlark-based rule evaluation (expressive, UDF-rich, hot-reloadable) and coop-lite-go's operational infrastructure (multi-tenant CRUD, REST API, RBAC, MRT, PostgreSQL persistence, React-ready frontend API). The result is a system where rules can be as simple as a threshold comparison or as complex as a multi-step Starlark script with counters, memoization, and custom logic -- all managed through a REST API, stored in PostgreSQL, and evaluated at scale.

### Design Principles

1. **One way to do each thing.** No dual engines, no dual storage, no dual abstractions. One rule format (Starlark). One storage backend (PostgreSQL). One evaluation model. No JSONB condition trees anywhere in the system.
2. **PostgreSQL does the work.** If Postgres can handle it (jobs, sessions, analytics, rule storage, JSON), do not add another system.
3. **Interfaces at boundaries, not everywhere.** Abstract only where swap is genuinely expected. The Starlark evaluator will always evaluate Starlark. Do not over-abstract it.
4. **Exploit the language.** Go gives goroutines, channels, atomic operations, single-binary deployment, and `go-starlark` as the canonical Starlark implementation. Use them all. **CGO is NOT required** -- `go.starlark.net` is pure Go, unlike fruitfly's DuckDB dependency which required CGO.
5. **Fruitfly's expressiveness is non-negotiable.** Every rule is a Starlark script. UDFs, counters, memoization, and custom logic are first-class citizens. The condition-tree model from coop is a subset that can be expressed in Starlark, not the other way around.
6. **Starlark source is the single source of truth.** Metadata like `event_types` and `priority` are declared in the Starlark script. Database columns for these fields are derived/computed values populated at compile time for query convenience. The Starlark source always wins in case of discrepancy.

### v1.0 Scope

v1.0 delivers the core platform: rule engine, CRUD, item submission, MRT, auth. The following are explicitly **deferred to v1.1+**:

- `reports` (user-submitted reports -> MRT queue)
- `user_strikes` (strike tracking and escalation)
- `analytics` service and endpoints (query execution logs via direct SQL or simple store queries initially)
- `investigation` service and endpoints (item lookup via direct store queries initially)
- MRT routing rules (use explicit `enqueue("queue-name")` UDF in Starlark rules for v1.0)

---

## 2. What Nest Takes from Each System

### From Fruitfly

| Concept | How Nest Uses It |
|---------|-----------------|
| Starlark rules with `evaluate(event)` | Core rule format. Every rule is a Starlark script stored in PostgreSQL. |
| Built-in UDFs (`verdict()`, `counter()`, `memo()`, `log()`, `hash()`, `regex_match()`, `now()`) | Shipped as built-in UDFs. Extended with coop-inspired UDFs (`signal()`, `enqueue()`). |
| Pre-compiled Starlark programs | Rules compiled on load/update, cached in-process. |
| Atomic hot reload via `atomic.Pointer` | Snapshot-based rule cache. On rule CRUD, new snapshot built and atomically swapped. |
| Worker pool with per-worker isolation | Goroutine pool evaluates rules. Each worker owns its Starlark thread. |
| In-memory time-bucketed counters | Per-worker atomic counters with cross-worker `CounterSum`. Optionally backed by PostgreSQL for persistence. |
| No-lock pipeline (channels + atomics) | Hot path uses channels and `atomic.Pointer`. No `sync.Mutex` in the evaluation pipeline. |
| Single-event memoization | `memo()` UDF caches values within a single event evaluation. |

### From Coop-Lite-Go

| Concept | How Nest Uses It |
|---------|-----------------|
| Multi-tenant organizations | `org_id` on every table, every query. |
| REST API with RBAC | Full CRUD for rules, actions, policies, item types, users, API keys. |
| PostgreSQL-only infrastructure | No Kafka, no Redis, no ClickHouse. PostgreSQL + river for jobs. |
| Signal adapter interface | Signals are available as UDFs inside Starlark (`signal("openai-moderation", text)`). |
| Manual Review Tool (MRT) | Queues, job assignment, compound decisions. `enqueue()` UDF from Starlark. |
| Webhook actions with RSA-PSS signing | Action publisher fires webhooks, signs payloads. |
| Temporal versioning (history table) | A single generic entity history table with JSONB. |
| Session auth + API key auth | Same dual-auth model. |
| Composition root (no DI framework) | Plain `main.go` struct construction. |
| `domain/` pure types package | Zero-import domain types form the dependency leaf. |
| river job queue | PostgreSQL-native async processing. |
| Partitioned execution logs | `rule_executions` and `action_executions` partitioned by month. |

### What Nest Drops

| Dropped | Why |
|---------|-----|
| Coop's condition tree model (AND/OR/leaf/signal/threshold) | Subsumed by Starlark. `if signal("text-regex", text).score >= 0.8` is more expressive than a JSONB condition tree and easier to read. |
| Coop's `ConditionSet`, `LeafCondition`, `Conjunction` types | Gone. The rule body IS the logic. No recursive tree evaluation, no cost-based ordering. |
| Coop's condition evaluator (`condition_set.go`) | Gone. Starlark is the evaluator. |
| Fruitfly's file-based rule storage | Rules live in PostgreSQL, managed via API. File-based loading available as a development mode only. |
| Fruitfly's DuckDB | PostgreSQL handles all storage. DuckDB was needed only because fruitfly had no external dependencies. Nest has PostgreSQL. |
| Fruitfly's single-tenant model | Multi-tenant from day one. |
| `lookup()` UDF | Deferred. It adds a hidden database dependency to the hot path with unspecified caching and error semantics. Not present in fruitfly. If needed later, it will be fully specified (schema, error behavior, caching, latency impact). |

---

## 3. Architecture Overview

```
                             React Frontend
                                  |
                           REST API (:8080)
                                  |
                    +-------------+-------------+
                    |                           |
              Session Auth               API Key Auth
              (UI requests)            (external clients)
                    |                           |
                    +-------------+-------------+
                                  |
                          chi Router (handlers)
                                  |
         +-------+--------+------+------+--------+
         |       |        |             |        |
       Config  Actions  Policies     Items     MRT
       (rules, (CRUD)   (CRUD)     Submit    Ops
       actions,                  sync/async
       policies,                    |
       item_types)     +-----------+-----------+
                       |                       |
                 Sync (inline)          Async (river job)
                       |                       |
                       +----------+------------+
                                  |
                         Rule Engine (Pool)
                                  |
                       atomic.Pointer<Snapshot>
                       (compiled Starlark rules)
                                  |
                   +-----+-----+-----+-----+
                   | W0  | W1  | W2  | ... | WN  (goroutines)
                   +-----+-----+-----+-----+
                       |
                 Starlark evaluate(event)
                 UDFs: verdict(), counter(), memo(),
                       signal(), enqueue(), log(),
                       hash(), regex_match(), now()
                       |
                 +-----+-----+
                 |           |
           ActionRequests  Execution Logs
                 |           |
          Action Publisher   store.LogRuleExecutions()
                 |
       +---------+---------+
       |                   |
    Webhook           MRT Enqueue
    (signed)          (PostgreSQL)
```

### Key Insight: Starlark Replaces the Condition Tree

In coop-lite-go, a rule is a JSONB condition tree:
```json
{
  "conjunction": "OR",
  "conditions": [
    {"signal": {"id": "openai-moderation"}, "field": {"name": "text"}, "threshold": {"operator": ">=", "value": 0.8}},
    {"signal": {"id": "text-regex"}, "field": {"name": "text"}, "config": {"pattern": "spam.*"}}
  ]
}
```

In Nest, the same rule is a Starlark script:
```python
rule_id = "block-hate-speech"
event_types = ["content"]
priority = 100

def evaluate(event):
    text = event["payload"]["text"]

    # Check AI signal first (short-circuits if true)
    ai = signal("openai-moderation", text)
    if ai.score >= 0.8 and ai.label == "hate":
        return verdict("block", reason="AI detected hate speech: " + ai.label,
                        actions=["webhook-1", "enqueue-urgent"])

    # Check regex pattern
    if regex_match("spam.*", text):
        return verdict("review", reason="spam pattern detected",
                        actions=["enqueue-manual-review"])

    # Rate limiting with counters
    if counter(event["payload"]["user_id"], "content", 3600) > 50:
        return verdict("review", reason="high post rate",
                        actions=["enqueue-rate-limit-queue"])

    return verdict("approve")
```

The Starlark version is more expressive (arbitrary logic, variable binding, conditional signal invocation, counter-based rate limiting), more readable, and more maintainable. The condition tree model is a strict subset: any condition tree can be mechanically translated to Starlark, but not vice versa.

### Key Insight: Actions Are Declared in Starlark

In coop-lite-go, actions are associated with rules via a `rules_actions` join table. In Nest, **actions are declared directly in the `verdict()` call**:

```python
return verdict("block", reason="spam", actions=["webhook-notify", "enqueue-urgent"])
```

The `actions` parameter is a list of action names (referencing the `actions` table by name within the org). This eliminates the `rules_actions` join table and gives rules full control over which actions fire based on runtime conditions:

```python
def evaluate(event):
    score = signal("openai-moderation", event["payload"]["text"]).score
    if score >= 0.95:
        # High confidence: block and notify immediately
        return verdict("block", reason="AI high confidence",
                        actions=["webhook-notify", "enqueue-escalation"])
    elif score >= 0.8:
        # Medium confidence: review only
        return verdict("review", reason="AI medium confidence",
                        actions=["enqueue-manual-review"])
    return verdict("approve")
```

This is strictly more powerful than a static join table: the action set can vary based on the evaluation path.

---

## 4. Technology Choices

| Layer | Choice | Why |
|-------|--------|-----|
| Language | **Go 1.23+** | Single binary, goroutines, `go-starlark` is the canonical Starlark implementation, rich stdlib. **No CGO required.** |
| Rules language | **Starlark** (via `go.starlark.net`) | Safe (no filesystem, no network, no system calls), deterministic, Python-like syntax, pre-compilable. Pure Go implementation -- no CGO dependency unlike fruitfly's DuckDB. |
| HTTP router | **chi v5** | Lightweight, stdlib-compatible, middleware support. |
| Database | **PostgreSQL 16** | Only external dependency. Rules, events, results, sessions, jobs -- everything. |
| Database driver | **pgx/v5** | Fastest pure-Go PostgreSQL driver. Native JSONB, arrays, connection pooling. |
| Job queue | **river** | PostgreSQL-native, built on pgx. Advisory locks, unique jobs, periodic jobs. |
| Password hashing | **golang.org/x/crypto/bcrypt** | Standard. |
| Logging | **log/slog** (stdlib) | Structured JSON, built-in since Go 1.21. |
| Configuration | **Environment variables** | Parsed into typed `Config` struct. No config library. |
| Frontend API contract | **OpenAPI 3.1** | Generate TypeScript types for React frontend. |
| Webhook signing | **crypto/rsa** (RSA-PSS, stdlib) | Same as coop-lite-go. |

### Starlark Version Pinning

Pin `go.starlark.net` to a specific commit hash or tagged version in `go.mod`. Starlark-the-language has no formal versioning; the Go module uses commit-based pseudo-versions. To ensure rule behavior stability:

1. Pin the exact `go.starlark.net` version in `go.mod` (e.g., `go.starlark.net v0.0.0-20240725214946-42f8f4cd09b4`).
2. Document the pinned version in `DEPENDENCIES.md`.
3. Upgrade only with a deliberate PR that runs the full rule test suite.
4. Include a Starlark conformance test suite (a set of `.star` files with expected outputs) that must pass before any `go.starlark.net` upgrade is merged.

### Production Dependencies (5 external modules)

| Module | Why |
|--------|-----|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/riverqueue/river` | Job queue |
| `golang.org/x/crypto` | bcrypt |
| `go.starlark.net` | Starlark interpreter (canonical Google implementation, **pure Go, no CGO**) |

Everything else is stdlib.

---

## 5. Directory Structure

```
nest/
|-- go.mod
|-- go.sum
|-- cmd/
|   |-- server/
|   |   |-- main.go              # Entry point: parse config, compose, serve
|   |-- migrate/
|   |   |-- main.go              # Database migration runner
|   |-- seed/
|       |-- main.go              # Seed data for development
|
|-- internal/
|   |-- config/
|   |   |-- config.go            # Config struct, env parsing
|   |
|   |-- domain/                   # Pure types (zero internal imports)
|   |   |-- event.go             # Event, ValidatedItem
|   |   |-- rule.go              # Rule (with Starlark source), RuleMetadata
|   |   |-- action.go            # Action, ActionType, ActionRequest, ActionResult
|   |   |-- verdict.go           # Verdict types (approve, block, review + custom)
|   |   |-- policy.go            # Policy types
|   |   |-- signal.go            # SignalInput, SignalOutput, SignalInputType
|   |   |-- mrt.go               # MRTJob, MRTDecision, MRTQueue
|   |   |-- user.go              # User, Role, Session
|   |   |-- org.go               # Org, OrgSettings
|   |   |-- errors.go            # NotFound, Forbidden, Conflict, Validation, Config errors
|   |   |-- pagination.go        # PaginatedResult, PageParams
|   |
|   |-- engine/                   # Starlark rule evaluation (fruitfly heritage)
|   |   |-- pool.go              # Worker pool: manages goroutines, channels, snapshot
|   |   |-- pool_test.go
|   |   |-- worker.go            # Single worker: owns Starlark thread, memo, counters
|   |   |-- worker_test.go
|   |   |-- snapshot.go          # Immutable compiled rule snapshot
|   |   |-- snapshot_test.go
|   |   |-- compiler.go          # Starlark source -> compiled Program
|   |   |-- compiler_test.go
|   |   |-- udf.go               # Built-in UDF registration (verdict, counter, memo, etc.)
|   |   |-- udf_test.go
|   |   |-- udf_signal.go        # signal() UDF: bridges Starlark to SignalAdapter
|   |   |-- udf_counter.go       # counter() UDF: in-memory time-bucketed counters
|   |   |-- action_publisher.go  # Execute actions (webhook, MRT enqueue)
|   |   |-- action_publisher_test.go
|   |   |-- cache.go             # TTL in-memory cache (sync.RWMutex + map)
|   |   |-- cache_test.go
|   |
|   |-- signal/                   # Signal adapters (coop-lite-go heritage)
|   |   |-- adapter.go           # SignalAdapter interface
|   |   |-- registry.go          # Signal registry
|   |   |-- text_regex.go        # Built-in: regex (subsumes text-contains)
|   |   |-- text_regex_test.go
|   |   |-- text_bank.go         # Built-in: text bank lookup
|   |   |-- text_bank_test.go
|   |   |-- http_signal.go       # Generic HTTP signal adapter (OpenAI, Google, custom)
|   |   |-- http_signal_test.go
|   |
|   |-- service/
|   |   |-- rules.go             # Rule CRUD + Starlark compilation + snapshot rebuild
|   |   |-- rules_test.go
|   |   |-- config.go            # Thin CRUD for actions, policies, item_types
|   |   |-- config_test.go
|   |   |-- mrt.go               # Manual review: enqueue, assign, decide
|   |   |-- mrt_test.go
|   |   |-- items.go             # Item validation + submission
|   |   |-- items_test.go
|   |   |-- users.go             # User CRUD, invites, password reset
|   |   |-- users_test.go
|   |   |-- api_keys.go          # API key create, verify, rotate
|   |   |-- signing_keys.go      # RSA-PSS keypair management
|   |   |-- text_banks.go        # Text bank management
|   |
|   |-- store/                    # Database access (pgx queries)
|   |   |-- db.go                # pgxpool wrapper, transaction helper
|   |   |-- rules.go             # Rule queries (source + metadata, history)
|   |   |-- config.go            # Actions, policies, item_types queries
|   |   |-- items.go
|   |   |-- mrt.go
|   |   |-- text_banks.go
|   |   |-- auth.go              # Sessions, API keys, password reset tokens
|   |   |-- signing_keys.go
|   |   |-- executions.go        # Rule/action execution logs
|   |   |-- orgs.go
|   |   |-- users.go
|   |   |-- counters.go          # Optional: persistent counter state
|   |   |-- history.go           # Generic entity history (single table)
|   |
|   |-- handler/                  # HTTP handlers
|   |   |-- rules.go             # Rule CRUD + test endpoints
|   |   |-- config.go            # Actions, policies, item_types CRUD
|   |   |-- items.go             # Item submission (sync + async)
|   |   |-- mrt.go               # MRT queue, assign, decide
|   |   |-- users.go             # User CRUD + invites
|   |   |-- auth.go              # Login, logout, session, password reset
|   |   |-- orgs.go              # Org management
|   |   |-- api_keys.go          # API key management
|   |   |-- signals.go           # Signal listing + test
|   |   |-- text_banks.go        # Text bank CRUD
|   |   |-- health.go            # Health check
|   |   |-- helpers.go           # JSON, Decode, Error, OrgID, UserID
|   |
|   |-- auth/                     # Auth + crypto (consolidated)
|   |   |-- middleware.go        # Session auth, API key auth, CSRF
|   |   |-- passwords.go        # bcrypt
|   |   |-- sessions.go         # PostgreSQL-backed session store
|   |   |-- rbac.go             # Role check
|   |   |-- context.go          # Auth context keys
|   |   |-- signing.go          # RSA-PSS webhook signing
|   |   |-- hashing.go          # SHA-256, API key hashing, token generation
|   |
|   |-- worker/                   # Background jobs (river)
|       |-- process_item.go      # Async item processing
|       |-- maintenance.go       # Snapshot rebuild, partition manager, session cleanup, counter flush
|
|-- api/
|   |-- openapi.yaml            # OpenAPI 3.1 specification
|
|-- migrations/
|   |-- 001_initial.sql         # Full schema DDL
|   |-- 002_partitions.sql      # Initial partition creation
|
|-- rules/                       # Example rules (dev mode file loading)
|   |-- examples/
|       |-- spam_check.star
|       |-- rate_limit.star
|       |-- ai_moderation.star
|
|-- Dockerfile
|-- Makefile
```

**File count: ~55 Go source files (v1.0 scope).**

### Package consolidation notes

- **`cache/` merged into `engine/`**: The TTL cache is only used by the engine for signal result caching and snapshot management. It does not warrant its own package.
- **`crypto/` merged into `auth/`**: Signing and hashing are auth-adjacent concerns. One package for all security primitives.
- **`worker/` consolidated**: `counter_flush.go`, `partition_manager.go`, `session_cleanup.go`, and `snapshot_rebuild.go` merged into `maintenance.go`. These are all short periodic jobs.
- **`service/` consolidated**: `actions.go`, `policies.go`, `item_types.go` merged into `config.go` for thin CRUD. `rules.go` remains separate because it has complex compilation and snapshot logic.
- **`handler/` consolidated**: Mirrors service consolidation. `config.go` handles actions, policies, item_types. ~11 handler files total.
- **`store/` consolidated**: `sessions.go` and `api_keys.go` merged into `auth.go`. `actions.go`, `policies.go`, `item_types.go` merged into `config.go`.

### Deferred files (v1.1+)

The following files are NOT in v1.0:
- `service/analytics.go`, `service/investigation.go`, `service/reports.go`, `service/user_strikes.go`
- `handler/analytics.go`, `handler/investigation.go`, `handler/reports.go`
- `store/user_strikes.go`, `store/reports.go`

---

## 6. Data Model

### PostgreSQL Schema (v1.0)

The schema extends coop-lite-go's schema with two critical changes: (1) the `rules` table stores Starlark source instead of a JSONB condition tree, and (2) a single generic `entity_history` table replaces per-entity history tables.

```sql
-- Core multi-tenancy
CREATE TABLE orgs (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  name        TEXT NOT NULL,
  settings    JSONB NOT NULL DEFAULT '{}',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users with roles
CREATE TABLE users (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  email       TEXT NOT NULL,
  name        TEXT NOT NULL,
  password    TEXT NOT NULL,
  role        TEXT NOT NULL CHECK (role IN ('ADMIN', 'MODERATOR', 'ANALYST')),
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, email)
);

CREATE TABLE password_reset_tokens (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  key_hash    TEXT NOT NULL,
  prefix      TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at  TIMESTAMPTZ
);

CREATE TABLE signing_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  public_key  TEXT NOT NULL,
  private_key TEXT NOT NULL,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Item type definitions
CREATE TABLE item_types (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK (kind IN ('CONTENT', 'USER', 'THREAD')),
  schema      JSONB NOT NULL,
  field_roles JSONB NOT NULL DEFAULT '{}',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

-- Policies
CREATE TABLE policies (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  description     TEXT,
  parent_id       TEXT REFERENCES policies(id),
  strike_penalty  INTEGER NOT NULL DEFAULT 0,
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- RULES: The core departure from coop-lite-go.
-- Instead of a JSONB condition_set, rules store Starlark source code.
-- Metadata (event_types, priority) is extracted from the Starlark globals
-- at compile time and stored as DERIVED/COMPUTED columns for query efficiency.
-- The Starlark source is the SINGLE SOURCE OF TRUTH for these values.
-- The database columns exist only for indexing and API query convenience.
CREATE TABLE rules (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  status          TEXT NOT NULL CHECK (status IN ('LIVE', 'BACKGROUND', 'DISABLED')),
  source          TEXT NOT NULL,              -- Starlark source code (SOURCE OF TRUTH)
  event_types     TEXT[] NOT NULL DEFAULT '{}', -- DERIVED: extracted from Starlark globals
  priority        INTEGER NOT NULL DEFAULT 0,   -- DERIVED: extracted from Starlark globals
  tags            TEXT[] NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Actions (referenced by name from Starlark verdict() calls)
CREATE TABLE actions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  action_type     TEXT NOT NULL CHECK (action_type IN ('WEBHOOK', 'ENQUEUE_TO_MRT')),
  config          JSONB NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

-- Generic entity history (replaces rules_history, actions_history, policies_history)
-- Stores full snapshots of any versioned entity as JSONB.
CREATE TABLE entity_history (
  id              TEXT NOT NULL,                -- entity ID
  entity_type     TEXT NOT NULL,                -- 'rule', 'action', 'policy'
  org_id          TEXT NOT NULL,
  version         INTEGER NOT NULL,
  snapshot        JSONB NOT NULL,               -- full entity state at this version
  valid_from      TIMESTAMPTZ NOT NULL,
  valid_to        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (entity_type, id, version)
);

-- Join tables (only 2 needed under the Starlark model)
-- rules_actions is ELIMINATED: actions are declared in verdict() calls within Starlark.
-- rules_item_types is ELIMINATED: rules declare event_types in Starlark;
--   item_type filtering uses event_type matching at the engine level.

CREATE TABLE rules_policies (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  policy_id   TEXT NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, policy_id)
);

CREATE TABLE actions_item_types (
  action_id    TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  item_type_id TEXT NOT NULL REFERENCES item_types(id) ON DELETE CASCADE,
  PRIMARY KEY (action_id, item_type_id)
);

-- Text banks
CREATE TABLE text_banks (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE text_bank_entries (
  id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  text_bank_id TEXT NOT NULL REFERENCES text_banks(id) ON DELETE CASCADE,
  value        TEXT NOT NULL,
  is_regex     BOOLEAN NOT NULL DEFAULT false,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Manual Review Tool
CREATE TABLE mrt_queues (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  description TEXT,
  is_default  BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

CREATE TABLE mrt_jobs (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  queue_id        TEXT NOT NULL REFERENCES mrt_queues(id),
  item_id         TEXT NOT NULL,
  item_type_id    TEXT NOT NULL REFERENCES item_types(id),
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL CHECK (status IN ('PENDING', 'ASSIGNED', 'DECIDED')) DEFAULT 'PENDING',
  assigned_to     TEXT REFERENCES users(id),
  policy_ids      TEXT[] NOT NULL DEFAULT '{}',
  enqueue_source  TEXT NOT NULL,
  source_info     JSONB NOT NULL DEFAULT '{}',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mrt_decisions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  job_id          TEXT NOT NULL REFERENCES mrt_jobs(id),
  user_id         TEXT NOT NULL REFERENCES users(id),
  verdict         TEXT NOT NULL,
  action_ids      TEXT[] NOT NULL DEFAULT '{}',
  policy_ids      TEXT[] NOT NULL DEFAULT '{}',
  reason          TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Execution logs (partitioned by month)
CREATE TABLE rule_executions (
  id              TEXT NOT NULL DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL,
  rule_id         TEXT NOT NULL,
  rule_version    INTEGER NOT NULL,
  item_id         TEXT NOT NULL,
  item_type_id    TEXT NOT NULL,
  verdict         TEXT,
  reason          TEXT,
  triggered_rules JSONB,
  latency_us      BIGINT,
  correlation_id  TEXT NOT NULL,
  executed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (executed_at, id)
) PARTITION BY RANGE (executed_at);

CREATE TABLE action_executions (
  id              TEXT NOT NULL DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL,
  action_id       TEXT NOT NULL,
  item_id         TEXT NOT NULL,
  item_type_id    TEXT NOT NULL,
  success         BOOLEAN NOT NULL,
  correlation_id  TEXT NOT NULL,
  executed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (executed_at, id)
) PARTITION BY RANGE (executed_at);

-- Items ledger
CREATE TABLE items (
  id              TEXT NOT NULL,
  org_id          TEXT NOT NULL,
  item_type_id    TEXT NOT NULL REFERENCES item_types(id),
  data            JSONB NOT NULL,
  submission_id   TEXT NOT NULL,
  creator_id      TEXT,
  creator_type_id TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, id, item_type_id, submission_id)
);

-- Sessions
CREATE TABLE sessions (
  sid     TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  data    JSONB NOT NULL DEFAULT '{}',
  expires_at  TIMESTAMPTZ NOT NULL
);

-- Indexes
CREATE INDEX idx_rules_org_status ON rules(org_id, status);
CREATE INDEX idx_rules_event_types ON rules USING GIN(event_types);
CREATE INDEX idx_mrt_jobs_queue_status ON mrt_jobs(queue_id, status);
CREATE INDEX idx_mrt_jobs_org_item ON mrt_jobs(org_id, item_id);
CREATE INDEX idx_rule_executions_org_rule ON rule_executions(org_id, rule_id, executed_at);
CREATE INDEX idx_rule_executions_org_time ON rule_executions(org_id, executed_at);
CREATE INDEX idx_action_executions_org_time ON action_executions(org_id, executed_at);
CREATE INDEX idx_items_org_id ON items(org_id, id, item_type_id);
CREATE INDEX idx_items_creator ON items(org_id, creator_id);
CREATE INDEX idx_actions_item_types_item ON actions_item_types(item_type_id);
CREATE INDEX idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
CREATE INDEX idx_entity_history_lookup ON entity_history(entity_type, id, valid_from);
CREATE INDEX idx_actions_org_name ON actions(org_id, name);
```

**Table count: 19** (down from 27).

Tables removed vs. original:
- `rules_history` -> merged into `entity_history`
- `actions_history` -> merged into `entity_history`
- `policies_history` -> merged into `entity_history`
- `rules_actions` -> eliminated (actions declared in Starlark `verdict()`)
- `rules_item_types` -> eliminated (event_types in Starlark handle filtering)
- `mrt_routing_rules` -> deferred (use `enqueue("queue-name")` UDF)
- `user_strikes` -> deferred to v1.1
- `reports` -> deferred to v1.1

### Why `event_types` and `priority` columns are kept

Despite being derived from Starlark source, these database columns serve two purposes:

1. **API query efficiency**: `GET /api/v1/rules?event_type=content` can use a GIN index rather than loading and parsing all Starlark sources.
2. **Snapshot bootstrap**: On cold start, the compiler re-extracts these from source. The DB columns are a cache that avoids recompilation for API queries.

The compiler is the authority. On every rule create/update, the compiler extracts `event_types` and `priority` from the Starlark source and writes them to the database. If someone manually edits the database columns (they should not), the next recompilation overwrites them.

---

## 7. Rule Engine -- The Heart of Nest

This is where fruitfly and coop-lite-go merge. The engine is fruitfly's architecture (worker pool, atomic snapshots, Starlark evaluation, UDFs) adapted for multi-tenancy and PostgreSQL-backed rule storage.

### 7.1 Rule Format

Every rule is a Starlark file stored as `source` TEXT in the `rules` table. Metadata is extracted from top-level globals at compile time.

```python
# Required globals (extracted at compile time, stored as derived DB columns)
rule_id = "spam-check-v1"
event_types = ["content", "comment"]  # or ["*"] for all event types
priority = 100                         # higher wins

# Required function
def evaluate(event):
    """Evaluate an event and return a verdict.

    Args:
        event: dict with keys: event_id, event_type, item_type, payload, org_id

    Returns:
        verdict("approve"|"block"|"review", reason="...", actions=[...])
    """
    text = event["payload"].get("text", "")

    # Built-in UDFs available:
    score = signal("openai-moderation", text)  # call a registered signal
    if score.score >= 0.9:
        return verdict("block", reason="AI: " + score.label,
                        actions=["webhook-alert", "enqueue-escalation"])

    if regex_match(r"buy\s+now\s+\d+%\s+off", text):
        return verdict("block", reason="spam pattern",
                        actions=["webhook-spam-log"])

    # Rate limiting
    user_id = event["payload"].get("user_id", "")
    if counter(user_id, event["event_type"], 3600) > 100:
        return verdict("review", reason="rate limit exceeded",
                        actions=["enqueue-rate-limit"])

    # Memoized expensive computation
    risk = memo("risk_score", lambda: compute_risk(event))
    if risk > 0.7:
        return verdict("review", reason="high risk score",
                        actions=["enqueue-manual-review"])

    return verdict("approve")

def compute_risk(event):
    # Custom logic -- this is just Starlark, any computation is fine
    payload = event["payload"]
    score = 0.0
    if payload.get("is_new_account", False):
        score += 0.3
    if len(payload.get("text", "")) < 10:
        score += 0.2
    if payload.get("has_links", False):
        score += 0.2
    return score
```

### event_types Wildcard Semantics

The `event_types` global supports a wildcard value `["*"]`:

- `event_types = ["content", "comment"]` -- rule matches only `content` and `comment` events.
- `event_types = ["*"]` -- rule matches ALL event types. The rule is included in every evaluation regardless of event type.
- The wildcard `"*"` must be the sole element: `["*"]`. Mixing wildcards with specific types (e.g., `["*", "content"]`) is a compile error.
- In the snapshot's `ByEvent` index, wildcard rules are stored under the key `"*"` and appended to every `RulesForEvent()` result.

### 7.2 Compilation and Snapshots

```go
// internal/engine/compiler.go

// Compiler compiles Starlark source into reusable Programs.
type Compiler struct{}

// CompileRule parses and compiles a single rule's Starlark source.
// Extracts metadata (rule_id, event_types, priority) from top-level globals.
// Returns a CompiledRule containing the Program and extracted metadata.
//
// Pre-conditions:
//   - source is valid Starlark with rule_id, event_types, priority globals
//   - source defines an evaluate(event) function
//   - if event_types contains "*", it must be the sole element
// Post-conditions:
//   - Returned CompiledRule.Program is reusable across evaluations
//   - Returned metadata matches the source globals exactly
// Errors:
//   - CompileError if Starlark syntax is invalid
//   - CompileError if required globals or evaluate function are missing
//   - CompileError if event_types mixes "*" with other types
func (c *Compiler) CompileRule(source string, filename string) (*CompiledRule, error)

// CompiledRule is a pre-compiled Starlark rule ready for evaluation.
type CompiledRule struct {
    ID         string
    EventTypes []string  // derived from Starlark source
    Priority   int       // derived from Starlark source
    Program    *starlark.Program
    Source     string
}
```

```go
// internal/engine/snapshot.go

// Snapshot is an immutable, pre-indexed collection of compiled rules for an org.
// Created atomically when rules change. Workers load via atomic.Pointer.
type Snapshot struct {
    ID        string                       // unique snapshot ID (UUIDv7 or timestamp)
    OrgID     string
    Rules     []*CompiledRule              // all rules, sorted by priority desc
    ByEvent   map[string][]*CompiledRule   // index: event_type -> rules (includes "*" key)
    LoadedAt  time.Time
}

// RulesForEvent returns compiled rules matching the given event type,
// including wildcard ("*") rules. Already sorted by priority (descending).
// Wildcard rules are merged into the result and sorted by priority alongside
// event-specific rules.
func (s *Snapshot) RulesForEvent(eventType string) []*CompiledRule
```

### 7.3 Worker Pool

```go
// internal/engine/pool.go

// Pool manages a set of workers that evaluate Starlark rules.
// Each worker owns its own Starlark thread, memo map, and counter shard.
//
// The pool holds per-org snapshots. The snapshots map is a sync.Map
// to support concurrent reads (hot path) and occasional writes (new org onboarding).
type Pool struct {
    workers    []*Worker
    snapshots  sync.Map                    // map[string]*atomic.Pointer[Snapshot]
    registry   *signal.Registry
    store      *store.Queries
    logger     *slog.Logger
    eventCh    chan EvalRequest            // bounded input channel
    resultCh   chan EvalResult             // bounded output channel
}

// NewPool creates a worker pool with the given worker count.
// Workers are started immediately.
func NewPool(
    workerCount int,
    registry *signal.Registry,
    store *store.Queries,
    logger *slog.Logger,
) *Pool

// Evaluate submits an event for rule evaluation.
// For sync mode: blocks until result is available.
// For async mode: called by river worker.
//
// Pre-conditions:
//   - event has been validated against its item type schema
//   - snapshot for event.OrgID has been loaded
// Post-conditions:
//   - All matching rules evaluated
//   - Execution logs written
//   - ActionRequests returned for all passing LIVE rules
func (p *Pool) Evaluate(ctx context.Context, event domain.Event) (*EvalResult, error)

// SwapSnapshot atomically replaces the rule snapshot for an org.
// If the org has no entry in the snapshots map yet, one is created.
// Uses sync.Map.LoadOrStore to handle concurrent first-access safely.
//
// This is the ONLY method that adds new orgs to the snapshots map.
// Called by service.RuleService after rule CRUD operations.
func (p *Pool) SwapSnapshot(orgID string, snap *Snapshot)

// CounterSum returns the sum of a counter across all workers.
// Used by the counter() UDF for cross-worker accurate counts.
func (p *Pool) CounterSum(orgID, entityID, eventType string, windowSeconds int) int64
```

#### Concurrency Design for `snapshots`

The original design used `map[string]*atomic.Pointer[Snapshot]`, which has a concurrency gap: adding a new org to a plain map requires a lock, but taking a lock on the hot path (every `Evaluate` call) defeats the lock-free design.

The solution uses `sync.Map`:

1. **Read path (hot, every evaluation)**: `sync.Map.Load(orgID)` returns `*atomic.Pointer[Snapshot]`. This is lock-free and wait-free on the read side.
2. **Write path (rare, on rule CRUD)**: `sync.Map.LoadOrStore(orgID, newAtomicPointer)` handles the race where two goroutines try to add the same org simultaneously. If the org already exists, the existing pointer is used and the snapshot is swapped atomically.
3. **No lock on the hot path**: `sync.Map` is optimized for the read-heavy, write-rare pattern. The evaluation pipeline remains lock-free.

```go
func (p *Pool) SwapSnapshot(orgID string, snap *Snapshot) {
    ptr := &atomic.Pointer[Snapshot]{}
    ptr.Store(snap)
    if existing, loaded := p.snapshots.LoadOrStore(orgID, ptr); loaded {
        // Org already existed; swap on the existing pointer.
        existing.(*atomic.Pointer[Snapshot]).Store(snap)
    }
    // If !loaded, our new ptr is already in the map with snap stored.
}

func (p *Pool) snapshotForOrg(orgID string) *Snapshot {
    if val, ok := p.snapshots.Load(orgID); ok {
        return val.(*atomic.Pointer[Snapshot]).Load()
    }
    return nil // no snapshot loaded for this org
}
```

### 7.4 Worker

```go
// internal/engine/worker.go

// Worker owns a Starlark thread and local state. Not shared between goroutines.
type Worker struct {
    id       int
    thread   *starlark.Thread
    memo     map[string]starlark.Value    // cleared between events
    counters map[counterKey]*atomic.Int64  // time-bucketed, per-worker
    evalCache map[string]starlark.Callable // rule_id -> cached evaluate fn
    lastSnap  string                       // snapshot ID, for cache invalidation
    pool     *Pool                         // back-reference for CounterSum
    logger   *slog.Logger
}

// processEvent evaluates all matching rules for a single event.
// Loads the snapshot once, evaluates rules in priority order, resolves verdict.
func (w *Worker) processEvent(ctx context.Context, event domain.Event) EvalResult
```

### 7.5 Built-in UDFs

| UDF | Signature in Starlark | Description |
|-----|----------------------|-------------|
| `verdict(type, reason="", actions=[])` | `verdict("block", reason="spam", actions=["webhook-1"])` | Return a verdict. Types: `approve`, `block`, `review`. `actions` is an optional list of action names to fire. |
| `signal(signal_id, text)` | `signal("openai-moderation", text)` | Call a registered signal adapter. Returns struct with `.score`, `.label`, `.metadata`. |
| `counter(entity_id, event_type, window_seconds)` | `counter("user:123", "post", 3600)` | Cross-worker in-memory counter. Returns count in time window. |
| `memo(key, func)` | `memo("score", lambda: expensive())` | Single-event memoization. Caches result per key within one event. |
| `log(message)` | `log("score=" + str(score))` | Structured log output from rule. Attached to evaluation result. |
| `now()` | `now()` | Current Unix timestamp (float). |
| `hash(value)` | `hash(email)` | SHA-256 hash of a string. |
| `regex_match(pattern, text)` | `regex_match(r"spam.*", text)` | RE2 regex match. Returns bool. |
| `enqueue(queue_name, reason)` | `enqueue("urgent", reason="needs review")` | Enqueue current item to an MRT queue. Returns bool. |

`lookup()` is intentionally excluded from v1.0. See section 2 (What Nest Drops) for rationale.

```go
// internal/engine/udf.go

// BuildUDFs constructs the predeclared Starlark dict for a worker.
// Called once at worker init. UDFs capture the worker reference for
// counter access, signal registry for signal(), and store for enqueue().
//
// The returned dict is stable for the worker's lifetime.
// Event-scoped state (memo map, current event) is passed via the
// Starlark thread's local storage, not via UDF closure.
func BuildUDFs(w *Worker) starlark.StringDict
```

### 7.6 verdict() Action Resolution

When a rule returns `verdict("block", actions=["webhook-alert", "enqueue-urgent"])`, the engine resolves action names to action definitions:

1. The `verdict()` UDF returns a Starlark struct containing the verdict type, reason, and action name list.
2. After all rules are evaluated and the final verdict is resolved, the engine collects all action names from the winning verdict.
3. Action names are resolved against the `actions` table (by `org_id` + `name`, using a UNIQUE index). Unresolved action names are logged as warnings but do not fail the evaluation.
4. Resolved actions are passed to the ActionPublisher.

This is strictly more powerful than the old `rules_actions` join table: different code paths in the same rule can trigger different action sets.

### 7.7 Signal UDF Bridge

The `signal()` UDF bridges fruitfly's Starlark world to coop-lite-go's signal adapter system:

```go
// internal/engine/udf_signal.go

// signalUDF implements the signal(signal_id, value) Starlark built-in.
// It looks up the signal adapter from the registry, calls Run(),
// and returns a Starlark struct with .score, .label, .metadata fields.
//
// Signal results are cached within a single event evaluation context
// to avoid redundant external API calls when multiple rules check the
// same signal on the same content.
func signalUDF(w *Worker) *starlark.Builtin
```

This is the critical design integration point. Signals are registered at startup (same as coop-lite-go) but invoked from Starlark (same as fruitfly UDFs). A rule author does not need to know whether a signal is built-in or an external HTTP call -- the `signal()` UDF abstracts it.

### 7.8 Verdict Resolution

Same as fruitfly. All matching rules for an event are evaluated. The final verdict is resolved:

1. Collect all successful rule verdicts.
2. Group by priority (highest first).
3. Among rules at the highest priority: resolve ties by verdict weight -- `block(3) > review(2) > approve(1)`.
4. If no rules returned a verdict: default to `approve`.

```go
// internal/engine/pool.go

// resolveVerdict determines the final verdict from multiple rule results.
// The winning verdict's actions list is used for action execution.
func resolveVerdict(results []ruleResult) domain.Verdict
```

### 7.9 Snapshot Lifecycle

```
Rule CRUD (POST/PUT/DELETE /api/v1/rules)
    |
    v
service.RuleService
    |-- Validate Starlark source (compile, check globals, validate event_types wildcard)
    |-- Extract event_types and priority from Starlark source (compiler is authority)
    |-- Persist to PostgreSQL (rules table + entity_history)
    |-- Write derived event_types and priority columns to rules table
    |-- Fetch all enabled rules for this org
    |-- Compile all rules into CompiledRule slice
    |-- Build new Snapshot (indexed by event_type, with wildcard rules under "*")
    |-- pool.SwapSnapshot(orgID, newSnapshot)
    |
    v
Workers see new rules on next event
(atomic.Pointer.Load())
```

This replaces fruitfly's fsnotify-based file watcher with API-driven snapshot rebuild. The rebuild is triggered synchronously on rule CRUD, so the caller knows the new rules are active before the API response returns. For bulk operations, a debounced river job can batch multiple changes into a single snapshot rebuild.

---

## 8. Data Flow

### Item Submission (Sync)

```
Client App
  |
  POST /api/v1/items { items: [...] }
  |
  APIKeyAuth middleware
  |
  handler.SubmitItemsSync(w, r)
  |   - Validate each item against item_type schema
  |   - Store items in items table
  |   - For each item:
  |       pool.Evaluate(ctx, event) -> EvalResult
  |           |
  |           - sync.Map.Load(orgID) -> *atomic.Pointer[Snapshot]
  |           - ptr.Load() -> Snapshot for orgID
  |           - snapshot.RulesForEvent(event.EventType)
  |             (includes wildcard "*" rules merged and sorted by priority)
  |           - For each matching rule (sorted by priority desc):
  |               - Init Starlark thread with UDFs
  |               - evaluate(event_dict) -> verdict (with optional actions list)
  |               - Collect verdict + metadata + action names
  |           - resolveVerdict(all_results)
  |           - Resolve action names -> Action definitions (org_id + name lookup)
  |           - Return EvalResult{Verdict, TriggeredRules, ActionRequests}
  |
  |   - actionPublisher.PublishActions(ctx, result.ActionRequests, target)
  |       |
  |       - WEBHOOK: http.Post(callbackURL, signedBody)
  |       - ENQUEUE_TO_MRT: store.InsertMRTJob(ctx, ...)
  |
  |   - store.LogRuleExecutions(ctx, executions)
  |   - store.LogActionExecutions(ctx, executions)
  |
  Return 200 { results: [{ item_id, verdict, triggered_rules, actions }] }
```

### Item Submission (Async)

Same flow, but `pool.Evaluate()` and action publishing happen in a river worker. HTTP returns 202 immediately.

### Manual Review Decision

```
Moderator UI
  |
  POST /api/v1/mrt/decisions { job_id, verdict, action_ids, policy_ids, reason }
  |
  SessionAuth + RequireRole(MODERATOR, ADMIN)
  |
  handler.RecordDecision(w, r)
  |   1. mrtService.RecordDecision(ctx, params) -> DecisionResult
  |   2. actionPublisher.PublishActions(ctx, result.ActionRequests, target)
  |
  Return 200 { decision_id, actions_executed }
```

---

## 9. Module Contracts

### 9.1 Domain Types

```go
// internal/domain/event.go

// Event is the input to the rule engine.
type Event struct {
    ID        string         `json:"event_id"`
    EventType string         `json:"event_type"`
    ItemType  string         `json:"item_type"`
    OrgID     string         `json:"org_id"`
    Payload   map[string]any `json:"payload"`
    Timestamp time.Time      `json:"timestamp"`
}
```

```go
// internal/domain/verdict.go

type VerdictType string

const (
    VerdictApprove VerdictType = "approve"
    VerdictBlock   VerdictType = "block"
    VerdictReview  VerdictType = "review"
)

// Verdict is the result of evaluating a single rule.
type Verdict struct {
    Type    VerdictType `json:"type"`
    Reason  string      `json:"reason,omitempty"`
    RuleID  string      `json:"rule_id"`
    Actions []string    `json:"actions,omitempty"` // action names declared in verdict()
}
```

```go
// internal/domain/rule.go

type RuleStatus string

const (
    RuleStatusLive       RuleStatus = "LIVE"
    RuleStatusBackground RuleStatus = "BACKGROUND"
    RuleStatusDisabled   RuleStatus = "DISABLED"
)

// Rule is the database representation of a Starlark rule.
// EventTypes and Priority are DERIVED from the Starlark source.
// The source field is the single source of truth.
type Rule struct {
    ID         string     `json:"id"`
    OrgID      string     `json:"org_id"`
    Name       string     `json:"name"`
    Status     RuleStatus `json:"status"`
    Source     string     `json:"source"`       // Starlark source code (SOURCE OF TRUTH)
    EventTypes []string   `json:"event_types"`  // DERIVED: extracted from Starlark globals
    Priority   int        `json:"priority"`     // DERIVED: extracted from Starlark globals
    Tags       []string   `json:"tags"`
    Version    int        `json:"version"`
    CreatedAt  time.Time  `json:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at"`
}
```

### 9.2 Signal Adapter Interface

Identical to coop-lite-go. The key difference is HOW signals are invoked: from Starlark via the `signal()` UDF, not from a condition evaluator.

```go
// internal/signal/adapter.go

type Adapter interface {
    ID() string
    DisplayName() string
    Description() string
    EligibleInputs() []domain.SignalInputType
    Cost() int
    Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error)
}
```

### 9.3 Rule Service

```go
// internal/service/rules.go

type RuleService struct {
    store    *store.Queries
    compiler *engine.Compiler
    pool     *engine.Pool
    logger   *slog.Logger
}

// CreateRule validates Starlark source, persists the rule, and rebuilds the snapshot.
// event_types and priority are extracted from the Starlark source by the compiler
// and written as derived columns. The caller does NOT supply these values.
//
// Pre-conditions:
//   - source compiles as valid Starlark
//   - source contains rule_id, event_types, priority globals
//   - source defines evaluate(event) function
//   - if event_types is ["*"], no other types are present
// Post-conditions:
//   - Rule persisted in rules table with derived event_types and priority
//   - Previous version written to entity_history
//   - Snapshot rebuilt and atomically swapped
//   - New rules are active for the next event evaluation
//
// Errors:
//   - ValidationError if Starlark compilation fails
//   - ValidationError if required globals are missing
//   - ValidationError if event_types wildcard is mixed with other types
func (s *RuleService) CreateRule(ctx context.Context, orgID string, params CreateRuleParams) (*domain.Rule, error)

// UpdateRule validates new source, saves old version to entity_history, rebuilds snapshot.
func (s *RuleService) UpdateRule(ctx context.Context, orgID, ruleID string, params UpdateRuleParams) (*domain.Rule, error)

// TestRule compiles and evaluates a rule against a sample event WITHOUT persisting.
// Used by the UI to preview rule behavior before saving.
func (s *RuleService) TestRule(ctx context.Context, orgID string, source string, event domain.Event) (*TestResult, error)
```

### 9.4 Config Service (Actions, Policies, Item Types)

```go
// internal/service/config.go

// ConfigService provides thin CRUD for actions, policies, and item types.
// These entities have simple create/read/update/delete semantics with
// version history tracked in the generic entity_history table.
type ConfigService struct {
    store  *store.Queries
    logger *slog.Logger
}

func (s *ConfigService) CreateAction(ctx context.Context, orgID string, params CreateActionParams) (*domain.Action, error)
func (s *ConfigService) UpdateAction(ctx context.Context, orgID, actionID string, params UpdateActionParams) (*domain.Action, error)
func (s *ConfigService) DeleteAction(ctx context.Context, orgID, actionID string) error
func (s *ConfigService) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error)

func (s *ConfigService) CreatePolicy(ctx context.Context, orgID string, params CreatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) UpdatePolicy(ctx context.Context, orgID, policyID string, params UpdatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) DeletePolicy(ctx context.Context, orgID, policyID string) error
func (s *ConfigService) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error)

func (s *ConfigService) CreateItemType(ctx context.Context, orgID string, params CreateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) UpdateItemType(ctx context.Context, orgID, itemTypeID string, params UpdateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error
func (s *ConfigService) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error)
```

### 9.5 Action Publisher

```go
// internal/engine/action_publisher.go

type Signer interface {
    Sign(ctx context.Context, orgID string, payload []byte) (string, error)
}

type ActionPublisher struct {
    store      *store.Queries
    signer     Signer
    httpClient *http.Client
    logger     *slog.Logger
}

// PublishActions executes actions concurrently. Never returns an error;
// individual failures are returned as ActionResult with Success=false.
func (p *ActionPublisher) PublishActions(
    ctx context.Context,
    actions []domain.ActionRequest,
    target ActionTarget,
) []domain.ActionResult
```

### 9.6 MRT Service

Same as coop-lite-go. `RecordDecision` returns `ActionRequest[]` for the handler to orchestrate. No circular dependency.

```go
// internal/service/mrt.go

type MRTService struct {
    store  *store.Queries
    logger *slog.Logger
}

func (s *MRTService) Enqueue(ctx context.Context, params EnqueueParams) (string, error)
func (s *MRTService) AssignNext(ctx context.Context, queueID, userID string) (*domain.MRTJob, error)
func (s *MRTService) RecordDecision(ctx context.Context, params DecisionParams) (*DecisionResult, error)
```

---

## 10. API Design (REST)

All endpoints prefixed with `/api/v1`. Same dual-auth model as coop-lite-go.

### External API (API key auth)

```
POST   /api/v1/items              # Submit items (sync evaluation)
POST   /api/v1/items/async        # Submit items (async, returns 202)
GET    /api/v1/policies           # List policies for org
```

### Internal API (Session auth)

```
# Rules -- includes Starlark source + test endpoint
GET    /api/v1/rules              # List rules
POST   /api/v1/rules              # Create rule (validates Starlark, rebuilds snapshot)
GET    /api/v1/rules/{id}         # Get rule detail (includes source)
PUT    /api/v1/rules/{id}         # Update rule (validates, rebuilds snapshot)
DELETE /api/v1/rules/{id}         # Delete rule (rebuilds snapshot)
POST   /api/v1/rules/test         # Test rule against sample event (no persist)
POST   /api/v1/rules/{id}/test    # Test existing rule against sample event

# Actions
GET    /api/v1/actions
POST   /api/v1/actions
GET    /api/v1/actions/{id}
PUT    /api/v1/actions/{id}
DELETE /api/v1/actions/{id}

# Policies
GET    /api/v1/policies
POST   /api/v1/policies
PUT    /api/v1/policies/{id}
DELETE /api/v1/policies/{id}

# Item Types
GET    /api/v1/item-types
POST   /api/v1/item-types
PUT    /api/v1/item-types/{id}
DELETE /api/v1/item-types/{id}

# MRT
GET    /api/v1/mrt/queues
GET    /api/v1/mrt/queues/{id}/jobs
POST   /api/v1/mrt/queues/{id}/assign
POST   /api/v1/mrt/decisions
GET    /api/v1/mrt/jobs/{id}

# Users
GET    /api/v1/users
POST   /api/v1/users/invite
PUT    /api/v1/users/{id}
DELETE /api/v1/users/{id}

# API Keys
GET    /api/v1/api-keys
POST   /api/v1/api-keys
DELETE /api/v1/api-keys/{id}

# Auth
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
GET    /api/v1/auth/me
POST   /api/v1/auth/reset-password

# Text Banks
GET    /api/v1/text-banks
POST   /api/v1/text-banks
GET    /api/v1/text-banks/{id}
POST   /api/v1/text-banks/{id}/entries
DELETE /api/v1/text-banks/{id}/entries/{entryId}

# Signals
GET    /api/v1/signals
POST   /api/v1/signals/test

# Signing Keys
GET    /api/v1/signing-keys
POST   /api/v1/signing-keys/rotate

# UDFs -- list available UDFs for rule authoring UI
GET    /api/v1/udfs               # List built-in UDFs with signatures and docs

# Health
GET    /api/v1/health
```

### Deferred endpoints (v1.1+)

```
# Analytics (v1.1)
GET    /api/v1/analytics/rule-executions
GET    /api/v1/analytics/action-executions
GET    /api/v1/analytics/queue-throughput

# Investigation (v1.1)
GET    /api/v1/investigation/items/{typeId}/{itemId}
GET    /api/v1/investigation/search

# Reports (v1.1)
POST   /api/v1/reports
```

### Frontend-Specific Considerations for React

The API is designed to support a React-based Python-or-TypeScript frontend:

1. **Rule Editor**: `GET /api/v1/rules/{id}` returns Starlark source. The frontend renders a code editor (CodeMirror or Monaco) with Starlark syntax highlighting. `POST /api/v1/rules/test` provides live preview.

2. **UDF Documentation**: `GET /api/v1/udfs` returns the list of available UDFs with their Starlark signatures, descriptions, and example usage. The frontend renders an autocomplete sidebar.

3. **Signal Picker**: `GET /api/v1/signals` returns available signals. The frontend suggests `signal("signal-id", ...)` snippets when authoring rules.

4. **OpenAPI Types**: `api/openapi.yaml` generates TypeScript types via `openapi-typescript`. These types are used by the React frontend for full type safety.

---

## 11. Concurrency Model

### Evaluation Pipeline (Fruitfly Heritage)

```
HTTP handler / river worker
        |
        v
  pool.Evaluate(ctx, event)
        |
        v
  sync.Map.Load(orgID) -> *atomic.Pointer[Snapshot]
  ptr.Load() -> Snapshot
        |
        v
  snapshot.RulesForEvent(eventType)
  (merges wildcard rules, sorted by priority)
        |
        +---> Worker goroutine (from pool)
        |       |
        |       +-- Starlark evaluate(event)
        |       |     |
        |       |     +-- UDFs: signal(), counter(), memo() ...
        |       |     |
        |       |     +-- verdict("block", actions=["webhook-1"])
        |       |
        |       +-- Return ruleResult (includes action names)
        |
  (all matching rules evaluated, bounded concurrency)
        |
        v
  resolveVerdict(results)
        |
        v
  Resolve action names -> Action definitions (org_id + name lookup)
        |
        v
  EvalResult{Verdict, TriggeredRules, ActionRequests}
```

### Lock Inventory

Same constraint as fruitfly: **zero `sync.Mutex` in the evaluation hot path.**

| Mechanism | Where | Purpose |
|-----------|-------|---------|
| `sync.Map` | Pool.snapshots | Org-to-snapshot-pointer map. Lock-free reads, rare writes. |
| `atomic.Pointer[Snapshot]` | Per org (inside sync.Map) | Atomic snapshot swap |
| `chan EvalRequest` | Pool | Bounded event input |
| `chan EvalResult` | Pool | Bounded result output |
| `atomic.Int64` | Worker counters | Per-worker counter increment |
| `sync.Map` | Signal cache per evaluation context | Per-event signal result caching |

`sync.RWMutex` is used only in:
- `engine.Cache` (not in the evaluation hot path -- used for action name resolution cache)
- `signal.Registry` (read-only after startup)

### Goroutine Budget

| Component | Count | Lifetime |
|-----------|-------|----------|
| HTTP server | Go stdlib (per-request) | Request duration |
| Worker pool | `runtime.NumCPU()` (default) | Application lifetime |
| river workers | Configurable (default 100) | Application lifetime |
| Webhook goroutines | 1 per action (short-lived) | ~100ms per webhook |
| Snapshot rebuilder | 1 per rebuild (short-lived) | ~10ms per rebuild |

---

## 12. Scalability Approach

### Vertical Scaling (Primary)

Nest is a single-binary, single-machine system like fruitfly. At the target scale (100-1000 events/second):

| Stage | Per-event cost | Capacity (8 cores) |
|-------|---------------|-------------------|
| JSON validation | ~10us | 100K evt/s |
| Snapshot load (sync.Map + atomic pointer) | ~5ns | Effectively infinite |
| Starlark evaluation (10 rules, cached eval fn) | ~5ms | 1,600 evt/s |
| Action name resolution (cached) | ~1us | Effectively infinite |
| PostgreSQL execution log (batched) | ~50us amortized | 20K evt/s |
| Webhook POST | ~5ms (async) | Goroutines handle it |

**Bottleneck**: Starlark evaluation, same as fruitfly. The eval cache (caching the `evaluate` callable per rule per worker) eliminates `Program.Init` on the hot path.

### Horizontal Scaling (When Needed)

If a single instance is insufficient:

1. **Stateless HTTP**: Run multiple Nest instances behind a load balancer. Each instance loads rules from PostgreSQL and maintains its own snapshot.
2. **Shared counters**: In-memory counters become per-instance. For exact cross-instance counters, switch `counter()` to query PostgreSQL (with TTL cache). This is a config flag, not a code change.
3. **river partitioning**: river supports multiple workers across instances with advisory lock-based coordination. Async item processing scales horizontally automatically.

### Memory Budget

| Component | Estimate |
|-----------|----------|
| Go runtime + binary | ~30MB |
| PostgreSQL connection pool (25 conns) | ~10MB |
| Rule snapshots (100 compiled rules, 10 orgs) | ~50MB |
| Per-worker state (8 workers) | ~5MB |
| Counter state (1 hour window) | ~20MB |
| In-memory cache (action name resolution) | ~2MB |
| **Total** | **~117MB typical** |

Significantly lighter than coop (Node.js ~80MB baseline + dependencies) and comparable to fruitfly (which added DuckDB at 256MB).

---

## 13. Design Decisions

### Decision 1: Starlark Over Condition Trees

**Decision**: All rules are Starlark scripts. The JSONB condition tree model from coop is eliminated entirely. No JSONB condition trees exist anywhere in the schema -- not in rules, not in routing rules, nowhere.

**Alternatives considered**:
- **Keep both**: Support both Starlark and condition trees. Rejected: two rule evaluation paths means two sets of bugs, two UIs, and a confusing developer experience. Violates principle 1 ("one way to do each thing").
- **Condition trees only**: Keep coop's model. Rejected: it cannot express rate limiting, custom risk scoring, conditional signal invocation, or any logic beyond threshold comparison.
- **CEL (Common Expression Language)**: Google's CEL is simpler than Starlark but less expressive. No UDFs, no function definitions, limited control flow. Rejected.

**Constraints**: `go.starlark.net` is the canonical Starlark implementation, battle-tested at Google/Bazel. Starlark is safe by default (no filesystem, no network, no system calls). UDFs are the controlled escape hatch for I/O (signals, counters, enqueue). CGO is NOT required -- `go.starlark.net` is pure Go.

### Decision 2: Rules in PostgreSQL, Not Files

**Decision**: Rules are stored in the `rules` table, managed via REST API. File-based loading is a dev-mode convenience only.

**Alternatives considered**:
- **File-only** (fruitfly model): Rejected for multi-tenant SaaS use cases. Cannot manage rules per-org via an API.
- **Git-backed**: Interesting but adds complexity (git as a dependency, webhook for change detection). Deferred. Could be added as an alternative rule source.

**Constraints**: Multi-tenancy requires per-org rule isolation. The API must support CRUD. PostgreSQL is already the single external dependency.

### Decision 3: Signals as UDFs, Not Condition Tree Nodes

**Decision**: Signals (OpenAI, text-regex, text-bank, etc.) are invoked via the `signal()` UDF inside Starlark, not as nodes in a condition tree.

**Alternatives considered**:
- **Signal-only evaluation** (no Starlark for signals): Signals are called outside Starlark and results injected. Rejected: loses the ability for rules to conditionally invoke signals (e.g., "only call OpenAI if the text is longer than 50 chars").

**Constraints**: Signals may be expensive (external API calls). Rules must be able to short-circuit signal invocation. The `signal()` UDF with per-event caching achieves this.

### Decision 4: sync.Map for Per-Org Snapshots

**Decision**: Each org has its own `atomic.Pointer[Snapshot]`, stored in a `sync.Map`. Snapshot rebuild is triggered by rule CRUD, not by a periodic poll.

**Alternatives considered**:
- **Plain map + mutex**: Simple but puts a lock on every hot-path read. Rejected.
- **Global snapshot** (fruitfly model): Single snapshot for all rules. Rejected: multi-tenant isolation requires per-org snapshots.
- **Periodic poll**: Poll PostgreSQL for rule changes every N seconds. Rejected: adds latency between rule save and activation. API-driven rebuild is immediate.
- **PostgreSQL LISTEN/NOTIFY**: Use PG notifications to trigger rebuild. Viable but adds complexity. API-driven is simpler and sufficient since all rule changes go through the API.

**Constraints**: The hot path (every evaluation) reads the snapshot. New orgs can be onboarded at any time. `sync.Map` is optimized for read-heavy, write-rare workloads and handles concurrent first-access via `LoadOrStore`.

### Decision 5: In-Memory Counters with Optional PostgreSQL Backing

**Decision**: Counters are in-memory (per-worker atomic, cross-worker sum) by default. A config flag enables PostgreSQL-backed counters for cross-instance accuracy.

**Alternatives considered**:
- **PostgreSQL-only counters**: Rejected for latency. A counter query on every rule evaluation adds ~1ms per counter call. At 10 counter calls per event, that is 10ms added to every evaluation.
- **Redis counters**: Rejected. Redis is an additional dependency.

**Constraints**: Single-instance deployments need fast counters. Multi-instance deployments need accurate counters. The abstraction supports both.

### Decision 6: Actions Declared in Starlark verdict()

**Decision**: Actions are declared in the `verdict()` call within Starlark rules, not in a `rules_actions` join table.

**Alternatives considered**:
- **Join table** (coop model): Static mapping of rules to actions. Rejected: cannot vary actions based on evaluation path within a single rule. A rule that blocks high-confidence content wants different actions than when it sends medium-confidence content to review.
- **Separate `actions` list in Starlark** (not inside verdict): Rejected. Actions are semantically tied to the verdict, not to the rule as a whole.

**Constraints**: Action names must reference existing entries in the `actions` table. Unknown action names are logged as warnings, not errors, to avoid breaking evaluation when an action is renamed or deleted.

### Decision 7: Single Generic Entity History Table

**Decision**: One `entity_history` table with JSONB snapshots replaces three separate `*_history` tables.

**Alternatives considered**:
- **Per-entity history tables** (coop model): Separate `rules_history`, `actions_history`, `policies_history`. Rejected: three tables with nearly identical schemas. Adding a new versioned entity requires a new migration.
- **Event sourcing**: Full event log. Rejected: overkill for simple "keep old versions" use case.

**Constraints**: History queries must be efficient. The composite primary key `(entity_type, id, version)` with an index on `(entity_type, id, valid_from)` supports efficient lookups. The JSONB snapshot stores the full entity state, which is slightly denormalized but eliminates the need to maintain parallel column sets.

---

## 14. Package Dependency DAG

```
cmd/server/main.go
  |
  |-- internal/config
  |-- internal/domain         (zero internal imports)
  |-- internal/store          --> domain
  |-- internal/auth           --> domain, store
  |-- internal/signal         --> domain
  |-- internal/engine         --> domain, store, signal (NO service import)
  |-- internal/service        --> domain, store, engine
  |-- internal/worker         --> domain, store, engine, service
  |-- internal/handler        --> domain, service, engine, auth
```

No cycles. `domain` is the leaf. `handler` and `cmd/server` are the roots. Go's compiler enforces this: circular imports are a compile error.

**Package count: 9** (down from 11). `cache/` merged into `engine/`, `crypto/` merged into `auth/`.

---

## 15. Invariants

1. **Every database query includes `org_id` in its WHERE clause.** Multi-tenant isolation is non-negotiable.

2. **No circular dependencies between packages.** The DAG in section 14 is enforced by the Go compiler.

3. **Zero `sync.Mutex` in the evaluation hot path.** All synchronization via `sync.Map`, `atomic.Pointer`, `atomic.Int64`, and channels.

4. **Starlark rule evaluation never panics.** Errors in individual rules are recovered and logged. The pool continues processing.

5. **All rules compile before activation.** A rule with invalid Starlark is rejected at API time with a 400 error. It never reaches the snapshot.

6. **Snapshot swap is atomic.** Workers never see a partial snapshot. They load the pointer once per event.

7. **`domain` package has zero imports from other `internal/` packages.** It is the dependency leaf.

8. **ActionPublisher and MRTService have no direct dependency.** The handler orchestrates their interaction.

9. **Entity history is append-only.** Old versions of rules, actions, and policies are preserved in the generic `entity_history` table.

10. **API keys are never stored in plaintext.** Only SHA-256 hashes.

11. **`context.Context` is the first parameter of every function that performs I/O.**

12. **All errors are returned, never swallowed.** `_` on an error return is prohibited except in deferred Close calls.

13. **Signal results are cached within a single event evaluation.** The same signal with the same input is never called twice for the same event.

14. **In-memory counters are eventually consistent across workers.** The `CounterSum` reads atomic values and may see slightly stale counts. This is acceptable for rate-limiting use cases.

15. **Starlark source is the single source of truth for `event_types` and `priority`.** Database columns are derived values, written by the compiler on every create/update.

16. **No JSONB condition trees anywhere in the schema.** The "one rule format" principle is absolute.

17. **CGO is not required.** All production dependencies are pure Go.

---

## 16. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Starlark execution too slow for high rule counts | Medium | Medium | Eval cache eliminates Program.Init on hot path. Per-worker parallelism. At 100 rules with caching, expect ~5ms per event on 8 cores. |
| Starlark sandbox escape | Very Low | Critical | `go-starlark` is designed for sandboxing. No filesystem, no network, no system calls. UDFs are the only I/O path and are controlled by us. |
| Malicious Starlark (infinite loop) | Low | Medium | Per-rule timeout (1s default) via `context.WithTimeout`. Per-event timeout (5s). Starlark thread cancellation via context. |
| Per-org snapshot memory at high org count | Low | Medium | Each snapshot is ~50KB per 100 rules. At 1000 orgs with 100 rules each: ~50MB. Acceptable. Lazy-load snapshots for inactive orgs if needed. |
| Rule authors write bad Starlark | Medium | Low | `POST /api/v1/rules/test` validates before save. Compilation catches syntax errors. Runtime errors are caught and logged per-rule, never crash the system. |
| PostgreSQL analytics too slow at scale | Medium | Medium | Execution logs partitioned by month. Analytics service deferred to v1.1 -- by then, data volume is known and optimization can be targeted. |
| Counter accuracy in multi-instance | Medium | Low | Config flag for PostgreSQL-backed counters. Documentation clearly states in-memory counters are per-instance. |
| Frontend Starlark editing UX is poor | Medium | Medium | Monaco editor with Starlark syntax highlighting. UDF autocomplete from `/api/v1/udfs`. Live testing from `/api/v1/rules/test`. Template rules for common patterns. |
| Migration from coop condition trees to Starlark | Medium | Medium | Write a one-time converter: JSONB condition tree -> Starlark source. The mapping is mechanical. Ship it as a CLI tool. |
| Starlark version drift breaks rule behavior | Low | High | Pin `go.starlark.net` to exact version. Starlark conformance test suite must pass before upgrades. See section 4 (Starlark Version Pinning). |
| Unknown action names in verdict() calls | Medium | Low | Logged as warnings, not errors. Evaluation succeeds. Action resolution is best-effort. Admin UI can show a "broken references" report. |
| JSONB entity_history grows large | Low | Low | JSONB snapshots are typically 1-5KB. Partitioning or archival policy can be added if needed. History is read rarely (audit use case). |

---

## 17. Estimated Size

| Component | Lines of Code |
|-----------|--------------|
| `domain/` (types, errors) | ~500 |
| `engine/` (pool, worker, snapshot, compiler, UDFs, action publisher, cache) | ~1,300 |
| `signal/` (interface, registry, 2 builtins, HTTP adapter) | ~400 |
| `handler/` (~11 handler files) | ~1,200 |
| `service/` (rules, config, mrt, items, users, api_keys, signing_keys, text_banks) | ~1,500 |
| `store/` (database queries) | ~1,000 |
| `auth/` (middleware, passwords, sessions, RBAC, signing, hashing) | ~500 |
| `worker/` (process_item, maintenance) | ~200 |
| `config/` | ~80 |
| `cmd/server/main.go` | ~200 |
| SQL migrations | ~250 |
| **Production total** | **~7,100** |
| Tests (~1:1 ratio) | ~7,100 |
| **Grand total** | **~14,200** |

For reference:
- Original coop: ~70,000+ lines (server + client)
- Coop-lite TypeScript: ~12,150 lines estimated
- Coop-lite Go: ~15,800 lines estimated (including tests)
- Fruitfly: ~3,000 lines estimated (single-purpose, no CRUD/API/MRT)
- **Nest: ~14,200 lines estimated** (fruitfly expressiveness + coop-lite-go features + tests)

The production code (~7,100 lines) is modest. The engine package (~1,300 lines) is the most complex component, carrying fruitfly's concurrency model and Starlark integration. Everything else is standard CRUD and infrastructure.

---

## 18. Implementation Order

### Phase 1: Foundation (Week 1-2)

1. Project scaffold (`go mod init`, directory structure, Makefile, Dockerfile)
2. `domain/` package (all types including Event, Rule with Source, Verdict with Actions)
3. `config/` package
4. `store/` package (pgxpool connection, migration runner)
5. SQL migrations (full v1.0 schema -- 19 tables)
6. `auth/` package (bcrypt, sessions, middleware, RBAC, signing, hashing)
7. `handler/helpers.go` (JSON, Decode, Error)
8. Health check, auth endpoints, user management
9. Org setup seed script

**Milestone**: Login, session management, user CRUD.

### Phase 2: Rule Engine Core (Week 3-4)

10. `engine/compiler.go` -- Starlark compilation + metadata extraction + wildcard validation
11. `engine/snapshot.go` -- immutable snapshot with event-type index (including wildcard merge)
12. `engine/worker.go` -- single worker with Starlark thread
13. `engine/udf.go` -- verdict() (with actions param), now(), hash(), regex_match(), log(), memo()
14. `engine/pool.go` -- worker pool with sync.Map + atomic snapshot swap
15. `service/rules.go` -- rule CRUD with compilation validation + derived column extraction + snapshot rebuild
16. Rule CRUD handler endpoints
17. `POST /api/v1/rules/test` -- test rule against sample event (no persist)

**Milestone**: Can create Starlark rules via API, compile them, test them against sample events.

### Phase 3: Signals + Evaluation (Week 5-6)

18. `signal/` package (Adapter interface, Registry, TextRegex, TextBank)
19. `engine/udf_signal.go` -- signal() UDF bridging Starlark to signal adapters
20. `engine/udf_counter.go` -- in-memory counters with cross-worker sum
21. `engine/action_publisher.go` -- webhook (with action name resolution), MRT enqueue
22. Item submission endpoints (sync + async)
23. river worker: ProcessItemWorker
24. End-to-end: submit item -> evaluate rules -> resolve action names -> fire actions

**Milestone**: Full automated evaluation loop. Items in, verdicts out, actions fired.

### Phase 4: CRUD + MRT (Week 7-8)

25. `service/config.go` -- Action CRUD, Policy CRUD, Item Type CRUD (with entity_history)
26. Text bank CRUD
27. API key management, signing key management
28. MRT service (enqueue, assign, decide)
29. MRT handler endpoints
30. `enqueue()` UDF for Starlark rules

**Milestone**: Full platform. CRUD for all entities. MRT workflow. Human review loop.

### Phase 5: Signals + Polish (Week 9-10)

31. HTTP signal adapter (generic, for OpenAI/Google/custom)
32. Signal listing and testing endpoints
33. `GET /api/v1/udfs` endpoint
34. river periodic workers (partition manager, session cleanup, counter flush -- all in maintenance.go)
35. OpenAPI spec, TypeScript type generation
36. Integration tests, benchmarks
37. Starlark conformance test suite (for version pinning validation)

**Milestone**: Production-ready v1.0. Full rule engine + CRUD + MRT.

### Phase 6: Frontend Support (Week 11-12)

38. Starlark editor component guidance (Monaco with Starlark syntax)
39. UDF autocomplete data from `/api/v1/udfs`
40. Rule template library (common patterns as starter Starlark)
41. Condition-tree-to-Starlark converter CLI (migration tool)
42. Load testing, documentation

**Total: ~12 weeks for 1-2 developers.**

### Deferred to v1.1+

- Analytics service and endpoints
- Investigation service and endpoints
- Reports service and endpoints
- User strikes tracking
- MRT routing rules (Starlark-based or `enqueue()` UDF covers most use cases)
- `lookup()` UDF (requires full specification of schema, caching, error behavior)

---

## 19. What This Looks Like In Practice

### Creating a rule via API

```bash
curl -X POST /api/v1/rules \
  -H "Cookie: session=..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Spam Detection v2",
    "status": "LIVE",
    "policy_ids": ["spam-policy"],
    "tags": ["spam", "automated"],
    "source": "rule_id = \"spam-v2\"\nevent_types = [\"content\"]\npriority = 100\n\ndef evaluate(event):\n    text = event[\"payload\"].get(\"text\", \"\")\n    if len(text) < 5:\n        return verdict(\"approve\")\n    ai = signal(\"openai-moderation\", text)\n    if ai.score >= 0.85:\n        return verdict(\"block\", reason=\"AI: \" + ai.label, actions=[\"webhook-alert\"])\n    if counter(event[\"payload\"][\"user_id\"], \"content\", 3600) > 50:\n        return verdict(\"review\", reason=\"rate limit\", actions=[\"enqueue-manual-review\"])\n    return verdict(\"approve\")"
  }'
```

Note: `event_types` and `priority` are NOT in the JSON request body. They are extracted from the Starlark source by the compiler and stored as derived columns. `action_ids` and `item_type_ids` are also NOT in the request -- actions are declared in `verdict()` calls within the Starlark source, and item type filtering uses `event_types`.

### Testing a rule before saving

```bash
curl -X POST /api/v1/rules/test \
  -H "Cookie: session=..." \
  -H "Content-Type: application/json" \
  -d '{
    "source": "rule_id = \"test\"\nevent_types = [\"*\"]\npriority = 0\n\ndef evaluate(event):\n    if event[\"payload\"].get(\"spam_score\", 0) > 0.9:\n        return verdict(\"block\", reason=\"high spam score\", actions=[\"webhook-spam\"])\n    return verdict(\"approve\")",
    "event": {
      "event_id": "test-1",
      "event_type": "content",
      "payload": {"spam_score": 0.95, "text": "Buy now!!!"}
    }
  }'
```

Response:
```json
{
  "verdict": "block",
  "reason": "high spam score",
  "rule_id": "test",
  "actions": ["webhook-spam"],
  "latency_us": 234,
  "logs": []
}
```

### Adding a new signal

Same as coop-lite-go: implement `signal.Adapter`, register in `main.go`. The signal becomes immediately available to all Starlark rules via `signal("new-signal-id", input)`. Zero engine changes.

### Adding a new UDF

1. Add a Go function that returns a `*starlark.Builtin` in `engine/udf.go` (or a new `engine/udf_*.go` file for complex UDFs).
2. Register it in `engine/udf.go`'s `BuildUDFs`.
3. Add documentation to the `/api/v1/udfs` endpoint.

The UDF is immediately available to all Starlark rules.

### MRT routing without routing rules

In v1.0, MRT routing is handled by Starlark rules directly via the `enqueue()` UDF:

```python
rule_id = "route-hate-speech-to-escalation"
event_types = ["content"]
priority = 50

def evaluate(event):
    ai = signal("openai-moderation", event["payload"]["text"])
    if ai.score >= 0.7 and ai.label == "hate":
        enqueue("escalation-queue", reason="AI flagged hate speech")
        return verdict("review", reason="hate speech detected",
                        actions=["webhook-log"])
    return verdict("approve")
```

This is more powerful than JSONB routing rules because the routing logic has full access to the event payload, signals, counters, and all other UDFs.

---

## 20. Validation Criteria

Concrete, testable statements for verifying this design:

1. **Table count**: The v1.0 schema has exactly 19 tables.
2. **No JSONB condition trees**: No table in the schema contains a `condition_set` column of type JSONB.
3. **No `rules_actions` join table**: Actions are declared in Starlark `verdict()` calls, not in a join table.
4. **Go file count**: v1.0 has ~55-60 Go source files (excluding tests).
5. **Package count**: 9 packages under `internal/` (config, domain, store, auth, signal, engine, service, worker, handler).
6. **No CGO**: `go build` succeeds with `CGO_ENABLED=0`.
7. **Starlark source is truth**: Modifying `event_types` or `priority` directly in the database has no effect on evaluation until the next rule update triggers recompilation.
8. **Wildcard semantics**: A rule with `event_types = ["*"]` is evaluated for every event type. A rule with `event_types = ["*", "content"]` fails compilation.
9. **sync.Map concurrency**: Two goroutines simultaneously calling `SwapSnapshot` for a new org do not race. The second call swaps on the pointer created by the first.
10. **Entity history**: Creating a rule, updating it, and querying `entity_history` returns both versions with correct `valid_from`/`valid_to` timestamps.
11. **Action resolution**: A verdict with `actions=["nonexistent"]` logs a warning but does not fail the evaluation.
12. **Zero `sync.Mutex` on hot path**: The evaluation pipeline uses only `sync.Map.Load`, `atomic.Pointer.Load`, `atomic.Int64`, and channels.
13. **Starlark version pinned**: `go.mod` contains an exact version for `go.starlark.net`, not a range.
