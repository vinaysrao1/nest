# Coop Lite Architecture

A radically simplified content moderation platform. Same domain, 10% of the code.

---

## 1. Design Philosophy

Three rules govern every decision in this document:

1. **One way to do each thing.** No dual ORMs, no dual component libraries, no dual chart libraries. Pick one, use it everywhere.
2. **PostgreSQL does the work.** If Postgres can handle it (queues, sessions, analytics, full-text search, JSON storage), do not add another system.
3. **Interfaces at boundaries, not everywhere.** Only abstract where you genuinely expect to swap implementations. The rule engine will always evaluate rules. Do not over-abstract the interior.

---

## 2. What Gets Cut (and Why)

### Eliminated Infrastructure
| Component | Reason |
|-----------|--------|
| **Kafka** | PostgreSQL LISTEN/NOTIFY + pg-boss handles our throughput. We are not processing millions of items per second. |
| **Scylla/Cassandra** | Investigation data goes into PostgreSQL. At the scale where Scylla matters, you have a team to build Scylla support. |
| **ClickHouse** | PostgreSQL with proper indexes and materialized views handles analytics for <100M rows. Add ClickHouse later behind the existing adapter interface if needed. |
| **Redis** | Sessions in PostgreSQL (custom Kysely-backed store). In-memory Map for hot caches (rules, policies). No BullMQ -- pg-boss replaces it. |
| **Snowflake** | Eliminated. Export to Snowflake can be a pg_dump cron job if someone wants it. |
| **Schema Registry (Confluent)** | No Kafka, no schema registry. TypeScript types + runtime validation via Zod. |
| **HMA (Python service)** | Deferred. Can be added as an external signal adapter later. |
| **Content Proxy** | Deferred. Use CSP headers + sandboxed iframes directly. |
| **OpenTelemetry** | Deferred. Use structured JSON logging to stdout. Add OTel when you need distributed tracing. |

### Eliminated Libraries/Patterns
| What | Replacement |
|------|-------------|
| Sequelize + Kysely (dual ORM) | **Kysely only** (type-safe, no legacy baggage) |
| Bottle.js IoC container (1,878 lines) | **Plain constructor injection** with a single composition root |
| Apollo Server + Apollo Client (GraphQL) | **REST API** with typed routes. GraphQL adds enormous complexity for an internal tool. |
| Ant Design + Radix UI + Tailwind (triple styling) | **React + Tailwind + shadcn/ui** (Radix primitives, zero runtime, copy-paste components) |
| Recharts + Google Charts | **Recharts only** |
| moment + date-fns | **Native Date + PostgreSQL date functions** (see Design Decision below) |
| lodash | **Native JS** (structuredClone, Array methods, Object methods). Zero lodash. |
| GraphQL codegen (938 generated types) | **Shared TypeScript types** in a `shared/` package, used by both client and server |
| Storybook (8 packages) | Eliminated. Build components in the app. |
| undici + axios + fetch | **Native fetch** (Node 18+) |
| Passport.js + passport-saml | **Custom auth middleware** (bcrypt + sessions). SAML added as optional plugin later. |
| express-session + connect-pg-simple | **Custom Kysely-backed session store** (~40 lines, see Design Decision below) |
| jsonwebtoken | **crypto.randomBytes + DB-stored hashed tokens** (see Design Decision below) |
| uuid | **PostgreSQL gen_random_uuid()** (see Design Decision below) |
| pino | **Custom structured JSON logger** (~15 lines, see Design Decision below) |
| openai | **Plain fetch** for external signal HTTP calls (see Design Decision below) |
| fuzzball | **Deferred to v1.1** (see Design Decision below) |

> **Design Decision: Drop `express-session` + `connect-pg-simple` (Feedback #5).**
> express-session is Express middleware. Making it work with Hono requires glue code that defeats the purpose of choosing Hono. A custom session store is ~40 lines of Kysely queries (create, read, delete, expire) behind our existing `sessions` table. This also eliminates the `connect-pg-simple` dependency. Net: -2 dependencies, -0 glue code.

> **Design Decision: Drop `jsonwebtoken` (Feedback #6).**
> The only use of JWTs was password reset tokens and invite links. These are better served by `crypto.randomBytes(32)` generating an opaque token, hashed with SHA-256 and stored in a `password_reset_tokens` table with an `expires_at` column. Simpler, no JWT parsing, no secret key rotation concerns. Net: -1 dependency.

> **Design Decision: Drop `uuid` (Feedback #7).**
> The schema already uses `gen_random_uuid()` for all primary keys. Server-side UUID generation is not needed -- IDs are always assigned by PostgreSQL. If application-layer UUIDs are ever needed, `crypto.randomUUID()` is built into Node.js 19+. Net: -1 dependency.

> **Design Decision: Drop `openai` from core deps (Feedback #8).**
> The `openai` npm package is 2MB+ and couples the core to a specific vendor. External signals (OpenAI, Google, custom) should use plain `fetch` with a thin typed wrapper. This keeps the signal adapter truly pluggable -- no vendor SDK baked into the dependency tree. The OpenAI moderation API is a single POST; `fetch` is sufficient. Net: -1 dependency.

> **Design Decision: Drop `fuzzball`, defer fuzzy matching to v1.1 (Feedback #9).**
> Fuzzy string matching is a niche signal. The v1.0 signal set is: `text-contains`, `text-regex`, `text-bank`. These cover the vast majority of text-based moderation rules. `fuzzball` adds ~500KB and a C++ binding. Ship v1.0 lean; add fuzzy matching in v1.1 when there is demand. Net: -1 dependency.

> **Design Decision: Drop `date-fns` on server (Feedback #10).**
> Server-side date formatting needs are minimal: ISO strings for API responses, PostgreSQL date functions for queries. Native `Date.toISOString()` and `Intl.DateTimeFormat` cover formatting. Kysely handles date parameters natively. Client keeps `date-fns` for user-facing display formatting where the API is genuinely useful. Net: -1 server dependency.

> **Design Decision: Drop `pino`, replace with custom logger (Feedback #11).**
> Structured JSON logging to stdout requires: a level check, `JSON.stringify`, and `process.stdout.write`. That is ~15 lines. Pino's value is in high-throughput log formatting (100k+ logs/sec), which is not our bottleneck. A custom logger also lets us define the `Logger` interface cleanly (see Feedback #14). Net: -1 dependency.

### Eliminated Features (Deferred, Not Deleted)
| Feature | Rationale |
|---------|-----------|
| NCMEC integration | Specialized compliance. Add as a plugin when needed. |
| SSO/SAML | Enterprise auth. The interface exists; plug in when customers demand it. |
| Hash banks / HMA | Perceptual hashing is a specialized signal. Add via signal plugin system. |
| Backtesting & retroactions | Valuable but complex. Add in v2 after the core is stable. |
| ML model tracking | Not core moderation. Use an external ML platform. |
| Rule anomaly detection | Nice-to-have analytics. Add after basic analytics work. |
| Aggregation signals | Complex stateful signals. Defer to v2. |
| Location banks / geo-containment | Niche signal type. Add via signal plugin. |
| Snowflake ingestion workers | No Snowflake. |
| GDPR deletion endpoints | Add when compliance requires it. The data model supports it (soft delete). |
| Fuzzy text matching (`text-fuzzy` signal) | Deferred to v1.1. See dependency reduction notes above. |

---

## 3. What Gets Kept

### CORE (v1.0)
- Multi-tenant organizations
- User authentication (email/password) with RBAC (Admin, Moderator, Analyst)
- Item types with custom schemas (Content, User, Thread)
- Rule engine with nested AND/OR condition trees
- Built-in signals: text-contains, text-regex, text-bank
- External signal adapter interface (for OpenAI, Google, custom HTTP -- using plain `fetch`)
- Actions: webhook callback (with signing), enqueue to review queue, enqueue author to review queue
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
| Runtime | Node.js 22+ | Native fetch, crypto, structured clone. Matches original. |
| Language | TypeScript 5.x (strict) | Same as original. Non-negotiable for this domain. |
| Server framework | **Hono** | 14KB, fastest Node.js framework, built-in middleware, TypeScript-first. Express is legacy. |
| Database | **PostgreSQL 16** | Only external dependency. Handles everything. |
| ORM / Query builder | **Kysely** | Already partially adopted in original. Type-safe, no magic. |
| Job queue | **pg-boss** | PostgreSQL-native job queue. No Redis needed. Handles async item processing, scheduled jobs. |
| Validation | **Zod** | Runtime + static types from one schema. Replaces AJV + hand-written types. |
| Auth | **bcryptjs + custom Kysely session store** | Minimal. Session in PG. No Express middleware compatibility layer. |
| Frontend | **React 19 + Vite + Tailwind + shadcn/ui** | Fast builds, modern tooling, copy-paste components. |
| Frontend data | **TanStack Query + fetch** | Simple, no GraphQL overhead. Type-safe with shared types. |
| Charts | **Recharts** | Already used in original. One chart library. |
| Testing | **Vitest** | Fast, Vite-native, compatible with Jest API. |

---

## 5. Directory Structure

```
coop-lite/
|-- package.json                    # Workspace root
|-- tsconfig.base.json
|
|-- packages/
|   |-- shared/                     # Shared types and utilities
|   |   |-- package.json
|   |   |-- src/
|   |   |   |-- index.ts
|   |   |   |-- types/
|   |   |   |   |-- item-types.ts   # Field, ScalarType, Container definitions
|   |   |   |   |-- rules.ts        # ConditionSet, LeafCondition, Conjunction
|   |   |   |   |-- actions.ts      # ActionType, Action definitions
|   |   |   |   |-- policies.ts     # Policy types
|   |   |   |   |-- signals.ts      # Signal types, input/output types
|   |   |   |   |-- auth.ts         # User roles, permissions
|   |   |   |   |-- mrt.ts          # Review queue types, decision types
|   |   |   |   |-- api.ts          # REST API request/response types
|   |   |   |   |-- logger.ts       # Logger interface definition
|   |   |   |   |-- jobs.ts         # JobQueue interface definition
|   |   |   |-- schemas/            # Zod schemas (source of truth for validation)
|   |   |   |   |-- items.ts
|   |   |   |   |-- rules.ts
|   |   |   |   |-- actions.ts
|   |   |   |   |-- policies.ts
|   |   |   |-- utils/
|   |   |       |-- errors.ts
|   |
|   |-- server/                     # Backend
|   |   |-- package.json
|   |   |-- src/
|   |   |   |-- index.ts            # Entry point: compose + start
|   |   |   |-- app.ts              # Hono app setup, middleware
|   |   |   |-- compose.ts          # Composition root (DI without a framework)
|   |   |   |
|   |   |   |-- db/
|   |   |   |   |-- connection.ts   # Kysely PostgreSQL connection
|   |   |   |   |-- schema.ts       # Kysely database interface (all tables)
|   |   |   |   |-- migrations/     # Kysely migrations (plain TypeScript)
|   |   |   |   |   |-- 001_initial.ts
|   |   |   |   |   |-- ...
|   |   |   |
|   |   |   |-- auth/
|   |   |   |   |-- middleware.ts    # Session auth + API key auth middleware
|   |   |   |   |-- passwords.ts    # bcrypt hash/verify
|   |   |   |   |-- sessions.ts     # Custom Kysely-backed session store (~40 lines)
|   |   |   |   |-- rbac.ts         # Role-based access control check
|   |   |   |
|   |   |   |-- routes/
|   |   |   |   |-- items.ts        # POST /api/v1/items (sync + async)
|   |   |   |   |-- rules.ts        # CRUD rules
|   |   |   |   |-- actions.ts      # CRUD actions
|   |   |   |   |-- policies.ts     # CRUD policies
|   |   |   |   |-- item-types.ts   # CRUD item types
|   |   |   |   |-- mrt.ts          # MRT: queues, jobs, decisions
|   |   |   |   |-- users.ts        # User management
|   |   |   |   |-- orgs.ts         # Org settings
|   |   |   |   |-- api-keys.ts     # API key management
|   |   |   |   |-- reports.ts      # User reports (intake -> default MRT queue)
|   |   |   |   |-- analytics.ts    # Dashboard data endpoints
|   |   |   |   |-- health.ts       # Health check
|   |   |   |
|   |   |   |-- engine/
|   |   |   |   |-- rule-engine.ts       # Top-level: run enabled rules for item
|   |   |   |   |-- rule-evaluator.ts    # Evaluate single rule's condition tree
|   |   |   |   |-- condition-set.ts     # Recursive AND/OR evaluation with short-circuit
|   |   |   |   |-- action-publisher.ts  # Execute actions (webhook, enqueue MRT)
|   |   |   |
|   |   |   |-- signals/
|   |   |   |   |-- interface.ts         # SignalAdapter interface
|   |   |   |   |-- registry.ts          # Signal registry (register, lookup, list)
|   |   |   |   |-- builtin/
|   |   |   |   |   |-- text-regex.ts    # Subsumes text-contains (literal strings auto-escaped)
|   |   |   |   |   |-- text-bank.ts
|   |   |   |   |-- external/
|   |   |   |       |-- http-signal.ts   # Generic HTTP signal adapter (OpenAI, Google, custom)
|   |   |   |
|   |   |   |-- services/
|   |   |   |   |-- moderation-config.ts # Rules, actions, policies, item types CRUD
|   |   |   |   |-- mrt.ts              # Manual review: enqueue, assign, decide
|   |   |   |   |-- item-processing.ts  # Validate + normalize incoming items
|   |   |   |   |-- user-management.ts  # User CRUD, invites, password reset
|   |   |   |   |-- api-keys.ts         # API key create, verify, rotate
|   |   |   |   |-- signing-keys.ts     # RSA-PSS keypair management
|   |   |   |   |-- analytics.ts        # Query execution/action logs for dashboards
|   |   |   |   |-- text-banks.ts       # Text/regex bank management
|   |   |   |   |-- user-strikes.ts     # Strike tracking and threshold enforcement
|   |   |   |   |-- investigation.ts    # Item lookup by ID/type
|   |   |   |   |-- reports.ts          # Report intake, default queue routing
|   |   |   |
|   |   |   |-- jobs/
|   |   |   |   |-- interface.ts         # JobQueue interface (enqueue + work)
|   |   |   |   |-- pg-boss-adapter.ts   # pg-boss implementation of JobQueue
|   |   |   |   |-- process-item.ts      # Async item processing worker
|   |   |   |   |-- refresh-views.ts     # Refresh materialized views periodically
|   |   |   |
|   |   |   |-- utils/
|   |   |       |-- crypto.ts           # Webhook signing, API key hashing, token generation
|   |   |       |-- cache.ts            # Simple TTL in-memory cache (~30 lines)
|   |   |       |-- errors.ts           # Error types and factory
|   |   |       |-- logger.ts           # Structured JSON logger implementation (~15 lines)
|   |   |
|   |   |-- vitest.config.ts
|   |
|   |-- client/                     # Frontend
|       |-- package.json
|       |-- vite.config.ts
|       |-- index.html
|       |-- src/
|           |-- main.tsx
|           |-- App.tsx
|           |-- api/                # Typed fetch wrappers (generated from shared types)
|           |   |-- client.ts       # Base fetch with auth headers
|           |   |-- hooks.ts        # TanStack Query hooks
|           |-- pages/
|           |   |-- Login.tsx
|           |   |-- Dashboard.tsx       # Overview analytics
|           |   |-- Rules.tsx           # Rule list
|           |   |-- RuleForm.tsx        # Create/edit rule
|           |   |-- RuleDetail.tsx      # Rule details + execution history
|           |   |-- ReviewQueue.tsx     # MRT queue list
|           |   |-- ReviewJob.tsx       # Single review job
|           |   |-- Policies.tsx        # Policy tree
|           |   |-- Settings.tsx        # Org settings (item types, actions, users, API keys)
|           |   |-- ItemTypes.tsx       # Item type management
|           |   |-- Actions.tsx         # Action management
|           |   |-- Users.tsx           # User management
|           |   |-- ApiKeys.tsx         # API key management
|           |   |-- TextBanks.tsx       # Text bank management
|           |   |-- Investigation.tsx   # Item investigation/lookup
|           |-- components/
|           |   |-- ui/             # shadcn/ui components (copied in, not a dep)
|           |   |-- layout/
|           |   |   |-- Shell.tsx   # App shell with sidebar nav
|           |   |   |-- Header.tsx
|           |   |-- rules/
|           |   |   |-- ConditionBuilder.tsx   # Visual condition tree builder
|           |   |   |-- SignalPicker.tsx
|           |   |-- mrt/
|           |   |   |-- JobCard.tsx
|           |   |   |-- DecisionPanel.tsx
|           |   |-- shared/
|           |       |-- DataTable.tsx    # Generic sortable/filterable table
|           |       |-- JsonViewer.tsx
|           |       |-- ConfirmDialog.tsx
|           |-- lib/
|               |-- auth.tsx        # Auth context + hooks
|               |-- utils.ts        # cn() helper, misc
```

**File count estimate: ~80 source files** (vs. hundreds in the original).

---

## 6. Data Model

### PostgreSQL Schema (single `public` schema, not 9 schemas)

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

-- Password reset tokens (replaces JWT-based resets)
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
  private_key TEXT NOT NULL,  -- Encrypted at rest
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

-- History table for policies (temporal versioning)
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

-- Rules (the core of the system)
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

-- History table for rules (temporal versioning)
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

-- Actions (what happens when rules match)
CREATE TABLE actions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  action_type     TEXT NOT NULL CHECK (action_type IN ('WEBHOOK', 'ENQUEUE_TO_MRT', 'ENQUEUE_AUTHOR_TO_MRT')),
  config          JSONB NOT NULL DEFAULT '{}',  -- callback_url, headers, body template, queue_id
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- History table for actions (temporal versioning)
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

-- Many-to-many: rules <-> actions
CREATE TABLE rules_actions (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  action_id   TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, action_id)
);

-- Many-to-many: rules <-> policies
CREATE TABLE rules_policies (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  policy_id   TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, policy_id)
);

-- Many-to-many: rules <-> item_types
CREATE TABLE rules_item_types (
  rule_id      TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  item_type_id TEXT NOT NULL REFERENCES item_types(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, item_type_id)
);

-- Many-to-many: actions <-> item_types (restricts which actions apply to which item types)
CREATE TABLE actions_item_types (
  action_id    TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  item_type_id TEXT NOT NULL REFERENCES item_types(id) ON DELETE CASCADE,
  PRIMARY KEY (action_id, item_type_id)
);

-- Text banks (shared word/phrase lists for signals)
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
  payload         JSONB NOT NULL,       -- Full item data + context at time of enqueue
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
  action_ids      TEXT[] NOT NULL DEFAULT '{}',  -- Actions to execute as part of this decision
  policy_ids      TEXT[] NOT NULL DEFAULT '{}',  -- Policies applied
  reason          TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Routing rules: which queue an item goes to
CREATE TABLE mrt_routing_rules (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  queue_id        TEXT NOT NULL REFERENCES mrt_queues(id),
  condition_set   JSONB NOT NULL,   -- Same condition format as rules
  priority        INTEGER NOT NULL DEFAULT 0,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User strikes
CREATE TABLE user_strikes (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  user_item_id    TEXT NOT NULL,      -- The moderated user's item ID
  user_type_id    TEXT NOT NULL,
  strike_count    INTEGER NOT NULL DEFAULT 0,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, user_item_id, user_type_id)
);

-- Execution logs (replaces ClickHouse/Snowflake for analytics)
-- Uses PostgreSQL declarative partitioning by month from day one.
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

-- Create initial partitions (migration creates current + next month)
-- A scheduled job creates future partitions and optionally detaches old ones.

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

-- Items ledger (replaces Scylla for investigation)
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

-- Sessions (custom Kysely-backed store, replaces connect-pg-simple)
CREATE TABLE sessions (
  sid     TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  data    JSONB NOT NULL DEFAULT '{}',
  expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Indexes for common queries
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

-- pg-boss uses its own schema, no manual table creation needed
```

**Table count: 25** (vs. 40+ across 9 schemas in the original).

> **Design Decision: Merge text-contains into text-regex (Feedback #15).**
> A regex signal with `new RegExp(escapeRegExp(literal))` subsumes exact text matching. There is no performance benefit to a separate contains signal -- `String.includes()` vs a compiled regex literal is negligible. One signal, fewer concepts to learn, fewer code paths to maintain. The `text-regex` signal accepts a `mode` parameter: `'literal'` (auto-escaped) or `'regex'` (raw pattern).

> **Design Decision: Drop XOR conjunction (Feedback #18).**
> The original codebase supports XOR in condition trees, but in practice it is unused. XOR between content moderation conditions ("exactly one of these signals fires") has no clear real-world use case. Removing it simplifies the condition evaluator and the UI condition builder. AND and OR are sufficient. If XOR is ever needed, it can be added without breaking changes since the `conjunction` field is already a string type.

> **Design Decision: Add `actions_item_types` join table (Feedback #4).**
> The original codebase has this relationship. Without it, there is no way to restrict which actions are applicable to which item types. This matters for the UI (action picker should only show compatible actions) and for validation (reject rule configurations that pair an action with an incompatible item type).

> **Design Decision: Add `actions_history` and `policies_history` tables (Feedback #16).**
> Invariant #6 states history tables are append-only for rules, actions, and policies. The original architecture doc only had `rules_history`. This was an inconsistency. All three entity types that support temporal versioning now have history tables.

> **Design Decision: Partition execution log tables from day one (Feedback #17).**
> `rule_executions` and `action_executions` are append-only, time-series data that grow without bound. Declarative partitioning by month costs nothing at small scale and prevents a painful migration later. A scheduled job (via pg-boss) creates future partitions monthly and optionally detaches/archives old ones.

> **Design Decision: Defer text-similarity signal (Feedback #20).**
> The original `text-similarity` signal was vague about its algorithm. If it means embedding-based similarity, that requires pgvector -- a significant dependency that adds complexity to deployment and schema management. If it means edit-distance, that is a v1.1 addition alongside fuzzy matching. Either way, it does not belong in v1.0. Removed from built-in signals.

---

## 7. Module Contracts

### 7.0 Logger Interface

```typescript
// packages/shared/src/types/logger.ts

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface Logger {
  debug(msg: string, data?: Record<string, unknown>): void;
  info(msg: string, data?: Record<string, unknown>): void;
  warn(msg: string, data?: Record<string, unknown>): void;
  error(msg: string, data?: Record<string, unknown>): void;
  child(bindings: Record<string, unknown>): Logger;
}
```

```typescript
// packages/server/src/utils/logger.ts

import { type Logger, type LogLevel } from '@coop-lite/shared';

/**
 * Structured JSON logger that writes to stdout.
 * ~15 lines of implementation. No dependencies.
 *
 * Output format: {"level":"info","msg":"...","ts":"...","key":"value"}\n
 */
export function createLogger(level: LogLevel): Logger;
```

### 7.1 JobQueue Interface

```typescript
// packages/server/src/jobs/interface.ts

export interface JobQueue {
  /**
   * Enqueue a job for async processing.
   *
   * Pre-conditions: name is a registered job type, data is JSON-serializable.
   * Post-conditions: job is durably stored and will be picked up by a worker.
   */
  enqueue(name: string, data: Record<string, unknown>): Promise<string>; // Returns job ID

  /**
   * Register a worker for a job type.
   *
   * Pre-conditions: name is a valid job type string.
   * Post-conditions: handler will be called for each job of this type, with at-least-once delivery.
   */
  work(name: string, handler: (data: Record<string, unknown>) => Promise<void>): Promise<void>;
}
```

```typescript
// packages/server/src/jobs/pg-boss-adapter.ts

import PgBoss from 'pg-boss';
import { type JobQueue } from './interface.js';

/**
 * pg-boss implementation of JobQueue.
 * Can be swapped for BullMQ/Redis or Kafka without changing callers.
 */
export class PgBossJobQueue implements JobQueue {
  constructor(private boss: PgBoss) {}
  async enqueue(name: string, data: Record<string, unknown>): Promise<string>;
  async work(name: string, handler: (data: Record<string, unknown>) => Promise<void>): Promise<void>;
}
```

### 7.2 Signal Adapter Interface

This is the most important interface in the system. Every signal -- built-in or external -- implements this.

```typescript
// packages/shared/src/types/signals.ts

export type SignalInputType = 'TEXT' | 'IMAGE' | 'VIDEO' | 'AUDIO' | 'FULL_ITEM';

export type SignalOutput = {
  score: number;          // 0.0 to 1.0 (normalized)
  label?: string;         // Optional classification label
  subcategory?: string;   // Optional subcategory ID
  metadata?: Record<string, unknown>;
};

export type SignalInput = {
  value: { type: SignalInputType; value: string } | Record<string, unknown>;
  matchingValues?: string[];   // For bank-based signals
  orgId: string;
};

// packages/server/src/signals/interface.ts

export interface SignalAdapter {
  readonly id: string;
  readonly displayName: string;
  readonly description: string;
  readonly eligibleInputs: readonly SignalInputType[];
  readonly cost: number;  // 0 = free/instant, higher = more expensive

  run(input: SignalInput): Promise<SignalOutput>;
}
```

**Why this matters:** New signals (AI models, hash matchers, custom HTTP endpoints) are added by implementing `SignalAdapter` and registering in the signal registry. Zero changes to the rule engine.

### 7.3 Rule Engine

```typescript
// packages/server/src/engine/rule-engine.ts

export interface RuleEngineResult {
  actionsTriggered: ActionRequest[];
  ruleResults: Map<string, { passed: boolean; conditionResults: ConditionSetResult }>;
}

export class RuleEngine {
  constructor(
    private db: Database,
    private signalRegistry: SignalRegistry,
    private cache: SimpleCache,
    private logger: Logger,
  ) {}

  /**
   * Run all enabled rules for an item type against the given item.
   *
   * Returns the list of ActionRequests that should be executed.
   * Does NOT execute actions itself -- the caller (route handler) orchestrates
   * action execution via ActionPublisher.
   *
   * Pre-conditions:
   *   - item.data has been validated against item type schema
   *   - orgId is a valid organization
   *
   * Post-conditions:
   *   - All LIVE rules for the item type have been evaluated
   *   - BACKGROUND rules evaluated but no actions included in result
   *   - Rule executions logged to rule_executions table
   *   - Returned ActionRequest[] contains deduplicated actions from all passing LIVE rules
   */
  async runEnabledRules(
    orgId: string,
    item: ValidatedItem,
    options?: { sync?: boolean },
  ): Promise<RuleEngineResult>;
}
```

### 7.4 Condition Evaluator

```typescript
// packages/server/src/engine/condition-set.ts

export type ConditionSetResult = {
  conjunction: 'AND' | 'OR';
  outcome: 'PASSED' | 'FAILED' | 'ERRORED';
  conditions: (LeafConditionResult | ConditionSetResult)[];
};

/**
 * Recursively evaluate a condition set with:
 *   - Cost-based ordering (cheapest signals first)
 *   - Short-circuit evaluation (AND stops on first false, OR on first true)
 *   - Three-valued logic (ERRORED propagates as null)
 *
 * Raises:
 *   Never throws. Errors in leaf conditions are caught and marked ERRORED.
 */
export async function evaluateConditionSet(
  conditionSet: ConditionSet,
  context: EvaluationContext,
): Promise<ConditionSetResult>;
```

### 7.5 Action Publisher

```typescript
// packages/server/src/engine/action-publisher.ts

export type ActionRequest = {
  actionId: string;
  actionType: 'WEBHOOK' | 'ENQUEUE_TO_MRT' | 'ENQUEUE_AUTHOR_TO_MRT';
  config: Record<string, unknown>;
  policyIds: string[];
  ruleIds: string[];
};

export class ActionPublisher {
  constructor(
    private db: Database,
    private signingKeys: SigningKeyService,
    private logger: Logger,
  ) {}

  /**
   * Execute a set of action requests.
   *
   * For WEBHOOK actions: POST to callback URL with signed body.
   * For ENQUEUE_TO_MRT actions: create MRT job in the appropriate queue.
   * For ENQUEUE_AUTHOR_TO_MRT actions: create MRT job for the content author.
   *
   * Retries webhook calls up to 5 times with exponential backoff (via pg-boss scheduled retry).
   * Never throws -- individual action failures are logged and returned as results.
   *
   * Pre-conditions:
   *   - actions have been validated against actions_item_types (if applicable)
   * Post-conditions:
   *   - Each action either succeeded or has a logged failure
   *   - MRT jobs created for ENQUEUE actions
   *   - action_executions logged for all actions
   */
  async publishActions(
    actions: ActionRequest[],
    target: { orgId: string; itemId: string; itemTypeId: string; item: ValidatedItem },
  ): Promise<ActionResult[]>;
}
```

### 7.6 MRT Service

```typescript
// packages/server/src/services/mrt.ts

export type DecisionParams = {
  orgId: string;
  jobId: string;
  userId: string;
  verdict: 'APPROVE' | 'REJECT' | 'ESCALATE' | 'IGNORE';
  actionIds: string[];    // Actions to execute as part of this decision
  policyIds: string[];    // Policies to apply
  reason?: string;
};

export type DecisionResult = {
  decisionId: string;
  actionRequests: ActionRequest[];  // Actions to execute, returned to caller for orchestration
};

export class MRTService {
  constructor(
    private db: Database,
    private logger: Logger,
  ) {}

  /** Enqueue an item for human review. Routes to correct queue via routing rules, falls back to default queue. */
  async enqueue(params: EnqueueParams): Promise<string>; // Returns job ID

  /** Get next unassigned job from a queue and assign to user. */
  async assignNext(queueId: string, userId: string): Promise<MRTJob | null>;

  /**
   * Record a moderator's decision.
   *
   * Returns ActionRequest[] for any actions the decision triggers.
   * Does NOT call ActionPublisher -- the route handler orchestrates.
   * This breaks the MRTService <-> ActionPublisher circular dependency.
   *
   * Pre-conditions:
   *   - job exists, is ASSIGNED, and is assigned to userId
   *   - actionIds reference valid actions for this org
   *   - policyIds reference valid policies for this org
   * Post-conditions:
   *   - mrt_jobs.status updated to 'DECIDED'
   *   - mrt_decisions row inserted with verdict + action_ids + policy_ids
   *   - Returned ActionRequest[] ready for ActionPublisher
   */
  async recordDecision(params: DecisionParams): Promise<DecisionResult>;

  /** Get queue statistics (pending, assigned, decided counts). */
  async getQueueStats(orgId: string): Promise<QueueStats[]>;

  /** Get jobs for a queue with filtering and pagination. */
  async getJobs(queueId: string, filters: JobFilters): Promise<PaginatedResult<MRTJob>>;
}
```

### 7.7 Reports Service

```typescript
// packages/server/src/services/reports.ts

export class ReportsService {
  constructor(
    private db: Database,
    private mrtService: MRTService,
    private logger: Logger,
  ) {}

  /**
   * Intake a user report and enqueue to the org's default MRT queue.
   *
   * Pre-conditions:
   *   - org has at least one MRT queue with is_default = true
   *   - reportedItemId + reportedItemTypeId reference a valid item
   * Post-conditions:
   *   - MRT job created with enqueue_source = 'REPORT'
   *   - source_info contains reporter details and report reason
   *
   * Raises:
   *   ConfigError: if no default MRT queue exists for the org
   */
  async submitReport(params: {
    orgId: string;
    reportedItemId: string;
    reportedItemTypeId: string;
    reporterItemId?: string;
    reason: string;
    metadata?: Record<string, unknown>;
  }): Promise<string>; // Returns MRT job ID
}
```

> **Design Decision: Reports always enqueue to default MRT queue (Feedback #1).**
> The original coop has a full reporting rules engine (`reportingRuleEngine.ts`, `ReportingRules.ts`, etc.) that evaluates conditions to route reports. This is a significant amount of complexity for v1.0. Instead, all reports enqueue to the org's default MRT queue with `enqueue_source = 'REPORT'`. The `source_info` JSONB column carries the report context (reporter, reason). Queue routing rules (v1.1) can later provide sophisticated routing. This covers the 90% case -- most orgs have a single review queue for reports.

### 7.8 Database Adapter

Kysely IS the database adapter. No abstraction layer on top -- Kysely already provides a clean interface and supports multiple dialects.

```typescript
// packages/server/src/db/connection.ts

import { Kysely, PostgresDialect } from 'kysely';
import { Pool } from 'pg';
import type { Database } from './schema.js';

export function createDatabase(connectionString: string): Kysely<Database> {
  return new Kysely<Database>({
    dialect: new PostgresDialect({
      pool: new Pool({ connectionString, max: 20 }),
    }),
  });
}
```

The `Database` type in `schema.ts` is the Kysely type-safe table definition, generated from and kept in sync with the migration SQL above.

### 7.9 Auth Middleware

```typescript
// packages/server/src/auth/middleware.ts

/**
 * Session-based auth for UI requests.
 * Reads session cookie, looks up session in sessions table via Kysely,
 * attaches user to context.
 *
 * Returns 401 if no valid session.
 */
export function sessionAuth(db: Database): MiddlewareHandler;

/**
 * API key auth for REST API requests.
 * Reads X-API-Key header, hashes it, looks up in api_keys table.
 *
 * Returns 401 if invalid or revoked key.
 */
export function apiKeyAuth(db: Database): MiddlewareHandler;

/**
 * Role-based access control. Checks that the authenticated user
 * has one of the required roles for this endpoint.
 *
 * Returns 403 if insufficient permissions.
 */
export function requireRole(...roles: UserRole[]): MiddlewareHandler;
```

### 7.10 Custom Session Store

```typescript
// packages/server/src/auth/sessions.ts

/**
 * Kysely-backed session store. ~40 lines.
 * Replaces express-session + connect-pg-simple.
 *
 * Session lifecycle:
 *   - create(userId): generates crypto.randomBytes(32) token, stores SHA-256 hash
 *   - validate(token): hashes token, looks up session, checks expiry
 *   - destroy(token): deletes session row
 *   - cleanup(): deletes expired sessions (called periodically via pg-boss job)
 *
 * The session token is set as an HttpOnly, Secure, SameSite=Lax cookie.
 */
export class SessionStore {
  constructor(private db: Database) {}

  async create(userId: string): Promise<string>;   // Returns raw session token
  async validate(token: string): Promise<{ userId: string; orgId: string } | null>;
  async destroy(token: string): Promise<void>;
  async cleanup(): Promise<number>;  // Returns count of expired sessions removed
}
```

---

## 8. Data Flow

### Item Submission (Async)

```
Client App
  |
  POST /api/v1/items { items: [...] }
  |
  apiKeyAuth middleware  -->  verify API key hash against api_keys table
  |
  ItemRoutes.submitItems()
  |   - Validate each item against its item_type schema (Zod)
  |   - Store items in `items` table
  |   - Enqueue job per item to JobQueue "process-item"
  |
  Return 202 Accepted
  |
  JobQueue worker picks up "process-item" job
  |
  processItem(item)
  |   - RuleEngine.runEnabledRules(orgId, item) -> RuleEngineResult
  |       |
  |       - Fetch enabled rules for item type (cached, 30s TTL)
  |       - Partition into LIVE and BACKGROUND
  |       - For each rule:
  |           - evaluateConditionSet(rule.conditionSet, context)
  |               |
  |               - Sort conditions by signal cost (ascending)
  |               - For each condition:
  |                   - If leaf: signalRegistry.run(signalId, input)
  |                   - If nested set: recurse
  |                   - Short-circuit if outcome determined
  |               - Return ConditionSetResult
  |       - For LIVE passing rules: collect ActionRequest[]
  |       - Deduplicate actions
  |       - Log rule_executions
  |       - Return RuleEngineResult with ActionRequest[]
  |
  |   - Route handler calls ActionPublisher.publishActions(result.actionsTriggered, target)
  |       |
  |       - WEBHOOK: fetch(callbackUrl, { body, headers, signature })
  |       - ENQUEUE_TO_MRT: db insert into mrt_jobs
  |       - ENQUEUE_AUTHOR_TO_MRT: resolve author from item, db insert into mrt_jobs
  |       - Log action_executions
  |
  Done
```

### Item Submission (Sync)

Same flow, except `runEnabledRules` is called inline (not via JobQueue), and the HTTP response waits for completion and returns the triggered actions.

### Manual Review Decision

```
Moderator UI
  |
  POST /api/v1/mrt/decisions { jobId, verdict, actionIds, policyIds, reason }
  |
  sessionAuth + requireRole('MODERATOR', 'ADMIN')
  |
  Route handler:
  |   1. MRTService.recordDecision(params) -> DecisionResult
  |       - Update mrt_jobs status to 'DECIDED'
  |       - Insert into mrt_decisions (verdict + action_ids + policy_ids)
  |       - Return DecisionResult with ActionRequest[]
  |
  |   2. ActionPublisher.publishActions(result.actionRequests, target)
  |       - Execute webhook, MRT enqueue, or author enqueue actions
  |       - Log action_executions
  |
  |   3. If policies have strike_penalty > 0:
  |       - Increment user_strikes
  |
  Return 200
```

> **Design Decision: Route handler orchestrates MRT -> ActionPublisher flow (Feedback #12).**
> In the original architecture, `MRTService` called `ActionPublisher` directly, creating a circular dependency (`MRTService -> ActionPublisher -> MRTService` for ENQUEUE actions). Now `MRTService.recordDecision()` returns `ActionRequest[]` and the route handler passes them to `ActionPublisher`. This makes the dependency graph a clean DAG: `routes -> {MRTService, ActionPublisher} -> db`. Neither service references the other.

### Report Intake

```
Client App
  |
  POST /api/v1/reports { reportedItemId, reportedItemTypeId, reason, ... }
  |
  apiKeyAuth middleware
  |
  ReportsService.submitReport(params)
  |   - Look up org's default MRT queue (is_default = true)
  |   - MRTService.enqueue({
  |       source: 'REPORT',
  |       sourceInfo: { reporterItemId, reason, metadata }
  |     })
  |
  Return 201 { jobId }
```

---

## 9. Composition Root (Replacing IoC Container)

The 1,878-line Bottle.js IoC container is replaced by a single ~80-line file that constructs everything explicitly.

```typescript
// packages/server/src/compose.ts

import { createDatabase } from './db/connection.js';
import { RuleEngine } from './engine/rule-engine.js';
import { RuleEvaluator } from './engine/rule-evaluator.js';
import { ActionPublisher } from './engine/action-publisher.js';
import { SignalRegistry } from './signals/registry.js';
import { MRTService } from './services/mrt.js';
import { ReportsService } from './services/reports.js';
import { ModerationConfigService } from './services/moderation-config.js';
import { UserManagementService } from './services/user-management.js';
import { ApiKeyService } from './services/api-keys.js';
import { SigningKeyService } from './services/signing-keys.js';
import { AnalyticsService } from './services/analytics.js';
import { TextBankService } from './services/text-banks.js';
import { UserStrikeService } from './services/user-strikes.js';
import { InvestigationService } from './services/investigation.js';
import { SessionStore } from './auth/sessions.js';
import { PgBossJobQueue } from './jobs/pg-boss-adapter.js';
import { SimpleCache } from './utils/cache.js';
import { createLogger } from './utils/logger.js';
// ... builtin signal imports

export function compose(config: AppConfig) {
  const logger = createLogger(config.logLevel);
  const db = createDatabase(config.databaseUrl);
  const cache = new SimpleCache();
  const sessionStore = new SessionStore(db);

  // Job queue
  const jobQueue = new PgBossJobQueue(/* pg-boss instance */);

  // Services (no circular dependencies)
  const apiKeyService = new ApiKeyService(db);
  const signingKeyService = new SigningKeyService(db);
  const userManagement = new UserManagementService(db);
  const configService = new ModerationConfigService(db, cache);
  const textBankService = new TextBankService(db);
  const userStrikeService = new UserStrikeService(db);
  const investigationService = new InvestigationService(db);
  const analyticsService = new AnalyticsService(db);

  // MRT + Reports (MRTService has no dependency on ActionPublisher)
  const mrtService = new MRTService(db, logger);
  const reportsService = new ReportsService(db, mrtService, logger);

  // Engine (ActionPublisher does not depend on MRTService)
  const actionPublisher = new ActionPublisher(db, signingKeyService, logger);

  // Signals
  const signalRegistry = new SignalRegistry();
  signalRegistry.register(new TextRegexSignal());  // Subsumes text-contains
  signalRegistry.register(new TextBankSignal(textBankService));
  // External signals registered via config or at runtime

  // Rule engine
  const ruleEvaluator = new RuleEvaluator(signalRegistry);
  const ruleEngine = new RuleEngine(db, signalRegistry, cache, logger);

  return {
    db, logger, cache, sessionStore, jobQueue,
    apiKeyService, signingKeyService, userManagement,
    configService, textBankService, userStrikeService,
    investigationService, analyticsService,
    signalRegistry, ruleEngine, ruleEvaluator,
    actionPublisher, mrtService, reportsService,
  };
}

export type App = ReturnType<typeof compose>;
```

Every service takes its dependencies as constructor arguments. No container registration, no string-based lookups, no magic. TypeScript checks everything at compile time. No circular dependencies -- `MRTService` and `ActionPublisher` are independent; the route handler orchestrates their interaction.

---

## 10. In-Memory Cache Strategy

Replace Redis + LRU_Map + distributed caching with a trivial in-memory cache.

```typescript
// packages/server/src/utils/cache.ts

export class SimpleCache {
  private store = new Map<string, { value: unknown; expires: number }>();

  get<T>(key: string): T | undefined {
    const entry = this.store.get(key);
    if (!entry) return undefined;
    if (Date.now() > entry.expires) {
      this.store.delete(key);
      return undefined;
    }
    return entry.value as T;
  }

  set<T>(key: string, value: T, ttlMs: number = 30_000): void {
    this.store.set(key, { value, expires: Date.now() + ttlMs });
  }

  invalidate(pattern: string): void {
    for (const key of this.store.keys()) {
      if (key.startsWith(pattern)) this.store.delete(key);
    }
  }
}
```

Cached items (with TTL):
- Enabled rules per item type: 30s
- Policies per rule: 30s
- Actions per rule: 30s
- Item type schemas: 60s
- Signal list per org: 60s

This is exactly what the original does (it uses 10-120s eventual consistency caches). The difference is we use a 30-line Map instead of Redis.

---

## 11. Dependency List

### Server (7 production dependencies)

| Package | Why |
|---------|-----|
| `hono` | HTTP framework. 14KB, fast, TypeScript-first. |
| `kysely` | Type-safe SQL query builder. Already partially adopted. |
| `pg` | PostgreSQL driver for Kysely. |
| `pg-boss` | PostgreSQL-native job queue. Replaces Kafka + BullMQ + Redis. |
| `zod` | Runtime validation + type inference. Replaces AJV + hand-written validation. |
| `bcryptjs` | Password hashing. Same as original. |
| `@hono/node-server` | Node.js adapter for Hono (if not using Hono's built-in). |

### Client (11 production dependencies)

| Package | Why |
|---------|-----|
| `react` + `react-dom` | UI library. |
| `react-router-dom` | Client-side routing. |
| `@tanstack/react-query` | Server state management. Replaces Apollo Client. |
| `recharts` | Charts for analytics dashboard. |
| `tailwindcss` | Utility CSS. |
| `date-fns` | Date formatting (client only -- user-facing display). |
| `clsx` + `tailwind-merge` | ClassName utilities for shadcn/ui. |
| `lucide-react` | Icon set. |
| `@tanstack/react-table` | Data tables with sort/filter. |
| `zod` | Form validation (shared schemas). |

### Dev dependencies (shared, ~10)
`typescript`, `vitest`, `@types/node`, `vite`, `eslint`, `prettier`, `tsx` (for running TS scripts), `@vitejs/plugin-react`

**Total production dependencies: ~18** (vs. 350+ in the original).

---

## 12. API Design (REST, Not GraphQL)

All endpoints prefixed with `/api/v1`. UI requests use session auth. External requests use API key auth.

### External API (API key auth)

```
POST   /api/v1/items              # Submit items (sync evaluation)
POST   /api/v1/items/async        # Submit items (async, returns 202)
POST   /api/v1/reports            # Submit user report -> enqueues to default MRT queue
POST   /api/v1/appeals            # Submit appeal
GET    /api/v1/policies           # List policies for org
```

### Internal API (Session auth)

```
# Rules
GET    /api/v1/rules              # List rules (filterable)
POST   /api/v1/rules              # Create rule
GET    /api/v1/rules/:id          # Get rule detail
PUT    /api/v1/rules/:id          # Update rule
DELETE /api/v1/rules/:id          # Delete rule (soft)

# Actions
GET    /api/v1/actions            # List actions
POST   /api/v1/actions            # Create action
PUT    /api/v1/actions/:id        # Update action
DELETE /api/v1/actions/:id        # Delete action

# Policies
GET    /api/v1/policies           # List/tree
POST   /api/v1/policies           # Create
PUT    /api/v1/policies/:id       # Update
DELETE /api/v1/policies/:id       # Delete

# Item Types
GET    /api/v1/item-types         # List
POST   /api/v1/item-types         # Create
PUT    /api/v1/item-types/:id     # Update
DELETE /api/v1/item-types/:id     # Delete

# MRT
GET    /api/v1/mrt/queues                 # List queues with stats
GET    /api/v1/mrt/queues/:id/jobs        # List jobs in queue
POST   /api/v1/mrt/queues/:id/assign      # Assign next job to current user
POST   /api/v1/mrt/decisions              # Record decision (verdict + actionIds + policyIds)
GET    /api/v1/mrt/jobs/:id               # Get job detail

# Users
GET    /api/v1/users              # List users in org
POST   /api/v1/users/invite       # Invite user
PUT    /api/v1/users/:id          # Update user role
DELETE /api/v1/users/:id          # Deactivate user

# API Keys
GET    /api/v1/api-keys           # List (prefix only, no secrets)
POST   /api/v1/api-keys           # Create (returns key once)
DELETE /api/v1/api-keys/:id       # Revoke

# Auth
POST   /api/v1/auth/login         # Login
POST   /api/v1/auth/logout        # Logout
GET    /api/v1/auth/me            # Current user
POST   /api/v1/auth/reset-password # Request password reset (opaque token, not JWT)

# Text Banks
GET    /api/v1/text-banks         # List banks
POST   /api/v1/text-banks         # Create bank
GET    /api/v1/text-banks/:id     # Get bank with entries
POST   /api/v1/text-banks/:id/entries  # Add entries
DELETE /api/v1/text-banks/:id/entries/:entryId  # Remove entry

# Analytics
GET    /api/v1/analytics/rule-executions     # Rule execution counts over time
GET    /api/v1/analytics/action-executions    # Action execution counts over time
GET    /api/v1/analytics/queue-throughput     # MRT throughput stats

# Investigation
GET    /api/v1/investigation/items/:typeId/:itemId   # Lookup item + history
GET    /api/v1/investigation/search                  # Search items

# Signals
GET    /api/v1/signals                       # List available signals
POST   /api/v1/signals/test                  # Test a signal against sample input

# Health
GET    /api/v1/health             # Health check
```

> **Design Decision: Rate limiting deferred from architecture, noted for implementation (Feedback #19).**
> Rate limiting for external API endpoints (`/items`, `/items/async`, `/reports`, `/appeals`) is important but is an implementation detail, not an architectural concern. It should be implemented as Hono middleware using a sliding window counter in PostgreSQL (or in-memory for single-instance deployments). Added to v1.1 scope. The middleware signature: `rateLimit({ windowMs: 60_000, max: 100 })` applied to external API routes.

---

## 13. Invariants

These must hold true at all times:

1. **Every database query includes `org_id` in its WHERE clause.** Multi-tenant isolation is non-negotiable. No query ever crosses org boundaries.

2. **No circular dependencies between modules.** The dependency graph is a DAG: `routes -> {services, engine} -> db`. Services do not depend on each other circularly. The route handler orchestrates cross-service interactions (e.g., MRT decision -> action publishing).

3. **All public API functions have full TypeScript type annotations.** No `any` in public interfaces.

4. **Rule condition evaluation never throws.** Errors in individual leaf conditions are caught and marked `ERRORED`. The condition set evaluation continues.

5. **Webhook action failures do not block other actions.** Each action executes independently. Failures are logged and returned in results.

6. **History tables are append-only.** When a rule, action, or policy is updated, the old version is inserted into the corresponding `*_history` table before the update. History tables exist for: `rules_history`, `actions_history`, `policies_history`.

7. **API keys are never stored in plaintext.** Only SHA-256 hashes are persisted. The raw key is returned exactly once on creation.

8. **Session auth and API key auth are mutually exclusive per request.** A request is authenticated by one mechanism only.

9. **ActionPublisher and MRTService have no direct dependency on each other.** Cross-cutting orchestration happens in route handlers, not in services.

10. **Condition trees use AND and OR conjunctions only.** XOR is not supported. This simplifies evaluation and UI.

11. **The JobQueue interface is the only abstraction over the job processing system.** All job enqueue/work calls go through the 2-method interface, never through pg-boss directly (except in the adapter implementation).

12. **The Logger interface is the only abstraction over logging.** All services accept `Logger` via constructor injection, never import a concrete logger directly.

---

## 14. Migration Path from Original Coop

This is not a rewrite-in-place. It is a new project that can run alongside the original.

### Phase 1: Data Migration
- Write a migration script that reads from the original's PostgreSQL (9 schemas) and writes to the new single-schema format.
- Map original rule `conditionSet` JSONB directly (format is compatible, minus XOR nodes which must be flagged for manual review).
- Map original actions, policies, item types.
- Map original MRT queues and pending jobs.
- Do NOT migrate ClickHouse/Scylla data (analytics can start fresh).

### Phase 2: API Compatibility Layer
- The external REST API (`/items`, `/content`, `/reports`) should accept the same request format as the original. This allows clients to switch endpoints without code changes.
- Webhook callback payloads should match the original format.

### Phase 3: Parallel Run
- Run both systems simultaneously, with the new system in shadow mode (processing items but not executing actions).
- Compare rule evaluation results between old and new.
- Switch over when results match.

---

## 15. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| PostgreSQL analytics too slow at scale | Medium | Medium | Execution log tables use declarative partitioning by month from day one. Partition pruning keeps queries fast. At true high scale (>100M rows/month), add ClickHouse behind an adapter interface. |
| pg-boss insufficient for high-throughput item processing | Low | High | pg-boss handles ~1,000 jobs/sec on modest hardware. If that is not enough, swap the `PgBossJobQueue` for a `BullMQJobQueue` or `KafkaJobQueue` implementation -- the `JobQueue` interface (2 methods) makes this a contained change. |
| Missing signal types block adoption | Medium | High | The `SignalAdapter` interface is designed for exactly this. Each signal is a standalone file implementing one interface. Adding a new signal (OpenAI moderation, Google Content Safety, image hashing) requires zero changes to the engine. External signals use plain `fetch`, no vendor SDKs. |
| No GraphQL hurts frontend developer experience | Low | Low | TanStack Query with typed fetch wrappers provides equivalent DX. The shared types package ensures client/server type agreement at compile time. There is no runtime schema to keep in sync. |
| Session auth insufficient for enterprise | Medium | Low | The auth middleware interface supports pluggable strategies. SAML/SSO can be added as an additional middleware that creates sessions in the same session store. The `users` table already supports it (add `sso_provider` and `sso_id` columns when needed). |
| Custom logger insufficient for production debugging | Low | Low | The `Logger` interface can be swapped to Pino or any structured logger without changing callers. Start simple, upgrade if needed. |
| ENQUEUE_AUTHOR_TO_MRT requires author resolution | Low | Medium | The `items` table stores `creator_id` and `creator_type_id`. If these are not populated at submission time, the action fails gracefully (logged, not thrown). The action publisher validates author existence before enqueue. |

---

## 16. What This Looks Like In Practice

### Adding a new signal (e.g., OpenAI Moderation)

Create one file using plain `fetch`:

```typescript
// packages/server/src/signals/external/openai-moderation.ts

import type { SignalAdapter, SignalInput, SignalOutput } from '../interface.js';

export class OpenAIModerationSignal implements SignalAdapter {
  readonly id = 'openai-moderation';
  readonly displayName = 'OpenAI Moderation';
  readonly description = 'OpenAI content moderation API';
  readonly eligibleInputs = ['TEXT'] as const;
  readonly cost = 10;

  constructor(private apiKey: string) {}

  async run(input: SignalInput): Promise<SignalOutput> {
    const response = await fetch('https://api.openai.com/v1/moderations', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.apiKey}`,
      },
      body: JSON.stringify({ input: (input.value as { value: string }).value }),
    });

    if (!response.ok) {
      throw new Error(`OpenAI API error: ${response.status}`);
    }

    const data = await response.json();
    const scores = data.results[0].category_scores;
    const maxScore = Math.max(...Object.values(scores) as number[]);
    const maxCategory = Object.entries(scores).find(([, v]) => v === maxScore)?.[0];

    return {
      score: maxScore,
      label: maxCategory,
      metadata: scores,
    };
  }
}
```

Register it in `compose.ts`:

```typescript
if (config.openaiApiKey) {
  signalRegistry.register(new OpenAIModerationSignal(config.openaiApiKey));
}
```

Done. No changes to the rule engine, condition evaluator, or any other module. No vendor SDK dependency.

### Adding a new action type (e.g., NCMEC reporting)

1. Add the type to the `actions.action_type` CHECK constraint via migration.
2. Add a case to `ActionPublisher.publishAction` switch statement.
3. Add a new service class if the action type has complex logic.

### Creating a rule (API)

```bash
curl -X POST /api/v1/rules \
  -H "Cookie: session=..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Block hate speech",
    "status": "LIVE",
    "itemTypeIds": ["content-type-1"],
    "actionIds": ["webhook-action-1", "mrt-action-1"],
    "policyIds": ["hate-speech-policy"],
    "conditionSet": {
      "conjunction": "OR",
      "conditions": [
        {
          "signal": { "id": "openai-moderation" },
          "field": { "name": "text", "type": "TEXT" },
          "threshold": { "operator": ">=", "value": 0.8 },
          "subcategory": "hate"
        },
        {
          "signal": { "id": "text-regex" },
          "field": { "name": "text", "type": "TEXT" },
          "threshold": { "operator": ">=", "value": 1.0 },
          "config": { "mode": "literal", "pattern": "slur_word_here" }
        }
      ]
    }
  }'
```

---

## 17. Estimated Size

| Component | Lines of Code (estimate) |
|-----------|-------------------------|
| Shared types + schemas + interfaces | ~900 |
| Server: routes | ~1,200 |
| Server: engine (rule engine, evaluator, conditions, action publisher) | ~600 |
| Server: signals (interface + 2 builtins + 1 external adapter) | ~300 |
| Server: services (9 services including reports) | ~2,100 |
| Server: db (connection, schema, migrations) | ~500 |
| Server: auth (middleware, passwords, sessions, rbac) | ~350 |
| Server: jobs (interface, pg-boss adapter, workers) | ~250 |
| Server: utils (crypto, cache, errors, logger) | ~200 |
| Server: compose + app | ~150 |
| Client: pages (15 pages) | ~3,000 |
| Client: components | ~2,000 |
| Client: api + hooks | ~400 |
| Client: lib + utils | ~200 |
| **Total** | **~12,150** |

For reference, the original coop server alone has ~40,000+ lines of TypeScript (excluding generated code and tests). The client has another ~30,000+. This design targets roughly **15-20%** of the original codebase while retaining **80%+** of the functionality that matters.

---

## 18. Implementation Order

Build in this sequence. Each phase produces a working system.

### Phase 1: Foundation (Week 1-2)
1. Project scaffold (monorepo, TypeScript config, Vite, Hono)
2. Database schema + Kysely types + initial migration (including partitioned execution tables)
3. Logger implementation + JobQueue interface + pg-boss adapter
4. Auth (custom session store, passwords, API keys, RBAC middleware)
5. User management + org setup CLI script
6. Item type CRUD + item submission endpoint

### Phase 2: Engine (Week 3-4)
7. Signal interface + 2 built-in signals (text-regex with literal mode, text-bank)
8. Condition evaluator (recursive, short-circuit, AND/OR, three-valued)
9. Rule engine (fetch rules, evaluate, return ActionRequest[])
10. Action publisher (webhook with signing, MRT enqueue, author MRT enqueue)
11. Route handler orchestration (rule engine -> action publisher)
12. pg-boss async item processing worker
13. Rule CRUD endpoints

### Phase 3: Review Tool + Reports (Week 5)
14. MRT service (enqueue, assign, decide -- returning ActionRequest[])
15. Reports service (intake -> default MRT queue)
16. MRT API endpoints + decision orchestration in route handler
17. Policy CRUD (with history tables)

### Phase 4: Frontend (Week 6-8)
18. App shell, auth pages, routing
19. Rule list + rule form (condition builder)
20. Review queue + job review page (compound decisions: verdict + actions + policies)
21. Settings pages (item types, actions, users, API keys)
22. Policy management
23. Text bank management

### Phase 5: Polish (Week 9-10)
24. Analytics service + dashboard
25. Item investigation
26. Partition management job (create future partitions, archive old)
27. External signal adapter example (OpenAI via fetch)
28. Testing, hardening, documentation

**Total: ~10 weeks for 1-2 developers.**

---

## Appendix: Feedback Disposition Summary

| # | Feedback | Disposition |
|---|----------|-------------|
| 1 | Reports intake path incomplete | **Accepted.** Added `ReportsService` that enqueues to default MRT queue. Full reporting rules deferred. |
| 2 | `ENQUEUE_AUTHOR_TO_MRT` silently dropped | **Accepted.** Added as action type in schema, ActionPublisher contract, and data flow. |
| 3 | MRT decisions oversimplified | **Accepted.** Decision now includes `verdict` + `actionIds[]` + `policyIds[]`. Schema and contracts updated. |
| 4 | Missing `actions_item_types` relationship | **Accepted.** Added join table and index. |
| 5 | Drop `express-session` + `connect-pg-simple` | **Accepted.** Replaced with custom `SessionStore` class using Kysely. |
| 6 | Drop `jsonwebtoken` | **Accepted.** Replaced with `crypto.randomBytes` + `password_reset_tokens` table. |
| 7 | Drop `uuid` | **Accepted.** PostgreSQL `gen_random_uuid()` + `crypto.randomUUID()` cover all cases. |
| 8 | Drop `openai` from core deps | **Accepted.** External signals use plain `fetch`. |
| 9 | Drop `fuzzball` | **Accepted.** Fuzzy matching deferred to v1.1. |
| 10 | Drop `date-fns` on server | **Accepted.** Server uses native Date + PG date functions. Client keeps `date-fns`. |
| 11 | Drop `pino` | **Accepted.** Custom ~15-line structured JSON logger. |
| 12 | Break MRTService <-> ActionPublisher circular dep | **Accepted.** `recordDecision` returns `ActionRequest[]`. Route handler orchestrates. Added Invariant #9. |
| 13 | Formalize JobQueue interface | **Accepted.** Added 2-method `JobQueue` interface + `PgBossJobQueue` adapter. Added Invariant #11. |
| 14 | Define Logger interface | **Accepted.** Added `Logger` interface in shared types + implementation contract. Added Invariant #12. |
| 15 | Merge text-contains and text-regex | **Accepted.** Single `text-regex` signal with `mode: 'literal' | 'regex'`. |
| 16 | Missing history tables | **Accepted.** Added `actions_history` and `policies_history` tables. |
| 17 | Partition execution log tables | **Accepted.** `rule_executions` and `action_executions` use declarative partitioning from day one. |
| 18 | Drop XOR conjunction | **Accepted.** Condition evaluator supports AND and OR only. Added Invariant #10. |
| 19 | No rate limiting | **Acknowledged.** Added to v1.1 scope with implementation note. Not an architectural concern. |
| 20 | `text-similarity` signal is vague | **Accepted.** Removed from v1.0 built-in signals. Deferred pending algorithm decision. |
