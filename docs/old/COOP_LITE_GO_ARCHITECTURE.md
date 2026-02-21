# Coop Lite -- Go Architecture

A radically simplified content moderation platform, rewritten in Go. Same domain, same PostgreSQL schema, 10% of the code, 10x the throughput.

---

## 1. Design Philosophy

Four rules govern every decision in this document:

1. **One way to do each thing.** No dual ORMs, no dual routers, no dual logging. Pick one, use it everywhere.
2. **PostgreSQL does the work.** If Postgres can handle it (queues, sessions, analytics, full-text search, JSON storage), do not add another system.
3. **Interfaces at boundaries, not everywhere.** Only abstract where you genuinely expect to swap implementations. The rule engine will always evaluate rules. Do not over-abstract the interior.
4. **Exploit the language.** Go gives us goroutines, channels, static typing without generics gymnastics, fast compilation, single-binary deployment, and a rich standard library. Use them. Do not write Java-in-Go or Node-in-Go.

### Why Go

The TypeScript Coop Lite architecture is sound. The Go rewrite preserves its design decisions (PostgreSQL-only, minimal dependencies, interface-driven boundaries, no DI framework) while gaining:

- **Single binary deployment.** No `node_modules`, no runtime version management. `COPY coop /usr/local/bin/` in a scratch Docker image.
- **Goroutine-based concurrency.** Parallel signal evaluation within a single rule, parallel rule evaluation across rules, all without callback complexity.
- **Lower memory footprint.** A Go process serving HTTP uses ~10MB RSS vs ~80MB+ for Node.js.
- **Compile-time safety.** No `any` leaks, no runtime type assertions. The type system catches more at build time.
- **RE2 regex engine.** Go's `regexp` package uses RE2 by default. No ReDoS risk. Ever.
- **Predictable latency.** No event loop stalls from large JSON parsing or GC pauses from millions of small heap objects.

---

## 2. What Gets Cut (and Why)

### Eliminated Infrastructure (same as TypeScript version)

| Component | Reason |
|-----------|--------|
| **Kafka** | PostgreSQL LISTEN/NOTIFY + river handles our throughput. |
| **Scylla/Cassandra** | Investigation data in PostgreSQL. |
| **ClickHouse** | PostgreSQL with partitioned tables and materialized views. |
| **Redis** | Sessions in PostgreSQL. In-memory TTL cache (sync.RWMutex + map) for hot caches. `sync.Map` used only for per-request signal caching in the evaluation context. No BullMQ -- river replaces it. |
| **Snowflake** | Eliminated. Export via `pg_dump` if needed. |
| **Schema Registry** | No Kafka, no schema registry. Go structs + JSON tags are the schema. |
| **HMA (Python service)** | Deferred. Signal adapter interface supports it later. |
| **Content Proxy** | Deferred. CSP headers + sandboxed iframes. |
| **OpenTelemetry** | Deferred. `slog` structured logging to stdout. Add OTel when needed. |

### Eliminated Libraries/Patterns

| What | Go Replacement |
|------|----------------|
| Sequelize + Kysely (dual ORM) | **pgx/v5 direct queries** (hand-written SQL or sqlc-generated) |
| Bottle.js IoC container (1,878 lines) | **Plain struct construction** in a single `main.go` composition root |
| Apollo Server + Apollo Client (GraphQL) | **REST API** with chi router |
| Express/Hono middleware chain | **chi middleware** + stdlib `net/http` handlers |
| Zod runtime validation | **Go struct tags** + custom validation functions (compile-time types are the primary validation) |
| bcryptjs | **`golang.org/x/crypto/bcrypt`** |
| jsonwebtoken | **`crypto/rand` + `crypto/sha256`** for opaque tokens |
| uuid | **PostgreSQL `gen_random_uuid()`** for all IDs |
| pino / custom logger | **`log/slog`** (stdlib, Go 1.21+) |
| openai SDK | **`net/http`** for external signal HTTP calls |
| date-fns | **`time` package** (stdlib) |
| lodash | **Go stdlib** (`slices`, `maps`, `sort`) |
| pg-boss | **river** (Go-native PostgreSQL job queue) |

### Eliminated Features (Deferred, Not Deleted)

Same as TypeScript version:

| Feature | Rationale |
|---------|-----------|
| NCMEC integration | Specialized compliance. Plugin later. |
| SSO/SAML | Enterprise auth. Interface exists; plug in later. |
| Hash banks / HMA | Perceptual hashing. Signal plugin system. |
| Backtesting & retroactions | Complex. v2 after core is stable. |
| ML model tracking | External ML platform. |
| Rule anomaly detection | v2 analytics. |
| Aggregation signals | Complex stateful signals. v2. |
| Location banks / geo-containment | Niche signal. Plugin. |
| Snowflake ingestion | No Snowflake. |
| GDPR deletion endpoints | Add when compliance requires it. |
| Fuzzy text matching | v1.1. |

---

## 3. What Gets Kept

### CORE (v1.0)

- Multi-tenant organizations
- User authentication (email/password) with RBAC (Admin, Moderator, Analyst)
- Item types with custom schemas (Content, User, Thread)
- Rule engine with nested AND/OR condition trees
- Built-in signals: text-regex (subsumes text-contains), text-bank
- External signal adapter interface (for OpenAI, Google, custom HTTP)
- Actions: webhook callback (with RSA-PSS signing), enqueue to review queue, enqueue author to review queue
- Policies (flat list with optional hierarchy)
- Manual Review Tool: queues, job assignment, compound decisions (verdict + actions + policies)
- API key management for REST API
- Webhook signing (RSA-PSS)
- Temporal versioning (history tables for rules, actions, policies)
- Reports intake with default MRT queue routing

### IMPORTANT (v1.1)

- Basic analytics dashboard (rule execution counts, action counts over time)
- Item investigation (lookup items by ID, view history)
- User strikes system
- Queue routing rules
- Bulk actioning in MRT
- CSV export for reports
- Fuzzy text matching signal
- Rate limiting for external API endpoints

---

## 4. Technology Choices

| Layer | Choice | Why |
|-------|--------|-----|
| Language | **Go 1.23+** | Single binary, goroutines, rich stdlib, fast compilation. |
| HTTP router | **chi v5** | Lightweight, stdlib-compatible, middleware support, route groups. ~1 dependency. |
| Database driver | **pgx/v5** | Fastest pure-Go PostgreSQL driver. Direct access to COPY, LISTEN/NOTIFY, connection pooling, JSONB. No ORM overhead. |
| Job queue | **river** | PostgreSQL-native job queue written in Go. Uses pgx. Advisory locks, unique jobs, periodic jobs. Go equivalent of pg-boss. |
| Password hashing | **golang.org/x/crypto/bcrypt** | Standard bcrypt implementation for Go. |
| Structured logging | **log/slog** (stdlib) | Structured JSON logging built into Go 1.21+. No external dependency. |
| Regex | **regexp** (stdlib) | RE2-safe. No ReDoS. No external dependency. |
| JSON | **encoding/json** (stdlib) | JSONB handling, API serialization. Consider `github.com/goccy/go-json` if benchmarks demand it. |
| Crypto | **crypto/\*** (stdlib) | RSA-PSS (`crypto/rsa`), SHA-256 (`crypto/sha256`), random tokens (`crypto/rand`). |
| HTTP client | **net/http** (stdlib) | For webhook delivery, external signal calls. |
| Testing | **testing** (stdlib) | `go test`, table-driven tests, `httptest` for handler tests. No test framework needed. |
| Context/cancellation | **context** (stdlib) | Threaded through every database call, HTTP handler, and signal evaluation. |
| Configuration | **Environment variables** | Parsed into a typed `Config` struct at startup. No config library needed. |
| Database | **PostgreSQL 16** | Only external runtime dependency. Handles everything. |
| OpenAPI | **ogen** or hand-written spec | Generate TypeScript client types from OpenAPI YAML. One source of truth for client/server contract. |

---

## 5. Directory Structure

```
coop-lite/
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
|-- internal/                     # All application code (unexported to external modules)
|   |-- config/
|   |   |-- config.go            # Config struct, env parsing, validation
|   |
|   |-- domain/                   # Pure domain types (no dependencies on infrastructure)
|   |   |-- item.go              # ItemType, Field, ScalarType, ValidatedItem
|   |   |-- rule.go              # Rule, ConditionSet, LeafCondition, Conjunction
|   |   |-- action.go            # Action, ActionType, ActionRequest, ActionResult
|   |   |-- policy.go            # Policy types
|   |   |-- signal.go            # SignalInput, SignalOutput, SignalInputType
|   |   |-- mrt.go               # MRTJob, MRTDecision, MRTQueue, Verdict
|   |   |-- user.go              # User, Role, Session
|   |   |-- org.go               # Org, OrgSettings
|   |   |-- report.go            # Report types
|   |   |-- errors.go            # Domain error types (NotFound, Forbidden, Conflict, etc.)
|   |   |-- pagination.go        # PaginatedResult, PageParams
|   |
|   |-- auth/
|   |   |-- middleware.go         # Session auth + API key auth chi middleware
|   |   |-- passwords.go         # bcrypt hash/verify
|   |   |-- sessions.go          # PostgreSQL-backed session store
|   |   |-- rbac.go              # Role-based access control check
|   |   |-- context.go           # Auth context keys and helpers
|   |
|   |-- engine/
|   |   |-- rule_engine.go       # Top-level: run enabled rules for item
|   |   |-- rule_engine_test.go
|   |   |-- evaluator.go         # Evaluate single rule's condition tree
|   |   |-- evaluator_test.go
|   |   |-- condition_set.go     # Recursive AND/OR evaluation with short-circuit
|   |   |-- condition_set_test.go
|   |   |-- action_publisher.go  # Execute actions (webhook, enqueue MRT)
|   |   |-- action_publisher_test.go
|   |
|   |-- signal/
|   |   |-- adapter.go           # SignalAdapter interface
|   |   |-- registry.go          # Signal registry (register, lookup, list)
|   |   |-- registry_test.go
|   |   |-- text_regex.go        # Built-in: regex signal (subsumes text-contains)
|   |   |-- text_regex_test.go
|   |   |-- text_bank.go         # Built-in: text bank signal
|   |   |-- text_bank_test.go
|   |   |-- http_signal.go       # Generic HTTP signal adapter (OpenAI, Google, custom)
|   |   |-- http_signal_test.go
|   |
|   |-- service/
|   |   |-- moderation_config.go # Rules, actions, policies, item types CRUD
|   |   |-- mrt.go               # Manual review: enqueue, assign, decide
|   |   |-- item_processing.go   # Validate + normalize incoming items
|   |   |-- user_management.go   # User CRUD, invites, password reset
|   |   |-- api_keys.go          # API key create, verify, rotate
|   |   |-- signing_keys.go      # RSA-PSS keypair management
|   |   |-- analytics.go         # Query execution/action logs for dashboards
|   |   |-- text_banks.go        # Text/regex bank management
|   |   |-- user_strikes.go      # Strike tracking and threshold enforcement
|   |   |-- investigation.go     # Item lookup by ID/type
|   |   |-- reports.go           # Report intake, default queue routing
|   |
|   |-- store/                    # Database access layer (pgx queries)
|   |   |-- db.go                # pgxpool.Pool wrapper, transaction helper
|   |   |-- orgs.go              # Org queries
|   |   |-- users.go             # User queries
|   |   |-- rules.go             # Rule queries (with history)
|   |   |-- actions.go           # Action queries (with history)
|   |   |-- policies.go          # Policy queries (with history)
|   |   |-- item_types.go        # Item type queries
|   |   |-- items.go             # Item ledger queries
|   |   |-- mrt.go               # MRT queue/job/decision queries
|   |   |-- text_banks.go        # Text bank queries
|   |   |-- sessions.go          # Session queries
|   |   |-- api_keys.go          # API key queries
|   |   |-- signing_keys.go      # Signing key queries
|   |   |-- executions.go        # Rule/action execution log queries
|   |   |-- user_strikes.go      # User strike queries
|   |   |-- reports.go           # Report queries
|   |   |-- migrations/          # SQL migration files
|   |       |-- 001_initial.sql
|   |       |-- ...
|   |
|   |-- handler/                  # HTTP handlers (chi route groups)
|   |   |-- items.go             # POST /api/v1/items (sync + async)
|   |   |-- rules.go             # CRUD rules
|   |   |-- actions.go           # CRUD actions
|   |   |-- policies.go          # CRUD policies
|   |   |-- item_types.go        # CRUD item types
|   |   |-- mrt.go               # MRT: queues, jobs, decisions
|   |   |-- users.go             # User management
|   |   |-- orgs.go              # Org settings
|   |   |-- api_keys.go          # API key management
|   |   |-- reports.go           # User reports (intake -> default MRT queue)
|   |   |-- analytics.go         # Dashboard data endpoints
|   |   |-- signals.go           # Signal listing, testing
|   |   |-- text_banks.go        # Text bank management
|   |   |-- investigation.go     # Item investigation
|   |   |-- auth.go              # Login, logout, me, password reset
|   |   |-- health.go            # Health check
|   |   |-- helpers.go           # JSON encode/decode, error response helpers
|   |
|   |-- worker/                   # Background job workers (river)
|   |   |-- process_item.go      # Async item processing worker
|   |   |-- refresh_views.go     # Refresh materialized views periodically
|   |   |-- partition_manager.go # Create future partitions, archive old ones
|   |   |-- session_cleanup.go   # Delete expired sessions
|   |
|   |-- cache/
|   |   |-- cache.go             # Generic TTL in-memory cache using sync.RWMutex + map
|   |   |-- cache_test.go
|   |
|   |-- crypto/
|       |-- signing.go           # RSA-PSS webhook signing
|       |-- hashing.go           # SHA-256, API key hashing, token generation
|       |-- hashing_test.go
|
|-- api/
|   |-- openapi.yaml             # OpenAPI 3.1 specification (source of truth)
|
|-- migrations/
|   |-- 001_initial.sql          # Full schema DDL
|   |-- 002_partitions.sql       # Initial partition creation
|
|-- Dockerfile                    # Multi-stage build -> scratch image
|-- Makefile                      # build, test, lint, migrate, generate targets
```

**File count estimate: ~65 Go source files** (vs. hundreds in the original TypeScript).

> **Design Decision: `internal/` package.**
> All application code lives under `internal/`. This is Go convention: the `internal` directory prevents external Go modules from importing our packages. It is not a restrictive pattern -- it is defensive. The only public API is the HTTP server and the CLI.

> **Design Decision: `store/` instead of repository pattern.**
> The `store` package contains direct pgx queries. No repository interfaces. pgx is already an interface (`pgxpool.Pool` satisfies an implicit interface), and we do not plan to swap PostgreSQL. If testing requires mocking the database, use `pgxpool` with a test database or use the store functions with a transaction that rolls back.

> **Design Decision: `domain/` is pure types.**
> The `domain` package has zero imports from any other internal package. It defines the language of the system. Every other package imports from `domain`, never the reverse. This is the one invariant that prevents import cycles.

---

## 6. Data Model

### PostgreSQL Schema (single `public` schema)

The schema is identical to the TypeScript Coop Lite architecture. Go uses `pgx` to interact with it directly. Key notes for Go:

- All `TEXT` primary keys map to Go `string` fields
- `JSONB` columns map to `json.RawMessage` (for pass-through) or typed Go structs (for condition sets, schemas)
- `TIMESTAMPTZ` maps to `time.Time`
- `TEXT[]` arrays map to `[]string` (pgx handles PostgreSQL arrays natively)
- `BOOLEAN` maps to `bool`
- `INTEGER` maps to `int`

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
  password    TEXT NOT NULL,  -- bcrypt hash
  role        TEXT NOT NULL CHECK (role IN ('ADMIN', 'MODERATOR', 'ANALYST')),
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, email)
);

-- Password reset tokens (opaque token, not JWT)
CREATE TABLE password_reset_tokens (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL,  -- SHA-256 hash of the token
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- API keys for REST API access
CREATE TABLE api_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  key_hash    TEXT NOT NULL,  -- SHA-256 hash of the API key
  prefix      TEXT NOT NULL,  -- First 8 chars for identification
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at  TIMESTAMPTZ
);

-- RSA-PSS signing keypairs for webhooks
CREATE TABLE signing_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  public_key  TEXT NOT NULL,
  private_key TEXT NOT NULL,  -- Encrypted at rest via application-level encryption
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Item type definitions (Content, User, Thread)
CREATE TABLE item_types (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK (kind IN ('CONTENT', 'USER', 'THREAD')),
  schema      JSONB NOT NULL,       -- Array of Field definitions
  field_roles JSONB NOT NULL DEFAULT '{}', -- Maps role names to field names
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, name)
);

-- Policies (violation categories)
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

CREATE TABLE policies_history (
  id              TEXT NOT NULL,
  org_id          TEXT NOT NULL,
  name            TEXT NOT NULL,
  description     TEXT,
  parent_id       TEXT,
  strike_penalty  INTEGER NOT NULL,
  version         INTEGER NOT NULL,
  valid_from      TIMESTAMPTZ NOT NULL,
  valid_to        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, version)
);

-- Rules
CREATE TABLE rules (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  status          TEXT NOT NULL CHECK (status IN ('LIVE', 'BACKGROUND', 'DISABLED')),
  condition_set   JSONB NOT NULL,     -- Nested ConditionSet tree
  tags            TEXT[] NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE rules_history (
  id              TEXT NOT NULL,
  org_id          TEXT NOT NULL,
  name            TEXT NOT NULL,
  status          TEXT NOT NULL,
  condition_set   JSONB NOT NULL,
  tags            TEXT[] NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL,
  valid_from      TIMESTAMPTZ NOT NULL,
  valid_to        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, version)
);

-- Actions
CREATE TABLE actions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  action_type     TEXT NOT NULL CHECK (action_type IN ('WEBHOOK', 'ENQUEUE_TO_MRT', 'ENQUEUE_AUTHOR_TO_MRT')),
  config          JSONB NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE actions_history (
  id              TEXT NOT NULL,
  org_id          TEXT NOT NULL,
  name            TEXT NOT NULL,
  action_type     TEXT NOT NULL,
  config          JSONB NOT NULL,
  version         INTEGER NOT NULL,
  valid_from      TIMESTAMPTZ NOT NULL,
  valid_to        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, version)
);

-- Join tables
CREATE TABLE rules_actions (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  action_id   TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, action_id)
);

CREATE TABLE rules_policies (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  policy_id   TEXT NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, policy_id)
);

CREATE TABLE rules_item_types (
  rule_id      TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  item_type_id TEXT NOT NULL REFERENCES item_types(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, item_type_id)
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
  enqueue_source  TEXT NOT NULL,         -- 'RULE_EXECUTION', 'REPORT', 'MANUAL'
  source_info     JSONB NOT NULL DEFAULT '{}',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mrt_decisions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  job_id          TEXT NOT NULL REFERENCES mrt_jobs(id),
  user_id         TEXT NOT NULL REFERENCES users(id),
  verdict         TEXT NOT NULL,   -- 'APPROVE', 'REJECT', 'ESCALATE', 'IGNORE'
  action_ids      TEXT[] NOT NULL DEFAULT '{}',
  policy_ids      TEXT[] NOT NULL DEFAULT '{}',
  reason          TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mrt_routing_rules (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  queue_id        TEXT NOT NULL REFERENCES mrt_queues(id),
  condition_set   JSONB NOT NULL,
  priority        INTEGER NOT NULL DEFAULT 0,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User strikes
CREATE TABLE user_strikes (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  user_item_id    TEXT NOT NULL,
  user_type_id    TEXT NOT NULL,
  strike_count    INTEGER NOT NULL DEFAULT 0,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, user_item_id, user_type_id)
);

-- Execution logs (partitioned by month)
CREATE TABLE rule_executions (
  id              TEXT NOT NULL DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL,
  rule_id         TEXT NOT NULL,
  rule_version    INTEGER NOT NULL,
  item_id         TEXT NOT NULL,
  item_type_id    TEXT NOT NULL,
  passed          BOOLEAN NOT NULL,
  environment     TEXT NOT NULL,
  condition_results JSONB,
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

-- Reports (first-class entity: tracks user reports before they become MRT jobs)
CREATE TABLE reports (
  id                    TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id                TEXT NOT NULL REFERENCES orgs(id),
  reported_item_id      TEXT NOT NULL,
  reported_item_type_id TEXT NOT NULL REFERENCES item_types(id),
  reporter_item_id      TEXT,            -- Optional: who filed the report
  reason                TEXT NOT NULL,
  metadata              JSONB NOT NULL DEFAULT '{}',
  mrt_job_id            TEXT REFERENCES mrt_jobs(id),  -- The resulting MRT job, if created
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reports_org ON reports(org_id, created_at);
CREATE INDEX idx_reports_item ON reports(org_id, reported_item_id);

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
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Indexes
CREATE INDEX idx_rules_org_status ON rules(org_id, status);
CREATE INDEX idx_rules_item_types_item ON rules_item_types(item_type_id);
CREATE INDEX idx_mrt_jobs_queue_status ON mrt_jobs(queue_id, status);
CREATE INDEX idx_mrt_jobs_org_item ON mrt_jobs(org_id, item_id);
CREATE INDEX idx_rule_executions_org_rule ON rule_executions(org_id, rule_id, executed_at);
CREATE INDEX idx_rule_executions_org_time ON rule_executions(org_id, executed_at);
CREATE INDEX idx_action_executions_org_time ON action_executions(org_id, executed_at);
CREATE INDEX idx_items_org_id ON items(org_id, id, item_type_id);
CREATE INDEX idx_items_creator ON items(org_id, creator_id);
CREATE INDEX idx_actions_item_types_item ON actions_item_types(item_type_id);
CREATE INDEX idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);

-- Initial partitions for execution log tables.
-- The migration MUST create current month + next month partitions to avoid
-- insert failures before the partition_manager worker runs.
-- Example (generated dynamically by migration runner based on current date):
--   CREATE TABLE rule_executions_2026_02 PARTITION OF rule_executions
--     FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
--   CREATE TABLE rule_executions_2026_03 PARTITION OF rule_executions
--     FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
--   (same for action_executions)
-- The partition_manager river worker handles future months going forward.

-- river uses its own schema for job queue tables (created automatically)
```

**Table count: 26** (25 from TypeScript version + `reports` table).

---

## 7. Module Contracts

### 7.0 Domain Types

```go
// internal/domain/signal.go

// SignalInputType represents the type of data a signal can process.
type SignalInputType string

const (
    SignalInputText     SignalInputType = "TEXT"
    SignalInputImage    SignalInputType = "IMAGE"
    SignalInputVideo    SignalInputType = "VIDEO"
    SignalInputAudio    SignalInputType = "AUDIO"
    SignalInputFullItem SignalInputType = "FULL_ITEM"
)

// SignalValue is the typed union for signal input data.
// Exactly one field is set, determined by SignalInput.Type.
// This replaces a bare `any` field to prevent runtime type assertion errors
// and make the contract explicit at the type level.
type SignalValue struct {
    Text string         `json:"text,omitempty"` // Set when Type is TEXT
    Item map[string]any `json:"item,omitempty"` // Set when Type is FULL_ITEM (or IMAGE/VIDEO/AUDIO with URL in map)
}

// SignalInput is the data passed to a signal adapter for evaluation.
type SignalInput struct {
    Type           SignalInputType `json:"type"`             // Discriminator: which field of Value is set
    Value          SignalValue     `json:"value"`            // Typed union: Text or Item
    MatchingValues []string        `json:"matching_values"`  // For bank-based signals
    OrgID          string          `json:"org_id"`
}

// SignalOutput is the result of evaluating a signal.
type SignalOutput struct {
    Score       float64        `json:"score"`                  // 0.0 to 1.0 (normalized)
    Label       string         `json:"label,omitempty"`        // Optional classification label
    Subcategory string         `json:"subcategory,omitempty"`  // Optional subcategory ID
    Metadata    map[string]any `json:"metadata,omitempty"`
}
```

```go
// internal/domain/rule.go

// Conjunction defines how conditions in a set are combined.
type Conjunction string

const (
    ConjunctionAND Conjunction = "AND"
    ConjunctionOR  Conjunction = "OR"
)

// RuleStatus defines the execution mode of a rule.
type RuleStatus string

const (
    RuleStatusLive       RuleStatus = "LIVE"
    RuleStatusBackground RuleStatus = "BACKGROUND"
    RuleStatusDisabled   RuleStatus = "DISABLED"
)

// ConditionSet is a recursive tree of conditions joined by AND or OR.
type ConditionSet struct {
    Conjunction Conjunction   `json:"conjunction"`
    Conditions  []Condition   `json:"conditions"`
}

// Condition is either a LeafCondition or a nested ConditionSet.
// Discriminated by the presence of the "conjunction" field in JSON.
type Condition struct {
    // If Conjunction is set, this is a nested ConditionSet.
    Conjunction *Conjunction   `json:"conjunction,omitempty"`
    Conditions  []Condition    `json:"conditions,omitempty"`

    // If Signal is set, this is a LeafCondition.
    Signal      *SignalRef     `json:"signal,omitempty"`
    Field       *FieldRef      `json:"field,omitempty"`
    Threshold   *Threshold     `json:"threshold,omitempty"`
    Subcategory string         `json:"subcategory,omitempty"`
    Config      map[string]any `json:"config,omitempty"`
}

// IsConditionSet returns true if this Condition is a nested set.
func (c *Condition) IsConditionSet() bool {
    return c.Conjunction != nil
}

// SignalRef identifies which signal to evaluate.
type SignalRef struct {
    ID string `json:"id"`
}

// FieldRef identifies which field on the item to evaluate.
type FieldRef struct {
    Name string          `json:"name"`
    Type SignalInputType `json:"type"`
}

// Threshold defines the comparison for a leaf condition.
type Threshold struct {
    Operator string  `json:"operator"` // ">=", "<=", ">", "<", "==", "!="
    Value    float64 `json:"value"`
}

// Rule is the core domain entity for automated moderation.
type Rule struct {
    ID           string       `json:"id"`
    OrgID        string       `json:"org_id"`
    Name         string       `json:"name"`
    Status       RuleStatus   `json:"status"`
    ConditionSet ConditionSet `json:"condition_set"`
    Tags         []string     `json:"tags"`
    Version      int          `json:"version"`
    CreatedAt    time.Time    `json:"created_at"`
    UpdatedAt    time.Time    `json:"updated_at"`
}

// ConditionOutcome represents the result of evaluating a condition.
type ConditionOutcome string

const (
    OutcomePassed  ConditionOutcome = "PASSED"
    OutcomeFailed  ConditionOutcome = "FAILED"
    OutcomeErrored ConditionOutcome = "ERRORED"
)

// ConditionSetResult is the result of evaluating a full condition set.
type ConditionSetResult struct {
    Conjunction Conjunction            `json:"conjunction"`
    Outcome     ConditionOutcome       `json:"outcome"`
    Conditions  []ConditionResult      `json:"conditions"`
}

// ConditionResult is the result of evaluating a single condition (leaf or set).
type ConditionResult struct {
    // For nested sets
    Conjunction *Conjunction       `json:"conjunction,omitempty"`
    Outcome     ConditionOutcome   `json:"outcome"`
    Conditions  []ConditionResult  `json:"conditions,omitempty"`

    // For leaf conditions
    SignalID    string             `json:"signal_id,omitempty"`
    Score       *float64           `json:"score,omitempty"`
}
```

```go
// internal/domain/action.go

// ActionType defines what happens when a rule matches.
type ActionType string

const (
    ActionTypeWebhook          ActionType = "WEBHOOK"
    ActionTypeEnqueueToMRT     ActionType = "ENQUEUE_TO_MRT"
    ActionTypeEnqueueAuthorMRT ActionType = "ENQUEUE_AUTHOR_TO_MRT"
)

// Action is a configured response to a rule match.
type Action struct {
    ID         string         `json:"id"`
    OrgID      string         `json:"org_id"`
    Name       string         `json:"name"`
    ActionType ActionType     `json:"action_type"`
    Config     map[string]any `json:"config"`
    Version    int            `json:"version"`
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
}

// ActionRequest is a request to execute an action, produced by the rule engine.
type ActionRequest struct {
    ActionID   string     `json:"action_id"`
    ActionType ActionType `json:"action_type"`
    Config     map[string]any `json:"config"`
    PolicyIDs  []string   `json:"policy_ids"`
    RuleIDs    []string   `json:"rule_ids"`
}

// ActionResult is the outcome of executing a single action.
type ActionResult struct {
    ActionID string `json:"action_id"`
    Success  bool   `json:"success"`
    Error    string `json:"error,omitempty"`
}
```

```go
// internal/domain/mrt.go

// Verdict is the moderator's decision on a review job.
type Verdict string

const (
    VerdictApprove  Verdict = "APPROVE"
    VerdictReject   Verdict = "REJECT"
    VerdictEscalate Verdict = "ESCALATE"
    VerdictIgnore   Verdict = "IGNORE"
)

// JobStatus tracks the lifecycle of a review job.
type JobStatus string

const (
    JobStatusPending  JobStatus = "PENDING"
    JobStatusAssigned JobStatus = "ASSIGNED"
    JobStatusDecided  JobStatus = "DECIDED"
)

// EnqueueSource identifies how a job entered the review queue.
type EnqueueSource string

const (
    EnqueueSourceRuleExecution EnqueueSource = "RULE_EXECUTION"
    EnqueueSourceReport        EnqueueSource = "REPORT"
    EnqueueSourceManual        EnqueueSource = "MANUAL"
)
```

```go
// internal/domain/user.go

// Role defines the access level of a user within an organization.
type Role string

const (
    RoleAdmin     Role = "ADMIN"
    RoleModerator Role = "MODERATOR"
    RoleAnalyst   Role = "ANALYST"
)

// User represents an operator of the moderation platform.
type User struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    Role      Role      `json:"role"`
    IsActive  bool      `json:"is_active"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

```go
// internal/domain/errors.go

// Domain error types. These are checked by HTTP handlers to produce
// appropriate status codes. They implement the error interface.

// NotFoundError indicates a requested resource does not exist.
type NotFoundError struct {
    Resource string
    ID       string
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s %s not found", e.Resource, e.ID)
}

// ForbiddenError indicates insufficient permissions.
type ForbiddenError struct {
    Message string
}

func (e *ForbiddenError) Error() string { return e.Message }

// ConflictError indicates a uniqueness constraint violation.
type ConflictError struct {
    Message string
}

func (e *ConflictError) Error() string { return e.Message }

// ValidationError indicates invalid input data.
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// ConfigError indicates a system misconfiguration.
type ConfigError struct {
    Message string
}

func (e *ConfigError) Error() string { return e.Message }
```

### 7.1 Signal Adapter Interface

This is the most important interface in the system. Every signal -- built-in or external -- implements this.

```go
// internal/signal/adapter.go

// Adapter is the interface that all signal implementations must satisfy.
// Built-in signals (text-regex, text-bank) and external signals (HTTP-based)
// both implement this single interface.
//
// New signals are added by implementing Adapter and registering with the Registry.
// Zero changes to the rule engine required.
type Adapter interface {
    // ID returns the unique identifier for this signal (e.g., "text-regex", "openai-moderation").
    ID() string

    // DisplayName returns a human-readable name for UI display.
    DisplayName() string

    // Description returns a brief description of what this signal detects.
    Description() string

    // EligibleInputs returns the set of input types this signal can process.
    EligibleInputs() []domain.SignalInputType

    // Cost returns a relative cost value used for evaluation ordering.
    // 0 = free/instant (regex, bank lookup), higher = more expensive (external API calls).
    // The condition evaluator runs cheapest signals first to enable short-circuit optimization.
    Cost() int

    // Run evaluates the signal against the given input and returns a result.
    // The context carries cancellation, timeout, and org-scoped metadata.
    //
    // Pre-conditions:
    //   - ctx is non-nil and carries a deadline or cancellation
    //   - input.Value is compatible with one of EligibleInputs()
    //
    // Post-conditions:
    //   - SignalOutput.Score is in range [0.0, 1.0]
    //   - Returns error only for infrastructure failures (network, timeout)
    //   - Signal-level validation errors (bad input type) return score 0.0, not error
    Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error)
}
```

```go
// internal/signal/registry.go

// Registry manages the set of available signal adapters.
// Thread-safe for concurrent reads after initial registration at startup.
type Registry struct {
    mu       sync.RWMutex
    adapters map[string]Adapter
}

// NewRegistry creates an empty signal registry.
func NewRegistry() *Registry

// Register adds a signal adapter. Panics if ID is already registered.
// Called only during application startup (composition root).
func (r *Registry) Register(adapter Adapter)

// Get returns the adapter for the given signal ID, or an error if not found.
func (r *Registry) Get(id string) (Adapter, error)

// List returns all registered adapters, sorted by ID.
func (r *Registry) List() []Adapter
```

### 7.2 Rule Engine

```go
// internal/engine/rule_engine.go

// RuleEngineResult contains the outcome of running all enabled rules for an item.
type RuleEngineResult struct {
    ActionsTriggered []domain.ActionRequest
    RuleResults      map[string]RuleResult // keyed by rule ID
}

// RuleResult is the outcome of evaluating a single rule.
type RuleResult struct {
    Passed           bool
    ConditionResults domain.ConditionSetResult
}

// RuleEngine runs all enabled rules for an item type against a given item.
// It is the top-level orchestrator for automated moderation.
type RuleEngine struct {
    store    *store.Queries
    registry *signal.Registry
    cache    *cache.Cache
    logger   *slog.Logger
}

// NewRuleEngine creates a RuleEngine with its dependencies.
func NewRuleEngine(
    store *store.Queries,
    registry *signal.Registry,
    cache *cache.Cache,
    logger *slog.Logger,
) *RuleEngine

// RunEnabledRules evaluates all enabled rules for the given item.
//
// Pre-conditions:
//   - item has been validated against its item type schema
//   - orgID is a valid organization
//
// Post-conditions:
//   - All LIVE rules for the item type have been evaluated
//   - BACKGROUND rules evaluated but no actions included in result
//   - Rule executions logged to rule_executions table
//   - Returned ActionRequest slice contains deduplicated actions from all passing LIVE rules
//
// Concurrency: Rules are evaluated concurrently using goroutines. Signal results
// are cached within a single evaluation context to avoid redundant calls.
// Each rule's condition tree is evaluated sequentially (cost-ordered, short-circuit),
// but multiple rules run in parallel.
func (e *RuleEngine) RunEnabledRules(
    ctx context.Context,
    orgID string,
    item domain.ValidatedItem,
) (*RuleEngineResult, error)
```

### 7.3 Condition Evaluator

```go
// internal/engine/condition_set.go

// EvaluationContext carries the data needed to evaluate conditions.
// It is created once per item evaluation and shared across all rules.
type EvaluationContext struct {
    OrgID    string
    Item     domain.ValidatedItem
    Registry *signal.Registry
    Logger   *slog.Logger

    // signalCache stores results of signal evaluations to avoid redundant calls.
    // Key format uses null byte separator to avoid collision when field values
    // contain colons: "signalID\x00fieldName\x00sha256(fieldValue)"
    // The fieldValue is hashed to keep keys bounded in length.
    // Value: SignalOutput
    signalCache sync.Map
}

// EvaluateConditionSet recursively evaluates a condition set with:
//   - Cost-based ordering (cheapest signals first)
//   - Short-circuit evaluation (AND stops on first false, OR on first true)
//   - Three-valued logic (ERRORED propagates as null)
//
// This function never returns an error. Errors in leaf conditions are caught
// and marked ERRORED. The condition set evaluation continues.
//
// Concurrency: Leaf conditions within a single condition set are evaluated
// sequentially (to enable short-circuit). Separate condition sets (at different
// branches of the tree) may be evaluated concurrently by the caller.
func EvaluateConditionSet(
    ctx context.Context,
    conditionSet domain.ConditionSet,
    evalCtx *EvaluationContext,
) domain.ConditionSetResult
```

```go
// internal/engine/evaluator.go

// evaluateLeafCondition evaluates a single leaf condition by running its signal.
//
// Returns PASSED if signal score satisfies threshold, FAILED otherwise.
// Returns ERRORED if signal execution fails.
// Never panics. Never returns error -- errors become ERRORED outcome.
func evaluateLeafCondition(
    ctx context.Context,
    condition domain.Condition,
    evalCtx *EvaluationContext,
) domain.ConditionResult

// compareThreshold checks if a signal score satisfies a threshold condition.
func compareThreshold(score float64, threshold domain.Threshold) bool
```

### 7.4 Action Publisher

```go
// internal/engine/action_publisher.go

// ActionTarget identifies the item and org that actions are being executed against.
type ActionTarget struct {
    OrgID      string
    ItemID     string
    ItemTypeID string
    Item       domain.ValidatedItem
}

// Signer abstracts webhook payload signing. Defined in the engine package
// so that ActionPublisher does not depend on internal/service directly.
// The service.SigningKeyService implements this interface.
type Signer interface {
    // Sign signs a payload using the org's active key.
    // Returns the base64-encoded signature.
    Sign(ctx context.Context, orgID string, payload []byte) (string, error)
}

// ActionPublisher executes actions triggered by rule evaluation or MRT decisions.
// It handles webhook delivery, MRT enqueue, and author-to-MRT enqueue.
type ActionPublisher struct {
    store      *store.Queries
    signer     Signer          // was: *service.SigningKeyService (direct dependency removed)
    httpClient *http.Client
    logger     *slog.Logger
}

// NewActionPublisher creates an ActionPublisher with its dependencies.
// The signer parameter accepts any implementation of the Signer interface;
// in production this is *service.SigningKeyService.
func NewActionPublisher(
    store *store.Queries,
    signer Signer,
    httpClient *http.Client,
    logger *slog.Logger,
) *ActionPublisher

// PublishActions executes a set of action requests against a target item.
//
// For WEBHOOK actions: POST to callback URL with RSA-PSS signed body.
// For ENQUEUE_TO_MRT actions: create MRT job in the appropriate queue.
// For ENQUEUE_AUTHOR_TO_MRT actions: resolve author from item, create MRT job.
//
// Actions execute concurrently. Individual action failures are logged and returned
// as ActionResult with Success=false. This function never returns an error --
// all failures are per-action.
//
// Pre-conditions:
//   - actions have been validated against actions_item_types
// Post-conditions:
//   - Each action either succeeded or has a logged failure
//   - MRT jobs created for ENQUEUE actions
//   - action_executions logged for all actions
func (p *ActionPublisher) PublishActions(
    ctx context.Context,
    actions []domain.ActionRequest,
    target ActionTarget,
) []domain.ActionResult
```

### 7.5 MRT Service

```go
// internal/service/mrt.go

// DecisionParams contains the parameters for recording a moderator's decision.
type DecisionParams struct {
    OrgID     string
    JobID     string
    UserID    string
    Verdict   domain.Verdict
    ActionIDs []string
    PolicyIDs []string
    Reason    string
}

// DecisionResult contains the outcome of recording a decision.
type DecisionResult struct {
    DecisionID     string
    ActionRequests []domain.ActionRequest
}

// EnqueueParams contains the parameters for adding an item to the review queue.
type EnqueueParams struct {
    OrgID        string
    ItemID       string
    ItemTypeID   string
    Payload      map[string]any
    Source       domain.EnqueueSource
    SourceInfo   map[string]any
    PolicyIDs    []string
    QueueID      string // Optional: if empty, routes via routing rules or default queue
}

// MRTService manages the Manual Review Tool: enqueueing items for human review,
// assigning jobs to moderators, and recording decisions.
type MRTService struct {
    store  *store.Queries
    logger *slog.Logger
}

// NewMRTService creates an MRTService.
func NewMRTService(store *store.Queries, logger *slog.Logger) *MRTService

// Enqueue adds an item to a review queue.
// Routes to the correct queue via routing rules, falls back to the org's default queue.
//
// Returns the MRT job ID.
func (s *MRTService) Enqueue(ctx context.Context, params EnqueueParams) (string, error)

// AssignNext gets the next unassigned job from a queue and assigns it to the user.
// Returns nil if no pending jobs exist.
//
// Uses SELECT ... FOR UPDATE SKIP LOCKED for safe concurrent assignment.
func (s *MRTService) AssignNext(ctx context.Context, queueID, userID string) (*domain.MRTJob, error)

// RecordDecision records a moderator's decision on an assigned job.
//
// Returns ActionRequests for any actions the decision triggers.
// Does NOT call ActionPublisher -- the route handler orchestrates.
// This breaks the MRTService <-> ActionPublisher circular dependency.
//
// Pre-conditions:
//   - job exists, is ASSIGNED, and is assigned to userID
//   - actionIDs reference valid actions for this org
//   - policyIDs reference valid policies for this org
// Post-conditions:
//   - mrt_jobs.status updated to 'DECIDED'
//   - mrt_decisions row inserted
//   - Returned ActionRequests ready for ActionPublisher
func (s *MRTService) RecordDecision(ctx context.Context, params DecisionParams) (*DecisionResult, error)

// GetQueueStats returns statistics (pending, assigned, decided counts) for all queues in an org.
func (s *MRTService) GetQueueStats(ctx context.Context, orgID string) ([]QueueStats, error)

// GetJobs returns jobs for a queue with filtering and pagination.
func (s *MRTService) GetJobs(ctx context.Context, queueID string, filters JobFilters) (*domain.PaginatedResult[domain.MRTJob], error)
```

### 7.6 Reports Service

```go
// internal/service/reports.go

// ReportParams contains the parameters for submitting a user report.
type ReportParams struct {
    OrgID              string
    ReportedItemID     string
    ReportedItemTypeID string
    ReporterItemID     string // Optional
    Reason             string
    Metadata           map[string]any
}

// ReportsService handles user report intake and routing to review queues.
type ReportsService struct {
    store  *store.Queries
    mrt    *MRTService
    logger *slog.Logger
}

// NewReportsService creates a ReportsService.
func NewReportsService(store *store.Queries, mrt *MRTService, logger *slog.Logger) *ReportsService

// SubmitReport intakes a user report, persists it to the reports table,
// and enqueues it to the org's default MRT queue.
//
// Pre-conditions:
//   - org has at least one MRT queue with is_default = true
//   - reportedItemID + reportedItemTypeID reference a valid item type
// Post-conditions:
//   - Row inserted into reports table with mrt_job_id set
//   - MRT job created with enqueue_source = 'REPORT'
//   - source_info contains reporter details, report reason, and report ID
//
// Returns the report ID and MRT job ID.
// Returns *domain.ConfigError if no default MRT queue exists.
func (s *ReportsService) SubmitReport(ctx context.Context, params ReportParams) (reportID string, jobID string, err error)

// GetReport returns a report by ID.
func (s *ReportsService) GetReport(ctx context.Context, orgID, reportID string) (*domain.Report, error)

// ListReports returns reports for an org with pagination.
func (s *ReportsService) ListReports(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Report], error)
```

### 7.7 Auth Middleware

```go
// internal/auth/middleware.go

// SessionAuth returns chi middleware that validates session cookies.
// Reads the session cookie, looks up the session in the sessions table,
// and attaches the authenticated user to the request context.
//
// Returns 401 if no valid session.
func SessionAuth(sessions *SessionStore, logger *slog.Logger) func(http.Handler) http.Handler

// APIKeyAuth returns chi middleware that validates API keys.
// Reads the X-API-Key header, hashes it with SHA-256,
// looks up the hash in the api_keys table.
//
// Returns 401 if invalid or revoked key.
func APIKeyAuth(store *store.Queries, logger *slog.Logger) func(http.Handler) http.Handler

// RequireRole returns chi middleware that checks the authenticated user's role.
// Must be applied after SessionAuth or APIKeyAuth.
//
// Returns 403 if the user does not have one of the required roles.
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler

// CSRFOriginCheck returns chi middleware that validates the Origin (or Referer)
// header on state-changing requests (POST, PUT, DELETE, PATCH).
// GET and HEAD requests are passed through without check.
//
// Strategy: Origin/Referer validation. The middleware compares the request's
// Origin header (falling back to Referer) against the list of allowed origins.
// If neither header is present or the origin does not match, returns 403.
//
// This protects session-authenticated mutation routes against cross-site
// request forgery. API-key-authenticated routes do not need CSRF protection
// because API keys are sent in headers, not cookies.
//
// Pre-conditions:
//   - allowedOrigins is non-empty (e.g., ["https://app.example.com"])
// Post-conditions:
//   - State-changing requests without a matching Origin/Referer are rejected with 403
//   - GET/HEAD requests pass through unconditionally
func CSRFOriginCheck(allowedOrigins []string) func(http.Handler) http.Handler
```

### 7.8 Session Store

```go
// internal/auth/sessions.go

// SessionStore is a PostgreSQL-backed session store.
// Replaces express-session + connect-pg-simple from the Node.js version.
// ~60 lines of implementation.
//
// Session lifecycle:
//   - Create(userID): generates crypto/rand token, stores SHA-256 hash
//   - Validate(token): hashes token, looks up session, checks expiry
//   - Destroy(token): deletes session row
//   - Cleanup(): deletes expired sessions (called periodically via river job)
//
// The session token is set as an HttpOnly, Secure, SameSite=Lax cookie.
type SessionStore struct {
    store *store.Queries
}

// NewSessionStore creates a SessionStore.
func NewSessionStore(store *store.Queries) *SessionStore

// Create generates a new session for the given user.
// Returns the raw session token (to be set as a cookie).
// The token is a 32-byte crypto/rand value, hex-encoded.
func (s *SessionStore) Create(ctx context.Context, userID string, ttl time.Duration) (string, error)

// Validate checks a session token and returns the associated user info.
// Returns nil if the session is invalid or expired.
func (s *SessionStore) Validate(ctx context.Context, token string) (*SessionData, error)

// Destroy deletes a session by its raw token.
func (s *SessionStore) Destroy(ctx context.Context, token string) error

// Cleanup deletes all expired sessions. Returns the count removed.
// Called periodically by a river scheduled job.
func (s *SessionStore) Cleanup(ctx context.Context) (int64, error)

// SessionData is the data associated with a valid session.
type SessionData struct {
    UserID string
    OrgID  string
    Role   domain.Role
}
```

### 7.9 Store (Database Access)

```go
// internal/store/db.go

// Queries wraps a pgxpool.Pool and provides typed query methods.
// This is not a repository abstraction -- it is a convenience wrapper
// that groups queries by domain entity.
type Queries struct {
    pool *pgxpool.Pool
}

// New creates a Queries instance from a connection pool.
func New(pool *pgxpool.Pool) *Queries

// Pool returns the underlying connection pool for use in transactions.
func (q *Queries) Pool() *pgxpool.Pool

// WithTx executes a function within a database transaction.
// The transaction is committed if fn returns nil, rolled back otherwise.
//
// Usage:
//   err := store.WithTx(ctx, func(tx pgx.Tx) error {
//       // use tx for queries
//       return nil
//   })
func (q *Queries) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
```

```go
// internal/store/rules.go (representative example of query methods)

// GetEnabledRulesForItemType returns all LIVE and BACKGROUND rules
// that apply to the given item type within the organization.
func (q *Queries) GetEnabledRulesForItemType(
    ctx context.Context, orgID, itemTypeID string,
) ([]domain.Rule, error)

// GetActionsForRule returns all actions associated with a rule.
func (q *Queries) GetActionsForRule(
    ctx context.Context, ruleID string,
) ([]domain.Action, error)

// GetPoliciesForRules returns policies keyed by rule ID.
func (q *Queries) GetPoliciesForRules(
    ctx context.Context, ruleIDs []string,
) (map[string][]domain.Policy, error)

// CreateRule inserts a new rule and its relationships.
// The old version is saved to rules_history before update.
func (q *Queries) CreateRule(ctx context.Context, params CreateRuleParams) (*domain.Rule, error)

// UpdateRule updates a rule, saving the old version to rules_history.
func (q *Queries) UpdateRule(ctx context.Context, params UpdateRuleParams) (*domain.Rule, error)

// LogRuleExecutions batch-inserts rule execution records.
func (q *Queries) LogRuleExecutions(ctx context.Context, executions []RuleExecutionRow) error

// LogActionExecutions batch-inserts action execution records.
func (q *Queries) LogActionExecutions(ctx context.Context, executions []ActionExecutionRow) error
```

### 7.10 Signing Keys Service

```go
// internal/service/signing_keys.go

// SigningKeyService manages RSA-PSS keypairs for webhook signing.
type SigningKeyService struct {
    store *store.Queries
    cache *cache.Cache
}

// NewSigningKeyService creates a SigningKeyService.
func NewSigningKeyService(store *store.Queries, cache *cache.Cache) *SigningKeyService

// GetActiveKey returns the active signing key for an org.
// Cached for 60 seconds.
func (s *SigningKeyService) GetActiveKey(ctx context.Context, orgID string) (*rsa.PrivateKey, error)

// Sign signs a payload using the org's active RSA-PSS key.
// Returns the base64-encoded signature.
func (s *SigningKeyService) Sign(ctx context.Context, orgID string, payload []byte) (string, error)

// RotateKey generates a new keypair, deactivates the old one.
func (s *SigningKeyService) RotateKey(ctx context.Context, orgID string) (*domain.SigningKeyPublic, error)

// GetPublicKey returns the active public key for webhook signature verification.
func (s *SigningKeyService) GetPublicKey(ctx context.Context, orgID string) (string, error)
```

### 7.11 API Key Service

```go
// internal/service/api_keys.go

// APIKeyService manages API keys for REST API access.
type APIKeyService struct {
    store *store.Queries
}

// NewAPIKeyService creates an APIKeyService.
func NewAPIKeyService(store *store.Queries) *APIKeyService

// Create generates a new API key for an org.
// Returns the raw key (shown once) and the stored key metadata.
//
// The raw key is: prefix (8 chars) + "." + random (32 bytes hex).
// Only the SHA-256 hash of the full key is stored.
func (s *APIKeyService) Create(ctx context.Context, orgID, name string) (rawKey string, key *domain.APIKey, err error)

// Verify checks an API key and returns the associated org.
// Returns nil if the key is invalid or revoked.
func (s *APIKeyService) Verify(ctx context.Context, rawKey string) (*domain.APIKey, error)

// Revoke marks an API key as revoked.
func (s *APIKeyService) Revoke(ctx context.Context, keyID string) error

// List returns all API keys for an org (prefix only, no secrets).
func (s *APIKeyService) List(ctx context.Context, orgID string) ([]domain.APIKey, error)
```

### 7.12 Job Queue (river)

```go
// internal/worker/process_item.go

// ProcessItemArgs is the payload for async item processing jobs.
type ProcessItemArgs struct {
    OrgID      string         `json:"org_id"`
    ItemID     string         `json:"item_id"`
    ItemTypeID string         `json:"item_type_id"`
    Data       map[string]any `json:"data"`
}

// ProcessItemArgs implements river.JobArgs.
func (ProcessItemArgs) Kind() string { return "process_item" }

// ProcessItemWorker handles async item processing.
type ProcessItemWorker struct {
    river.WorkerDefaults[ProcessItemArgs]
    engine    *engine.RuleEngine
    publisher *engine.ActionPublisher
    store     *store.Queries
    logger    *slog.Logger
}

// Work processes a single item: runs rules, publishes actions.
func (w *ProcessItemWorker) Work(ctx context.Context, job *river.Job[ProcessItemArgs]) error
```

```go
// internal/worker/partition_manager.go

// PartitionManagerArgs is the payload for the periodic partition management job.
type PartitionManagerArgs struct{}

func (PartitionManagerArgs) Kind() string { return "partition_manager" }

// PartitionManagerWorker creates future partitions for rule_executions
// and action_executions tables, and optionally detaches old ones.
type PartitionManagerWorker struct {
    river.WorkerDefaults[PartitionManagerArgs]
    store  *store.Queries
    logger *slog.Logger
}

func (w *PartitionManagerWorker) Work(ctx context.Context, job *river.Job[PartitionManagerArgs]) error
```

### 7.13 Configuration

```go
// internal/config/config.go

// Config holds all application configuration, parsed from environment variables.
type Config struct {
    // Server
    Port    int    `env:"PORT" default:"8080"`
    Host    string `env:"HOST" default:"0.0.0.0"`

    // Database
    DatabaseURL     string `env:"DATABASE_URL" required:"true"`
    // Pool sizing note: river workers each hold a DB connection while processing.
    // With MaxWorkers=100, you need at least 100 connections for river alone,
    // plus connections for HTTP handlers. Default of 25 is suitable for
    // development only. Production should be at least MaxWorkers + expected
    // concurrent HTTP requests (e.g., 100 + 50 = 150). Monitor with
    // pgxpool.Stat() and adjust.
    MaxDBConns      int    `env:"MAX_DB_CONNS" default:"25"`

    // Auth
    SessionTTL      time.Duration `env:"SESSION_TTL" default:"24h"`
    CookieDomain    string        `env:"COOKIE_DOMAIN" default:""`
    CookieSecure    bool          `env:"COOKIE_SECURE" default:"true"`
    AllowedOrigins  []string      `env:"ALLOWED_ORIGINS" required:"true"` // CSRF origin check

    // Logging
    LogLevel slog.Level `env:"LOG_LEVEL" default:"INFO"`

    // Webhook
    WebhookTimeout  time.Duration `env:"WEBHOOK_TIMEOUT" default:"10s"`
    WebhookRetries  int           `env:"WEBHOOK_RETRIES" default:"5"`

    // External Signals (optional)
    OpenAIAPIKey string `env:"OPENAI_API_KEY" default:""`
}

// Load parses environment variables into a Config struct.
// Returns an error if required variables are missing.
//
// Implementation note: the `env:` and `default:` struct tags above are
// documentation annotations only. Load() uses manual os.Getenv calls
// with explicit parsing and defaults -- no reflection-based env-parsing
// library. This keeps the dependency count at zero for config and makes
// the parsing logic fully visible. Example:
//
//   cfg.Port = getEnvInt("PORT", 8080)
//   cfg.DatabaseURL = requireEnv("DATABASE_URL") // returns error if empty
//   cfg.AllowedOrigins = strings.Split(requireEnv("ALLOWED_ORIGINS"), ",")
//
// If the config struct grows beyond ~20 fields, consider adding
// github.com/caarlos0/env/v11 (single dependency, struct-tag-driven).
func Load() (*Config, error)
```

### 7.14 HTTP Handler Helpers

```go
// internal/handler/helpers.go

// JSON encodes v as JSON and writes it to w with the given status code.
// Sets Content-Type: application/json.
func JSON(w http.ResponseWriter, status int, v any)

// Decode reads the request body as JSON into v.
// Limits the request body to 1MB via http.MaxBytesReader to prevent
// denial-of-service from oversized payloads. Returns a ValidationError
// if the body is invalid JSON or exceeds the size limit.
func Decode(r *http.Request, v any) error

// Error maps domain errors to HTTP status codes and writes a JSON error response.
//   - *domain.NotFoundError    -> 404
//   - *domain.ForbiddenError   -> 403
//   - *domain.ConflictError    -> 409
//   - *domain.ValidationError  -> 400
//   - *domain.ConfigError      -> 500
//   - all other errors         -> 500
func Error(w http.ResponseWriter, r *http.Request, err error)

// OrgID extracts the authenticated org ID from the request context.
func OrgID(r *http.Request) string

// UserID extracts the authenticated user ID from the request context.
func UserID(r *http.Request) string
```

### 7.15 Input Validation Pattern

All request structs that arrive from HTTP endpoints implement a `Validate() error` method. This is the standard pattern for runtime input validation in Coop Lite Go. The handler calls `Validate()` after `Decode()` and before passing data to services.

```go
// internal/handler/validation.go

// Validatable is implemented by all request structs that need runtime validation.
// The handler layer calls Validate() after JSON decoding.
type Validatable interface {
    Validate() error
}

// DecodeAndValidate decodes the request body and validates it.
// Returns a *domain.ValidationError if validation fails.
func DecodeAndValidate(r *http.Request, v Validatable) error {
    if err := Decode(r, v); err != nil {
        return err
    }
    return v.Validate()
}
```

Concrete example:

```go
// internal/handler/rules.go

// CreateRuleRequest is the request body for POST /api/v1/rules.
type CreateRuleRequest struct {
    Name         string              `json:"name"`
    Status       domain.RuleStatus   `json:"status"`
    ItemTypeIDs  []string            `json:"item_type_ids"`
    ActionIDs    []string            `json:"action_ids"`
    PolicyIDs    []string            `json:"policy_ids"`
    ConditionSet domain.ConditionSet `json:"condition_set"`
    Tags         []string            `json:"tags"`
}

func (r *CreateRuleRequest) Validate() error {
    if strings.TrimSpace(r.Name) == "" {
        return &domain.ValidationError{Field: "name", Message: "must not be empty"}
    }
    switch r.Status {
    case domain.RuleStatusLive, domain.RuleStatusBackground, domain.RuleStatusDisabled:
        // valid
    default:
        return &domain.ValidationError{Field: "status", Message: "must be LIVE, BACKGROUND, or DISABLED"}
    }
    if len(r.ItemTypeIDs) == 0 {
        return &domain.ValidationError{Field: "item_type_ids", Message: "must have at least one item type"}
    }
    if len(r.ConditionSet.Conditions) == 0 {
        return &domain.ValidationError{Field: "condition_set", Message: "must have at least one condition"}
    }
    return nil
}
```

Handler usage:

```go
func CreateRule(cfg *service.ModerationConfigService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req CreateRuleRequest
        if err := DecodeAndValidate(r, &req); err != nil {
            Error(w, r, err)
            return
        }
        // ... pass to service
    }
}
```

> **Design Decision: `Validate()` method on request structs.**
> Go's type system handles structural validation at compile time (wrong types do not decode). Runtime validation covers semantic rules: non-empty strings, valid enum values, minimum array lengths, cross-field constraints. A `Validate() error` method per request struct is explicit, testable, and requires no validation framework. Each method is 10-20 lines.

---

## 8. Data Flow

### Item Submission (Async)

```
Client App
  |
  POST /api/v1/items/async { items: [...] }
  |
  APIKeyAuth middleware -> verify API key hash against api_keys table
  |
  handler.SubmitItemsAsync(w, r)
  |   - Decode request body
  |   - For each item:
  |       - Validate against item_type schema
  |       - INSERT into items table
  |       - Enqueue river job: ProcessItemArgs{orgID, itemID, itemTypeID, data}
  |
  Return 202 Accepted { correlation_ids: [...] }
  |
  river worker picks up ProcessItemArgs job
  |
  ProcessItemWorker.Work(ctx, job)
  |   - engine.RunEnabledRules(ctx, orgID, item) -> RuleEngineResult
  |       |
  |       - store.GetEnabledRulesForItemType(ctx, orgID, itemTypeID) [cached 30s]
  |       - Partition into LIVE and BACKGROUND
  |       - For each rule (goroutines, bounded by semaphore):
  |           - EvaluateConditionSet(ctx, rule.ConditionSet, evalCtx)
  |               |
  |               - Sort conditions by signal cost (ascending)
  |               - For each condition (sequential within set):
  |                   - If leaf: registry.Get(signalID).Run(ctx, input)
  |                   - If nested set: recurse
  |                   - Short-circuit if outcome determined
  |               - Return ConditionSetResult
  |       - For LIVE passing rules: collect ActionRequests
  |       - Deduplicate actions by ID
  |       - store.LogRuleExecutions(ctx, executions) [batch INSERT]
  |       - Return RuleEngineResult
  |
  |   - publisher.PublishActions(ctx, result.ActionsTriggered, target)
  |       |
  |       - For each action (goroutines):
  |           - WEBHOOK: http.Post(callbackURL, signedBody)
  |           - ENQUEUE_TO_MRT: store.InsertMRTJob(ctx, ...)
  |           - ENQUEUE_AUTHOR_TO_MRT: resolve author, store.InsertMRTJob(ctx, ...)
  |       - store.LogActionExecutions(ctx, executions) [batch INSERT]
  |
  Done
```

### Item Submission (Sync)

Same flow, except `RunEnabledRules` is called inline in the HTTP handler (not via river), and the response waits for completion and returns the triggered actions.

```
Client App
  |
  POST /api/v1/items { items: [...] }
  |
  handler.SubmitItemsSync(w, r)
  |   - Validate, store items
  |   - For each item:
  |       engine.RunEnabledRules(ctx, orgID, item) -> RuleEngineResult
  |       publisher.PublishActions(ctx, result.ActionsTriggered, target)
  |
  Return 200 { results: [{ item_id, actions_triggered: [...] }] }
```

### Manual Review Decision

```
Moderator UI
  |
  POST /api/v1/mrt/decisions { job_id, verdict, action_ids, policy_ids, reason }
  |
  SessionAuth + RequireRole(MODERATOR, ADMIN)
  |
  handler.RecordDecision(w, r)
  |   1. mrt.RecordDecision(ctx, params) -> DecisionResult
  |       - UPDATE mrt_jobs SET status = 'DECIDED'
  |       - INSERT INTO mrt_decisions (verdict, action_ids, policy_ids)
  |       - Build ActionRequests from action_ids
  |       - Return DecisionResult
  |
  |   2. publisher.PublishActions(ctx, result.ActionRequests, target)
  |       - Execute webhooks, MRT enqueue, author enqueue
  |       - Log action_executions
  |
  |   3. If policies have strike_penalty > 0:
  |       - store.IncrementUserStrikes(ctx, ...)
  |
  Return 200 { decision_id, actions_executed: [...] }
```

### Report Intake

```
Client App
  |
  POST /api/v1/reports { reported_item_id, reported_item_type_id, reason, ... }
  |
  APIKeyAuth middleware
  |
  handler.SubmitReport(w, r)
  |   reports.SubmitReport(ctx, params)
  |       - store.GetDefaultQueue(ctx, orgID)
  |       - mrt.Enqueue(ctx, EnqueueParams{source: "REPORT", ...})
  |
  Return 201 { job_id }
```

> **Design Decision: Route handler orchestrates MRT -> ActionPublisher flow.**
> `MRTService.RecordDecision()` returns `ActionRequest[]` and the route handler passes them to `ActionPublisher`. This makes the dependency graph a clean DAG: `handler -> {MRTService, ActionPublisher} -> store`. Neither service references the other.

---

## 9. Composition Root

The Bottle.js IoC container (1,878 lines) is replaced by a single `main.go` that constructs everything explicitly. This is idiomatic Go -- no DI framework, no container, no reflection.

```go
// cmd/server/main.go

func main() {
    // --- Configuration ---
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("config: %v", err)
    }

    // --- Logging ---
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: cfg.LogLevel,
    }))

    // --- Database ---
    poolConfig, _ := pgxpool.ParseConfig(cfg.DatabaseURL)
    poolConfig.MaxConns = int32(cfg.MaxDBConns)
    pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
    if err != nil {
        logger.Error("database connection failed", "error", err)
        os.Exit(1)
    }
    // pool.Close() is called in the graceful shutdown handler below.
    // Do not defer it here -- shutdown order matters (HTTP -> river -> pool).
    queries := store.New(pool)

    // --- Cache ---
    appCache := cache.New()

    // --- Auth ---
    sessionStore := auth.NewSessionStore(queries)

    // --- Services (no circular dependencies) ---
    apiKeyService := service.NewAPIKeyService(queries)
    signingKeyService := service.NewSigningKeyService(queries, appCache)
    userManagement := service.NewUserManagementService(queries)
    configService := service.NewModerationConfigService(queries, appCache)
    textBankService := service.NewTextBankService(queries)
    userStrikeService := service.NewUserStrikeService(queries)
    investigationService := service.NewInvestigationService(queries)
    analyticsService := service.NewAnalyticsService(queries)

    // --- MRT + Reports (MRTService has no dependency on ActionPublisher) ---
    mrtService := service.NewMRTService(queries, logger)
    reportsService := service.NewReportsService(queries, mrtService, logger)

    // --- Signals ---
    signalRegistry := signal.NewRegistry()
    signalRegistry.Register(signal.NewTextRegex())
    signalRegistry.Register(signal.NewTextBank(textBankService))
    if cfg.OpenAIAPIKey != "" {
        signalRegistry.Register(signal.NewHTTPSignal(signal.HTTPSignalConfig{
            ID:          "openai-moderation",
            DisplayName: "OpenAI Moderation",
            Description: "OpenAI content moderation API",
            Inputs:      []domain.SignalInputType{domain.SignalInputText},
            Cost:        10,
            URL:         "https://api.openai.com/v1/moderations",
            Headers:     map[string]string{"Authorization": "Bearer " + cfg.OpenAIAPIKey},
            // MapResponse provided as a function
        }))
    }

    // --- Engine ---
    httpClient := &http.Client{Timeout: cfg.WebhookTimeout}
    // signingKeyService implements engine.Signer interface
    actionPublisher := engine.NewActionPublisher(queries, signingKeyService, httpClient, logger)
    ruleEngine := engine.NewRuleEngine(queries, signalRegistry, appCache, logger)

    // --- River job queue ---
    // Workers must be declared and populated BEFORE creating the river client.
    workers := river.NewWorkers()
    river.AddWorker(workers, &worker.ProcessItemWorker{
        Engine: ruleEngine, Publisher: actionPublisher, Store: queries, Logger: logger,
    })
    river.AddWorker(workers, &worker.PartitionManagerWorker{Store: queries, Logger: logger})
    river.AddWorker(workers, &worker.SessionCleanupWorker{Sessions: sessionStore, Logger: logger})

    riverClient, _ := river.NewClient(riverpgxv5.New(pool), &river.Config{
        Queues: map[string]river.QueueConfig{
            river.QueueDefault: {MaxWorkers: 100},
        },
        Workers: workers,
    })

    // --- HTTP Router ---
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(slogMiddleware(logger)) // request logging

    // Health (no auth)
    r.Get("/api/v1/health", handler.Health(pool))

    // External API (API key auth)
    r.Group(func(r chi.Router) {
        r.Use(auth.APIKeyAuth(queries, logger))
        r.Post("/api/v1/items", handler.SubmitItemsSync(ruleEngine, actionPublisher, queries, logger))
        r.Post("/api/v1/items/async", handler.SubmitItemsAsync(queries, riverClient, logger))
        r.Post("/api/v1/reports", handler.SubmitReport(reportsService, logger))
        r.Get("/api/v1/policies", handler.ListPolicies(configService))
    })

    // Internal API (session auth + CSRF protection for mutations)
    r.Group(func(r chi.Router) {
        r.Use(auth.SessionAuth(sessionStore, logger))
        // CSRF: Origin/Referer check for all state-changing methods (POST/PUT/DELETE).
        // Strategy: verify that the Origin (or Referer) header matches the expected
        // server origin. This is simpler than double-submit cookies and works because
        // session auth is cookie-based and SameSite=Lax only protects against top-level
        // cross-site POSTs, not XHR/fetch from malicious origins.
        r.Use(auth.CSRFOriginCheck(cfg.AllowedOrigins))

        // Auth
        r.Post("/api/v1/auth/logout", handler.Logout(sessionStore))
        r.Get("/api/v1/auth/me", handler.Me())

        // Rules (Admin only for mutations)
        r.Get("/api/v1/rules", handler.ListRules(configService))
        r.With(auth.RequireRole(domain.RoleAdmin)).Post("/api/v1/rules", handler.CreateRule(configService))
        // ... etc for all CRUD endpoints

        // MRT
        r.Get("/api/v1/mrt/queues", handler.ListQueues(mrtService))
        r.With(auth.RequireRole(domain.RoleModerator, domain.RoleAdmin)).
            Post("/api/v1/mrt/decisions", handler.RecordDecision(mrtService, actionPublisher, userStrikeService, logger))
    })

    // Public auth endpoints (no auth middleware)
    r.Post("/api/v1/auth/login", handler.Login(userManagement, sessionStore, cfg))
    r.Post("/api/v1/auth/reset-password", handler.ResetPassword(userManagement))

    // --- Start ---
    if err := riverClient.Start(context.Background()); err != nil {
        logger.Error("river start failed", "error", err)
        os.Exit(1)
    }

    server := &http.Server{
        Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        Handler:      r,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // --- Graceful Shutdown ---
    // Listen for OS signals in a separate goroutine. On SIGINT or SIGTERM,
    // drain the HTTP server, stop river workers, and close the DB pool.
    // This is not optional -- without it, in-flight requests and river jobs
    // are killed mid-execution on deploy.
    shutdownCh := make(chan os.Signal, 1)
    signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        sig := <-shutdownCh
        logger.Info("shutdown signal received", "signal", sig)

        // Give in-flight HTTP requests up to 30 seconds to complete.
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        // 1. Stop accepting new HTTP connections, drain existing ones.
        if err := server.Shutdown(shutdownCtx); err != nil {
            logger.Error("http server shutdown error", "error", err)
        }

        // 2. Stop river workers (finishes in-progress jobs, stops fetching new ones).
        if err := riverClient.Stop(shutdownCtx); err != nil {
            logger.Error("river shutdown error", "error", err)
        }

        // 3. Close the database connection pool.
        pool.Close()

        logger.Info("shutdown complete")
    }()

    logger.Info("server starting", "addr", server.Addr)
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

Every service takes its dependencies as constructor arguments. No container registration, no string-based lookups, no reflection. The Go compiler checks everything. No circular dependencies -- `MRTService` and `ActionPublisher` are independent; the route handler orchestrates their interaction.

---

## 10. In-Memory Cache Strategy

Replace Redis + LRU_Map + distributed caching with a goroutine-safe in-memory cache.

```go
// internal/cache/cache.go

// Cache is a simple TTL-based in-memory cache.
// Thread-safe via sync.RWMutex. Suitable for single-process deployments.
// At multi-process scale, switch to cache invalidation via PostgreSQL LISTEN/NOTIFY.
type Cache struct {
    mu    sync.RWMutex
    store map[string]entry
}

type entry struct {
    value     any
    expiresAt time.Time
}

// New creates an empty cache.
func New() *Cache

// Get retrieves a value by key. Returns (value, true) if found and not expired.
// Returns (nil, false) if missing or expired.
func (c *Cache) Get(key string) (any, bool)

// Set stores a value with the given TTL.
func (c *Cache) Set(key string, value any, ttl time.Duration)

// Invalidate removes all keys with the given prefix.
// Used to invalidate related cache entries when config changes.
func (c *Cache) Invalidate(prefix string)

// GetOrSet atomically retrieves a value, or calls fn to compute it, stores it, and returns it.
// This prevents thundering herd on cache miss.
//
// WARNING: fn must NOT call any method on this Cache instance (Get, Set, Invalidate, GetOrSet).
// Doing so will deadlock because GetOrSet holds the write lock while calling fn.
// If callers need nested caching, use golang.org/x/sync/singleflight instead.
// TODO: Consider migrating to singleflight if cache-within-cache patterns emerge.
func (c *Cache) GetOrSet(key string, ttl time.Duration, fn func() (any, error)) (any, error)
```

Cached items (with TTL):

| Data | TTL | Key Pattern |
|------|-----|-------------|
| Enabled rules per item type | 30s | `rules:{orgID}:{itemTypeID}` |
| Policies per rule | 30s | `policies:{ruleID}` |
| Actions per rule | 30s | `actions:{ruleID}` |
| Item type schemas | 60s | `item_type:{orgID}:{itemTypeID}` |
| Signal list per org | 60s | `signals:{orgID}` |
| Active signing key | 60s | `signing_key:{orgID}` |

This is exactly what the original does (10-120s eventual consistency caches). The difference is we use a ~50-line `sync.RWMutex` + `map[string]entry` implementation instead of Redis.

---

## 11. Dependency List

### Production Dependencies (4 external modules)

| Module | Why |
|--------|-----|
| `github.com/go-chi/chi/v5` | HTTP router. stdlib-compatible, lightweight, middleware support. |
| `github.com/jackc/pgx/v5` | PostgreSQL driver. Fastest pure-Go driver, native JSONB, arrays, COPY, connection pooling. |
| `github.com/riverqueue/river` | PostgreSQL-native job queue. Built on pgx. Advisory locks, unique jobs, periodic jobs. |
| `golang.org/x/crypto` | bcrypt for password hashing. The only `x/` package needed. |

### Stdlib Dependencies (0 external)

| Stdlib Package | Use |
|----------------|-----|
| `net/http` | HTTP server, client (webhooks, external signals) |
| `encoding/json` | JSON serialization, JSONB handling |
| `crypto/rsa` | RSA-PSS webhook signing |
| `crypto/sha256` | API key hashing, session token hashing |
| `crypto/rand` | Secure random token generation |
| `regexp` | RE2-safe regex for text-regex signal |
| `log/slog` | Structured JSON logging |
| `context` | Cancellation, timeouts |
| `time` | Duration, time formatting |
| `sync` | RWMutex, Map, WaitGroup for cache and concurrency |
| `testing` | Unit tests, benchmarks |
| `net/http/httptest` | HTTP handler tests |
| `database/sql` | Only for migration tooling if needed |

### Dev Dependencies (~3)

| Module | Why |
|--------|-----|
| `github.com/riverqueue/river/riverdriver/riverpgxv5` | River pgx driver adapter |
| `golang.org/x/tools` | `go vet`, `staticcheck` |
| `github.com/oapi-codegen/oapi-codegen` | (Optional) Generate Go types/handlers from OpenAPI spec |

**Total production dependencies: 4** (vs. 350+ in the original, 18 in the TypeScript Lite version).

> **Design Decision: No ORM.**
> pgx provides type-safe scanning, batch operations, COPY protocol, prepared statements, and native array/JSONB support. An ORM (GORM, Ent, sqlx) would add abstraction without value for hand-written queries. If query generation is desired later, sqlc can be added without changing the store interface -- it generates the same `pgx.Rows` scanning code we write by hand.

> **Design Decision: No validation library.**
> Go's type system handles most validation at compile time. Runtime validation (e.g., checking that a rule status is one of LIVE/BACKGROUND/DISABLED) is done with simple functions in the handler layer. Zod's power comes from runtime type inference, which Go does not need. A 10-line `validate` function per request type is clearer than a validation framework.

---

## 12. API Design (REST)

All endpoints prefixed with `/api/v1`. UI requests use session auth. External requests use API key auth.

The API surface is identical to the TypeScript version. The request/response bodies are JSON with snake_case field names (Go struct tags handle the mapping).

### External API (API key auth)

```
POST   /api/v1/items              # Submit items (sync evaluation)
POST   /api/v1/items/async        # Submit items (async, returns 202)
POST   /api/v1/reports            # Submit user report -> enqueues to default MRT queue
POST   /api/v1/appeals            # Submit appeal  [DEFERRED: no backing service/table in v1.0; planned for v1.1 with appeals workflow]
GET    /api/v1/policies           # List policies for org
```

### Internal API (Session auth)

```
# Rules
GET    /api/v1/rules              # List rules (filterable)
POST   /api/v1/rules              # Create rule
GET    /api/v1/rules/{id}         # Get rule detail
PUT    /api/v1/rules/{id}         # Update rule
DELETE /api/v1/rules/{id}         # Delete rule (soft)

# Actions
GET    /api/v1/actions            # List actions
POST   /api/v1/actions            # Create action
GET    /api/v1/actions/{id}       # Get action detail
PUT    /api/v1/actions/{id}       # Update action
DELETE /api/v1/actions/{id}       # Delete action

# Policies
GET    /api/v1/policies           # List/tree
POST   /api/v1/policies           # Create
PUT    /api/v1/policies/{id}      # Update
DELETE /api/v1/policies/{id}      # Delete

# Item Types
GET    /api/v1/item-types         # List
POST   /api/v1/item-types         # Create
PUT    /api/v1/item-types/{id}    # Update
DELETE /api/v1/item-types/{id}    # Delete

# MRT
GET    /api/v1/mrt/queues                 # List queues with stats
GET    /api/v1/mrt/queues/{id}/jobs       # List jobs in queue
POST   /api/v1/mrt/queues/{id}/assign     # Assign next job to current user
POST   /api/v1/mrt/decisions              # Record decision
GET    /api/v1/mrt/jobs/{id}              # Get job detail

# Users
GET    /api/v1/users              # List users in org
POST   /api/v1/users/invite       # Invite user
PUT    /api/v1/users/{id}         # Update user role
DELETE /api/v1/users/{id}         # Deactivate user

# API Keys
GET    /api/v1/api-keys           # List (prefix only, no secrets)
POST   /api/v1/api-keys           # Create (returns key once)
DELETE /api/v1/api-keys/{id}      # Revoke

# Auth
POST   /api/v1/auth/login         # Login
POST   /api/v1/auth/logout        # Logout
GET    /api/v1/auth/me            # Current user
POST   /api/v1/auth/reset-password # Request password reset

# Text Banks
GET    /api/v1/text-banks                        # List banks
POST   /api/v1/text-banks                        # Create bank
GET    /api/v1/text-banks/{id}                   # Get bank with entries
POST   /api/v1/text-banks/{id}/entries           # Add entries
DELETE /api/v1/text-banks/{id}/entries/{entryId}  # Remove entry

# Analytics
GET    /api/v1/analytics/rule-executions     # Rule execution counts over time
GET    /api/v1/analytics/action-executions    # Action execution counts over time
GET    /api/v1/analytics/queue-throughput     # MRT throughput stats

# Investigation
GET    /api/v1/investigation/items/{typeId}/{itemId}   # Lookup item + history
GET    /api/v1/investigation/search                    # Search items

# Signals
GET    /api/v1/signals                       # List available signals
POST   /api/v1/signals/test                  # Test a signal against sample input

# Signing Keys (new in Go version -- see design decision below)
GET    /api/v1/signing-keys                  # Get active public key
POST   /api/v1/signing-keys/rotate           # Rotate signing key

# Health
GET    /api/v1/health             # Health check (DB ping)
```

### OpenAPI Spec and TypeScript Client Types

The API contract is defined in `api/openapi.yaml` (OpenAPI 3.1). From this single source of truth:

1. **Go server**: Handler request/response types are defined manually in Go structs (with json tags matching the spec). The OpenAPI spec is kept in sync with the Go types, not the other way around. Alternatively, `oapi-codegen` can generate Go types and chi-compatible handler interfaces from the spec.

2. **TypeScript client**: Run `openapi-typescript` against the YAML to generate TypeScript types. The React frontend imports these types for full type safety across the wire boundary. No GraphQL codegen, no shared TypeScript packages.

```
api/openapi.yaml  --(openapi-typescript)--> client/src/api/types.ts
```

This decouples the Go backend from the TypeScript frontend while maintaining type safety at the HTTP boundary.

> **Design Decision: Signing key endpoints are new.**
> The TypeScript Coop Lite version manages signing keys internally (auto-generated on org creation) but does not expose key rotation or public key retrieval via the API. The Go version adds `GET /api/v1/signing-keys` and `POST /api/v1/signing-keys/rotate` intentionally. Rationale: webhook consumers need to fetch the public key programmatically for signature verification, and key rotation should be an operator-initiated action (not just an internal concern). This aligns with the existing pattern in the original Coop codebase where signing key management was exposed.

---

## 13. Invariants

These must hold true at all times:

1. **Every database query includes `org_id` in its WHERE clause.** Multi-tenant isolation is non-negotiable. No query ever crosses org boundaries.

2. **No circular dependencies between packages.** The dependency graph is a DAG: `handler -> {service, engine} -> store -> domain`. `domain` imports nothing from `internal/`. Services do not depend on each other circularly. The handler orchestrates cross-service interactions.

3. **All exported functions document their contracts.** Every exported function in a package has a GoDoc comment stating pre-conditions, post-conditions, and error behavior.

4. **Rule condition evaluation never panics.** Errors in individual leaf conditions are recovered and marked `ERRORED`. The condition set evaluation continues.

5. **Webhook action failures do not block other actions.** Each action executes in its own goroutine. Failures are logged and returned in results.

6. **History tables are append-only.** When a rule, action, or policy is updated, the old version is inserted into the corresponding `*_history` table before the update. History tables exist for: `rules_history`, `actions_history`, `policies_history`.

7. **API keys are never stored in plaintext.** Only SHA-256 hashes are persisted. The raw key is returned exactly once on creation.

8. **Session auth and API key auth are mutually exclusive per request.** A request is authenticated by one mechanism only, determined by middleware routing.

9. **ActionPublisher and MRTService have no direct dependency on each other.** Cross-cutting orchestration happens in handlers, not in services.

10. **Condition trees use AND and OR conjunctions only.** XOR is not supported.

11. **`context.Context` is the first parameter of every function that performs I/O.** Database queries, HTTP calls, signal evaluation -- all accept `ctx context.Context` as parameter one.

12. **The `domain` package has zero imports from other `internal/` packages.** It is the foundation of the dependency DAG. All other packages depend on it, never the reverse.

13. **All errors are returned, never swallowed.** Go's explicit error returns are used consistently. `_` on an error return is prohibited by linting except in deferred Close calls.

14. **The signal `Adapter` interface is the only abstraction for signal execution.** All signal invocations go through the interface, never through concrete types directly (except in tests).

---

## 14. Migration Path from Original Coop

This is not a rewrite-in-place. It is a new project that can run alongside the original.

### Phase 1: Data Migration

- Write a migration script (Go CLI tool in `cmd/migrate/`) that reads from the original's PostgreSQL (9 schemas) and writes to the new single-schema format.
- Map original rule `conditionSet` JSONB directly (format is compatible, minus XOR nodes which must be flagged for manual review and converted to OR).
- Map original actions, policies, item types.
- Map original MRT queues and pending jobs.
- Do NOT migrate ClickHouse/Scylla data (analytics start fresh).

### Phase 2: API Compatibility Layer

- The external REST API (`/items`, `/reports`) accepts the same request format as the original. Clients switch endpoints without code changes.
- Webhook callback payloads match the original format (same JSON structure, same RSA-PSS signing).

### Phase 3: Parallel Run

- Run both systems simultaneously, with the Go system in shadow mode (processing items but not executing webhook actions -- only logging what would have been sent).
- Compare rule evaluation results between old and new using correlation IDs.
- Switch over when results match for 7+ days.

### Phase 4: Frontend Migration

The Go backend serves the same REST API, so the React frontend can point at either backend. Options:

1. **Keep the React frontend.** It talks to the Go backend via the same REST API. No frontend changes needed.
2. **Serve static files from Go.** Embed the built React app using `embed.FS` and serve it from the Go binary. Single binary deployment.
3. **Replace with htmx/templ.** (Future option) If the team prefers server-rendered UI, Go's `html/template` or the `templ` library can replace React entirely. This eliminates the TypeScript frontend build entirely.

---

## 15. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| PostgreSQL analytics too slow at scale | Medium | Medium | Execution log tables use declarative partitioning by month from day one. At >100M rows/month, add ClickHouse behind the store interface. |
| river insufficient for high-throughput | Low | High | river handles thousands of jobs/sec on modest hardware. The `river.Client` interface allows swapping to a custom worker pool if needed. |
| Missing signal types block adoption | Medium | High | The `signal.Adapter` interface is designed for this. Each signal is a standalone file. Adding OpenAI, Google, image hashing requires zero engine changes. |
| No GraphQL hurts frontend DX | Low | Low | OpenAPI-generated TypeScript types provide equivalent type safety. TanStack Query + typed fetch wrappers cover the same use cases. |
| Go regex (RE2) less powerful than PCRE | Low | Low | RE2 covers 99% of moderation regex use cases. No lookbehinds, but the tradeoff (guaranteed O(n) matching, no ReDoS) is worth it for a system processing user content. |
| Session auth insufficient for enterprise | Medium | Low | Auth middleware interface supports pluggable strategies. SAML/SSO is a new middleware that creates sessions in the same session store. |
| JSONB handling verbose in Go | Medium | Low | Define typed Go structs for known JSONB shapes (condition sets, schemas). Use `json.RawMessage` for pass-through. pgx handles JSONB natively via `pgtype.JSONB`. |
| No hot reload during development | Low | Low | Use `air` or `watchexec` for file watching + rebuild. Go compiles in <2 seconds for this project size. |
| Team unfamiliar with Go | Medium | Medium | The codebase is intentionally simple Go. No generics gymnastics, no channel-heavy patterns, no reflection. A competent TypeScript developer can read and write this Go within a week. |

---

## 16. What This Looks Like In Practice

### Adding a new signal (e.g., OpenAI Moderation)

Create one file:

```go
// internal/signal/openai_moderation.go

type OpenAIModeration struct {
    apiKey string
    client *http.Client
}

func NewOpenAIModeration(apiKey string) *OpenAIModeration {
    return &OpenAIModeration{
        apiKey: apiKey,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

func (s *OpenAIModeration) ID() string               { return "openai-moderation" }
func (s *OpenAIModeration) DisplayName() string       { return "OpenAI Moderation" }
func (s *OpenAIModeration) Description() string       { return "OpenAI content moderation API" }
func (s *OpenAIModeration) EligibleInputs() []domain.SignalInputType {
    return []domain.SignalInputType{domain.SignalInputText}
}
func (s *OpenAIModeration) Cost() int { return 10 }

func (s *OpenAIModeration) Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error) {
    if input.Type != domain.SignalInputText || input.Value.Text == "" {
        return domain.SignalOutput{Score: 0}, nil
    }

    body, _ := json.Marshal(map[string]string{"input": input.Value.Text})
    req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/moderations", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+s.apiKey)

    resp, err := s.client.Do(req)
    if err != nil {
        return domain.SignalOutput{}, fmt.Errorf("openai request failed: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        Results []struct {
            CategoryScores map[string]float64 `json:"category_scores"`
        } `json:"results"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return domain.SignalOutput{}, fmt.Errorf("openai decode failed: %w", err)
    }

    maxScore := 0.0
    maxCategory := ""
    for cat, score := range result.Results[0].CategoryScores {
        if score > maxScore {
            maxScore = score
            maxCategory = cat
        }
    }

    return domain.SignalOutput{
        Score:    maxScore,
        Label:    maxCategory,
        Metadata: map[string]any{"scores": result.Results[0].CategoryScores},
    }, nil
}
```

Register in `main.go`:

```go
if cfg.OpenAIAPIKey != "" {
    signalRegistry.Register(signal.NewOpenAIModeration(cfg.OpenAIAPIKey))
}
```

Done. No changes to the rule engine, condition evaluator, or any other package.

### Adding a new action type (e.g., NCMEC reporting)

1. Add the type to the `actions.action_type` CHECK constraint via SQL migration.
2. Add a `const ActionTypeNCMEC ActionType = "ENQUEUE_TO_NCMEC"` to `domain/action.go`.
3. Add a case to `ActionPublisher.publishAction` switch statement.
4. Add a new service if the action type has complex logic.

### Example handler (representative)

```go
// internal/handler/rules.go

func ListRules(cfg *service.ModerationConfigService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        orgID := OrgID(r)
        status := r.URL.Query().Get("status")
        itemTypeID := r.URL.Query().Get("item_type_id")

        rules, err := cfg.ListRules(r.Context(), orgID, service.ListRulesFilter{
            Status:     domain.RuleStatus(status),
            ItemTypeID: itemTypeID,
        })
        if err != nil {
            Error(w, r, err)
            return
        }
        JSON(w, http.StatusOK, rules)
    }
}

func CreateRule(cfg *service.ModerationConfigService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        orgID := OrgID(r)

        var req struct {
            Name         string              `json:"name"`
            Status       domain.RuleStatus   `json:"status"`
            ItemTypeIDs  []string            `json:"item_type_ids"`
            ActionIDs    []string            `json:"action_ids"`
            PolicyIDs    []string            `json:"policy_ids"`
            ConditionSet domain.ConditionSet `json:"condition_set"`
            Tags         []string            `json:"tags"`
        }
        if err := Decode(r, &req); err != nil {
            Error(w, r, err)
            return
        }

        rule, err := cfg.CreateRule(r.Context(), orgID, service.CreateRuleParams{
            Name:         req.Name,
            Status:       req.Status,
            ItemTypeIDs:  req.ItemTypeIDs,
            ActionIDs:    req.ActionIDs,
            PolicyIDs:    req.PolicyIDs,
            ConditionSet: req.ConditionSet,
            Tags:         req.Tags,
        })
        if err != nil {
            Error(w, r, err)
            return
        }
        JSON(w, http.StatusCreated, rule)
    }
}
```

---

## 17. Estimated Size

| Component | Lines of Code (estimate) |
|-----------|-------------------------|
| `domain/` (types, errors) | ~500 |
| `handler/` (HTTP handlers) | ~1,500 |
| `engine/` (rule engine, evaluator, conditions, action publisher) | ~700 |
| `signal/` (interface, registry, 2 builtins, 1 HTTP adapter) | ~400 |
| `service/` (10 services including reports) | ~2,000 |
| `store/` (database queries) | ~1,200 |
| `auth/` (middleware, passwords, sessions, rbac) | ~400 |
| `worker/` (river workers) | ~300 |
| `cache/` | ~80 |
| `crypto/` (signing, hashing) | ~150 |
| `config/` | ~80 |
| `cmd/server/main.go` (composition root) | ~200 |
| `cmd/migrate/` | ~100 |
| SQL migrations | ~300 |
| Tests | ~7,900 |
| **Total** | **~15,800** |

For reference:
- Original coop server: ~40,000+ lines TypeScript
- TypeScript Lite design: ~12,150 lines estimated
- **Go Lite: ~15,800 lines estimated** (includes tests at ~1:1 ratio, excludes frontend)

Go is more verbose in some areas (explicit error handling, struct definitions) but far more concise in others (no type gymnastics, no async/await boilerplate, no import ceremony). The production code (~7,900 lines) is roughly 35% less than the TypeScript Lite version. The test estimate uses a 1:1 test-to-code ratio, which is standard for Go projects with table-driven tests, integration tests, and HTTP handler tests.

The frontend is not included because the Go backend serves the same REST API. The existing React frontend (or a future one) connects unchanged.

---

## 18. Implementation Order

Build in this sequence. Each phase produces a working, testable system.

### Phase 1: Foundation (Week 1-2)

1. Project scaffold (`go mod init`, directory structure, Makefile, Dockerfile)
2. `domain/` package (all types, errors, constants)
3. `config/` package (env parsing)
4. `store/` package (pgxpool connection, migration runner)
5. SQL migrations (full schema including partitioned tables)
6. `auth/` package (bcrypt passwords, session store, middleware, RBAC)
7. `handler/` helpers (JSON, Decode, Error)
8. Health check endpoint
9. Auth endpoints (login, logout, me)
10. User management CRUD
11. Org setup (seed script in `cmd/seed/`)

**Milestone: Can create org, create users, login, manage sessions.**

### Phase 2: Configuration (Week 3)

12. Item type CRUD (store + service + handler)
13. Policy CRUD with history tables
14. Action CRUD with history tables
15. Text bank CRUD
16. API key management (create, verify, revoke, list)
17. Signing key management (generate, rotate, get public key)

**Milestone: Can configure all moderation entities via API.**

### Phase 3: Engine (Week 4-5)

18. `signal/` package: `Adapter` interface, `Registry`
19. Built-in signals: `TextRegex` (with literal mode), `TextBank`
20. `engine/condition_set.go`: recursive evaluator with cost-ordering and short-circuit
21. `engine/rule_engine.go`: fetch rules, evaluate concurrently, deduplicate actions, log executions
22. `engine/action_publisher.go`: webhook delivery (with RSA-PSS signing), MRT enqueue, author enqueue
23. Item submission endpoints (sync and async)
24. river workers: `ProcessItemWorker`
25. Rule CRUD endpoints (with item type, action, policy associations)

**Milestone: Can submit items, rules evaluate, actions fire. Full automated moderation loop.**

### Phase 4: Review Tool + Reports (Week 6)

26. MRT service (enqueue, assign with `SELECT ... FOR UPDATE SKIP LOCKED`, decide)
27. MRT handler endpoints (queues, jobs, assign, decision)
28. Decision orchestration in handler (MRT -> ActionPublisher)
29. Reports service (intake -> default MRT queue)
30. Reports endpoint

**Milestone: Full MRT workflow. Human review loop complete.**

### Phase 5: Signals + Workers (Week 7)

31. HTTP signal adapter (generic, for OpenAI/Google/custom)
32. OpenAI moderation signal (concrete implementation)
33. Signal listing and testing endpoints
34. river periodic workers: partition manager, session cleanup
35. Investigation service + endpoints

**Milestone: External signal integration. Background maintenance automated.**

### Phase 6: Analytics + Polish (Week 8-9)

36. Analytics service (rule execution counts, action counts, queue throughput)
37. Analytics endpoints
38. User strikes service
39. Rate limiting middleware (token bucket, in-memory)
40. OpenAPI spec (`api/openapi.yaml`)
41. TypeScript type generation from OpenAPI
42. Integration tests (full request lifecycle)
43. Benchmarks (rule evaluation throughput, concurrent item processing)

**Milestone: Production-ready. Full feature parity with TypeScript Lite design.**

### Phase 7: Deployment (Week 10)

44. Dockerfile (multi-stage: build -> scratch)
45. ~~Graceful shutdown~~ (already in main.go composition root from Phase 1 -- not a deploy-time addition)
46. Health check with dependency probes (DB ping, river status)
47. Structured log correlation (request ID propagation via context)
48. Production config validation
49. Load testing

**Total: ~10 weeks for 1-2 developers.** Same timeline as TypeScript version, but the result is a single binary with 4 dependencies instead of a Node.js app with 18+ production dependencies.

---

## Appendix A: Concurrency Model

### Rule Evaluation

```
RunEnabledRules(ctx, orgID, item)
|
|-- Fetch rules (from cache or DB)
|
|-- For each rule (bounded goroutine pool, max 10):
|     |
|     |-- EvaluateConditionSet (sequential within a rule)
|     |     |
|     |     |-- Sort conditions by cost
|     |     |-- For each condition:
|     |     |     If leaf: signal.Run(ctx, input) [may hit cache]
|     |     |     If set: recurse
|     |     |     Short-circuit if outcome known
|     |
|     |-- Return RuleResult
|
|-- Collect all RuleResults
|-- Deduplicate ActionRequests
|-- Return RuleEngineResult
```

The semaphore-bounded goroutine pool prevents resource exhaustion when many rules exist. Within a single rule, conditions are evaluated sequentially to enable short-circuit optimization. Across rules, evaluation is parallel because rules are independent.

### Action Publishing

```
PublishActions(ctx, actions, target)
|
|-- For each action (goroutines, no bound -- actions are typically 1-5):
|     |
|     |-- WEBHOOK: http.Post with timeout from ctx
|     |-- ENQUEUE_TO_MRT: store.InsertMRTJob
|     |-- ENQUEUE_AUTHOR_TO_MRT: resolve author + store.InsertMRTJob
|
|-- sync.WaitGroup.Wait()
|-- Collect ActionResults
```

### Signal Caching Within Evaluation

When multiple rules reference the same signal with the same input field, the signal is evaluated once and cached in the `EvaluationContext.signalCache` (a `sync.Map`). This is safe because the evaluation context is scoped to a single item submission.

---

## Appendix B: Testing Strategy

### Unit Tests

- `domain/` -- pure type tests, validation logic
- `engine/condition_set_test.go` -- table-driven tests for AND/OR evaluation, short-circuit, three-valued logic
- `engine/evaluator_test.go` -- leaf condition evaluation with mock signals
- `signal/*_test.go` -- each signal tested in isolation
- `cache/cache_test.go` -- TTL expiry, concurrent access, invalidation
- `crypto/*_test.go` -- signing round-trip, hashing determinism

### Integration Tests

- Full item submission lifecycle (submit -> rules -> actions -> execution logs)
- MRT workflow (enqueue -> assign -> decide -> action execution)
- Report intake -> MRT queue
- Auth lifecycle (login -> session -> request -> logout)

### HTTP Handler Tests

Using `httptest.NewRecorder()` and `httptest.NewRequest()`:

```go
func TestListRules(t *testing.T) {
    // Setup: create test DB, seed data, build handler
    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/api/v1/rules", nil)
    r = r.WithContext(auth.WithUser(r.Context(), testUser))

    handler.ListRules(configService)(w, r)

    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", w.Code)
    }
    // Decode and assert response body
}
```

### Benchmarks

```go
func BenchmarkEvaluateConditionSet(b *testing.B) {
    // Setup: create a condition set with 10 conditions, mock signals
    for i := 0; i < b.N; i++ {
        EvaluateConditionSet(ctx, conditionSet, evalCtx)
    }
}

func BenchmarkRuleEngineParallel(b *testing.B) {
    // Setup: 50 rules, 5 conditions each
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            engine.RunEnabledRules(ctx, orgID, item)
        }
    })
}
```

---

## Appendix C: Package Dependency DAG

```
cmd/server/main.go
  |
  |-- internal/config
  |-- internal/domain         (zero internal imports)
  |-- internal/store          --> domain
  |-- internal/cache          (zero internal imports)
  |-- internal/crypto         --> domain
  |-- internal/auth           --> domain, store, crypto
  |-- internal/signal         --> domain
  |-- internal/service        --> domain, store, cache
  |-- internal/engine         --> domain, store, cache, signal (no service import; uses engine.Signer interface)
  |-- internal/worker         --> domain, store, engine
  |-- internal/handler        --> domain, service, engine, auth
```

No cycles. Every arrow points downward. `domain` is the leaf. `handler` and `cmd/server` are the roots.

This DAG is enforced by Go's compiler: circular imports are a compile error, not a runtime surprise.
