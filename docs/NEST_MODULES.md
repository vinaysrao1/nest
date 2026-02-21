# Nest Modules -- Independent Module Definitions

This document defines the clean, independent modules required to build the Nest system. Each module has strict data and API contracts with its upstream and downstream dependencies. This decomposition covers the full scope described in NEST_DESIGN.md (Go backend) and NEST_UI.md (Python frontend).

These module definitions serve as the prerequisite for defining development stages.

---

## Module Index

### Go Backend
1. [domain](#1-domain) -- Pure types, zero imports
2. [config](#2-config) -- Environment configuration
3. [store](#3-store) -- PostgreSQL data access
4. [auth](#4-auth) -- Authentication, authorization, cryptography
5. [signal](#5-signal) -- Signal adapter framework
6. [engine](#6-engine) -- Starlark rule evaluation engine
7. [service](#7-service) -- Business logic orchestration
8. [worker](#8-worker) -- Background job processing
9. [handler](#9-handler) -- HTTP handlers and routing
10. [cmd/server](#10-cmdserver) -- Composition root and entry point
11. [cmd/migrate](#11-cmdmigrate) -- Database migration runner
12. [cmd/seed](#12-cmdseed) -- Development seed data

### Database
13. [migrations](#13-migrations) -- Schema DDL and partitions

### Python Frontend
14. [ui/api](#14-uiapi) -- Typed HTTP client and data types
15. [ui/auth](#15-uiauth) -- Session state and auth middleware
16. [ui/components](#16-uicomponents) -- Shared UI components
17. [ui/pages](#17-uipages) -- Page modules
18. [ui/main](#18-uimain) -- UI entry point

---

## 1. domain

**Purpose**: Define all shared domain types as pure Go structs and constants. This is the dependency leaf of the entire system -- it imports nothing from any other internal package.

### Responsibilities
- Define all entity types: Event, Rule, Verdict, Action, Policy, ItemType, Item, Signal types, MRT types, User, Org, Session, ApiKey, SigningKey, PasswordResetToken, TextBank, TextBankEntry, RuleExecution, ActionExecution, EntityHistoryEntry
- Define all enums: VerdictType, RuleStatus, ActionType, UserRole, MRTJobStatus, ItemTypeKind
- Define error types: NotFound, Forbidden, Conflict, Validation, ConfigError, CompileError
- Define pagination types: PaginatedResult, PageParams
- Define engine-specific types: EvalRequest, EvalResult, ActionRequest, ActionResult, ActionTarget
- Define API response types: EvalResultResponse

### Data Contracts

```go
// Event -- input to the rule engine
type Event struct {
    ID        string         `json:"event_id"`
    EventType string         `json:"event_type"`
    ItemType  string         `json:"item_type"`
    OrgID     string         `json:"org_id"`
    Payload   map[string]any `json:"payload"`
    Timestamp time.Time      `json:"timestamp"`
}

// Rule -- database representation, source is single source of truth
type Rule struct {
    ID         string     `json:"id"`
    OrgID      string     `json:"org_id"`
    Name       string     `json:"name"`
    Status     RuleStatus `json:"status"`
    Source     string     `json:"source"`
    EventTypes []string   `json:"event_types"`   // DERIVED from Starlark
    Priority   int        `json:"priority"`      // DERIVED from Starlark
    Tags       []string   `json:"tags"`
    Version    int        `json:"version"`
    CreatedAt  time.Time  `json:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at"`
}

type RuleStatus string
const (
    RuleStatusLive       RuleStatus = "LIVE"
    RuleStatusBackground RuleStatus = "BACKGROUND"
    RuleStatusDisabled   RuleStatus = "DISABLED"
)

type VerdictType string
const (
    VerdictApprove VerdictType = "approve"
    VerdictBlock   VerdictType = "block"
    VerdictReview  VerdictType = "review"
)

type Verdict struct {
    Type    VerdictType `json:"type"`
    Reason  string      `json:"reason,omitempty"`
    RuleID  string      `json:"rule_id"`
    Actions []string    `json:"actions,omitempty"`
}

type Action struct {
    ID         string     `json:"id"`
    OrgID      string     `json:"org_id"`
    Name       string     `json:"name"`
    ActionType ActionType `json:"action_type"`
    Config     map[string]any `json:"config"`
    Version    int        `json:"version"`
    CreatedAt  time.Time  `json:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at"`
}

type ActionType string
const (
    ActionTypeWebhook      ActionType = "WEBHOOK"
    ActionTypeEnqueueToMRT ActionType = "ENQUEUE_TO_MRT"
)

type ActionRequest struct {
    Action Action
    ItemID string
    Payload map[string]any
    CorrelationID string
}

type ActionResult struct {
    ActionID string `json:"action_id"`
    Success  bool   `json:"success"`
    Error    string `json:"error,omitempty"`
}

type Policy struct {
    ID            string  `json:"id"`
    OrgID         string  `json:"org_id"`
    Name          string  `json:"name"`
    Description   string  `json:"description,omitempty"`
    ParentID      *string `json:"parent_id,omitempty"`
    StrikePenalty int     `json:"strike_penalty"`
    Version       int     `json:"version"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type ItemType struct {
    ID         string         `json:"id"`
    OrgID      string         `json:"org_id"`
    Name       string         `json:"name"`
    Kind       ItemTypeKind   `json:"kind"`
    Schema     map[string]any `json:"schema"`
    FieldRoles map[string]any `json:"field_roles"`
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
}

type ItemTypeKind string
const (
    ItemTypeKindContent ItemTypeKind = "CONTENT"
    ItemTypeKindUser    ItemTypeKind = "USER"
    ItemTypeKindThread  ItemTypeKind = "THREAD"
)

// Item -- a submitted content item stored in the items ledger
type Item struct {
    ID            string         `json:"id"`
    OrgID         string         `json:"org_id"`
    ItemTypeID    string         `json:"item_type_id"`
    Data          map[string]any `json:"data"`
    SubmissionID  string         `json:"submission_id"`
    CreatorID     string         `json:"creator_id,omitempty"`
    CreatorTypeID string         `json:"creator_type_id,omitempty"`
    CreatedAt     time.Time      `json:"created_at"`
}

type SignalInput struct {
    Type  SignalInputType
    Value string
}

type SignalInputType string

type SignalOutput struct {
    Score    float64        `json:"score"`
    Label    string         `json:"label"`
    Metadata map[string]any `json:"metadata,omitempty"`
}

type User struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    Password  string    `json:"-"`
    Role      UserRole  `json:"role"`
    IsActive  bool      `json:"is_active"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type UserRole string
const (
    UserRoleAdmin     UserRole = "ADMIN"
    UserRoleModerator UserRole = "MODERATOR"
    UserRoleAnalyst   UserRole = "ANALYST"
)

type Org struct {
    ID        string         `json:"id"`
    Name      string         `json:"name"`
    Settings  map[string]any `json:"settings"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}

// Session -- server-side session for UI authentication
type Session struct {
    SID       string         `json:"sid"`
    UserID    string         `json:"user_id"`
    Data      map[string]any `json:"data"`
    ExpiresAt time.Time      `json:"expires_at"`
}

// ApiKey -- API key metadata (plaintext key is never stored)
type ApiKey struct {
    ID        string     `json:"id"`
    OrgID     string     `json:"org_id"`
    Name      string     `json:"name"`
    KeyHash   string     `json:"-"`
    Prefix    string     `json:"prefix"`
    CreatedAt time.Time  `json:"created_at"`
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// SigningKey -- RSA key pair for webhook payload signing
type SigningKey struct {
    ID         string    `json:"id"`
    OrgID      string    `json:"org_id"`
    PublicKey  string    `json:"public_key"`
    PrivateKey string    `json:"-"`
    IsActive   bool      `json:"is_active"`
    CreatedAt  time.Time `json:"created_at"`
}

// PasswordResetToken -- one-time password reset token
type PasswordResetToken struct {
    ID        string     `json:"id"`
    UserID    string     `json:"user_id"`
    TokenHash string     `json:"-"`
    ExpiresAt time.Time  `json:"expires_at"`
    UsedAt    *time.Time `json:"used_at,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}

// TextBank -- a named collection of text patterns for signal matching
type TextBank struct {
    ID          string    `json:"id"`
    OrgID       string    `json:"org_id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// TextBankEntry -- a single entry in a text bank
type TextBankEntry struct {
    ID         string    `json:"id"`
    TextBankID string    `json:"text_bank_id"`
    Value      string    `json:"value"`
    IsRegex    bool      `json:"is_regex"`
    CreatedAt  time.Time `json:"created_at"`
}

// RuleExecution -- a log entry for a single rule evaluation
type RuleExecution struct {
    ID             string         `json:"id"`
    OrgID          string         `json:"org_id"`
    RuleID         string         `json:"rule_id"`
    RuleVersion    int            `json:"rule_version"`
    ItemID         string         `json:"item_id"`
    ItemTypeID     string         `json:"item_type_id"`
    Verdict        string         `json:"verdict,omitempty"`
    Reason         string         `json:"reason,omitempty"`
    TriggeredRules map[string]any `json:"triggered_rules,omitempty"`
    LatencyUs      int64          `json:"latency_us,omitempty"`
    CorrelationID  string         `json:"correlation_id"`
    ExecutedAt     time.Time      `json:"executed_at"`
}

// ActionExecution -- a log entry for a single action execution
type ActionExecution struct {
    ID            string    `json:"id"`
    OrgID         string    `json:"org_id"`
    ActionID      string    `json:"action_id"`
    ItemID        string    `json:"item_id"`
    ItemTypeID    string    `json:"item_type_id"`
    Success       bool      `json:"success"`
    CorrelationID string    `json:"correlation_id"`
    ExecutedAt    time.Time `json:"executed_at"`
}

// EntityHistoryEntry -- a versioned snapshot of an entity
type EntityHistoryEntry struct {
    ID         string         `json:"id"`
    EntityType string         `json:"entity_type"`
    OrgID      string         `json:"org_id"`
    Version    int            `json:"version"`
    Snapshot   map[string]any `json:"snapshot"`
    ValidFrom  time.Time      `json:"valid_from"`
    ValidTo    time.Time      `json:"valid_to"`
}

type MRTQueue struct {
    ID          string `json:"id"`
    OrgID       string `json:"org_id"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    IsDefault   bool   `json:"is_default"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type MRTJob struct {
    ID            string         `json:"id"`
    OrgID         string         `json:"org_id"`
    QueueID       string         `json:"queue_id"`
    ItemID        string         `json:"item_id"`
    ItemTypeID    string         `json:"item_type_id"`
    Payload       map[string]any `json:"payload"`
    Status        MRTJobStatus   `json:"status"`
    AssignedTo    *string        `json:"assigned_to,omitempty"`
    PolicyIDs     []string       `json:"policy_ids"`
    EnqueueSource string         `json:"enqueue_source"`
    SourceInfo    map[string]any `json:"source_info"`
    CreatedAt     time.Time      `json:"created_at"`
    UpdatedAt     time.Time      `json:"updated_at"`
}

type MRTJobStatus string
const (
    MRTJobStatusPending  MRTJobStatus = "PENDING"
    MRTJobStatusAssigned MRTJobStatus = "ASSIGNED"
    MRTJobStatusDecided  MRTJobStatus = "DECIDED"
)

type MRTDecision struct {
    ID        string   `json:"id"`
    OrgID     string   `json:"org_id"`
    JobID     string   `json:"job_id"`
    UserID    string   `json:"user_id"`
    Verdict   string   `json:"verdict"`
    ActionIDs []string `json:"action_ids"`
    PolicyIDs []string `json:"policy_ids"`
    Reason    string   `json:"reason,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}

// EvalResultResponse -- the per-item response shape returned by POST /api/v1/items.
// Matches NEST_DESIGN.md section 8 response: { results: [{ item_id, verdict, triggered_rules, actions }] }
type EvalResultResponse struct {
    ItemID         string          `json:"item_id"`
    Verdict        VerdictType     `json:"verdict"`
    TriggeredRules []TriggeredRule `json:"triggered_rules"`
    Actions        []ActionResult  `json:"actions"`
}

// TriggeredRule -- per-rule evaluation result included in EvalResultResponse.
type TriggeredRule struct {
    RuleID    string      `json:"rule_id"`
    Version   int         `json:"version"`
    Verdict   VerdictType `json:"verdict"`
    Reason    string      `json:"reason,omitempty"`
    LatencyUs int64       `json:"latency_us"`
}

type PaginatedResult[T any] struct {
    Items      []T `json:"items"`
    Total      int `json:"total"`
    Page       int `json:"page"`
    PageSize   int `json:"page_size"`
    TotalPages int `json:"total_pages"`
}

type PageParams struct {
    Page     int
    PageSize int
}

// Structured error types
type NotFoundError struct{ Message string }
type ForbiddenError struct{ Message string }
type ConflictError struct{ Message string }
type ValidationError struct{ Message string; Details map[string]string }
type ConfigError struct{ Message string }

// CompileError -- returned when Starlark source fails compilation.
// Used by the engine compiler to report syntax errors, missing globals,
// or invalid metadata (e.g., wildcard mixed with specific event_types).
type CompileError struct {
    Message  string `json:"message"`
    Line     int    `json:"line,omitempty"`
    Column   int    `json:"column,omitempty"`
    Filename string `json:"filename,omitempty"`
}
```

**Note on EvalRequest**: The `EvalRequest` type (used by the engine to pair an `Event` with a `context.Context`) is defined in the `engine` package rather than `domain` because it carries `context.Context`, which is an engine-internal concern. Domain types are pure data structures without runtime semantics.

### API Contracts
- **Exposes**: All types above as public exports. No constructors -- plain struct initialization.
- **Consumes**: Nothing. Zero imports from other internal packages.

### Upstream Dependencies
- None. This is the dependency leaf.

### Downstream Dependents
- Every other module in the system imports `domain`.

### Key Implementation Notes
- All types must have JSON tags for API serialization.
- Password field on User must use `json:"-"` to prevent serialization.
- KeyHash on ApiKey and PrivateKey on SigningKey use `json:"-"` to prevent serialization.
- TokenHash on PasswordResetToken uses `json:"-"` to prevent serialization.
- Error types implement the `error` interface via `Error() string` method.
- No methods with side effects. Pure data structures only.
- Files: `event.go`, `rule.go`, `action.go`, `verdict.go`, `policy.go`, `signal.go`, `mrt.go`, `user.go`, `org.go`, `item.go`, `auth_types.go` (Session, ApiKey, SigningKey, PasswordResetToken), `text_bank.go`, `execution.go` (RuleExecution, ActionExecution), `history.go` (EntityHistoryEntry), `errors.go`, `pagination.go`, `responses.go` (EvalResultResponse, TriggeredRule)
- Estimated size: ~700 lines

---

## 2. config

**Purpose**: Parse environment variables into a typed configuration struct. Single source of runtime configuration.

### Responsibilities
- Parse all environment variables into a `Config` struct
- Validate required configuration (database URL, port, secrets)
- Provide typed access to configuration values

### Data Contracts

```go
type Config struct {
    Port              int
    DatabaseURL       string
    SessionSecret     string
    WorkerCount       int       // default: runtime.NumCPU()
    RiverWorkerCount  int       // default: 100
    RuleTimeout       time.Duration // default: 1s per rule
    EventTimeout      time.Duration // default: 5s per event
    LogLevel          string    // default: "info"
    DevMode           bool      // default: false
    CounterBackend    string    // "memory" or "postgres", default: "memory"
}
```

### API Contracts

```go
// Load parses environment variables into a Config struct.
// Returns ConfigError if required variables are missing.
//
// Required env vars: DATABASE_URL
// Optional env vars: PORT (8080), SESSION_SECRET (generated), WORKER_COUNT,
//   RIVER_WORKER_COUNT, RULE_TIMEOUT, EVENT_TIMEOUT, LOG_LEVEL, DEV_MODE,
//   COUNTER_BACKEND
func Load() (*Config, error)
```

### Upstream Dependencies
- `domain` (for ConfigError type)

### Downstream Dependents
- `cmd/server` (composition root reads config)
- `cmd/migrate` (reads DATABASE_URL)
- `cmd/seed` (reads DATABASE_URL)

### Key Implementation Notes
- No config file support. Environment variables only.
- No external config library. Use `os.Getenv` with typed parsing.
- Defaults are set for all optional values.
- File: `config.go`
- Estimated size: ~80 lines

---

## 3. store

**Purpose**: All PostgreSQL data access. Every database query lives here. No other module executes SQL directly.

### Responsibilities
- Manage pgxpool connection lifecycle
- Provide transaction helper
- Execute all CRUD queries for all entities
- Write execution logs (partitioned tables)
- Write and query entity history
- Manage sessions, API keys, password reset tokens
- Manage MRT queue CRUD
- Manage actions_item_types join table
- Optional: persistent counter state

### Data Contracts

**Input**: All query methods accept `context.Context` as the first parameter, `orgID string` for multi-tenant isolation, and typed parameters matching `domain` types.

**Output**: All query methods return `domain` types or slices of `domain` types, plus `error`.

### API Contracts

```go
// Queries wraps a pgxpool.Pool and provides all database operations.
type Queries struct {
    pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queries

// Transaction helper
func (q *Queries) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error

// --- Rules ---
func (q *Queries) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error)
func (q *Queries) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error)
func (q *Queries) CreateRule(ctx context.Context, rule *domain.Rule) error
func (q *Queries) UpdateRule(ctx context.Context, rule *domain.Rule) error
func (q *Queries) DeleteRule(ctx context.Context, orgID, ruleID string) error
func (q *Queries) ListEnabledRules(ctx context.Context, orgID string) ([]domain.Rule, error)

// --- Config entities (Actions, Policies, ItemTypes) ---
func (q *Queries) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error)
func (q *Queries) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error)
func (q *Queries) GetActionByName(ctx context.Context, orgID, name string) (*domain.Action, error)
func (q *Queries) CreateAction(ctx context.Context, action *domain.Action) error
func (q *Queries) UpdateAction(ctx context.Context, action *domain.Action) error
func (q *Queries) DeleteAction(ctx context.Context, orgID, actionID string) error

func (q *Queries) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error)
func (q *Queries) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error)
func (q *Queries) CreatePolicy(ctx context.Context, policy *domain.Policy) error
func (q *Queries) UpdatePolicy(ctx context.Context, policy *domain.Policy) error
func (q *Queries) DeletePolicy(ctx context.Context, orgID, policyID string) error

func (q *Queries) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error)
func (q *Queries) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error)
func (q *Queries) CreateItemType(ctx context.Context, itemType *domain.ItemType) error
func (q *Queries) UpdateItemType(ctx context.Context, itemType *domain.ItemType) error
func (q *Queries) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error

// --- Items ---
func (q *Queries) InsertItem(ctx context.Context, orgID string, item domain.Item) error

// --- MRT ---
func (q *Queries) ListMRTQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error)
func (q *Queries) GetMRTQueue(ctx context.Context, orgID, queueID string) (*domain.MRTQueue, error)
func (q *Queries) CreateMRTQueue(ctx context.Context, queue *domain.MRTQueue) error
func (q *Queries) ListMRTJobs(ctx context.Context, orgID, queueID string, status *string, page domain.PageParams) (*domain.PaginatedResult[domain.MRTJob], error)
func (q *Queries) GetMRTJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error)
func (q *Queries) InsertMRTJob(ctx context.Context, job *domain.MRTJob) error
func (q *Queries) AssignNextMRTJob(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error)
func (q *Queries) InsertMRTDecision(ctx context.Context, decision *domain.MRTDecision) error
func (q *Queries) UpdateMRTJobStatus(ctx context.Context, orgID, jobID string, status domain.MRTJobStatus, assignedTo *string) error

// --- Text Banks ---
func (q *Queries) ListTextBanks(ctx context.Context, orgID string) ([]domain.TextBank, error)
func (q *Queries) GetTextBank(ctx context.Context, orgID, bankID string) (*domain.TextBank, error)
func (q *Queries) CreateTextBank(ctx context.Context, bank *domain.TextBank) error
func (q *Queries) AddTextBankEntry(ctx context.Context, orgID string, entry *domain.TextBankEntry) error
func (q *Queries) DeleteTextBankEntry(ctx context.Context, orgID, bankID, entryID string) error
func (q *Queries) GetTextBankEntries(ctx context.Context, orgID, bankID string) ([]domain.TextBankEntry, error)

// --- Auth ---
func (q *Queries) GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
func (q *Queries) GetUserByID(ctx context.Context, orgID, userID string) (*domain.User, error)
func (q *Queries) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error)
func (q *Queries) CreateUser(ctx context.Context, user *domain.User) error
func (q *Queries) UpdateUser(ctx context.Context, user *domain.User) error
func (q *Queries) DeleteUser(ctx context.Context, orgID, userID string) error

func (q *Queries) CreateSession(ctx context.Context, session domain.Session) error
func (q *Queries) GetSession(ctx context.Context, sid string) (*domain.Session, error)
func (q *Queries) DeleteSession(ctx context.Context, sid string) error
func (q *Queries) CleanExpiredSessions(ctx context.Context) (int64, error)

func (q *Queries) CreateAPIKey(ctx context.Context, key domain.ApiKey) error
func (q *Queries) GetAPIKeyByHash(ctx context.Context, keyHash string) (*domain.ApiKey, error)
func (q *Queries) ListAPIKeys(ctx context.Context, orgID string) ([]domain.ApiKey, error)
func (q *Queries) RevokeAPIKey(ctx context.Context, orgID, keyID string) error

func (q *Queries) CreatePasswordResetToken(ctx context.Context, token domain.PasswordResetToken) error
func (q *Queries) GetPasswordResetToken(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error)
func (q *Queries) MarkPasswordResetTokenUsed(ctx context.Context, tokenID string) error

// --- Signing Keys ---
func (q *Queries) ListSigningKeys(ctx context.Context, orgID string) ([]domain.SigningKey, error)
func (q *Queries) GetActiveSigningKey(ctx context.Context, orgID string) (*domain.SigningKey, error)
func (q *Queries) CreateSigningKey(ctx context.Context, key domain.SigningKey) error
func (q *Queries) DeactivateSigningKeys(ctx context.Context, orgID string) error

// --- Orgs ---
func (q *Queries) GetOrg(ctx context.Context, orgID string) (*domain.Org, error)

// --- Execution Logs ---
func (q *Queries) LogRuleExecutions(ctx context.Context, executions []domain.RuleExecution) error
func (q *Queries) LogActionExecutions(ctx context.Context, executions []domain.ActionExecution) error

// --- Entity History ---
func (q *Queries) InsertEntityHistory(ctx context.Context, entityType, id, orgID string, version int, snapshot any) error
func (q *Queries) GetEntityHistory(ctx context.Context, entityType, id string) ([]domain.EntityHistoryEntry, error)

// --- Counters (optional, for persistent counter backend) ---
func (q *Queries) IncrementCounter(ctx context.Context, orgID, entityID, eventType string, window int, count int64) error
func (q *Queries) GetCounterSum(ctx context.Context, orgID, entityID, eventType string, window int) (int64, error)

// --- Rules-Policies join ---
// Rule-policy associations are managed via rule CRUD endpoints.
// When a rule is created or updated with policy_ids, the service layer calls
// SetRulePolicies within the same transaction. There is no standalone
// endpoint for managing this join table.
func (q *Queries) SetRulePolicies(ctx context.Context, ruleID string, policyIDs []string) error
func (q *Queries) GetRulePolicies(ctx context.Context, ruleID string) ([]string, error)

// --- Actions-ItemTypes join ---
// The actions_item_types table (defined in NEST_DESIGN.md section 6) restricts
// which actions apply to which item types. In v1.0, this join table exists in
// the schema but has no dedicated CRUD surface. Action-to-item-type associations
// are managed as part of action CRUD: when an action is created or updated,
// an optional item_type_ids field specifies the associations. If omitted, the
// action applies to all item types.
func (q *Queries) SetActionItemTypes(ctx context.Context, actionID string, itemTypeIDs []string) error
func (q *Queries) GetActionItemTypes(ctx context.Context, actionID string) ([]string, error)
```

### Upstream Dependencies
- `domain`

### Downstream Dependents
- `auth`, `signal`, `engine`, `service`, `worker`, `handler` (indirectly via service)

### Key Implementation Notes
- Every query includes `org_id` in its WHERE clause for multi-tenant isolation.
- Uses pgxpool for connection pooling.
- Transaction helper wraps `pgxpool.Pool.Begin` with automatic rollback on error.
- Execution logs use partitioned tables (PARTITION BY RANGE on executed_at).
- Entity history stores JSONB snapshots via a single generic table.
- `GetUserByEmail` does NOT take orgID because during login the org is unknown. It returns the User (which includes OrgID), and the auth layer uses that to establish the org context. If multiple orgs share the same email domain, the users table UNIQUE constraint on (org_id, email) ensures uniqueness per org; the login flow uses the single matching user or returns an error if ambiguous.
- Text bank entry operations (`AddTextBankEntry`, `DeleteTextBankEntry`, `GetTextBankEntries`) include orgID to enforce multi-tenant isolation. The store joins through text_banks to verify org ownership.
- MRT queue creation (`CreateMRTQueue`) is used by `cmd/seed` for development setup and by the service layer if queue CRUD is exposed in future versions. In v1.0, queues are created during org provisioning.
- Files: `db.go`, `rules.go`, `config.go`, `items.go`, `mrt.go`, `text_banks.go`, `auth.go`, `signing_keys.go`, `executions.go`, `orgs.go`, `users.go`, `counters.go`, `history.go`
- Estimated size: ~1,100 lines

---

## 4. auth

**Purpose**: All authentication, authorization, and cryptographic operations. Consolidates session management, API key verification, RBAC, password hashing, webhook signing, and token generation.

### Responsibilities
- HTTP middleware: session auth, API key auth, CSRF protection
- Password hashing (bcrypt)
- PostgreSQL-backed session store
- RBAC role checking
- RSA-PSS webhook payload signing
- SHA-256 hashing for API keys and tokens
- Auth context key management (inject user/org into request context)

### Data Contracts

**Context values set by middleware**:
```go
// Set on authenticated requests, readable by handlers
type AuthContext struct {
    UserID string
    OrgID  string
    Role   domain.UserRole
}
```

### API Contracts

```go
// --- Middleware ---

// SessionAuth reads the session cookie, validates against the sessions table,
// and injects AuthContext into the request context. Returns 401 on failure.
func SessionAuth(store *store.Queries) func(http.Handler) http.Handler

// APIKeyAuth reads the X-API-Key header, hashes it, looks up the key,
// and injects AuthContext into the request context. Returns 401 on failure.
func APIKeyAuth(store *store.Queries) func(http.Handler) http.Handler

// RequireRole returns middleware that checks the user's role from context.
// Returns 403 if the user's role is below the minimum required.
func RequireRole(minRole ...domain.UserRole) func(http.Handler) http.Handler

// CSRFProtect validates CSRF tokens on state-changing requests for session auth.
func CSRFProtect() func(http.Handler) http.Handler

// --- Context helpers ---
func GetAuthContext(ctx context.Context) *AuthContext
func UserIDFromContext(ctx context.Context) string
func OrgIDFromContext(ctx context.Context) string
func RoleFromContext(ctx context.Context) domain.UserRole

// --- Passwords ---
func HashPassword(password string) (string, error)
func CheckPassword(hash, password string) bool

// --- Sessions ---
func GenerateSessionID() string
func GenerateToken() (plaintext string, hash string, err error)

// --- Signing ---

// Signer signs webhook payloads using RSA-PSS.
type Signer struct {
    store *store.Queries
}
func NewSigner(store *store.Queries) *Signer

// Sign retrieves the active signing key for the org and signs the payload.
func (s *Signer) Sign(ctx context.Context, orgID string, payload []byte) (string, error)

// --- Hashing ---
func HashAPIKey(key string) string     // SHA-256
func GenerateAPIKey() (key string, prefix string, hash string)
```

### Upstream Dependencies
- `domain` (for UserRole, error types)
- `store` (for session/API key/signing key queries)

### Downstream Dependents
- `handler` (uses middleware and context helpers)
- `engine` (uses Signer interface via ActionPublisher)

### Key Implementation Notes
- `SessionAuth` and `APIKeyAuth` are mutually exclusive per route group. Session for internal UI routes, API key for external client routes.
- API keys are never stored in plaintext. Only SHA-256 hashes.
- RSA-PSS signing uses stdlib `crypto/rsa`. Key generation uses 2048-bit RSA.
- CSRF protection applies only to session-authenticated state-changing requests (POST/PUT/DELETE).
- Files: `middleware.go`, `passwords.go`, `sessions.go`, `rbac.go`, `context.go`, `signing.go`, `hashing.go`
- Estimated size: ~500 lines

---

## 5. signal

**Purpose**: Signal adapter framework. Defines the interface for external and built-in content analysis signals, and provides a registry for runtime lookup.

### Responsibilities
- Define the Adapter interface
- Provide a thread-safe signal registry
- Implement built-in adapters: TextRegex, TextBank
- Implement generic HTTP signal adapter (for OpenAI, Google, custom)
- Signal registration at application startup

### Data Contracts

**Input to adapters**: `domain.SignalInput` (type + value string)

**Output from adapters**: `domain.SignalOutput` (score, label, metadata)

### API Contracts

```go
// Adapter is the interface all signal adapters implement.
// Identical to coop-lite-go's signal adapter interface.
type Adapter interface {
    ID() string
    DisplayName() string
    Description() string
    EligibleInputs() []domain.SignalInputType
    Cost() int
    Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error)
}

// Registry holds all registered signal adapters, keyed by ID.
// Thread-safe for concurrent reads. Read-only after startup.
type Registry struct { /* ... */ }

func NewRegistry() *Registry

// Register adds an adapter. Called at startup only.
func (r *Registry) Register(adapter Adapter)

// Get returns the adapter for the given signal ID, or nil.
func (r *Registry) Get(id string) Adapter

// All returns all registered adapters. Used by GET /api/v1/signals.
func (r *Registry) All() []Adapter

// --- Built-in adapters ---

// TextRegexAdapter matches input text against RE2 regex patterns.
type TextRegexAdapter struct{}

// TextBankAdapter matches input text against entries in a text bank.
// Requires store access for loading bank entries.
type TextBankAdapter struct {
    store *store.Queries
}

// HTTPSignalAdapter is a generic adapter for external HTTP-based signals.
// Configured with URL, headers, request/response mapping.
type HTTPSignalAdapter struct {
    id          string
    displayName string
    description string
    url         string
    headers     map[string]string
    httpClient  *http.Client
}
```

### Upstream Dependencies
- `domain` (for SignalInput, SignalOutput, SignalInputType)
- `store` (TextBankAdapter only, for loading bank entries)

### Downstream Dependents
- `engine` (signal() UDF calls Registry.Get() to invoke adapters)
- `handler` (GET /api/v1/signals lists adapters, POST /api/v1/signals/test tests them)

### Key Implementation Notes
- Registry uses `sync.RWMutex` internally but is read-only after startup, so the mutex is never contended on the hot path.
- New adapters are added by implementing the Adapter interface and registering in `cmd/server/main.go`.
- The TextBankAdapter loads entries from the store on each invocation (with optional caching).
- The HTTPSignalAdapter is configurable for any external API (OpenAI, Google, custom) via URL, headers, and request/response field mapping.
- Files: `adapter.go`, `registry.go`, `text_regex.go`, `text_bank.go`, `http_signal.go`
- Estimated size: ~400 lines

---

## 6. engine

**Purpose**: Starlark rule evaluation engine. This is the heart of Nest -- the merger of fruitfly's evaluation architecture with coop-lite-go's multi-tenant infrastructure. Handles compilation, caching, worker pool management, UDF registration, verdict resolution, and action publishing.

### Responsibilities
- Compile Starlark source into reusable Programs
- Extract metadata (rule_id, event_types, priority) from Starlark globals
- Build and manage immutable per-org Snapshots indexed by event type
- Manage a goroutine worker pool with per-worker Starlark thread isolation
- Register and execute built-in UDFs: verdict(), signal(), counter(), memo(), log(), now(), hash(), regex_match(), enqueue()
- Resolve verdicts across multiple rule results (priority-based, tie-break by verdict weight)
- Resolve action names to Action definitions
- Publish actions (webhooks with RSA-PSS signing, MRT enqueue)
- In-memory time-bucketed counters with cross-worker summing
- TTL in-memory cache for action name resolution

### Data Contracts

```go
// CompiledRule -- pre-compiled Starlark rule ready for evaluation
type CompiledRule struct {
    ID         string
    EventTypes []string
    Priority   int
    Program    *starlark.Program
    Source     string
}

// Snapshot -- immutable, pre-indexed collection of compiled rules for an org
type Snapshot struct {
    ID       string
    OrgID    string
    Rules    []*CompiledRule
    ByEvent  map[string][]*CompiledRule  // event_type -> rules (includes "*")
    LoadedAt time.Time
}

// EvalRequest -- submitted to the pool for evaluation.
// Defined in engine (not domain) because it carries context.Context,
// which is an engine-internal runtime concern.
type EvalRequest struct {
    Event domain.Event
    Ctx   context.Context
}

// EvalResult -- returned from evaluation
type EvalResult struct {
    Verdict        domain.Verdict
    TriggeredRules []domain.TriggeredRule
    ActionRequests []domain.ActionRequest
    Logs           []string
    LatencyUs      int64
    CorrelationID  string
}

// ActionTarget -- context for action execution
type ActionTarget struct {
    ItemID     string
    ItemTypeID string
    OrgID      string
    Payload    map[string]any
    CorrelationID string
}
```

### API Contracts

```go
// --- Compiler ---
type Compiler struct{}

// CompileRule parses Starlark source, extracts metadata, returns compiled rule.
// Pre-conditions: valid Starlark with rule_id, event_types, priority globals
//   and evaluate(event) function. Wildcard ["*"] must be sole element.
// Post-conditions: returned CompiledRule.Program is reusable.
// Errors: domain.CompileError on invalid syntax, missing globals, or bad wildcard.
func (c *Compiler) CompileRule(source string, filename string) (*CompiledRule, error)

// --- Snapshot ---

// RulesForEvent returns compiled rules matching the event type,
// including wildcard ("*") rules, sorted by priority descending.
func (s *Snapshot) RulesForEvent(eventType string) []*CompiledRule

// NewSnapshot builds an immutable snapshot from compiled rules.
func NewSnapshot(orgID string, rules []*CompiledRule) *Snapshot

// --- Pool ---
type Pool struct { /* ... */ }

func NewPool(workerCount int, registry *signal.Registry, store *store.Queries, logger *slog.Logger) *Pool

// Evaluate submits an event for rule evaluation. Blocks until result.
// Pre-conditions: event validated, snapshot loaded for event.OrgID.
// Post-conditions: all matching rules evaluated, execution logs written,
//   ActionRequests returned for LIVE rules.
func (p *Pool) Evaluate(ctx context.Context, event domain.Event) (*EvalResult, error)

// SwapSnapshot atomically replaces the snapshot for an org.
// Uses sync.Map + atomic.Pointer. Thread-safe for concurrent calls.
func (p *Pool) SwapSnapshot(orgID string, snap *Snapshot)

// CounterSum returns counter total across all workers for a given key.
func (p *Pool) CounterSum(orgID, entityID, eventType string, windowSeconds int) int64

// Stop gracefully shuts down the worker pool.
func (p *Pool) Stop()

// --- Worker (internal, not public API) ---
// Workers own their Starlark thread, memo map, counter shard, and eval cache.
// Not shared between goroutines.

// --- UDFs ---

// BuildUDFs constructs the predeclared Starlark dict for a worker.
// Called once at worker init. Event-scoped state via thread locals.
func BuildUDFs(w *Worker) starlark.StringDict

// enqueue() UDF implementation.
// File: udf_enqueue.go
// The enqueue(queue_name, reason) UDF creates an MRT job for the current
// item being evaluated. It accesses store.Queries (via the worker's back-reference
// to the pool) to call store.InsertMRTJob. The current event's item_id,
// item_type_id, org_id, and payload are extracted from the Starlark thread's
// local storage (set at the start of each event evaluation).
//
// enqueue() is called within Starlark rule evaluation:
//   enqueue("urgent-queue", reason="flagged by AI")
//
// It resolves the queue name to a queue ID via store.GetMRTQueueByName
// (with an in-process cache to avoid repeated lookups), then inserts the job.
// Returns true on success, false on failure (queue not found, insert error).
// Failures are logged but do not abort rule evaluation.

// --- Verdict Resolution ---

// resolveVerdict determines the final verdict from multiple rule results.
// Highest priority wins. Ties broken by verdict weight: block(3) > review(2) > approve(1).
func resolveVerdict(results []ruleResult) domain.Verdict

// --- Action Publisher ---

type Signer interface {
    Sign(ctx context.Context, orgID string, payload []byte) (string, error)
}

type ActionPublisher struct { /* ... */ }

func NewActionPublisher(store *store.Queries, signer Signer, httpClient *http.Client, logger *slog.Logger) *ActionPublisher

// PublishActions executes actions concurrently. Never returns an error;
// individual failures returned as ActionResult with Success=false.
func (p *ActionPublisher) PublishActions(ctx context.Context, actions []domain.ActionRequest, target ActionTarget) []domain.ActionResult

// --- Cache ---

// Cache is a TTL in-memory cache (sync.RWMutex + map).
// Used for action name resolution, NOT on the evaluation hot path.
type Cache struct { /* ... */ }
func NewCache(ttl time.Duration) *Cache
func (c *Cache) Get(key string) (any, bool)
func (c *Cache) Set(key string, value any)
```

### Upstream Dependencies
- `domain` (types: Event, Rule, Verdict, Action, ActionRequest, ActionResult, CompileError, TriggeredRule)
- `store` (for action name resolution, MRT enqueue, execution logging)
- `signal` (Registry for signal() UDF)

### Downstream Dependents
- `service` (calls Pool.Evaluate, Pool.SwapSnapshot, Compiler.CompileRule)
- `handler` (accesses ActionPublisher for MRT decision actions)
- `worker` (river workers call Pool.Evaluate for async items)

### Key Implementation Notes
- Zero `sync.Mutex` on the evaluation hot path. Only `sync.Map.Load`, `atomic.Pointer.Load`, `atomic.Int64`, and channels.
- Per-org snapshots stored in `sync.Map` of `*atomic.Pointer[Snapshot]`.
- Workers own their Starlark thread -- no thread sharing between goroutines.
- Eval cache (per-worker map of rule_id to cached `evaluate` callable) eliminates `Program.Init` on hot path.
- Signal results cached within a single event evaluation context via `sync.Map` (per-event, not per-worker).
- Counters are per-worker `atomic.Int64` with time-bucketed keys. `CounterSum` aggregates across workers.
- Starlark rule evaluation never panics -- errors in individual rules are recovered and logged.
- Per-rule timeout (1s default), per-event timeout (5s default) via `context.WithTimeout`.
- Files: `pool.go`, `worker.go`, `snapshot.go`, `compiler.go`, `udf.go`, `udf_signal.go`, `udf_counter.go`, `udf_enqueue.go`, `action_publisher.go`, `cache.go`
- Estimated size: ~1,400 lines

---

## 7. service

**Purpose**: Business logic orchestration layer. Sits between handlers and store/engine. Implements multi-step operations that coordinate across multiple store queries and the engine.

### Responsibilities
- Rule CRUD with Starlark compilation validation, derived column extraction, entity history, and snapshot rebuild
- Rule testing (compile and evaluate without persisting)
- Thin CRUD for Actions, Policies, Item Types with entity history
- Item validation against item type schema and submission
- MRT operations: enqueue, assign, decide (returns ActionRequests for handler to orchestrate)
- User CRUD: create, update, deactivate, invite
- API key lifecycle: create (return plaintext once), verify, revoke
- Signing key management: list, rotate
- Text bank management: create bank, add/delete entries

### Data Contracts

**Service parameter types** (not persisted, used for create/update operations):

```go
// --- Rule params ---
type CreateRuleParams struct {
    Name      string
    Status    domain.RuleStatus
    Source    string   // Starlark source (event_types, priority extracted by compiler)
    Tags      []string
    PolicyIDs []string
}

type UpdateRuleParams struct {
    Name      *string
    Status    *domain.RuleStatus
    Source    *string
    Tags      *[]string
    PolicyIDs *[]string
}

type TestResult struct {
    Verdict   domain.VerdictType `json:"verdict"`
    Reason    string             `json:"reason"`
    RuleID    string             `json:"rule_id"`
    Actions   []string           `json:"actions"`
    Logs      []string           `json:"logs"`
    LatencyUs int64              `json:"latency_us"`
}

// --- Config params ---
type CreateActionParams struct {
    Name        string
    ActionType  domain.ActionType
    Config      map[string]any
    ItemTypeIDs []string // optional: restrict action to specific item types
}

type UpdateActionParams struct {
    Name        *string
    ActionType  *domain.ActionType
    Config      *map[string]any
    ItemTypeIDs *[]string
}

type CreatePolicyParams struct {
    Name          string
    Description   *string
    ParentID      *string
    StrikePenalty int
}

type UpdatePolicyParams struct {
    Name          *string
    Description   *string
    ParentID      *string
    StrikePenalty *int
}

type CreateItemTypeParams struct {
    Name       string
    Kind       domain.ItemTypeKind
    Schema     map[string]any
    FieldRoles map[string]any
}

type UpdateItemTypeParams struct {
    Name       *string
    Kind       *domain.ItemTypeKind
    Schema     *map[string]any
    FieldRoles *map[string]any
}

// --- MRT params ---
type EnqueueParams struct {
    OrgID         string
    QueueName     string
    ItemID        string
    ItemTypeID    string
    Payload       map[string]any
    EnqueueSource string
    SourceInfo    map[string]any
    PolicyIDs     []string
}

type DecisionParams struct {
    OrgID     string
    JobID     string
    UserID    string
    Verdict   string
    ActionIDs []string
    PolicyIDs []string
    Reason    string
}

type DecisionResult struct {
    Decision       domain.MRTDecision
    ActionRequests []domain.ActionRequest
}

// --- Item params ---
type SubmitItemParams struct {
    ItemID        string
    ItemTypeID    string
    OrgID         string
    Payload       map[string]any
    CreatorID     string
    CreatorTypeID string
}

// --- User params ---
type UserUpdateParams struct {
    Name     *string
    Role     *domain.UserRole
    IsActive *bool
}
```

### API Contracts

```go
// --- RuleService ---
type RuleService struct {
    store    *store.Queries
    compiler *engine.Compiler
    pool     *engine.Pool
    logger   *slog.Logger
}

func NewRuleService(store *store.Queries, compiler *engine.Compiler, pool *engine.Pool, logger *slog.Logger) *RuleService

// CreateRule validates Starlark, extracts derived columns, persists, rebuilds snapshot.
// Returns: created Rule with derived event_types and priority.
// Errors: ValidationError if compilation fails or globals missing.
func (s *RuleService) CreateRule(ctx context.Context, orgID string, params CreateRuleParams) (*domain.Rule, error)

// UpdateRule validates new source, saves old version to entity_history, rebuilds snapshot.
func (s *RuleService) UpdateRule(ctx context.Context, orgID, ruleID string, params UpdateRuleParams) (*domain.Rule, error)

// DeleteRule removes the rule and rebuilds the snapshot.
func (s *RuleService) DeleteRule(ctx context.Context, orgID, ruleID string) error

// GetRule returns a single rule by ID.
func (s *RuleService) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error)

// ListRules returns paginated rules.
func (s *RuleService) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error)

// TestRule compiles and evaluates against sample event WITHOUT persisting.
func (s *RuleService) TestRule(ctx context.Context, orgID string, source string, event domain.Event) (*TestResult, error)

// TestExistingRule evaluates a persisted rule against a sample event.
func (s *RuleService) TestExistingRule(ctx context.Context, orgID, ruleID string, event domain.Event) (*TestResult, error)

// RebuildSnapshot fetches all enabled rules for an org, compiles, and swaps snapshot.
// Called on startup for all active orgs.
func (s *RuleService) RebuildSnapshot(ctx context.Context, orgID string) error

// --- ConfigService ---
type ConfigService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewConfigService(store *store.Queries, logger *slog.Logger) *ConfigService

func (s *ConfigService) CreateAction(ctx context.Context, orgID string, params CreateActionParams) (*domain.Action, error)
func (s *ConfigService) UpdateAction(ctx context.Context, orgID, actionID string, params UpdateActionParams) (*domain.Action, error)
func (s *ConfigService) DeleteAction(ctx context.Context, orgID, actionID string) error
func (s *ConfigService) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error)
func (s *ConfigService) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error)

func (s *ConfigService) CreatePolicy(ctx context.Context, orgID string, params CreatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) UpdatePolicy(ctx context.Context, orgID, policyID string, params UpdatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) DeletePolicy(ctx context.Context, orgID, policyID string) error
func (s *ConfigService) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error)
func (s *ConfigService) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error)

func (s *ConfigService) CreateItemType(ctx context.Context, orgID string, params CreateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) UpdateItemType(ctx context.Context, orgID, itemTypeID string, params UpdateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error
func (s *ConfigService) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error)
func (s *ConfigService) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error)

// --- MRTService ---
type MRTService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewMRTService(store *store.Queries, logger *slog.Logger) *MRTService

// Enqueue creates a new MRT job in the specified queue.
func (s *MRTService) Enqueue(ctx context.Context, params EnqueueParams) (string, error)

// AssignNext assigns the next pending job in the queue to the user.
func (s *MRTService) AssignNext(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error)

// RecordDecision records a verdict and returns ActionRequests for the handler to execute.
// Does NOT execute actions itself -- avoids circular dependency with ActionPublisher.
func (s *MRTService) RecordDecision(ctx context.Context, params DecisionParams) (*DecisionResult, error)

// ListQueues returns all MRT queues for the org.
func (s *MRTService) ListQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error)

// ListJobs returns paginated jobs for a queue.
func (s *MRTService) ListJobs(ctx context.Context, orgID, queueID string, status *string, page domain.PageParams) (*domain.PaginatedResult[domain.MRTJob], error)

// GetJob returns a single MRT job.
func (s *MRTService) GetJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error)

// --- ItemService ---
type ItemService struct {
    store     *store.Queries
    pool      *engine.Pool
    publisher *engine.ActionPublisher
    logger    *slog.Logger
}

func NewItemService(store *store.Queries, pool *engine.Pool, publisher *engine.ActionPublisher, logger *slog.Logger) *ItemService

// SubmitSync validates items, evaluates rules, executes actions, returns results.
// Returns []domain.EvalResultResponse matching the shape defined in domain.
func (s *ItemService) SubmitSync(ctx context.Context, orgID string, items []SubmitItemParams) ([]domain.EvalResultResponse, error)

// SubmitAsync validates items and enqueues river jobs. Returns 202-style response.
func (s *ItemService) SubmitAsync(ctx context.Context, orgID string, items []SubmitItemParams) ([]string, error)

// --- UserService ---
type UserService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewUserService(store *store.Queries, logger *slog.Logger) *UserService

func (s *UserService) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error)
func (s *UserService) InviteUser(ctx context.Context, orgID string, email, name string, role domain.UserRole) (*domain.User, error)
func (s *UserService) UpdateUser(ctx context.Context, orgID, userID string, params UserUpdateParams) (*domain.User, error)
func (s *UserService) DeactivateUser(ctx context.Context, orgID, userID string) error

// --- APIKeyService ---
type APIKeyService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewAPIKeyService(store *store.Queries, logger *slog.Logger) *APIKeyService

func (s *APIKeyService) Create(ctx context.Context, orgID, name string) (key string, apiKey *domain.ApiKey, err error)
func (s *APIKeyService) List(ctx context.Context, orgID string) ([]domain.ApiKey, error)
func (s *APIKeyService) Revoke(ctx context.Context, orgID, keyID string) error

// --- SigningKeyService ---
type SigningKeyService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewSigningKeyService(store *store.Queries, logger *slog.Logger) *SigningKeyService

func (s *SigningKeyService) List(ctx context.Context, orgID string) ([]domain.SigningKey, error)
func (s *SigningKeyService) Rotate(ctx context.Context, orgID string) (*domain.SigningKey, error)

// --- TextBankService ---
type TextBankService struct {
    store  *store.Queries
    logger *slog.Logger
}

func NewTextBankService(store *store.Queries, logger *slog.Logger) *TextBankService

func (s *TextBankService) List(ctx context.Context, orgID string) ([]domain.TextBank, error)
func (s *TextBankService) Get(ctx context.Context, orgID, bankID string) (*domain.TextBank, error)
func (s *TextBankService) Create(ctx context.Context, orgID, name, description string) (*domain.TextBank, error)
func (s *TextBankService) AddEntry(ctx context.Context, orgID, bankID, value string, isRegex bool) (*domain.TextBankEntry, error)
func (s *TextBankService) DeleteEntry(ctx context.Context, orgID, bankID, entryID string) error
```

### Upstream Dependencies
- `domain` (all types)
- `store` (all database operations)
- `engine` (Compiler, Pool, ActionPublisher)

### Downstream Dependents
- `handler` (calls service methods)
- `worker` (river workers call ItemService for async processing)

### Key Implementation Notes
- RuleService is the most complex service: it coordinates compilation, entity history, derived column extraction, and snapshot rebuild on every rule CRUD operation.
- MRTService.RecordDecision returns ActionRequests but does NOT execute them. The handler orchestrates action execution via ActionPublisher. This avoids a circular dependency.
- ConfigService is thin CRUD with entity history for actions, policies, and item types.
- Every create/update operation that changes a versioned entity writes to entity_history.
- TextBankService methods include orgID for multi-tenant isolation. The service passes orgID to store methods which join through text_banks to verify org ownership.
- Files: `rules.go`, `config.go`, `mrt.go`, `items.go`, `users.go`, `api_keys.go`, `signing_keys.go`, `text_banks.go`
- Estimated size: ~1,500 lines

---

## 8. worker

**Purpose**: Background job processing via river (PostgreSQL-native job queue). Handles async item processing and periodic maintenance tasks.

### Responsibilities
- Async item processing: receive river job, call Pool.Evaluate, publish actions, log results
- Periodic maintenance jobs:
  - Snapshot rebuild (cold start and periodic refresh)
  - Partition manager (create future execution log partitions)
  - Session cleanup (delete expired sessions)
  - Counter flush (optional: flush in-memory counters to PostgreSQL)

### Data Contracts

```go
// ProcessItemArgs -- river job arguments for async item processing
type ProcessItemArgs struct {
    OrgID      string         `json:"org_id"`
    ItemID     string         `json:"item_id"`
    ItemTypeID string         `json:"item_type_id"`
    EventType  string         `json:"event_type"`
    Payload    map[string]any `json:"payload"`
}
```

### API Contracts

```go
// ProcessItemWorker handles async item evaluation via river.
type ProcessItemWorker struct {
    pool      *engine.Pool
    publisher *engine.ActionPublisher
    store     *store.Queries
    logger    *slog.Logger
}

func NewProcessItemWorker(pool *engine.Pool, publisher *engine.ActionPublisher, store *store.Queries, logger *slog.Logger) *ProcessItemWorker

// Work implements river.Worker. Evaluates rules and publishes actions.
func (w *ProcessItemWorker) Work(ctx context.Context, job *river.Job[ProcessItemArgs]) error

// RegisterMaintenanceJobs registers all periodic maintenance jobs with the river client.
// Jobs: snapshot rebuild, partition manager, session cleanup, counter flush.
func RegisterMaintenanceJobs(
    client *river.Client[pgx.Tx],
    ruleService *service.RuleService,
    store *store.Queries,
    pool *engine.Pool,
    logger *slog.Logger,
)
```

### Upstream Dependencies
- `domain` (types)
- `store` (database operations)
- `engine` (Pool.Evaluate, ActionPublisher)
- `service` (RuleService.RebuildSnapshot for maintenance)

### Downstream Dependents
- `cmd/server` (registers workers with river client)

### Key Implementation Notes
- Uses river's PostgreSQL advisory locks for job coordination across multiple instances.
- ProcessItemWorker mirrors the sync evaluation path but runs in a river goroutine.
- Maintenance jobs are consolidated into `maintenance.go` -- each is a short periodic function.
- Partition manager creates next month's partitions for `rule_executions` and `action_executions`.
- Files: `process_item.go`, `maintenance.go`
- Estimated size: ~200 lines

---

## 9. handler

**Purpose**: HTTP handlers and chi router configuration. Translates HTTP requests into service calls and service responses into HTTP responses. No business logic.

### Responsibilities
- Define chi router with all routes (internal session-auth, external API-key-auth)
- Parse and validate HTTP request bodies
- Call appropriate service methods
- Serialize responses as JSON
- Apply middleware (auth, RBAC, CSRF)
- Common helpers: JSON encoding/decoding, error response formatting, pagination parsing
- Health check endpoint
- UDF listing endpoint

### Data Contracts

**Request/Response types**: JSON request bodies map to service param types. JSON responses map to domain types (serialized via JSON tags). Error responses follow a consistent format:

```go
// ErrorResponse -- standard error format
type ErrorResponse struct {
    Error   string            `json:"error"`
    Details map[string]string `json:"details,omitempty"`
}
```

### API Contracts

```go
// NewRouter constructs the complete chi router with all routes and middleware.
// This is the single function that defines all HTTP routes.
//
// Note: store is NOT a parameter. Auth middleware accesses store through the
// auth module (SessionAuth/APIKeyAuth take store at construction time in
// cmd/server, and the resulting middleware closures are passed to NewRouter).
// This preserves the invariant that handler does not import store.
func NewRouter(
    ruleService    *service.RuleService,
    configService  *service.ConfigService,
    mrtService     *service.MRTService,
    itemService    *service.ItemService,
    userService    *service.UserService,
    apiKeyService  *service.APIKeyService,
    signingKeyService *service.SigningKeyService,
    textBankService *service.TextBankService,
    publisher      *engine.ActionPublisher,
    signalRegistry *signal.Registry,
    sessionAuthMw  func(http.Handler) http.Handler,
    apiKeyAuthMw   func(http.Handler) http.Handler,
    logger         *slog.Logger,
) chi.Router

// Route groups:

// External API (API key auth):
//   POST   /api/v1/items
//   POST   /api/v1/items/async
//   GET    /api/v1/policies

// Internal API (Session auth):
//   GET/POST        /api/v1/rules
//   GET/PUT/DELETE  /api/v1/rules/{id}
//   POST            /api/v1/rules/test
//   POST            /api/v1/rules/{id}/test
//   GET/POST        /api/v1/actions
//   GET/PUT/DELETE  /api/v1/actions/{id}
//   GET/POST        /api/v1/policies
//   GET/PUT/DELETE  /api/v1/policies/{id}
//   GET/POST        /api/v1/item-types
//   GET/PUT/DELETE  /api/v1/item-types/{id}
//   GET             /api/v1/mrt/queues
//   GET             /api/v1/mrt/queues/{id}/jobs
//   POST            /api/v1/mrt/queues/{id}/assign
//   POST            /api/v1/mrt/decisions
//   GET             /api/v1/mrt/jobs/{id}
//   GET/POST        /api/v1/users, /api/v1/users/invite
//   PUT/DELETE      /api/v1/users/{id}
//   GET/POST        /api/v1/api-keys
//   DELETE          /api/v1/api-keys/{id}
//   POST            /api/v1/auth/login
//   POST            /api/v1/auth/logout
//   GET             /api/v1/auth/me
//   POST            /api/v1/auth/reset-password
//   GET/POST        /api/v1/text-banks
//   GET             /api/v1/text-banks/{id}
//   POST            /api/v1/text-banks/{id}/entries
//   DELETE          /api/v1/text-banks/{id}/entries/{entryId}
//   GET             /api/v1/signals
//   POST            /api/v1/signals/test
//   GET             /api/v1/signing-keys
//   POST            /api/v1/signing-keys/rotate
//   GET             /api/v1/udfs
//   GET             /api/v1/health

// --- Helper functions (in helpers.go) ---

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any)

// Decode reads and JSON-decodes the request body into v.
func Decode(r *http.Request, v any) error

// Error writes a structured error response.
func Error(w http.ResponseWriter, status int, msg string)

// OrgID extracts the org ID from the request context (set by auth middleware).
func OrgID(r *http.Request) string

// UserID extracts the user ID from the request context.
func UserID(r *http.Request) string

// PageParamsFromRequest extracts pagination parameters from query string.
func PageParamsFromRequest(r *http.Request) domain.PageParams
```

### Upstream Dependencies
- `domain` (types for response serialization)
- `service` (all service types for business logic)
- `engine` (ActionPublisher for MRT decision action execution)
- `auth` (context helpers: OrgIDFromContext, UserIDFromContext, RoleFromContext)
- `signal` (Registry for signal listing and testing)

### Downstream Dependents
- `cmd/server` (creates the router)

### Key Implementation Notes
- Handlers are thin: parse request, call service, write response. No business logic.
- MRT decision handler orchestrates the flow: calls MRTService.RecordDecision, then ActionPublisher.PublishActions with the returned ActionRequests.
- Auth handler manages login (create session), logout (delete session), and me (return current user). The login handler calls the auth module (not store directly) for password verification and session creation.
- UDF listing handler returns a hardcoded list of UDF definitions (name, signature, description, example).
- Auth middleware closures (sessionAuthMw, apiKeyAuthMw) are constructed in cmd/server and passed as parameters, so handler never imports store.
- Files: `rules.go`, `config.go`, `items.go`, `mrt.go`, `users.go`, `auth.go`, `orgs.go`, `api_keys.go`, `signals.go`, `text_banks.go`, `health.go`, `helpers.go`
- Estimated size: ~1,200 lines

---

## 10. cmd/server

**Purpose**: Composition root. Parses configuration, constructs all dependencies, wires them together, and starts the HTTP server and river workers.

### Responsibilities
- Parse configuration via `config.Load()`
- Create PostgreSQL connection pool
- Construct store, auth middleware, signal registry, engine pool, all services
- Register signal adapters
- Construct auth middleware closures (SessionAuth, APIKeyAuth) with store dependency
- Construct chi router via `handler.NewRouter()`, passing middleware closures instead of store
- Start river workers
- Start HTTP server
- Handle graceful shutdown (SIGINT/SIGTERM)

### Data Contracts
- None. This module consumes all other modules and exposes nothing.

### API Contracts

```go
// main.go -- no public API, this is the entry point
func main()
```

### Upstream Dependencies
- All other internal modules: `config`, `domain`, `store`, `auth`, `signal`, `engine`, `service`, `worker`, `handler`

### Downstream Dependents
- None. This is the root.

### Key Implementation Notes
- Plain `main()` function with struct construction. No DI framework.
- Graceful shutdown: catch SIGINT/SIGTERM, drain HTTP server, stop river client, stop engine pool, close database pool.
- On startup, rebuilds snapshots for all active orgs from the database.
- Signal adapters registered here: TextRegex, TextBank, any configured HTTP signals.
- Auth middleware is constructed here with store dependency, then passed as closures to handler.NewRouter. This is the key wiring that keeps handler independent of store.
- File: `cmd/server/main.go`
- Estimated size: ~200 lines

---

## 11. cmd/migrate

**Purpose**: Database migration runner. Applies sequential SQL migration files to PostgreSQL.

### Responsibilities
- Connect to PostgreSQL using config.Load() DATABASE_URL
- Track applied migrations in a `schema_migrations` table
- Apply pending migrations in order
- Support `up` (apply all pending) and `status` (show applied/pending) commands

### Data Contracts
- None. Reads SQL files from the `migrations/` directory.

### API Contracts

```go
// main.go -- no public API, this is a CLI entry point
// Usage: go run cmd/migrate/main.go [up|status]
func main()
```

### Upstream Dependencies
- `config` (for DATABASE_URL)

### Downstream Dependents
- None. Run manually or in CI before server startup.

### Key Implementation Notes
- Uses pgx directly (no ORM, no migration library).
- Migration files are sequentially numbered: `001_initial.sql`, `002_partitions.sql`.
- The `schema_migrations` table tracks which files have been applied.
- File: `cmd/migrate/main.go`
- Estimated size: ~80 lines

---

## 12. cmd/seed

**Purpose**: Seed development data. Creates an initial org, admin user, default MRT queues, and sample entities for local development.

### Responsibilities
- Connect to PostgreSQL using config.Load() DATABASE_URL
- Create a default org
- Create an admin user with a known password
- Create default MRT queues (e.g., "default", "urgent", "escalation")
- Optionally create sample rules, actions, policies, and item types

### Data Contracts
- None. Writes directly to the database via store.Queries.

### API Contracts

```go
// main.go -- no public API, this is a CLI entry point
// Usage: go run cmd/seed/main.go
func main()
```

### Upstream Dependencies
- `config` (for DATABASE_URL)
- `domain` (for entity types)
- `store` (for database writes)
- `auth` (for password hashing)

### Downstream Dependents
- None. Run manually during development setup.

### Key Implementation Notes
- MRT queue creation happens here. Queues are infrastructure created during org provisioning, not through the REST API in v1.0. The store.CreateMRTQueue method is called directly.
- Idempotent: can be run multiple times without creating duplicates (uses INSERT ... ON CONFLICT DO NOTHING).
- File: `cmd/seed/main.go`
- Estimated size: ~120 lines

---

## 13. migrations

**Purpose**: PostgreSQL schema DDL. Defines all 19 tables, indexes, and initial partitions for the v1.0 schema.

### Responsibilities
- Define all tables: orgs, users, password_reset_tokens, api_keys, signing_keys, item_types, policies, rules, actions, entity_history, rules_policies, actions_item_types, text_banks, text_bank_entries, mrt_queues, mrt_jobs, mrt_decisions, rule_executions, action_executions, items, sessions
- Define all indexes
- Create initial time-range partitions for execution log tables

### Data Contracts
- The SQL schema is the contract. All 19 tables with their column types, constraints, foreign keys, check constraints, and unique indexes as specified in NEST_DESIGN.md section 6.

### API Contracts
- Migrations are executed by `cmd/migrate/main.go` or at server startup.
- Migration files are sequentially numbered: `001_initial.sql`, `002_partitions.sql`

### Upstream Dependencies
- None (pure SQL).

### Downstream Dependents
- `store` (queries depend on this schema)
- All modules indirectly (the schema defines the data model)

### Key Implementation Notes
- 19 tables total. Tables removed vs coop-lite-go: `rules_history`, `actions_history`, `policies_history` (merged into `entity_history`), `rules_actions` (eliminated), `rules_item_types` (eliminated), `mrt_routing_rules` (deferred), `user_strikes` (deferred), `reports` (deferred).
- `rule_executions` and `action_executions` are partitioned by month (PARTITION BY RANGE on executed_at).
- `entity_history` uses composite primary key `(entity_type, id, version)`.
- `rules.event_types` uses GIN index for array containment queries.
- `actions` has UNIQUE constraint on `(org_id, name)` for action name resolution.
- Files: `migrations/001_initial.sql`, `migrations/002_partitions.sql`
- Estimated size: ~250 lines

---

## 14. ui/api

**Purpose**: Typed async HTTP client and data type definitions for the Python UI. This is the only module that makes HTTP calls. All pages go through this layer.

### Responsibilities
- Define all request/response dataclasses mirroring Nest domain types
- Provide typed async methods for every Nest REST endpoint
- Centralize HTTP error handling patterns

### Data Contracts

```python
# api/types.py -- mirrors domain types from the Go backend

@dataclass
class Rule:
    id: str
    org_id: str
    name: str
    status: str        # "LIVE" | "BACKGROUND" | "DISABLED"
    source: str
    event_types: list[str]
    priority: int
    tags: list[str]
    version: int
    created_at: str
    updated_at: str

@dataclass
class Action:
    id: str
    org_id: str
    name: str
    action_type: str   # "WEBHOOK" | "ENQUEUE_TO_MRT"
    config: dict
    version: int
    created_at: str
    updated_at: str

@dataclass
class Policy:
    id: str
    org_id: str
    name: str
    description: str | None
    parent_id: str | None
    strike_penalty: int
    version: int
    created_at: str
    updated_at: str

@dataclass
class ItemType:
    id: str
    org_id: str
    name: str
    kind: str          # "CONTENT" | "USER" | "THREAD"
    schema: dict
    field_roles: dict
    created_at: str
    updated_at: str

@dataclass
class User:
    id: str
    org_id: str
    email: str
    name: str
    role: str          # "ADMIN" | "MODERATOR" | "ANALYST"
    is_active: bool
    created_at: str
    updated_at: str

@dataclass
class ApiKey:
    id: str
    org_id: str
    name: str
    prefix: str
    created_at: str
    revoked_at: str | None

@dataclass
class MRTQueue:
    id: str
    org_id: str
    name: str
    description: str | None
    is_default: bool

@dataclass
class MRTJob:
    id: str
    org_id: str
    queue_id: str
    item_id: str
    item_type_id: str
    payload: dict
    status: str        # "PENDING" | "ASSIGNED" | "DECIDED"
    assigned_to: str | None
    policy_ids: list[str]
    enqueue_source: str
    source_info: dict

@dataclass
class MRTDecision:
    id: str
    org_id: str
    job_id: str
    user_id: str
    verdict: str
    action_ids: list[str]
    policy_ids: list[str]
    reason: str | None

@dataclass
class Signal:
    id: str
    display_name: str
    description: str
    eligible_inputs: list[str]
    cost: int

@dataclass
class SigningKey:
    id: str
    org_id: str
    public_key: str
    is_active: bool
    created_at: str

@dataclass
class TextBank:
    id: str
    org_id: str
    name: str
    description: str | None
    entries: list[dict] | None  # populated on get, not on list

@dataclass
class PaginatedResult[T]:
    items: list[T]
    total: int
    page: int
    page_size: int
    total_pages: int
```

### API Contracts

```python
class NestClient:
    """Typed async HTTP client for all Nest REST endpoints.

    Does NOT own the httpx.AsyncClient. Receives it via constructor.
    One NestClient instance per page load, sharing the session's http client.
    """

    def __init__(self, http: httpx.AsyncClient) -> None: ...

    # Auth
    async def login(self, email: str, password: str) -> dict: ...
    async def logout(self) -> None: ...
    async def me(self) -> User: ...

    # Rules
    async def list_rules(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Rule]: ...
    async def get_rule(self, rule_id: str) -> Rule: ...
    async def create_rule(self, name: str, status: str, source: str,
                          tags: list[str] | None = None,
                          policy_ids: list[str] | None = None) -> Rule: ...
    async def update_rule(self, rule_id: str, *,
                          name: str | None = None, status: str | None = None,
                          source: str | None = None, tags: list[str] | None = None,
                          policy_ids: list[str] | None = None) -> Rule: ...
    async def delete_rule(self, rule_id: str) -> None: ...
    async def test_rule(self, source: str, event: dict) -> dict: ...
    async def test_existing_rule(self, rule_id: str, event: dict) -> dict: ...

    # Actions
    async def list_actions(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Action]: ...
    async def get_action(self, action_id: str) -> Action: ...
    async def create_action(self, name: str, action_type: str, config: dict) -> Action: ...
    async def update_action(self, action_id: str, *,
                            name: str | None = None, action_type: str | None = None,
                            config: dict | None = None) -> Action: ...
    async def delete_action(self, action_id: str) -> None: ...

    # Policies
    async def list_policies(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Policy]: ...
    async def get_policy(self, policy_id: str) -> Policy: ...
    async def create_policy(self, name: str, description: str | None = None,
                            parent_id: str | None = None, strike_penalty: int = 0) -> Policy: ...
    async def update_policy(self, policy_id: str, *,
                            name: str | None = None, description: str | None = None,
                            parent_id: str | None = None, strike_penalty: int | None = None) -> Policy: ...
    async def delete_policy(self, policy_id: str) -> None: ...

    # Item Types
    async def list_item_types(self, page: int = 1, page_size: int = 50) -> PaginatedResult[ItemType]: ...
    async def get_item_type(self, item_type_id: str) -> ItemType: ...
    async def create_item_type(self, name: str, kind: str, schema: dict,
                               field_roles: dict | None = None) -> ItemType: ...
    async def update_item_type(self, item_type_id: str, *,
                               name: str | None = None, kind: str | None = None,
                               schema: dict | None = None, field_roles: dict | None = None) -> ItemType: ...
    async def delete_item_type(self, item_type_id: str) -> None: ...

    # MRT
    async def list_mrt_queues(self) -> list[MRTQueue]: ...
    async def list_mrt_jobs(self, queue_id: str, status: str | None = None,
                            page: int = 1, page_size: int = 50) -> PaginatedResult[MRTJob]: ...
    async def assign_next_job(self, queue_id: str) -> MRTJob | None: ...
    async def get_mrt_job(self, job_id: str) -> MRTJob: ...
    async def record_decision(self, job_id: str, verdict: str,
                              action_ids: list[str] | None = None,
                              policy_ids: list[str] | None = None,
                              reason: str | None = None) -> MRTDecision: ...

    # Users
    async def list_users(self, page: int = 1, page_size: int = 50) -> PaginatedResult[User]: ...
    async def invite_user(self, email: str, name: str, role: str) -> User: ...
    async def update_user(self, user_id: str, *,
                          name: str | None = None, role: str | None = None,
                          is_active: bool | None = None) -> User: ...
    async def deactivate_user(self, user_id: str) -> None: ...

    # API Keys
    async def list_api_keys(self) -> list[ApiKey]: ...
    async def create_api_key(self, name: str) -> dict: ...
    async def revoke_api_key(self, key_id: str) -> None: ...

    # Text Banks
    async def list_text_banks(self) -> list[TextBank]: ...
    async def get_text_bank(self, bank_id: str) -> TextBank: ...
    async def create_text_bank(self, name: str, description: str | None = None) -> TextBank: ...
    async def add_text_bank_entry(self, bank_id: str, value: str, is_regex: bool = False) -> dict: ...
    async def delete_text_bank_entry(self, bank_id: str, entry_id: str) -> None: ...

    # Signals
    async def list_signals(self) -> list[Signal]: ...
    async def test_signal(self, signal_id: str, input_value: str) -> dict: ...

    # UDFs
    async def list_udfs(self) -> list[dict]: ...

    # Signing Keys
    async def list_signing_keys(self) -> list[SigningKey]: ...
    async def rotate_signing_key(self) -> SigningKey: ...

    # Health
    async def health(self) -> dict: ...
```

### Upstream Dependencies
- `httpx` (async HTTP client, external)

### Downstream Dependents
- `ui/auth` (calls login, logout, me)
- `ui/pages` (all pages use NestClient)

### Key Implementation Notes
- No `**kwargs` in any method. All parameters are explicitly typed.
- All methods are async. Returns typed dataclasses.
- Raises `httpx.HTTPStatusError` on non-2xx responses. Pages handle errors per the standard pattern.
- httpx.AsyncClient is NOT owned by NestClient. Created once at login, stored in app.storage.user.
- Pagination: all list methods accept page and page_size parameters for consistency (A1 fix).
- Files: `api/client.py`, `api/types.py`
- Estimated size: ~370 lines

---

## 15. ui/auth

**Purpose**: Authentication state management and route guard middleware for the Python UI.

### Responsibilities
- Store and retrieve session token and user from NiceGUI's app.storage.user
- Provide RBAC helper functions (user_role, can_edit, is_moderator_or_above)
- Auth guard middleware: redirect to /login if not authenticated
- Session validation: verify session is still valid via GET /api/v1/auth/me

### Data Contracts
- Reads/writes `app.storage.user['session_token']` (string)
- Reads/writes `app.storage.user['user']` (dict with id, name, email, role, org_id)
- Reads/writes `app.storage.user['http_client']` (httpx.AsyncClient instance)

### API Contracts

```python
# auth/state.py

def user_role() -> str:
    """Return current user's role from session storage."""

def can_edit(resource: str) -> bool:
    """Check if current user can edit a resource type. Returns True for ADMIN only."""

def is_moderator_or_above() -> bool:
    """Check if current user is MODERATOR or ADMIN."""


# auth/middleware.py

def require_auth(page_func):
    """Decorator for @ui.page functions. Redirects to /login if not authenticated.
    Validates session via GET /api/v1/auth/me on each page load.
    Clears session and redirects on 401."""
```

### Upstream Dependencies
- `nicegui` (app.storage)
- `httpx` (for session validation HTTP call)

### Downstream Dependents
- `ui/pages` (all pages use require_auth decorator and RBAC helpers)
- `ui/components` (layout uses RBAC helpers for sidebar filtering)

### Key Implementation Notes
- RBAC is advisory only. The Go backend enforces access control authoritatively.
- Session validation on every page load ensures stale sessions are caught immediately.
- Files: `auth/state.py`, `auth/middleware.py`
- Estimated size: ~50 lines

---

## 16. ui/components

**Purpose**: Shared UI components used across multiple pages. Kept minimal -- only components that justify their own file.

### Responsibilities
- App shell layout: sidebar navigation (RBAC-filtered), header, logout button
- Confirmation dialog utility
- Starlark code editor with UDF/signal reference sidebar

### Data Contracts
- Layout reads user role from `app.storage.user` for sidebar filtering.
- Starlark editor accepts UDF and signal metadata lists (from GET /api/v1/udfs and GET /api/v1/signals).

### API Contracts

```python
# components/layout.py

def layout(title: str) -> ui.column:
    """App shell with RBAC-filtered sidebar and header.
    Returns a ui.column context manager for page content.

    Usage:
        with layout('Rules'):
            ui.label('content here')
    """

async def confirm(message: str, title: str = 'Confirm') -> bool:
    """Show a confirmation dialog. Returns True if user confirms."""


# components/starlark_editor.py

def starlark_editor(
    value: str = '',
    on_change=None,
    udfs: list[dict] | None = None,
    signals: list[dict] | None = None,
) -> ui.codemirror:
    """Starlark code editor with UDF and signal reference panel.

    Args:
        value: Initial Starlark source code.
        on_change: Callback when source changes.
        udfs: UDF definitions from GET /api/v1/udfs.
        signals: Signal definitions from GET /api/v1/signals.

    Returns:
        The codemirror element for binding.
    """
```

### Upstream Dependencies
- `nicegui` (UI framework)
- `ui/auth` (RBAC helpers for sidebar filtering)

### Downstream Dependents
- `ui/pages` (all pages use layout, rules page uses starlark_editor)

### Key Implementation Notes
- Exactly 2 files. No wrappers around single NiceGUI calls.
- Sidebar nav items are filtered by role rank: ANALYST < MODERATOR < ADMIN.
- Starlark editor uses Python syntax highlighting (closest to Starlark available in CodeMirror).
- Files: `components/layout.py`, `components/starlark_editor.py`
- Estimated size: ~100 lines

---

## 17. ui/pages

**Purpose**: Individual page modules. Each page is a self-contained Python file that registers its own routes, fetches data from the API, and renders UI components.

### Responsibilities
- Login page: email/password form, session creation, HTTP client creation
- Dashboard: overview counts, quick links
- Rules: list + create/edit with Starlark editor, templates, test panel
- Actions: CRUD for webhook and MRT enqueue actions
- Policies: CRUD with hierarchical display
- Item Types: CRUD with schema editor
- MRT: queue list, job review, decision recording
- Text Banks: bank list, entry management
- Users: list, invite, role management (ADMIN only)
- API Keys: list, create (show key once), revoke
- Signals: list, test
- Settings: org info, signing key rotation

### Data Contracts
- Each page fetches data from `NestClient` (typed responses).
- Each page renders domain types into NiceGUI tables and forms.
- No page stores persistent local state beyond the current render.

### API Contracts
- Each page file registers one or more routes via `@ui.page('/path')`.
- No page exposes functions for other pages to import.
- Pages import only from: `api.client`, `api.types`, `auth.state`, `auth.middleware`, `components.layout`, `components.starlark_editor`.

**Route registrations**:

| File | Routes |
|------|--------|
| `login.py` | `/login` |
| `dashboard.py` | `/dashboard` |
| `rules.py` | `/rules`, `/rules/new`, `/rules/{id}` |
| `actions.py` | `/actions`, `/actions/new`, `/actions/{id}` |
| `policies.py` | `/policies`, `/policies/new`, `/policies/{id}` |
| `item_types.py` | `/item-types`, `/item-types/new`, `/item-types/{id}` |
| `mrt.py` | `/mrt`, `/mrt/queues/{id}`, `/mrt/jobs/{id}` |
| `text_banks.py` | `/text-banks`, `/text-banks/{id}` |
| `users.py` | `/users` |
| `api_keys.py` | `/api-keys` |
| `signals.py` | `/signals` |
| `settings.py` | `/settings` |

### Upstream Dependencies
- `nicegui` (UI framework)
- `ui/api` (NestClient, all types)
- `ui/auth` (require_auth, RBAC helpers)
- `ui/components` (layout, starlark_editor)

### Downstream Dependents
- `ui/main` (imports all page modules to register routes)

### Key Implementation Notes
- No page imports from another page. Pages are fully independent.
- `rules.py` is the most complex page (~250 lines) with Starlark editor, template dropdown, test panel, and metadata form.
- `mrt.py` is the second most complex (~160 lines) with queue list, job assignment, and decision form.
- All other pages are 35-90 lines of standard CRUD patterns.
- Error handling follows the inline pattern from NEST_UI.md section 8 (try/except on HTTPStatusError).
- Files: `login.py`, `dashboard.py`, `rules.py`, `actions.py`, `policies.py`, `item_types.py`, `mrt.py`, `text_banks.py`, `users.py`, `api_keys.py`, `signals.py`, `settings.py`
- Estimated size: ~1,105 lines

---

## 18. ui/main

**Purpose**: UI application entry point. Configures NiceGUI, imports all page modules, and starts the server.

### Responsibilities
- Read environment variables (NEST_API_URL, UI_PORT, UI_SECRET)
- Configure NiceGUI app (storage secret, static files)
- Import all page modules (triggers route registration)
- Define root redirect (/ -> /dashboard)
- Start NiceGUI server

### Data Contracts
- Environment variables: `NEST_API_URL` (default: http://localhost:8080), `UI_PORT` (default: 3000), `UI_SECRET`, `DEV` (enables hot reload)

### API Contracts

```python
# main.py -- no public API, this is the entry point

# Start: python main.py
# Or: NEST_API_URL=http://backend:8080 UI_SECRET=<secret> python main.py
```

### Upstream Dependencies
- `nicegui` (framework)
- `ui/pages` (all page modules)

### Downstream Dependents
- None. This is the root.

### Key Implementation Notes
- ~40 lines. No business logic.
- No build step. `python main.py` starts everything.
- Hot reload in dev mode via `DEV=1`.
- File: `main.py`
- Estimated size: ~40 lines

---

## Cross-Module Dependency DAG

```
Go Backend:
                    cmd/server
                   /    |    \
                  /     |     \
            handler   worker   (startup orchestration)
           /  |  \      |
          /   |   \     |
     service  auth  signal
      / | \     |     |
     /  |  \    |     |
  engine store  |   store
    |  \   |    |
    |   \  |    |
  signal store domain
    |      |
  store  domain
    |
  domain


config
  |
domain

Python Frontend:
              ui/main
                |
            ui/pages (all)
           /    |     \
          /     |      \
    ui/api  ui/auth  ui/components
                |         |
              ui/auth   ui/auth
```

No circular dependencies in either system.

### DAG Notes
- **signal -> store**: The signal module has a direct dependency on store because TextBankAdapter requires store.Queries to load bank entries.
- **handler -> signal**: The handler module depends on signal for GET /api/v1/signals (listing adapters) and POST /api/v1/signals/test (testing adapters).
- **config -> domain**: The config module depends on domain for the ConfigError type.
- **handler does NOT depend on store**: Auth middleware closures are constructed in cmd/server and passed to handler.NewRouter as function parameters. Handler accesses auth context helpers (OrgIDFromContext, etc.) from the auth module, but never imports store directly.

---

## Module Invariants

These invariants must hold across all modules:

1. **domain has zero internal imports.** It is the dependency leaf for the Go backend.
2. **No circular dependencies.** The DAG above is enforced by the Go compiler (backend) and by import discipline (frontend).
3. **Every database query includes org_id.** Multi-tenant isolation is enforced at the store layer. Exception: `GetUserByEmail` omits orgID for the login flow (see store section for resolution).
4. **engine does not import service.** Service depends on engine, not the other way around.
5. **handler does not import store.** Auth middleware closures are constructed in cmd/server and passed to handler as function parameters. All data access goes through service or pre-constructed middleware.
6. **No page imports from another page.** UI pages are independent modules.
7. **NestClient is the only module that makes HTTP calls** in the UI. No page constructs raw HTTP requests.
8. **All public Go APIs have context.Context as first parameter** when performing I/O.
9. **Starlark source is the single source of truth** for rule event_types and priority. Database columns are derived values written by the compiler.
10. **Zero sync.Mutex on the evaluation hot path.** Only sync.Map, atomic.Pointer, atomic.Int64, and channels.
11. **All errors are returned, never swallowed.** No `_` on error returns except in deferred Close calls.
12. **CGO is not required.** All Go production dependencies are pure Go.
13. **Two Python production dependencies maximum.** nicegui and httpx. Nothing else.
14. **No business logic in the UI.** Every action calls a Nest REST endpoint. The UI is a thin presentation layer.
