# Coop Codebase: Comprehensive Analysis & Synthesis

## What Coop Is

Coop is an **open-source content moderation platform** (formerly "Cove", now maintained by ROOST). It allows organizations to define rules, policies, and actions for moderating user-generated content at scale. It provides both automated enforcement (rule engine) and human review workflows (Manual Review Tool).

---

## 1. Core Domain Concepts

| Concept | Purpose |
|---------|---------|
| **Item Types** | Schemas defining content structure (Content, User, Thread) with custom fields |
| **Rules** | Automated conditions that evaluate items and trigger actions when matched |
| **Signals** | Transformation functions that analyze content (text matching, AI models, hashing) |
| **Conditions** | Nested AND/OR/XOR logic trees that compose signals with thresholds |
| **Actions** | What happens when a rule matches (webhook callback, enqueue to review, NCMEC report) |
| **Policies** | Hierarchical categories for violations (Hate, Violence, CSAM, Spam, etc.) |
| **Manual Review Tool (MRT)** | Queue-based system for human moderators to review flagged content |
| **Organizations** | Multi-tenant isolation — each org has its own rules, actions, policies |
| **User Strikes** | Cumulative penalty tracking per end-user with configurable thresholds |

---

## 2. Architecture Overview

### Tech Stack
- **Frontend**: React 18 + TypeScript + Apollo Client + Ant Design + Tailwind CSS + Radix UI
- **Backend**: Express 4 + Apollo Server (GraphQL) + REST API
- **Primary DB**: PostgreSQL (with pgvector) via Sequelize + Kysely
- **Analytics DB**: ClickHouse (default) or Snowflake (legacy)
- **Time-series DB**: Scylla/Cassandra (item investigation, user strikes)
- **Queue**: Kafka (async item processing) + BullMQ/Redis (job queues)
- **Cache**: Redis + in-memory LRU caches
- **Auth**: Passport.js (password + SAML SSO) + API keys for REST

### Monorepo Structure (6 packages)
```
coop/
├── client/          # React frontend (92 prod deps)
├── server/          # Express/Apollo backend (118 prod deps)
├── types/           # Shared TypeScript types (@roostorg/types)
├── migrator/        # Database migration tool (@roostorg/db-migrator)
├── content-proxy/   # Secure iframe content proxy
├── nodejs-instrumentation/  # OpenTelemetry setup
├── hma/             # Hash Matching Algorithm (Python, Docker)
└── .devops/         # Helm charts, CDK, Pulumi, Terraform
```

### External Dependencies: ~350+ production packages

---

## 3. Data Flow: End-to-End Item Processing

```
Client App (REST API)
  │
  ├─ POST /items/async  ──→  Kafka (ITEM_SUBMISSION_EVENTS)
  │                              │
  │                              ▼
  │                        ItemProcessingWorker
  │                              │
  │                              ▼
  │                        RuleEngine.runEnabledRules()
  │                              │
  │                         ┌────┴────┐
  │                         │         │
  │                    LIVE rules   BACKGROUND rules
  │                         │         │ (no actions)
  │                         ▼
  │                    RuleEvaluator.runRule()
  │                         │
  │                         ▼
  │                    ConditionSet evaluation
  │                    (recursive AND/OR/XOR)
  │                         │
  │                    ┌────┴────┐
  │                    │         │
  │               LeafCondition  ConditionSet (nested)
  │                    │
  │                    ▼
  │              Signal execution
  │              (text match, AI, hash, etc.)
  │                    │
  │                    ▼
  │              Threshold comparison
  │                    │
  │               ┌────┴────┐
  │               │         │
  │            PASSED     FAILED
  │               │
  │               ▼
  │         ActionPublisher.publishActions()
  │               │
  │          ┌────┼────┬────────┐
  │          │    │    │        │
  │    CUSTOM  MRT   NCMEC  AUTHOR_MRT
  │   (webhook) (queue) (report) (queue)
  │
  ├─ POST /content  ──→  Synchronous rule evaluation (same pipeline)
  │
  └─ POST /report   ──→  Report ingestion + optional MRT enqueueing
```

---

## 4. Database Schema (9 PostgreSQL schemas, 40+ tables)

### Core Tables (public schema)
- **orgs** - Organizations (multi-tenant root)
- **users** - Platform users with roles (ADMIN, MODERATOR, ANALYST, etc.)
- **rules** - Automation rules with JSONB conditionSet
- **actions** - Enforcement actions (webhook, MRT enqueue, NCMEC)
- **policies** - Hierarchical violation categories with strikes
- **item_types** - Content schema definitions with custom fields
- **api_keys** - SHA256-hashed API keys per org
- **signing_keys** - RSA-PSS webhook signing keypairs

### Association Tables (many-to-many, all with temporal history)
- rules_and_actions, rules_and_policies, rules_and_item_types
- actions_and_item_types

### Matching Banks
- **text_banks** - Text/regex pattern banks
- **location_banks** + **location_bank_locations** - GeoJSON locations
- **hash_banks** - Perceptual image hashes (HMA integration)

### Manual Review Tool (manual_review_tool schema)
- **manual_review_queues** - Review queues per org
- **manual_review_decisions** - Moderator decisions with job payloads
- **routing_rules** - Condition-based queue routing
- **appeals_routing_rules** - Appeal-specific routing

### Other Schemas
- **ncmec_reporting** - NCMEC org settings, reports, errors
- **reporting_rules** - Report-triggered rules
- **models_service** - ML model tracking
- **signal_auth_service** - Third-party API credentials (OpenAI, Google)
- **user_management_service** - Password reset tokens, UI settings
- **user_statistics_service** - User risk scores
- **jobs** - Scheduled job tracking

### Temporal Versioning
All major tables have `_history` companion tables with `sys_period` (tstzrange) for full audit trail. Materialized views track latest versions.

---

## 5. Rule Engine Deep Dive

### Condition Evaluation
- **Cost-based ordering**: Cheapest signals evaluated first
- **Short-circuit logic**: AND stops on first FALSE, OR stops on first TRUE
- **Three-valued logic**: PASSED→true, FAILED→false, ERRORED→null
- **Recursive nesting**: ConditionSets can contain LeafConditions or nested ConditionSets

### Signal System (20+ built-in signals)
- **Text**: contains, regex, similarity, fuzzy matching, text bank lookup
- **Image**: similarity score, HMA hash matching
- **AI/Third-party**: OpenAI moderation (7 categories), Google Content Safety, Whisper transcription
- **User**: score, strikes, geo-containment
- **Aggregation**: Custom statistical aggregations

### Signal Architecture
- Registration pattern (NOT plugin-based — hardcoded imports)
- Per-execution transient caching (signal results cached within single rule-set run)
- Cost model for optimization (0 for text matching, higher for API calls)
- Custom signals not yet implemented (`case 'CUSTOM': throw new Error('not implemented')`)

### Action Types
- **CUSTOM_ACTION** - HTTP POST to callback URL with signed body
- **ENQUEUE_TO_MRT** - Add to manual review queue
- **ENQUEUE_TO_NCMEC** - Escalate to NCMEC reporting
- **ENQUEUE_AUTHOR_TO_MRT** - Queue the content author for review

---

## 6. Frontend Architecture

### 20+ Pages organized by domain:
- **Auth**: Login, SSO, Signup, Password Reset
- **Rules**: Dashboard, Form (create/edit), Info (details + insights)
- **MRT**: Queue dashboard, Job review, Analytics, Investigation, Bulk actioning
- **Policies**: Hierarchical tree management
- **Settings**: Item types, Actions, Users, API keys, SSO, NCMEC, Integrations
- **Banks**: Text, Location, Hash bank management
- **Overview**: Analytics dashboard with charts

### Key Patterns
- Apollo Client for all GraphQL communication
- Generated hooks from schema (`useGQL*Query`, `useGQL*Mutation`)
- Permission-based UI rendering (24 granular permissions)
- React Router v6 with lazy loading
- useReducer for complex forms (RuleForm)
- react-table for data tables with filtering/sorting

---

## 7. Authentication & Security

- **Session-based auth**: Express-session + PostgreSQL store (30-day TTL)
- **Password hashing**: bcryptjs
- **SAML SSO**: Per-org multi-SAML strategy via passport-saml
- **API keys**: SHA256-hashed, rotation support, per-org
- **Webhook signing**: RSA-PSS 2048-bit with SHA-256
- **RBAC**: 7 roles, 14+ permissions
- **GraphQL auth wrapper**: All resolvers wrapped; `@publicResolver` for exceptions

---

## 8. Feature Classification

### CORE (Required for basic content moderation)
- Item types (custom schemas)
- Rules engine (condition evaluation + signals)
- Actions (enforcement execution)
- Policies (violation categories)
- Basic MRT (review queues, job review, decisions)
- User management & authentication
- Organization settings
- API key management

### IMPORTANT (Highly useful, not essential)
- Analytics & reporting dashboards
- Investigation tool
- Bulk actioning
- User strikes system
- Rule anomaly detection
- Webhook signing
- Queue routing rules

### OPTIONAL (Can be deferred)
- NCMEC integration (specialized compliance)
- SSO/SAML (enterprise auth)
- Hash banks / HMA (perceptual hashing)
- Backtesting & retroactions
- ML model tracking
- Snowflake integration (ClickHouse is default)
- Content proxy
- OpenTelemetry instrumentation

---

## 9. External Service Dependencies

### What MUST exist
- **PostgreSQL** - Primary database, no alternative

### What could be simplified away
- **Kafka** → Could use PostgreSQL-based job queue (pg-boss) or BullMQ-only
- **Scylla/Cassandra** → Could store investigation data in PostgreSQL
- **ClickHouse** → Could use PostgreSQL for analytics (smaller scale)
- **Redis** → Could use PostgreSQL for sessions; in-memory for caching
- **Snowflake** → Already optional
- **HMA service** → Optional Python service

### Minimum viable infrastructure
PostgreSQL + Redis (for BullMQ) — everything else can be abstracted

---

## 10. Coupling & Complexity Hotspots

### Tightly Coupled Areas
1. **ManualReviewToolService.onRecordDecision** — 533 lines handling 6 decision types
2. **IoC Container** (iocContainer/index.ts) — 1,878 lines, single file
3. **NcmecService ↔ ManualReviewToolService** — bidirectional dependency
4. **Sequelize + Kysely dual ORM** — migration in progress

### Accidental Complexity
- 350+ npm dependencies (many redundant: moment + date-fns, lodash for simple ops)
- 938 generated GraphQL types
- Multiple chart libraries (Recharts + Google Charts)
- Both Ant Design and Radix UI component systems
- Multiple HTTP clients (undici, axios, fetch)
- Storybook setup (8 packages) for component dev

### Essential Complexity
- Rule evaluation with nested conditions and diverse signal types
- Multi-tenant isolation at every layer
- Temporal versioning for audit trails
- Queue-based async processing for scale
- Multiple content types (Content, User, Thread) with custom schemas

---

## 11. Key Architectural Patterns

1. **IoC/DI via Bottle.js** — Type-safe dependency injection with `inject()` utility
2. **Eventual consistency** — 10-120 second caches on rule/policy/action lookups
3. **Temporal tables** — PostgreSQL history tables with tstzrange for audit
4. **Materialized views** — Auto-refreshed via triggers for version lookups
5. **Cost-based evaluation** — Signals sorted by execution cost for optimization
6. **Three-valued logic** — Error handling without crashing rule evaluation
7. **Schema registry** — Kafka messages validated via Confluent Schema Registry
8. **Warehouse abstraction** — Interface-based adapter pattern for analytics backends
