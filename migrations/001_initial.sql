-- Nest v1.0 Initial Schema
-- Full DDL: all tables, indexes, and constraints
-- Source of truth: docs/NEST_DESIGN.md section 6
--
-- Table inventory (21 tables):
--   1.  orgs
--   2.  users
--   3.  password_reset_tokens
--   4.  api_keys
--   5.  signing_keys
--   6.  item_types
--   7.  policies
--   8.  rules               (source TEXT -- NOT a JSONB condition tree)
--   9.  actions
--   10. entity_history      (composite PK: entity_type, id, version)
--   11. rules_policies      (join table)
--   12. actions_item_types  (join table)
--   13. text_banks
--   14. text_bank_entries
--   15. mrt_queues
--   16. mrt_jobs
--   17. mrt_decisions
--   18. rule_executions     (PARTITION BY RANGE on executed_at)
--   19. action_executions   (PARTITION BY RANGE on executed_at)
--   20. items               (composite PK: org_id, id, item_type_id, submission_id)
--   21. sessions

-- ============================================================
-- TABLES
-- ============================================================

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

-- Password reset tokens (one-time use, time-limited)
CREATE TABLE password_reset_tokens (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- API keys (plaintext never stored; only SHA-256 hash persisted)
CREATE TABLE api_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  key_hash    TEXT NOT NULL,
  prefix      TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at  TIMESTAMPTZ
);

-- RSA key pairs for webhook payload signing (RSA-PSS)
CREATE TABLE signing_keys (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  public_key  TEXT NOT NULL,
  private_key TEXT NOT NULL,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Item type definitions (schema + field roles per org)
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

-- Content moderation policies (hierarchical via parent_id)
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
-- source (TEXT) stores Starlark source code -- it is the SINGLE SOURCE OF TRUTH.
-- event_types and priority are DERIVED values extracted from the Starlark globals
-- at compile time and stored here for query efficiency (GIN index on event_types).
-- The Starlark source always wins in case of any discrepancy.
-- IMPORTANT: There is NO condition_set JSONB column. No JSONB condition trees anywhere.
CREATE TABLE rules (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id          TEXT NOT NULL REFERENCES orgs(id),
  name            TEXT NOT NULL,
  status          TEXT NOT NULL CHECK (status IN ('LIVE', 'BACKGROUND', 'DISABLED')),
  source          TEXT NOT NULL,                 -- Starlark source code (SOURCE OF TRUTH)
  event_types     TEXT[] NOT NULL DEFAULT '{}',  -- DERIVED: extracted from Starlark globals
  priority        INTEGER NOT NULL DEFAULT 0,    -- DERIVED: extracted from Starlark globals
  tags            TEXT[] NOT NULL DEFAULT '{}',
  version         INTEGER NOT NULL DEFAULT 1,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Actions (referenced by name from Starlark verdict() calls)
-- No rules_actions join table: actions are declared inside verdict() at runtime.
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

-- Generic entity history (replaces rules_history, actions_history, policies_history).
-- Stores full JSONB snapshots for any versioned entity.
-- Composite PK: (entity_type, id, version) -- append-only, never updated.
CREATE TABLE entity_history (
  id              TEXT NOT NULL,          -- entity ID (rule/action/policy id)
  entity_type     TEXT NOT NULL,          -- 'rule', 'action', 'policy'
  org_id          TEXT NOT NULL,
  version         INTEGER NOT NULL,
  snapshot        JSONB NOT NULL,         -- full entity state at this version
  valid_from      TIMESTAMPTZ NOT NULL,
  valid_to        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (entity_type, id, version)
);

-- Join table: rules to policies (many-to-many)
CREATE TABLE rules_policies (
  rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  policy_id   TEXT NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, policy_id)
);

-- Join table: actions to item types (many-to-many)
-- rules_actions is ELIMINATED: actions are declared in verdict() calls within Starlark.
-- rules_item_types is ELIMINATED: rules declare event_types in Starlark.
CREATE TABLE actions_item_types (
  action_id    TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
  item_type_id TEXT NOT NULL REFERENCES item_types(id) ON DELETE CASCADE,
  PRIMARY KEY (action_id, item_type_id)
);

-- Text banks (named collections of text patterns for the text-bank signal adapter)
CREATE TABLE text_banks (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  org_id      TEXT NOT NULL REFERENCES orgs(id),
  name        TEXT NOT NULL,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Text bank entries (individual patterns, optionally regex)
CREATE TABLE text_bank_entries (
  id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  text_bank_id TEXT NOT NULL REFERENCES text_banks(id) ON DELETE CASCADE,
  value        TEXT NOT NULL,
  is_regex     BOOLEAN NOT NULL DEFAULT false,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Manual Review Tool: named queues
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

-- Manual Review Tool: jobs awaiting human review
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

-- Manual Review Tool: decisions recorded by moderators
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

-- Rule execution logs (partitioned by month via executed_at).
-- Partitions created in 002_partitions.sql and maintained by the worker.
-- PRIMARY KEY must include the partition key (executed_at) for PostgreSQL 11+.
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

-- Action execution logs (partitioned by month via executed_at).
-- Partitions created in 002_partitions.sql and maintained by the worker.
-- PRIMARY KEY must include the partition key (executed_at) for PostgreSQL 11+.
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

-- Items ledger: submitted content stored per org.
-- Composite PK: (org_id, id, item_type_id, submission_id) -- supports re-submission tracking.
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

-- Sessions (server-side, PostgreSQL-backed; no client-side secrets)
CREATE TABLE sessions (
  sid         TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users(id),
  data        JSONB NOT NULL DEFAULT '{}',
  expires_at  TIMESTAMPTZ NOT NULL
);

-- ============================================================
-- INDEXES
-- ============================================================

-- Rules: filter by org + status (e.g. list LIVE rules)
CREATE INDEX idx_rules_org_status ON rules(org_id, status);

-- Rules: filter by event type -- GIN index for TEXT[] containment queries
CREATE INDEX idx_rules_event_types ON rules USING GIN(event_types);

-- MRT jobs: list jobs in a queue by status
CREATE INDEX idx_mrt_jobs_queue_status ON mrt_jobs(queue_id, status);

-- MRT jobs: look up jobs for a specific item within an org
CREATE INDEX idx_mrt_jobs_org_item ON mrt_jobs(org_id, item_id);

-- Rule execution logs: analytics by rule within org over time
CREATE INDEX idx_rule_executions_org_rule ON rule_executions(org_id, rule_id, executed_at);

-- Rule execution logs: analytics by org over time
CREATE INDEX idx_rule_executions_org_time ON rule_executions(org_id, executed_at);

-- Action execution logs: analytics by org over time
CREATE INDEX idx_action_executions_org_time ON action_executions(org_id, executed_at);

-- Items: look up items by org + id + type (composite ledger key without submission_id)
CREATE INDEX idx_items_org_id ON items(org_id, id, item_type_id);

-- Items: look up items by creator within an org
CREATE INDEX idx_items_creator ON items(org_id, creator_id);

-- Actions/item types join: look up which actions apply to an item type
CREATE INDEX idx_actions_item_types_item ON actions_item_types(item_type_id);

-- Password reset tokens: look up by token hash (constant-time safe via index)
CREATE INDEX idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);

-- Sessions: expire old sessions efficiently
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Entity history: look up version history for a specific entity
CREATE INDEX idx_entity_history_lookup ON entity_history(entity_type, id, valid_from);

-- Actions: look up action by name within org (action name resolution from verdict() calls)
CREATE INDEX idx_actions_org_name ON actions(org_id, name);
