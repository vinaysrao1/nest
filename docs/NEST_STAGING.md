# Nest Staging Document -- Implementation Execution Plan

This document defines the exact order, parallelism strategy, and exit criteria for building every module in the Nest system. It is the authoritative guide for SWE agent teams. Each stage builds on the tested, reviewed foundation of prior stages. No stage begins until all prerequisites are validated.

Reference documents: `NEST_DESIGN.md` (architecture), `NEST_MODULES.md` (module contracts), `NEST_UI.md` (Python frontend).

---

## Master Checklist

- [ ] **Stage 1: Domain Types + Config + SQL Migrations**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 2: Store (PostgreSQL Data Access)**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 3: Auth + Signal (Parallel)**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 4: Engine (Starlark Rule Evaluation)**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 5: Service (Business Logic)**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 6: Worker + Handler (Parallel)**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 7: cmd/server + cmd/migrate + cmd/seed**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 8: Python Frontend -- API Layer + Auth + Components**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off
- [ ] **Stage 9: Python Frontend -- Pages + Entry Point**
  - [ ] Implementation complete
  - [ ] Unit tests passing
  - [ ] Integration tests passing
  - [ ] Review: correctness
  - [ ] Review: scalability
  - [ ] Review: maintainability
  - [ ] Review: invariants respected
  - [ ] Review: no dead/duplicate code
  - [ ] Validation agent sign-off

---

## Stage 1: Domain Types + Config + SQL Migrations

### Prerequisites
None. This is the foundation.

### Modules
- `internal/domain/` -- Pure types, zero imports
- `internal/config/` -- Environment variable parsing
- `migrations/` -- SQL schema DDL (001_initial.sql, 002_partitions.sql)
- Project scaffold: `go.mod`, `go.sum`, `Makefile`, `Dockerfile`, directory structure

### SWE Agent Teams (3 parallel tracks)

**Team 1A: Project Scaffold + Domain Types**
Create the Go module and full directory tree. Implement all domain types.

**Team 1B: Config**
Implement environment variable parsing into typed Config struct.

**Team 1C: SQL Migrations**
Write the complete v1.0 schema DDL and partition creation.

### Implementation Scope

#### Project Scaffold
```
go mod init github.com/<org>/nest
```
Production dependencies in `go.mod`:
- `github.com/go-chi/chi/v5`
- `github.com/jackc/pgx/v5`
- `github.com/riverqueue/river`
- `golang.org/x/crypto`
- `go.starlark.net` (pinned to exact version)

Directory tree:
```
nest/
  cmd/server/
  cmd/migrate/
  cmd/seed/
  internal/config/
  internal/domain/
  internal/store/
  internal/auth/
  internal/signal/
  internal/engine/
  internal/service/
  internal/worker/
  internal/handler/
  migrations/
  rules/examples/
  api/
  nest-ui/
    api/
    auth/
    components/
    pages/
```

#### domain/ (~700 lines, 17 files)

Files and their exports:

**`internal/domain/event.go`**:
```go
type Event struct {
    ID        string         `json:"event_id"`
    EventType string         `json:"event_type"`
    ItemType  string         `json:"item_type"`
    OrgID     string         `json:"org_id"`
    Payload   map[string]any `json:"payload"`
    Timestamp time.Time      `json:"timestamp"`
}
```

**`internal/domain/rule.go`**:
```go
type RuleStatus string
const (
    RuleStatusLive       RuleStatus = "LIVE"
    RuleStatusBackground RuleStatus = "BACKGROUND"
    RuleStatusDisabled   RuleStatus = "DISABLED"
)

type Rule struct {
    ID         string     `json:"id"`
    OrgID      string     `json:"org_id"`
    Name       string     `json:"name"`
    Status     RuleStatus `json:"status"`
    Source     string     `json:"source"`
    EventTypes []string   `json:"event_types"`
    Priority   int        `json:"priority"`
    Tags       []string   `json:"tags"`
    Version    int        `json:"version"`
    CreatedAt  time.Time  `json:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at"`
}
```

**`internal/domain/verdict.go`**:
```go
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
```

**`internal/domain/action.go`**:
```go
type ActionType string
const (
    ActionTypeWebhook      ActionType = "WEBHOOK"
    ActionTypeEnqueueToMRT ActionType = "ENQUEUE_TO_MRT"
)

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

type ActionRequest struct {
    Action        Action
    ItemID        string
    Payload       map[string]any
    CorrelationID string
}

type ActionResult struct {
    ActionID string `json:"action_id"`
    Success  bool   `json:"success"`
    Error    string `json:"error,omitempty"`
}
```

**`internal/domain/policy.go`**:
```go
type Policy struct {
    ID            string    `json:"id"`
    OrgID         string    `json:"org_id"`
    Name          string    `json:"name"`
    Description   string    `json:"description,omitempty"`
    ParentID      *string   `json:"parent_id,omitempty"`
    StrikePenalty int       `json:"strike_penalty"`
    Version       int       `json:"version"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

**`internal/domain/signal.go`**:
```go
type SignalInputType string

type SignalInput struct {
    Type  SignalInputType
    Value string
}

type SignalOutput struct {
    Score    float64        `json:"score"`
    Label    string         `json:"label"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

**`internal/domain/mrt.go`**:
```go
type MRTJobStatus string
const (
    MRTJobStatusPending  MRTJobStatus = "PENDING"
    MRTJobStatusAssigned MRTJobStatus = "ASSIGNED"
    MRTJobStatusDecided  MRTJobStatus = "DECIDED"
)

type MRTQueue struct {
    ID          string    `json:"id"`
    OrgID       string    `json:"org_id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    IsDefault   bool      `json:"is_default"`
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

type MRTDecision struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    JobID     string    `json:"job_id"`
    UserID    string    `json:"user_id"`
    Verdict   string    `json:"verdict"`
    ActionIDs []string  `json:"action_ids"`
    PolicyIDs []string  `json:"policy_ids"`
    Reason    string    `json:"reason,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}
```

**`internal/domain/user.go`**:
```go
type UserRole string
const (
    UserRoleAdmin     UserRole = "ADMIN"
    UserRoleModerator UserRole = "MODERATOR"
    UserRoleAnalyst   UserRole = "ANALYST"
)

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
```

**`internal/domain/org.go`**:
```go
type Org struct {
    ID        string         `json:"id"`
    Name      string         `json:"name"`
    Settings  map[string]any `json:"settings"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}
```

**`internal/domain/item.go`**:
```go
type ItemTypeKind string
const (
    ItemTypeKindContent ItemTypeKind = "CONTENT"
    ItemTypeKindUser    ItemTypeKind = "USER"
    ItemTypeKindThread  ItemTypeKind = "THREAD"
)

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
```

**`internal/domain/auth_types.go`**:
```go
type Session struct {
    SID       string         `json:"sid"`
    UserID    string         `json:"user_id"`
    Data      map[string]any `json:"data"`
    ExpiresAt time.Time      `json:"expires_at"`
}

type ApiKey struct {
    ID        string     `json:"id"`
    OrgID     string     `json:"org_id"`
    Name      string     `json:"name"`
    KeyHash   string     `json:"-"`
    Prefix    string     `json:"prefix"`
    CreatedAt time.Time  `json:"created_at"`
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type SigningKey struct {
    ID         string    `json:"id"`
    OrgID      string    `json:"org_id"`
    PublicKey  string    `json:"public_key"`
    PrivateKey string    `json:"-"`
    IsActive   bool      `json:"is_active"`
    CreatedAt  time.Time `json:"created_at"`
}

type PasswordResetToken struct {
    ID        string     `json:"id"`
    UserID    string     `json:"user_id"`
    TokenHash string     `json:"-"`
    ExpiresAt time.Time  `json:"expires_at"`
    UsedAt    *time.Time `json:"used_at,omitempty"`
    CreatedAt time.Time  `json:"created_at"`
}
```

**`internal/domain/text_bank.go`**:
```go
type TextBank struct {
    ID          string    `json:"id"`
    OrgID       string    `json:"org_id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type TextBankEntry struct {
    ID         string    `json:"id"`
    TextBankID string    `json:"text_bank_id"`
    Value      string    `json:"value"`
    IsRegex    bool      `json:"is_regex"`
    CreatedAt  time.Time `json:"created_at"`
}
```

**`internal/domain/execution.go`**:
```go
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
```

**`internal/domain/history.go`**:
```go
type EntityHistoryEntry struct {
    ID         string         `json:"id"`
    EntityType string         `json:"entity_type"`
    OrgID      string         `json:"org_id"`
    Version    int            `json:"version"`
    Snapshot   map[string]any `json:"snapshot"`
    ValidFrom  time.Time      `json:"valid_from"`
    ValidTo    time.Time      `json:"valid_to"`
}
```

**`internal/domain/errors.go`**:
```go
type NotFoundError struct{ Message string }
func (e *NotFoundError) Error() string { return e.Message }

type ForbiddenError struct{ Message string }
func (e *ForbiddenError) Error() string { return e.Message }

type ConflictError struct{ Message string }
func (e *ConflictError) Error() string { return e.Message }

type ValidationError struct {
    Message string
    Details map[string]string
}
func (e *ValidationError) Error() string { return e.Message }

type ConfigError struct{ Message string }
func (e *ConfigError) Error() string { return e.Message }

type CompileError struct {
    Message  string `json:"message"`
    Line     int    `json:"line,omitempty"`
    Column   int    `json:"column,omitempty"`
    Filename string `json:"filename,omitempty"`
}
func (e *CompileError) Error() string { return e.Message }
```

**`internal/domain/pagination.go`**:
```go
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
```

**`internal/domain/responses.go`**:
```go
type EvalResultResponse struct {
    ItemID         string          `json:"item_id"`
    Verdict        VerdictType     `json:"verdict"`
    TriggeredRules []TriggeredRule `json:"triggered_rules"`
    Actions        []ActionResult  `json:"actions"`
}

type TriggeredRule struct {
    RuleID    string      `json:"rule_id"`
    Version   int         `json:"version"`
    Verdict   VerdictType `json:"verdict"`
    Reason    string      `json:"reason,omitempty"`
    LatencyUs int64       `json:"latency_us"`
}
```

#### config/ (~80 lines, 1 file)

**`internal/config/config.go`**:
```go
type Config struct {
    Port             int
    DatabaseURL      string
    SessionSecret    string
    WorkerCount      int
    RiverWorkerCount int
    RuleTimeout      time.Duration
    EventTimeout     time.Duration
    LogLevel         string
    DevMode          bool
    CounterBackend   string
}

func Load() (*Config, error)
```

#### migrations/ (~250 lines, 2 files)

**`migrations/001_initial.sql`**: All 21 tables, all indexes, all constraints as specified in `NEST_DESIGN.md` section 6. Tables: orgs, users, password_reset_tokens, api_keys, signing_keys, item_types, policies, rules, actions, entity_history, rules_policies, actions_item_types, text_banks, text_bank_entries, mrt_queues, mrt_jobs, mrt_decisions, rule_executions (partitioned), action_executions (partitioned), items, sessions.

**`migrations/002_partitions.sql`**: Create initial monthly partitions for rule_executions and action_executions covering the current month and next 3 months.

### Unit Tests

**domain/**:
- Test all error types implement `error` interface
- Test JSON serialization of all types (verify `json:"-"` on Password, KeyHash, PrivateKey, TokenHash)
- Test VerdictType, RuleStatus, ActionType, UserRole, MRTJobStatus, ItemTypeKind constants have expected string values
- Test PaginatedResult with various generic type parameters
- Test CompileError fields serialize correctly

**config/**:
- Test Load() with all env vars set
- Test Load() with only required vars (verify defaults)
- Test Load() with missing DATABASE_URL returns ConfigError
- Test Load() with invalid PORT returns ConfigError
- Test Load() with each optional var individually

### Integration Tests
- Run `001_initial.sql` and `002_partitions.sql` against a live PostgreSQL instance
- Verify all 21 tables exist
- Verify all indexes exist
- Verify all constraints (CHECK, UNIQUE, FOREIGN KEY) work
- Verify partition creation (insert into current month partition succeeds)
- Verify `rules.event_types` GIN index works with array containment queries
- **(F5a)** Insert a rule_execution with `executed_at` outside all existing partitions -- verify PostgreSQL returns a clear error (no silent data loss)
- **(F5b)** Verify partition maintenance job creates next month's partitions before the current month ends (simulate rollover)

### Invariants to Enforce
- **Invariant 7**: domain has zero imports from other internal packages
- **Invariant 12**: All errors returned, never swallowed (error types implement error interface)
- **Invariant 16**: No JSONB condition trees in schema (rules table has `source TEXT`, not condition_set JSONB)
- **Invariant 17**: CGO not required (`go build` succeeds with `CGO_ENABLED=0`)

### Exit Criteria
1. `go build ./...` succeeds with `CGO_ENABLED=0`
2. `go vet ./...` reports zero issues
3. All domain types have JSON tags
4. `json:"-"` on User.Password, ApiKey.KeyHash, SigningKey.PrivateKey, PasswordResetToken.TokenHash
5. domain/ imports only stdlib (`time`, `encoding/json`)
6. config/ imports only domain/ and stdlib
7. All 21 tables created successfully in PostgreSQL
8. All indexes verified
9. Partitions for execution log tables verified
10. All unit tests pass
11. All integration tests pass
12. `go.starlark.net` pinned to exact version in go.mod
13. **(W3)** Table-driven tests (subtests) used for all multi-case functions
14. **(W6)** `errcheck ./internal/...` passes with zero unhandled errors

### Review Checklist
- [ ] domain/ has zero imports from internal/ packages
- [ ] Every error type has an `Error() string` method
- [ ] JSON tags match the API response format in NEST_DESIGN.md
- [ ] SQL schema matches NEST_DESIGN.md section 6 exactly (21 tables, all constraints)
- [ ] GIN index on rules.event_types present
- [ ] UNIQUE constraint on (org_id, name) for actions present
- [ ] entity_history has composite PK (entity_type, id, version)
- [ ] rule_executions and action_executions are PARTITION BY RANGE (executed_at)
- [ ] Config defaults match spec: Port=8080, WorkerCount=runtime.NumCPU(), RiverWorkerCount=100, RuleTimeout=1s, EventTimeout=5s, LogLevel="info", CounterBackend="memory"
- [ ] No dead code, no duplicate type definitions

---

## Stage 2: Store (PostgreSQL Data Access)

### Prerequisites
- Stage 1 complete and validated (domain types, config, SQL migrations)
- **(W1)** Integration tests use `testcontainers-go` for ephemeral PostgreSQL containers. This is the test database strategy for all stages requiring a live database.

### Modules
- `internal/store/` -- All PostgreSQL data access via pgxpool

### SWE Agent Teams (3 parallel tracks)

**Team 2A: Core Store + Rules + Config Entities**
Files: `db.go`, `rules.go`, `config.go`, `orgs.go`, `history.go`

**Team 2B: Auth + Users + Text Banks**
Files: `auth.go`, `users.go`, `signing_keys.go`, `text_banks.go`

**Team 2C: Items + MRT + Executions + Counters**
Files: `items.go`, `mrt.go`, `executions.go`, `counters.go`

### Implementation Scope

**`internal/store/db.go`**:
```go
type Queries struct {
    pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queries
func (q *Queries) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
```

**`internal/store/rules.go`**:
```go
func (q *Queries) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error)
func (q *Queries) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error)
func (q *Queries) CreateRule(ctx context.Context, rule *domain.Rule) error
func (q *Queries) UpdateRule(ctx context.Context, rule *domain.Rule) error
func (q *Queries) DeleteRule(ctx context.Context, orgID, ruleID string) error
func (q *Queries) ListEnabledRules(ctx context.Context, orgID string) ([]domain.Rule, error)
func (q *Queries) SetRulePolicies(ctx context.Context, ruleID string, policyIDs []string) error
func (q *Queries) GetRulePolicies(ctx context.Context, ruleID string) ([]string, error)
```

**`internal/store/config.go`**:
```go
// Actions
func (q *Queries) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error)
func (q *Queries) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error)
func (q *Queries) GetActionByName(ctx context.Context, orgID, name string) (*domain.Action, error)
func (q *Queries) CreateAction(ctx context.Context, action *domain.Action) error
func (q *Queries) UpdateAction(ctx context.Context, action *domain.Action) error
func (q *Queries) DeleteAction(ctx context.Context, orgID, actionID string) error
func (q *Queries) SetActionItemTypes(ctx context.Context, actionID string, itemTypeIDs []string) error
func (q *Queries) GetActionItemTypes(ctx context.Context, actionID string) ([]string, error)

// Policies
func (q *Queries) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error)
func (q *Queries) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error)
func (q *Queries) CreatePolicy(ctx context.Context, policy *domain.Policy) error
func (q *Queries) UpdatePolicy(ctx context.Context, policy *domain.Policy) error
func (q *Queries) DeletePolicy(ctx context.Context, orgID, policyID string) error

// Item Types
func (q *Queries) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error)
func (q *Queries) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error)
func (q *Queries) CreateItemType(ctx context.Context, itemType *domain.ItemType) error
func (q *Queries) UpdateItemType(ctx context.Context, itemType *domain.ItemType) error
func (q *Queries) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error
```

**`internal/store/auth.go`**:
```go
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
```

**`internal/store/users.go`**:
```go
func (q *Queries) GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
func (q *Queries) GetUserByID(ctx context.Context, orgID, userID string) (*domain.User, error)
func (q *Queries) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error)
func (q *Queries) CreateUser(ctx context.Context, user *domain.User) error
func (q *Queries) UpdateUser(ctx context.Context, user *domain.User) error
func (q *Queries) DeleteUser(ctx context.Context, orgID, userID string) error
```

**`internal/store/signing_keys.go`**:
```go
func (q *Queries) ListSigningKeys(ctx context.Context, orgID string) ([]domain.SigningKey, error)
func (q *Queries) GetActiveSigningKey(ctx context.Context, orgID string) (*domain.SigningKey, error)
func (q *Queries) CreateSigningKey(ctx context.Context, key domain.SigningKey) error
func (q *Queries) DeactivateSigningKeys(ctx context.Context, orgID string) error
```

**`internal/store/text_banks.go`**:
```go
func (q *Queries) ListTextBanks(ctx context.Context, orgID string) ([]domain.TextBank, error)
func (q *Queries) GetTextBank(ctx context.Context, orgID, bankID string) (*domain.TextBank, error)
func (q *Queries) CreateTextBank(ctx context.Context, bank *domain.TextBank) error
func (q *Queries) AddTextBankEntry(ctx context.Context, orgID string, entry *domain.TextBankEntry) error
func (q *Queries) DeleteTextBankEntry(ctx context.Context, orgID, bankID, entryID string) error
func (q *Queries) GetTextBankEntries(ctx context.Context, orgID, bankID string) ([]domain.TextBankEntry, error)
```

**`internal/store/items.go`**:
```go
func (q *Queries) InsertItem(ctx context.Context, orgID string, item domain.Item) error
```

**`internal/store/mrt.go`**:
```go
func (q *Queries) ListMRTQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error)
func (q *Queries) GetMRTQueue(ctx context.Context, orgID, queueID string) (*domain.MRTQueue, error)
func (q *Queries) GetMRTQueueByName(ctx context.Context, orgID, name string) (*domain.MRTQueue, error) // Note: addition to store contract beyond NEST_MODULES.md; required for the enqueue() UDF's queue name resolution
func (q *Queries) CreateMRTQueue(ctx context.Context, queue *domain.MRTQueue) error
func (q *Queries) ListMRTJobs(ctx context.Context, orgID, queueID string, status *string, page domain.PageParams) (*domain.PaginatedResult[domain.MRTJob], error)
func (q *Queries) GetMRTJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error)
func (q *Queries) InsertMRTJob(ctx context.Context, job *domain.MRTJob) error
func (q *Queries) AssignNextMRTJob(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error)
func (q *Queries) InsertMRTDecision(ctx context.Context, decision *domain.MRTDecision) error
func (q *Queries) UpdateMRTJobStatus(ctx context.Context, orgID, jobID string, status domain.MRTJobStatus, assignedTo *string) error
```

**`internal/store/executions.go`**:
```go
func (q *Queries) LogRuleExecutions(ctx context.Context, executions []domain.RuleExecution) error
func (q *Queries) LogActionExecutions(ctx context.Context, executions []domain.ActionExecution) error
```

**`internal/store/orgs.go`**:
```go
func (q *Queries) GetOrg(ctx context.Context, orgID string) (*domain.Org, error)
func (q *Queries) CreateOrg(ctx context.Context, org *domain.Org) error
```

**`internal/store/history.go`**:
```go
func (q *Queries) InsertEntityHistory(ctx context.Context, entityType, id, orgID string, version int, snapshot any) error
func (q *Queries) GetEntityHistory(ctx context.Context, entityType, id string) ([]domain.EntityHistoryEntry, error)
```

**`internal/store/counters.go`**:
```go
func (q *Queries) IncrementCounter(ctx context.Context, orgID, entityID, eventType string, window int, count int64) error
func (q *Queries) GetCounterSum(ctx context.Context, orgID, entityID, eventType string, window int) (int64, error)
```

### Unit Tests
- Test WithTx rollback on error
- Test WithTx commit on success
- Test every CRUD method for each entity type (create, read, update, delete)
- Test pagination (correct total, page boundaries, empty pages)
- Test ListEnabledRules returns only LIVE and BACKGROUND status
- Test GetActionByName with correct org_id scoping
- Test GetUserByEmail without orgID (login flow)
- Test AssignNextMRTJob updates status and assigned_to atomically
- Test SetRulePolicies replaces existing associations
- Test LogRuleExecutions with batch insert
- Test InsertEntityHistory and GetEntityHistory round-trip

### Integration Tests
- Full CRUD cycle for every entity type against live PostgreSQL
- Multi-tenant isolation: create entities in org1, verify they are not visible from org2 queries
- Transaction rollback: verify partial operations are rolled back
- Concurrent AssignNextMRTJob: two goroutines, verify no double-assign
- Entity history: create rule, update rule, verify two history entries with correct version numbers and timestamps
- Execution log insert into partitioned tables
- **(F1)** Table-driven org_id isolation test: call EVERY store query method (GetRule, GetAction, GetPolicy, GetItemType, GetUser, GetMRTJob, GetMRTQueue, GetTextBank, GetSession, GetActiveSigningKey, GetEntityHistory, GetCounterSum, etc.) with a mismatched org_id and assert `NotFoundError` or empty result. This must be a hard exit criterion, not a code review item.
- **(F8)** SQL injection test: attempt SQL injection via rule name (`'; DROP TABLE rules; --`), action config URL, item payload text, and org name. Verify values are stored literally without SQL interpretation. Create each entity with injection payload, read it back, and assert exact round-trip equality.

### Invariants to Enforce
- **Invariant 1**: Every query includes org_id in WHERE clause (except GetUserByEmail for login)
- **Invariant 9**: Entity history is append-only (no updates or deletes to entity_history)
- **Invariant 11**: context.Context is first parameter of all I/O functions
- **Invariant 12**: All errors returned, never swallowed

### Exit Criteria
1. All CRUD methods implemented for all 13 store files
2. Every query includes `org_id` in WHERE (verified by code review)
3. All unit tests pass
4. All integration tests pass (requires PostgreSQL)
5. Multi-tenant isolation test passes (cross-org data not visible)
6. Transaction rollback test passes
7. Batch execution log insert works on partitioned tables
8. `go vet ./internal/store/...` reports zero issues
9. **(F1)** Table-driven org_id isolation integration test passes for ALL store query methods (hard gate, not code review)
10. **(F8)** SQL injection round-trip test passes for rule name, action config, item payload, and org name
11. **(W2)** `go test -race ./internal/store/...` passes
12. **(W3)** Table-driven tests (subtests) used for all multi-case functions
13. **(W6)** `errcheck ./internal/store/...` passes with zero unhandled errors

### Review Checklist
- [ ] Every SQL query uses parameterized queries (no string interpolation)
- [ ] Every query includes org_id except GetUserByEmail
- [ ] WithTx handles rollback correctly on error, commit on success
- [ ] Pagination calculates TotalPages correctly (ceiling division)
- [ ] JSONB fields (Config, Schema, FieldRoles, Settings, Snapshot, Payload, Data, SourceInfo) use pgx JSONB scanning
- [ ] pgx.ErrNoRows mapped to domain.NotFoundError where appropriate
- [ ] No direct SQL string construction -- all queries use constants or prepared strings
- [ ] Batch operations (LogRuleExecutions, LogActionExecutions) use efficient multi-row INSERT

---

## Stage 3: Auth + Signal (Parallel)

### Prerequisites
- Stage 2 complete and validated (store)

### Modules
- `internal/auth/` -- Authentication, authorization, cryptography
- `internal/signal/` -- Signal adapter framework

### SWE Agent Teams (2 parallel tracks)

**Team 3A: Auth** (depends on domain, store)
Files: `context.go`, `passwords.go`, `sessions.go`, `hashing.go`, `rbac.go`, `signing.go`, `middleware.go`

**Team 3B: Signal** (depends on domain; TextBankAdapter depends on store)
Files: `adapter.go`, `registry.go`, `text_regex.go`, `text_bank.go`, `http_signal.go`

> **Design doc deviation note:** The signal package depends on store (for TextBankAdapter). This deviates from the dependency DAG in NEST_DESIGN.md section 14, which shows signal depending only on domain. NEST_MODULES.md is the authoritative source for module dependencies and explicitly specifies the signal->store dependency for TextBankAdapter.

### Implementation Scope

#### auth/ (~500 lines, 7 files)

**`internal/auth/context.go`**:
```go
type AuthContext struct {
    UserID string
    OrgID  string
    Role   domain.UserRole
}

func GetAuthContext(ctx context.Context) *AuthContext
func UserIDFromContext(ctx context.Context) string
func OrgIDFromContext(ctx context.Context) string
func RoleFromContext(ctx context.Context) domain.UserRole
```

**`internal/auth/passwords.go`**:
```go
func HashPassword(password string) (string, error)
func CheckPassword(hash, password string) bool
```

**`internal/auth/sessions.go`**:
```go
func GenerateSessionID() string
```

**`internal/auth/hashing.go`**:
```go
func HashAPIKey(key string) string
func GenerateAPIKey() (key string, prefix string, hash string)
func GenerateToken() (plaintext string, hash string, err error)
```

**`internal/auth/rbac.go`**:
```go
func RequireRole(minRole ...domain.UserRole) func(http.Handler) http.Handler
```

**`internal/auth/signing.go`**:
```go
type Signer struct {
    store *store.Queries
}

func NewSigner(store *store.Queries) *Signer
func (s *Signer) Sign(ctx context.Context, orgID string, payload []byte) (string, error)
```
Implements RSA-PSS signing using the active signing key for the org.

**`internal/auth/middleware.go`**:
```go
func SessionAuth(store *store.Queries) func(http.Handler) http.Handler
func APIKeyAuth(store *store.Queries) func(http.Handler) http.Handler
func CSRFProtect() func(http.Handler) http.Handler
```

#### signal/ (~400 lines, 5 files)

**`internal/signal/adapter.go`**:
```go
type Adapter interface {
    ID() string
    DisplayName() string
    Description() string
    EligibleInputs() []domain.SignalInputType
    Cost() int
    Run(ctx context.Context, input domain.SignalInput) (domain.SignalOutput, error)
}
```

**`internal/signal/registry.go`**:
```go
type Registry struct { /* sync.RWMutex + map[string]Adapter */ }

func NewRegistry() *Registry
func (r *Registry) Register(adapter Adapter)
func (r *Registry) Get(id string) Adapter
func (r *Registry) All() []Adapter
```

**`internal/signal/text_regex.go`**:
```go
type TextRegexAdapter struct{}
// Implements Adapter. Matches input against RE2 regex pattern.
// The pattern is passed in SignalInput.Value as "pattern:text" format
// or via a structured SignalInput type.
```

**`internal/signal/text_bank.go`**:
```go
type TextBankAdapter struct {
    store *store.Queries
}
// Implements Adapter. Matches input text against entries in a named text bank.
// Loads entries from store on each invocation.
```

**`internal/signal/http_signal.go`**:
```go
type HTTPSignalAdapter struct {
    id          string
    displayName string
    description string
    url         string
    headers     map[string]string
    httpClient  *http.Client
}
// Implements Adapter. Generic HTTP signal for external APIs.
```

### Unit Tests

**auth/**:
- Test HashPassword and CheckPassword round-trip
- Test HashPassword produces different hashes for same input (bcrypt salt)
- Test CheckPassword returns false for wrong password
- Test GenerateSessionID produces unique, non-empty strings
- Test GenerateAPIKey produces key, prefix, hash; hash matches HashAPIKey(key); prefix is first 8 chars of key
- Test GenerateToken produces plaintext and matching hash
- Test HashAPIKey is deterministic (same input -> same hash)
- Test RequireRole middleware returns 403 for insufficient role
- Test RequireRole middleware passes for sufficient role
- Test AuthContext round-trip: set context values, read them back
- Test SessionAuth middleware with valid session cookie
- Test SessionAuth middleware returns 401 with invalid/expired session
- Test APIKeyAuth middleware with valid X-API-Key header
- Test APIKeyAuth middleware returns 401 with invalid/revoked key
- Test CSRFProtect passes GET requests, blocks POST without token
- Test Signer.Sign produces valid RSA-PSS signature that can be verified

**signal/**:
- Test Registry Register and Get round-trip
- Test Registry Get returns nil for unknown signal
- Test Registry All returns all registered adapters
- Test TextRegexAdapter.Run with matching pattern returns score 1.0
- Test TextRegexAdapter.Run with non-matching pattern returns score 0.0
- Test TextRegexAdapter.Run with invalid regex returns error
- Test TextBankAdapter.Run with matching entry returns score 1.0
- Test TextBankAdapter.Run with regex entry matching
- Test TextBankAdapter.Run with no matches returns score 0.0
- Test HTTPSignalAdapter.Run makes correct HTTP request (use httptest.Server)
- Test HTTPSignalAdapter.Run handles timeout gracefully
- Test HTTPSignalAdapter.Run handles non-200 response

### Integration Tests

**auth + store:**
- Create user, hash password, create session, validate session via SessionAuth middleware
- Create API key via store, verify via APIKeyAuth middleware
- Create signing key, sign payload, verify signature with public key
- Session expiry: create session with past expiry, verify SessionAuth returns 401

**signal + store:**
- Create text bank with entries, run TextBankAdapter, verify correct matching
- TextBankAdapter multi-tenant: bank in org1 not accessible when querying with org2

### Invariants to Enforce
- **Invariant 2**: No circular dependencies (auth imports store, not vice versa; signal imports store for TextBank only)
- **Invariant 10**: API keys never stored in plaintext -- only SHA-256 hashes
- **Invariant 11**: context.Context first parameter on I/O functions
- **Invariant 12**: All errors returned

### Exit Criteria
1. All auth middleware functions implemented and tested
2. All signal adapters implement the Adapter interface
3. API key flow: generate -> hash -> store hash -> lookup by hash works end-to-end
4. RSA-PSS signing and verification works
5. TextBankAdapter queries store correctly with org_id isolation
6. Registry is thread-safe (verified by concurrent test)
7. All unit tests pass
8. All integration tests pass
9. `go vet ./internal/auth/... ./internal/signal/...` reports zero issues
10. **(W2)** `go test -race ./internal/auth/... ./internal/signal/...` passes
11. **(W3)** Table-driven tests (subtests) used for all multi-case functions
12. **(W6)** `errcheck ./internal/auth/... ./internal/signal/...` passes with zero unhandled errors

### Review Checklist
- [ ] API keys: plaintext key never persisted anywhere after initial return to caller
- [ ] bcrypt cost factor is appropriate (default 10 or higher)
- [ ] RSA key size is 2048 bits
- [ ] SessionAuth reads cookie named "session" (or configurable name)
- [ ] APIKeyAuth reads header "X-API-Key"
- [ ] RequireRole supports checking against multiple acceptable roles
- [ ] Signal Registry uses sync.RWMutex; lock only on Register, RLock on Get/All
- [ ] TextBankAdapter joins through text_banks table to verify org ownership
- [ ] HTTPSignalAdapter has configurable timeout
- [ ] No secrets logged (password hashes, API keys, signing key material)

---

## Stage 4: Engine (Starlark Rule Evaluation)

### Prerequisites
- Stage 3 complete and validated (auth, signal)

### Modules
- `internal/engine/` -- Compiler, Snapshot, Worker, Pool, UDFs, ActionPublisher, Cache

### SWE Agent Teams (3 parallel tracks)

**Team 4A: Compiler + Snapshot**
Files: `compiler.go`, `snapshot.go`

**Team 4B: UDFs**
Files: `udf.go`, `udf_signal.go`, `udf_counter.go`, `udf_enqueue.go`

**Team 4C: Worker + Pool + ActionPublisher + Cache**
Files: `worker.go`, `pool.go`, `action_publisher.go`, `cache.go`

Note: Team 4A must complete before 4C can fully integrate. Teams 4A and 4B can run in parallel. Team 4C starts with the Pool/Worker skeleton and integrates Compiler+Snapshot+UDFs as they become available.

### Implementation Scope

**`internal/engine/compiler.go`**:
```go
type Compiler struct{}

type CompiledRule struct {
    ID         string
    EventTypes []string
    Priority   int
    Program    *starlark.Program
    Source     string
}

// CompileRule parses Starlark source, extracts metadata, validates.
// Returns domain.CompileError on:
//   - Invalid Starlark syntax
//   - Missing rule_id, event_types, or priority globals
//   - Missing evaluate(event) function
//   - event_types contains "*" mixed with other types
func (c *Compiler) CompileRule(source string, filename string) (*CompiledRule, error)
```

**`internal/engine/snapshot.go`**:
```go
type Snapshot struct {
    ID       string
    OrgID    string
    Rules    []*CompiledRule
    ByEvent  map[string][]*CompiledRule
    LoadedAt time.Time
}

func NewSnapshot(orgID string, rules []*CompiledRule) *Snapshot
func (s *Snapshot) RulesForEvent(eventType string) []*CompiledRule
```

**`internal/engine/udf.go`**:
```go
func BuildUDFs(w *Worker) starlark.StringDict
```
Registers: verdict(), log(), now(), hash(), regex_match(), memo()

**`internal/engine/udf_signal.go`**:
```go
// signalUDF bridges Starlark to signal.Registry
// Results cached per-event via sync.Map on the worker's current evaluation context
func signalUDF(w *Worker) *starlark.Builtin
```

**`internal/engine/udf_counter.go`**:
```go
// counterUDF implements counter(entity_id, event_type, window_seconds)
// Uses per-worker atomic.Int64 with time-bucketed keys
// CounterSum aggregates across all workers
```

**`internal/engine/udf_enqueue.go`**:
> **Design doc deviation note:** This file is an addition beyond the directory listing in NEST_DESIGN.md. It is required by the modules doc's UDF specification, which defines the `enqueue(queue_name, reason)` Starlark built-in for creating MRT jobs from within rules.

```go
// enqueueUDF implements enqueue(queue_name, reason)
// Resolves queue name to queue ID via store (cached)
// Inserts MRT job via store
```

**`internal/engine/worker.go`**:
```go
type Worker struct {
    id        int
    thread    *starlark.Thread
    memo      map[string]starlark.Value
    counters  map[counterKey]*atomic.Int64
    evalCache map[string]starlark.Callable
    lastSnap  string
    pool      *Pool
    logger    *slog.Logger
}

func (w *Worker) processEvent(ctx context.Context, event domain.Event) EvalResult
```

**`internal/engine/pool.go`**:
```go
type EvalRequest struct {
    Event domain.Event
    Ctx   context.Context
}

type EvalResult struct {
    Verdict        domain.Verdict
    TriggeredRules []domain.TriggeredRule
    ActionRequests []domain.ActionRequest
    Logs           []string
    LatencyUs      int64
    CorrelationID  string
}

type Pool struct {
    workers   []*Worker
    snapshots sync.Map  // map[string]*atomic.Pointer[Snapshot]
    registry  *signal.Registry
    store     *store.Queries
    logger    *slog.Logger
    eventCh   chan EvalRequest
    resultCh  chan EvalResult
}

func NewPool(workerCount int, registry *signal.Registry, store *store.Queries, logger *slog.Logger) *Pool
func (p *Pool) Evaluate(ctx context.Context, event domain.Event) (*EvalResult, error)
func (p *Pool) SwapSnapshot(orgID string, snap *Snapshot)
func (p *Pool) CounterSum(orgID, entityID, eventType string, windowSeconds int) int64
func (p *Pool) Stop()

// Internal: verdict resolution
func resolveVerdict(results []ruleResult) domain.Verdict
```

**`internal/engine/action_publisher.go`**:
```go
type Signer interface {
    Sign(ctx context.Context, orgID string, payload []byte) (string, error)
}

type ActionPublisher struct {
    store      *store.Queries
    signer     Signer
    httpClient *http.Client
    logger     *slog.Logger
}

func NewActionPublisher(store *store.Queries, signer Signer, httpClient *http.Client, logger *slog.Logger) *ActionPublisher
func (p *ActionPublisher) PublishActions(ctx context.Context, actions []domain.ActionRequest, target ActionTarget) []domain.ActionResult

type ActionTarget struct {
    ItemID        string
    ItemTypeID    string
    OrgID         string
    Payload       map[string]any
    CorrelationID string
}
```

**`internal/engine/cache.go`**:
```go
type Cache struct { /* sync.RWMutex + map + TTL */ }
func NewCache(ttl time.Duration) *Cache
func (c *Cache) Get(key string) (any, bool)
func (c *Cache) Set(key string, value any)
```

### Unit Tests

**compiler.go:**
- Test CompileRule with valid Starlark source extracts rule_id, event_types, priority
- Test CompileRule with event_types=["*"] succeeds
- Test CompileRule with event_types=["*", "content"] returns CompileError
- Test CompileRule with missing rule_id returns CompileError
- Test CompileRule with missing event_types returns CompileError
- Test CompileRule with missing priority returns CompileError
- Test CompileRule with missing evaluate function returns CompileError
- Test CompileRule with syntax error returns CompileError with line/column
- Test CompileRule compiled program is reusable (run evaluate twice)

**snapshot.go:**
- Test NewSnapshot indexes rules by event type
- Test NewSnapshot stores wildcard rules under "*" key
- Test RulesForEvent returns event-specific + wildcard rules merged and sorted by priority desc
- Test RulesForEvent with unknown event type returns only wildcard rules
- Test RulesForEvent with no matching rules returns empty slice
- Test snapshot is immutable (modifying returned slice does not affect snapshot)

**pool.go:**
- Test NewPool creates specified number of workers
- Test SwapSnapshot for new org creates entry in sync.Map
- Test SwapSnapshot for existing org atomically replaces snapshot
- Test concurrent SwapSnapshot for same new org (no race, no panic)
- Test Evaluate with matching rules returns correct verdict
- Test Evaluate with no matching rules returns approve default
- Test resolveVerdict: highest priority wins
- Test resolveVerdict: tie broken by verdict weight (block > review > approve)
- Test resolveVerdict: no verdicts returns approve
- Test CounterSum aggregates across workers
- Test Pool.Stop drains channels and stops workers

**worker.go:**
- Test processEvent clears memo between events
- Test processEvent uses eval cache for repeated rule evaluations on same snapshot
- Test processEvent invalidates eval cache when snapshot changes
- **(F3)** Table-driven Starlark panic recovery tests (each is an explicit subtest):
  - (a) Starlark infinite loop interrupted by 1s per-rule timeout -- verify rule returns error, not hang
  - (b) Go panic inside a UDF (e.g., signal adapter panics) -- verify panic is recovered, rule returns error
  - (c) Nil dereference in counter UDF -- verify panic is recovered, rule returns error
  - (d) Pool continues processing the NEXT event after a panic is recovered -- verify next event evaluates correctly
- Test per-rule timeout (1s) cancels long-running Starlark
- **(F4)** Per-event timeout (5s) test: submit an event where aggregate rule evaluation exceeds 5 seconds (e.g., 10 rules each sleeping ~600ms). Verify evaluation returns a timeout error within ~5s, not a hang.

**UDF tests:**
- Test verdict() returns correct Starlark struct with type, reason, actions
- Test signal() calls correct adapter via registry and returns score/label/metadata
- Test signal() caches results within single event (same signal+input called twice, adapter.Run called once)
- Test counter() increments and reads correctly across calls
- Test counter() respects time window (expired buckets not counted)
- **(W8)** Counter from expired time window is not included in CounterSum: increment counter, advance time past window, verify CounterSum returns 0
- Test memo() caches within single event
- Test memo() cleared between events
- Test log() appends to evaluation logs
- Test now() returns current Unix timestamp
- Test hash() returns SHA-256 hex string
- Test regex_match() returns true/false correctly
- Test enqueue() inserts MRT job (mock store)
- Test enqueue() returns false for unknown queue name

**action_publisher.go:**
- Test PublishActions with WEBHOOK action sends signed HTTP POST
- Test PublishActions with ENQUEUE_TO_MRT action inserts MRT job
- Test PublishActions handles webhook failure (returns Success=false, no panic)
- Test PublishActions handles multiple actions concurrently
- **(F6)** Table-driven webhook failure tests (each is an explicit subtest):
  - (a) Webhook endpoint returns 500 -- verify `ActionResult{Success: false}`
  - (b) Webhook endpoint times out -- verify `ActionResult{Success: false}`
  - (c) Webhook endpoint returns non-2xx (e.g., 301, 403, 422) -- verify `ActionResult{Success: false}`
  - (d) RSA-PSS signing fails (no active signing key) -- verify `ActionResult{Success: false, Error: "..."}`

**cache.go:**
- Test Set and Get round-trip
- Test Get returns false after TTL expiry
- Test concurrent Set and Get (no race)

### Integration Tests
- Compile a rule from source, build snapshot, evaluate event, verify correct verdict
- Multi-rule evaluation: 3 rules at different priorities, verify highest priority block wins
- Wildcard rule: rule with event_types=["*"] evaluates for any event type
- Signal UDF integration: register TextRegexAdapter, compile rule that calls signal(), verify result
- Counter integration: evaluate event twice with counter(), verify count increments
- Action publisher: set up httptest.Server, publish WEBHOOK action, verify signed request received
- Full pipeline: compile rules -> build snapshot -> swap -> evaluate -> resolve verdict -> action requests
- Org isolation: create rules for org1 and org2 with different snapshots, evaluate event for org1, verify org2's rules are not evaluated (org1's snapshot does not contain org2's rules)

### Invariants to Enforce
- **Invariant 3**: Zero sync.Mutex on evaluation hot path (only sync.Map.Load, atomic.Pointer.Load, atomic.Int64, channels)
- **Invariant 4**: Starlark rule evaluation never panics (errors recovered per-rule)
- **Invariant 5**: All rules compile before activation (CompileError at compile time)
- **Invariant 6**: Snapshot swap is atomic (atomic.Pointer)
- **Invariant 13**: Signal results cached within single event
- **Invariant 14**: In-memory counters eventually consistent
- **Invariant 15**: Starlark source is single source of truth (event_types, priority extracted by compiler)

### Exit Criteria
1. Compiler extracts metadata correctly from valid Starlark
2. Compiler rejects invalid Starlark with descriptive CompileError
3. Wildcard event_types=["*"] works; mixing wildcard with specific types fails compilation
4. Snapshot is immutable and correctly indexed
5. Pool manages workers, channels, and snapshots without data races (`go test -race`)
6. All UDFs work from within Starlark evaluation
7. Signal caching within single event verified (adapter.Run call count)
8. Counter cross-worker summing verified
9. Verdict resolution correct for priority and tie-breaking
10. Action publisher handles both action types and failures gracefully
11. Zero sync.Mutex in pool.go, worker.go, snapshot.go (verified by code review)
12. All unit tests pass
13. All integration tests pass
14. `go test -race ./internal/engine/...` passes
15. **(F2)** CI gate grep check passes: `grep -rn "sync.Mutex" internal/engine/pool.go internal/engine/worker.go internal/engine/snapshot.go` returns zero results
16. **(F3)** Table-driven Starlark panic recovery test passes for all 4 scenarios (infinite loop, Go panic in UDF, nil deref, pool continues after panic)
17. **(F4)** Per-event 5s timeout test passes (10 rules x ~600ms returns timeout, not hang)
18. **(W3)** Table-driven tests (subtests) used for all multi-case functions
19. **(W4)** Starlark conformance test suite: a set of `.star` files with expected outputs that must pass. This suite gates any future `go.starlark.net` upgrades.
20. **(W5)** `BenchmarkEvaluate` with 10 compiled rules records p50/p99 latency. Baseline must be documented for regression detection.
21. **(W6)** `errcheck ./internal/engine/...` passes with zero unhandled errors
22. **(F6)** Table-driven webhook failure test passes for all 4 subtests: HTTP 500 response, timeout, non-2xx response, RSA-PSS signing failure

### Review Checklist
- [ ] No sync.Mutex in pool.go, worker.go, or snapshot.go
- [ ] sync.Map used for Pool.snapshots (not plain map)
- [ ] atomic.Pointer[Snapshot] for per-org snapshot swap
- [ ] atomic.Int64 for per-worker counters
- [ ] SwapSnapshot uses LoadOrStore pattern (handles concurrent first-access)
- [ ] Worker clears memo map between events
- [ ] Worker invalidates evalCache when snapshot ID changes
- [ ] Per-rule timeout via context.WithTimeout
- [ ] Per-event timeout via context.WithTimeout
- [ ] Starlark thread cancellation on context cancellation
- [ ] defer/recover around Starlark evaluation to catch panics
- [ ] Signal results cached in sync.Map per evaluation context
- [ ] enqueue() UDF caches queue name -> ID resolution
- [ ] ActionPublisher.PublishActions never returns error (individual failures in ActionResult)
- [ ] Cache uses sync.RWMutex (not on hot path -- for action name resolution)

---

## Stage 5: Service (Business Logic)

### Prerequisites
- Stage 4 complete and validated (engine)

### Modules
- `internal/service/` -- Business logic orchestration

### SWE Agent Teams (3 parallel tracks)

**Team 5A: RuleService + ConfigService**
Files: `rules.go`, `config.go`

**Team 5B: ItemService + MRTService**
Files: `items.go`, `mrt.go`

**Team 5C: UserService + APIKeyService + SigningKeyService + TextBankService**
Files: `users.go`, `api_keys.go`, `signing_keys.go`, `text_banks.go`

### Implementation Scope

**`internal/service/rules.go`**:
```go
type RuleService struct {
    store    *store.Queries
    compiler *engine.Compiler
    pool     *engine.Pool
    logger   *slog.Logger
}

func NewRuleService(store *store.Queries, compiler *engine.Compiler, pool *engine.Pool, logger *slog.Logger) *RuleService

type CreateRuleParams struct {
    Name      string
    Status    domain.RuleStatus
    Source    string
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

func (s *RuleService) CreateRule(ctx context.Context, orgID string, params CreateRuleParams) (*domain.Rule, error)
func (s *RuleService) UpdateRule(ctx context.Context, orgID, ruleID string, params UpdateRuleParams) (*domain.Rule, error)
func (s *RuleService) DeleteRule(ctx context.Context, orgID, ruleID string) error
func (s *RuleService) GetRule(ctx context.Context, orgID, ruleID string) (*domain.Rule, error)
func (s *RuleService) ListRules(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Rule], error)
func (s *RuleService) TestRule(ctx context.Context, orgID string, source string, event domain.Event) (*TestResult, error)
func (s *RuleService) TestExistingRule(ctx context.Context, orgID, ruleID string, event domain.Event) (*TestResult, error)
func (s *RuleService) RebuildSnapshot(ctx context.Context, orgID string) error
```

CreateRule flow:
1. Compile Starlark source via compiler.CompileRule
2. Extract event_types and priority from compiled metadata
3. Persist rule to store with derived event_types and priority
4. Set rule-policy associations via store.SetRulePolicies
5. Write current version to entity_history
6. Rebuild snapshot: fetch all enabled rules, compile all, build new Snapshot, SwapSnapshot

UpdateRule flow:
1. Get existing rule
2. If source changed: compile new source, extract metadata
3. Write old version to entity_history
4. Update rule in store with new derived columns
5. If policy_ids changed: update rule-policy associations
6. Rebuild snapshot

**`internal/service/config.go`**:
```go
type ConfigService struct {
    store  *store.Queries
    logger *slog.Logger
}

type CreateActionParams struct { Name string; ActionType domain.ActionType; Config map[string]any; ItemTypeIDs []string }
type UpdateActionParams struct { Name *string; ActionType *domain.ActionType; Config *map[string]any; ItemTypeIDs *[]string }
type CreatePolicyParams struct { Name string; Description *string; ParentID *string; StrikePenalty int }
type UpdatePolicyParams struct { Name *string; Description *string; ParentID *string; StrikePenalty *int }
type CreateItemTypeParams struct { Name string; Kind domain.ItemTypeKind; Schema map[string]any; FieldRoles map[string]any }
type UpdateItemTypeParams struct { Name *string; Kind *domain.ItemTypeKind; Schema *map[string]any; FieldRoles *map[string]any }

func NewConfigService(store *store.Queries, logger *slog.Logger) *ConfigService

// Actions CRUD (with entity history on create/update)
func (s *ConfigService) CreateAction(ctx context.Context, orgID string, params CreateActionParams) (*domain.Action, error)
func (s *ConfigService) UpdateAction(ctx context.Context, orgID, actionID string, params UpdateActionParams) (*domain.Action, error)
func (s *ConfigService) DeleteAction(ctx context.Context, orgID, actionID string) error
func (s *ConfigService) GetAction(ctx context.Context, orgID, actionID string) (*domain.Action, error)
func (s *ConfigService) ListActions(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Action], error)

// Policies CRUD
func (s *ConfigService) CreatePolicy(ctx context.Context, orgID string, params CreatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) UpdatePolicy(ctx context.Context, orgID, policyID string, params UpdatePolicyParams) (*domain.Policy, error)
func (s *ConfigService) DeletePolicy(ctx context.Context, orgID, policyID string) error
func (s *ConfigService) GetPolicy(ctx context.Context, orgID, policyID string) (*domain.Policy, error)
func (s *ConfigService) ListPolicies(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.Policy], error)

// Item Types CRUD
func (s *ConfigService) CreateItemType(ctx context.Context, orgID string, params CreateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) UpdateItemType(ctx context.Context, orgID, itemTypeID string, params UpdateItemTypeParams) (*domain.ItemType, error)
func (s *ConfigService) DeleteItemType(ctx context.Context, orgID, itemTypeID string) error
func (s *ConfigService) GetItemType(ctx context.Context, orgID, itemTypeID string) (*domain.ItemType, error)
func (s *ConfigService) ListItemTypes(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.ItemType], error)
```

**`internal/service/items.go`**:
```go
type ItemService struct {
    store     *store.Queries
    pool      *engine.Pool
    publisher *engine.ActionPublisher
    logger    *slog.Logger
}

type SubmitItemParams struct {
    ItemID        string
    ItemTypeID    string
    OrgID         string
    Payload       map[string]any
    CreatorID     string
    CreatorTypeID string
}

func NewItemService(store *store.Queries, pool *engine.Pool, publisher *engine.ActionPublisher, logger *slog.Logger) *ItemService
func (s *ItemService) SubmitSync(ctx context.Context, orgID string, items []SubmitItemParams) ([]domain.EvalResultResponse, error)
func (s *ItemService) SubmitAsync(ctx context.Context, orgID string, items []SubmitItemParams) ([]string, error)
```

SubmitSync flow per item:
1. Look up item type, validate payload against schema
2. Store item in items table
3. Build domain.Event from item
4. pool.Evaluate(ctx, event) -> EvalResult
5. publisher.PublishActions(ctx, result.ActionRequests, target)
6. Log rule and action executions
7. Build EvalResultResponse

**`internal/service/mrt.go`**:
```go
type MRTService struct {
    store  *store.Queries
    logger *slog.Logger
}

type EnqueueParams struct {
    OrgID string; QueueName string; ItemID string; ItemTypeID string
    Payload map[string]any; EnqueueSource string; SourceInfo map[string]any; PolicyIDs []string
}

type DecisionParams struct {
    OrgID string; JobID string; UserID string; Verdict string
    ActionIDs []string; PolicyIDs []string; Reason string
}

type DecisionResult struct {
    Decision       domain.MRTDecision
    ActionRequests []domain.ActionRequest
}

func NewMRTService(store *store.Queries, logger *slog.Logger) *MRTService
func (s *MRTService) Enqueue(ctx context.Context, params EnqueueParams) (string, error)
func (s *MRTService) AssignNext(ctx context.Context, orgID, queueID, userID string) (*domain.MRTJob, error)
func (s *MRTService) RecordDecision(ctx context.Context, params DecisionParams) (*DecisionResult, error)
func (s *MRTService) ListQueues(ctx context.Context, orgID string) ([]domain.MRTQueue, error)
func (s *MRTService) ListJobs(ctx context.Context, orgID, queueID string, status *string, page domain.PageParams) (*domain.PaginatedResult[domain.MRTJob], error)
func (s *MRTService) GetJob(ctx context.Context, orgID, jobID string) (*domain.MRTJob, error)
```

RecordDecision flow:
1. Get job, verify it is in ASSIGNED status and assigned to the user
2. Insert MRT decision
3. Update job status to DECIDED
4. Resolve action_ids to Action definitions via store
5. Build ActionRequests and return in DecisionResult (handler executes them)

**`internal/service/users.go`**:
```go
type UserService struct { store *store.Queries; logger *slog.Logger }
func NewUserService(store *store.Queries, logger *slog.Logger) *UserService
func (s *UserService) ListUsers(ctx context.Context, orgID string, page domain.PageParams) (*domain.PaginatedResult[domain.User], error)
func (s *UserService) InviteUser(ctx context.Context, orgID string, email, name string, role domain.UserRole) (*domain.User, error)
func (s *UserService) UpdateUser(ctx context.Context, orgID, userID string, params UserUpdateParams) (*domain.User, error)
func (s *UserService) DeactivateUser(ctx context.Context, orgID, userID string) error
func (s *UserService) RequestPasswordReset(ctx context.Context, email string) error
func (s *UserService) ResetPassword(ctx context.Context, token, newPassword string) error

type UserUpdateParams struct { Name *string; Role *domain.UserRole; IsActive *bool }
```

**`internal/service/api_keys.go`**:
```go
type APIKeyService struct { store *store.Queries; logger *slog.Logger }
func NewAPIKeyService(store *store.Queries, logger *slog.Logger) *APIKeyService
func (s *APIKeyService) Create(ctx context.Context, orgID, name string) (key string, apiKey *domain.ApiKey, err error)
func (s *APIKeyService) List(ctx context.Context, orgID string) ([]domain.ApiKey, error)
func (s *APIKeyService) Revoke(ctx context.Context, orgID, keyID string) error
```

**`internal/service/signing_keys.go`**:
```go
type SigningKeyService struct { store *store.Queries; logger *slog.Logger }
func NewSigningKeyService(store *store.Queries, logger *slog.Logger) *SigningKeyService
func (s *SigningKeyService) List(ctx context.Context, orgID string) ([]domain.SigningKey, error)
func (s *SigningKeyService) Rotate(ctx context.Context, orgID string) (*domain.SigningKey, error)
```

**`internal/service/text_banks.go`**:
```go
type TextBankService struct { store *store.Queries; logger *slog.Logger }
func NewTextBankService(store *store.Queries, logger *slog.Logger) *TextBankService
func (s *TextBankService) List(ctx context.Context, orgID string) ([]domain.TextBank, error)
func (s *TextBankService) Get(ctx context.Context, orgID, bankID string) (*domain.TextBank, error)
func (s *TextBankService) Create(ctx context.Context, orgID, name, description string) (*domain.TextBank, error)
func (s *TextBankService) AddEntry(ctx context.Context, orgID, bankID, value string, isRegex bool) (*domain.TextBankEntry, error)
func (s *TextBankService) DeleteEntry(ctx context.Context, orgID, bankID, entryID string) error
```

### Unit Tests

**rules.go:**
- Test CreateRule with valid Starlark: rule persisted, event_types and priority derived from source, snapshot rebuilt
- Test CreateRule with invalid Starlark: returns ValidationError, nothing persisted
- Test CreateRule with wildcard event_types=["*"]: succeeds
- Test CreateRule with mixed wildcard event_types=["*", "x"]: returns ValidationError
- Test UpdateRule: old version written to entity_history, new version persisted, snapshot rebuilt
- Test UpdateRule source change: metadata re-extracted
- Test DeleteRule: rule removed, snapshot rebuilt
- Test TestRule: compiles and evaluates without persisting (verify no store writes)
- Test RebuildSnapshot: fetches all enabled rules, compiles, swaps snapshot

**config.go:**
- Test CRUD for each entity type (action, policy, item type)
- Test entity history written on create and update
- Test action-item-type associations on create and update

**items.go:**
- Test SubmitSync with matching rules returns correct verdict
- Test SubmitSync stores item in items table
- Test SubmitSync with invalid item type returns error
- Test SubmitAsync enqueues river job (mock river client)

**mrt.go:**
- Test Enqueue creates job in correct queue
- Test AssignNext returns pending job and updates status to ASSIGNED
- Test AssignNext with empty queue returns nil
- Test RecordDecision: inserts decision, updates job status, returns ActionRequests
- Test RecordDecision with wrong user (not assigned) returns error

**users.go, api_keys.go, signing_keys.go, text_banks.go:**
- Standard CRUD tests for each service
- Test InviteUser hashes password
- Test APIKeyService.Create returns plaintext key exactly once
- Test SigningKeyService.Rotate deactivates old keys and creates new one

### Integration Tests
- Full rule lifecycle: create rule -> get rule -> update rule -> verify entity history -> delete rule -> verify snapshot rebuilt
- Item submission end-to-end: create item type, create rule, submit item, verify verdict returned with correct actions
- MRT end-to-end: submit item with rule that calls enqueue() -> verify MRT job created -> assign job -> record decision -> verify action requests returned
- Config entity history: create action, update action, query history, verify two entries with correct versions

### Invariants to Enforce
- **Invariant 5**: All rules compile before activation (CreateRule validates before persisting)
- **Invariant 8**: ActionPublisher and MRTService have no direct dependency (handler orchestrates)
- **Invariant 9**: Entity history append-only
- **Invariant 15**: Starlark source is single source of truth (derived columns written by compiler)

### Exit Criteria
1. All 8 service files implemented with full method signatures
2. RuleService correctly derives event_types and priority from Starlark source
3. RuleService rebuilds snapshot on every rule CRUD operation
4. Entity history written on every create/update of versioned entities
5. MRTService.RecordDecision returns ActionRequests (does not execute them)
6. ItemService.SubmitSync runs full evaluation pipeline
7. All unit tests pass
8. All integration tests pass
9. `go test -race ./internal/service/...` passes
10. **(W3)** Table-driven tests (subtests) used for all multi-case functions
11. **(W6)** `errcheck ./internal/service/...` passes with zero unhandled errors

### Review Checklist
- [ ] RuleService.CreateRule compiles source BEFORE persisting (fail fast)
- [ ] RuleService extracts event_types and priority from compiler, not from request params
- [ ] Entity history written within same transaction as entity update
- [ ] MRTService.RecordDecision does NOT call ActionPublisher (invariant 8)
- [ ] ItemService.SubmitSync logs both rule and action executions
- [ ] UserService.InviteUser hashes password via auth.HashPassword
- [ ] APIKeyService.Create uses auth.GenerateAPIKey, stores only hash
- [ ] No service imports another service (services are independent)

---

## Stage 6: Worker + Handler (Parallel)

### Prerequisites
- Stage 5 complete and validated (service)

### Modules
- `internal/worker/` -- Background job processing via river
- `internal/handler/` -- HTTP handlers and chi router

### SWE Agent Teams (2 parallel tracks)

**Team 6A: Worker**
Files: `process_item.go`, `maintenance.go`

**Team 6B: Handler**
Files: `router.go`, `rules.go`, `config.go`, `items.go`, `mrt.go`, `users.go`, `auth.go`, `orgs.go`, `api_keys.go`, `signing_keys.go`, `signals.go`, `text_banks.go`, `health.go`, `helpers.go`

### Implementation Scope

#### worker/ (~200 lines, 2 files)

**`internal/worker/process_item.go`**:
```go
type ProcessItemArgs struct {
    OrgID      string         `json:"org_id"`
    ItemID     string         `json:"item_id"`
    ItemTypeID string         `json:"item_type_id"`
    EventType  string         `json:"event_type"`
    Payload    map[string]any `json:"payload"`
}

type ProcessItemWorker struct {
    pool      *engine.Pool
    publisher *engine.ActionPublisher
    store     *store.Queries
    logger    *slog.Logger
}

func NewProcessItemWorker(pool *engine.Pool, publisher *engine.ActionPublisher, store *store.Queries, logger *slog.Logger) *ProcessItemWorker
func (w *ProcessItemWorker) Work(ctx context.Context, job *river.Job[ProcessItemArgs]) error
```

**`internal/worker/maintenance.go`**:
```go
func RegisterMaintenanceJobs(
    client *river.Client[pgx.Tx],
    ruleService *service.RuleService,
    store *store.Queries,
    pool *engine.Pool,
    logger *slog.Logger,
)
```
Registers periodic jobs:
- Snapshot rebuild (every 5 minutes -- catch missed CRUD events)
- Partition manager (daily -- create next month's partitions)
- Session cleanup (hourly -- delete expired sessions)
- Counter flush (every 30 seconds -- optional, if CounterBackend="postgres")

#### handler/ (~1,200 lines, 14 files)

**`internal/handler/helpers.go`**:
```go
type ErrorResponse struct {
    Error   string            `json:"error"`
    Details map[string]string `json:"details,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any)
func Decode(r *http.Request, v any) error
func Error(w http.ResponseWriter, status int, msg string)
func OrgID(r *http.Request) string
func UserID(r *http.Request) string
func PageParamsFromRequest(r *http.Request) domain.PageParams
```

**`internal/handler/` route handler files**:
Each file contains handler functions for one resource group. All handlers follow the pattern: parse request -> call service -> write response.

**`internal/handler/router.go`** -- Router construction:
```go
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
```

Route groups:
- External API (API key auth): POST /api/v1/items, POST /api/v1/items/async, GET /api/v1/policies
- Internal API (Session auth): All other routes per NEST_DESIGN.md section 10, plus:
  - `GET /api/v1/item-types/{id}` -- required by UI edit page (documented as backend gap in NEST_UI.md section 16)
  - `GET /api/v1/policies/{id}` -- required by UI edit page (documented as backend gap in NEST_UI.md section 16)
- Public (no auth): GET /api/v1/health, POST /api/v1/auth/login

Key handler: `mrt.go` RecordDecision handler orchestrates MRTService.RecordDecision then ActionPublisher.PublishActions with the returned ActionRequests. This is where invariant 8 is enforced.

### Unit Tests

**worker/:**
- Test ProcessItemWorker.Work builds correct Event and calls Pool.Evaluate
- Test ProcessItemWorker.Work publishes actions from evaluation result
- Test ProcessItemWorker.Work logs executions
- Test ProcessItemWorker.Work handles evaluation error gracefully

**handler/:**
- Test each endpoint with valid request returns expected status code and body shape
- Test each endpoint with invalid request body returns 400/422
- Test each endpoint without auth returns 401
- Test each endpoint with wrong role returns 403
- Test ErrorResponse format consistency
- Test PageParamsFromRequest with various query strings
- Test OrgID and UserID extraction from context
- Test MRT decision handler: calls MRTService.RecordDecision then ActionPublisher.PublishActions
- Test rules test endpoint returns TestResult without persisting
- Test UDF listing endpoint returns hardcoded UDF definitions
- Test health endpoint returns 200

### Integration Tests
- HTTP integration: start HTTP server, make requests via httptest, verify full round-trip through handler -> service -> store
- Auth flow: POST /api/v1/auth/login -> receive session cookie -> use session cookie on subsequent requests
- Rule CRUD via HTTP: create, read, update, delete rule via HTTP endpoints
- Item submission via HTTP: POST /api/v1/items with valid items, verify evaluation results in response
- MRT flow via HTTP: submit item that triggers enqueue -> list queues -> assign job -> record decision
- Async item submission: POST /api/v1/items/async returns 202 (river job enqueued)
- **(F5c)** Partition boundary: insert an execution log on the first day of a new month after the partition has been created by the maintenance job -- verify insert succeeds
- **(F7)** River job failure and retry: `ProcessItemWorker.Work` returns error -- verify river retries up to `max_attempts`. After `max_attempts` exhausted, verify job moves to discarded state. Use river's test utilities for verification.

### Invariants to Enforce
- **Invariant 2**: No circular dependencies (handler does not import store)
- **Invariant 8**: ActionPublisher and MRTService have no direct dependency (handler orchestrates in MRT decision handler)
- **Invariant 11**: context.Context first parameter
- **Invariant 12**: All errors returned

### Exit Criteria
1. All HTTP endpoints from NEST_DESIGN.md section 10 implemented
2. Handler does not import store (verified by `go build` -- store not in handler's imports)
3. Auth middleware applied correctly to each route group
4. Worker processes async items correctly via river
5. Maintenance jobs registered for all 4 periodic tasks
6. All unit tests pass
7. All integration tests pass
8. `go vet ./internal/handler/... ./internal/worker/...` reports zero issues
9. **(F7)** River job failure/retry integration test passes (max_attempts exhausted -> discarded)
10. **(W3)** Table-driven tests (subtests) used for all multi-case functions
11. **(W6)** `errcheck ./internal/handler/... ./internal/worker/...` passes with zero unhandled errors

### Review Checklist
- [ ] Handler never imports store (auth middleware passed as closures from cmd/server)
- [ ] Handler imports auth only for context helpers (GetAuthContext, OrgIDFromContext, etc.), never for middleware construction
- [ ] MRT decision handler calls mrtService.RecordDecision then publisher.PublishActions (invariant 8)
- [ ] Login handler creates session, returns session cookie
- [ ] Logout handler deletes session
- [ ] Every handler maps domain errors to HTTP status codes (NotFoundError->404, ValidationError->400/422, ForbiddenError->403, ConflictError->409)
- [ ] UDF listing returns accurate signatures matching NEST_DESIGN.md section 7.5
- [ ] Worker ProcessItemWorker handles river job errors without panicking
- [ ] Maintenance jobs have reasonable schedules (not too frequent, not too rare)

---

## Stage 7: cmd/server + cmd/migrate + cmd/seed

### Prerequisites
- Stage 6 complete and validated (worker, handler)

### Modules
- `cmd/server/main.go` -- Composition root
- `cmd/migrate/main.go` -- Migration runner
- `cmd/seed/main.go` -- Development seed data

### SWE Agent Teams (2 parallel tracks)

**Team 7A: cmd/server**
The composition root wires all dependencies.

**Team 7B: cmd/migrate + cmd/seed** (parallel with 7A)

### Implementation Scope

**`cmd/server/main.go`** (~200 lines):
```go
func main()
```
1. Parse config via config.Load()
2. Set up slog logger
3. Create pgxpool connection
4. Create store.Queries
5. Create auth.Signer
6. Create auth middleware closures: sessionAuthMw = auth.SessionAuth(store), apiKeyAuthMw = auth.APIKeyAuth(store)
7. Create signal.Registry, register adapters (TextRegex, TextBank(store), any configured HTTP signals)
8. Create engine.Compiler
9. Create engine.Pool(config.WorkerCount, registry, store, logger)
10. Create engine.ActionPublisher(store, signer, http.DefaultClient, logger)
11. Create all services: RuleService, ConfigService, MRTService, ItemService, UserService, APIKeyService, SigningKeyService, TextBankService
12. Rebuild snapshots for all active orgs
13. Create chi router via handler.NewRouter(..., sessionAuthMw, apiKeyAuthMw, ...)
14. Set up river client, register ProcessItemWorker, register maintenance jobs
15. Start HTTP server
16. Handle graceful shutdown: SIGINT/SIGTERM -> drain HTTP -> stop river -> stop pool -> close DB

**`cmd/migrate/main.go`** (~80 lines):
1. Parse DATABASE_URL from environment
2. Connect to PostgreSQL
3. Create schema_migrations table if not exists
4. Read migration files from migrations/ directory
5. Apply pending migrations in order
6. Support `up` and `status` subcommands

**`cmd/seed/main.go`** (~120 lines):
1. Parse DATABASE_URL from environment
2. Connect to PostgreSQL, create store.Queries
3. Create default org (idempotent)
4. Create admin user with known password (idempotent)
5. Create default MRT queues: "default", "urgent", "escalation" (idempotent)
6. Optionally create sample rules, actions, policies, item types

### Unit Tests
- Test cmd/server wiring does not panic with mock dependencies
- Test cmd/migrate detects pending migrations
- Test cmd/seed is idempotent (run twice, no duplicates)

### Integration Tests
- Full system test: start server, run migrations, seed data, login, create rule, submit item, verify evaluation
- **(W7)** Graceful shutdown: start server, send SIGINT, verify clean shutdown using `go.uber.org/goleak` (`goleak.VerifyMain`) -- no goroutine leaks
- Cold start: start server, verify snapshots rebuilt for all orgs from database

### Invariants to Enforce
- **Invariant 2**: No circular dependencies (composition root is the only module that imports everything)
- **Invariant 17**: CGO not required (`CGO_ENABLED=0 go build ./cmd/server/`)

### Exit Criteria
1. `CGO_ENABLED=0 go build ./cmd/server/` succeeds
2. `CGO_ENABLED=0 go build ./cmd/migrate/` succeeds
3. `CGO_ENABLED=0 go build ./cmd/seed/` succeeds
4. Server starts, accepts requests, shuts down gracefully
5. Migrations apply cleanly to fresh database
6. Seed data creates org, admin user, and MRT queues
7. Full end-to-end test passes: seed -> login -> create rule -> submit item -> evaluate -> verdict
8. **(W7)** `go.uber.org/goleak` added to test dependencies. Graceful shutdown test uses `goleak.VerifyMain` to detect goroutine leaks.
9. **(W9)** Smoke test: 50 concurrent `POST /api/v1/items` requests for 10 seconds complete without errors or goroutine leaks
10. **(W3)** Table-driven tests (subtests) used for all multi-case functions
11. **(W6)** `errcheck ./internal/...` passes with zero unhandled errors

### Review Checklist
- [ ] Composition root constructs auth middleware closures and passes to NewRouter (handler never imports store)
- [ ] Signal adapters registered at startup (TextRegex, TextBank, any HTTP signals from config)
- [ ] Snapshot rebuild on startup for all active orgs
- [ ] Graceful shutdown sequence: HTTP drain -> river stop -> pool stop -> DB close
- [ ] cmd/seed uses INSERT ... ON CONFLICT DO NOTHING for idempotency
- [ ] No hardcoded secrets in seed data (use env vars or obvious dev-only defaults)
- [ ] `go.starlark.net` pinned to exact version in go.mod (verify)

---

## Stage 8: Python Frontend -- API Layer + Auth + Components

### Prerequisites
- Stage 7 complete and validated (Go backend fully operational)

### Modules
- `nest-ui/api/types.py` -- Response dataclasses
- `nest-ui/api/client.py` -- NestClient typed HTTP wrapper
- `nest-ui/auth/state.py` -- RBAC state helpers
- `nest-ui/auth/middleware.py` -- Auth guard
- `nest-ui/components/layout.py` -- App shell with RBAC sidebar
- `nest-ui/components/starlark_editor.py` -- Starlark editor

### SWE Agent Teams (2 parallel tracks)

**Team 8A: API Layer (types.py + client.py)**

**Team 8B: Auth + Components (state.py, middleware.py, layout.py, starlark_editor.py)**

### Implementation Scope

**`nest-ui/api/types.py`** (~120 lines):
All response dataclasses mirroring Go domain types:
Rule, Action, Policy, ItemType, User, ApiKey, MRTQueue, MRTJob, MRTDecision, Signal, SigningKey, TextBank, PaginatedResult[T]

**`nest-ui/api/client.py`** (~250 lines):
NestClient class with typed async methods for every Nest REST endpoint. Does NOT own httpx.AsyncClient. All methods async, all return typed dataclasses. No **kwargs anywhere.

**`nest-ui/auth/state.py`** (~20 lines):
```python
def user_role() -> str: ...
def can_edit(resource: str) -> bool: ...
def is_moderator_or_above() -> bool: ...
```

**`nest-ui/auth/middleware.py`** (~30 lines):
```python
def require_auth(page_func): ...
```
Decorator that validates session on every page load.

**`nest-ui/components/layout.py`** (~60 lines):
App shell with RBAC-filtered sidebar, header, logout button, confirm() dialog utility.

**`nest-ui/components/starlark_editor.py`** (~40 lines):
Code editor with UDF/signal reference sidebar.

### Unit Tests
- Test all dataclasses in types.py can be instantiated from dict (simulating API response)
- Test PaginatedResult generic works with multiple types
- Test NestClient methods construct correct HTTP requests (mock httpx)
- Test NestClient handles 401, 403, 422, 500 responses correctly
- Test user_role() reads from app.storage.user correctly
- Test can_edit() returns True only for ADMIN
- Test is_moderator_or_above() returns True for MODERATOR and ADMIN
- Test layout sidebar filtering by role

### Integration Tests
- NestClient against live Nest backend: login, list rules, create rule, test rule, delete rule
- Auth flow: login -> middleware validates session -> access pages -> logout -> middleware redirects to login
- Full round-trip: NestClient.login() -> NestClient.me() -> NestClient.list_rules() -> NestClient.create_rule()

### Invariants to Enforce
- **UI Invariant 3**: NestClient is the only module that makes HTTP calls
- **UI Invariant 6**: Two production dependencies maximum (nicegui, httpx)
- **UI Invariant 9**: One httpx.AsyncClient per session
- **UI Invariant 10**: No **kwargs in NestClient

### Exit Criteria
1. All dataclasses match Go domain types
2. NestClient has typed methods for every endpoint in NEST_DESIGN.md section 10
3. No **kwargs in any NestClient method
4. Auth state helpers work correctly
5. Layout renders sidebar filtered by role
6. Starlark editor renders with UDF/signal panels
7. All unit tests pass
8. All integration tests pass (against running backend)
9. `ruff check` reports zero errors
10. `pyright` reports zero errors

### Review Checklist
- [ ] All NestClient methods are async
- [ ] All NestClient methods have explicit type annotations (no Any except where justified)
- [ ] No **kwargs anywhere
- [ ] httpx.AsyncClient not created inside NestClient (passed via constructor)
- [ ] Auth middleware validates session via GET /api/v1/auth/me on each page load
- [ ] Layout sidebar items match RBAC table from NEST_UI.md section 12
- [ ] Starlark editor uses Python syntax highlighting (closest to Starlark)
- [ ] No business logic in any file

---

## Stage 9: Python Frontend -- Pages + Entry Point

### Prerequisites
- Stage 8 complete and validated (API layer, auth, components)

### Modules
- `nest-ui/pages/*.py` -- All 12 page modules
- `nest-ui/main.py` -- Entry point

### SWE Agent Teams (3 parallel tracks)

**Team 9A: Core Pages**
Files: `login.py`, `dashboard.py`, `rules.py` (most complex), `mrt.py` (second most complex)

**Team 9B: CRUD Pages**
Files: `actions.py`, `policies.py`, `item_types.py`, `text_banks.py`

**Team 9C: Admin Pages + Entry Point**
Files: `users.py`, `api_keys.py`, `signals.py`, `settings.py`, `main.py`

### Implementation Scope

**`nest-ui/pages/login.py`** (~35 lines):
- Email + password form
- Calls NestClient.login()
- Creates httpx.AsyncClient, stores in app.storage.user['http_client']
- Stores session token and user in app.storage.user
- Redirects to /dashboard
- No sidebar, no layout wrapper

**`nest-ui/pages/dashboard.py`** (~45 lines):
- Welcome message with user name and role
- Quick links to Rules, MRT, Actions, Policies
- Counts: total rules, active rules, pending MRT jobs, total users

**`nest-ui/pages/rules.py`** (~250 lines):
- List view: table with Name, Status, Event Types, Priority, Tags, Updated At
- Filter by status
- Editor view: Starlark editor, metadata fields, policy associations, template dropdown
- Test panel: JSON event input, test button, result display
- Save button

**`nest-ui/pages/actions.py`** (~90 lines):
- Table: Name, Type, Updated At
- Create/edit form: Name, Type dropdown, Config JSON editor
- Delete with confirmation

**`nest-ui/pages/policies.py`** (~90 lines):
- Table: Name, Description, Parent, Strike Penalty, Version
- Create/edit form with parent dropdown
- Hierarchical display

**`nest-ui/pages/item_types.py`** (~90 lines):
- Table: Name, Kind, Field Count, Updated At
- Create/edit form: Name, Kind dropdown, Schema JSON editor, Field Roles

**`nest-ui/pages/mrt.py`** (~160 lines):
- Queue list with pending/assigned/decided counts
- Job review: "Get Next Job" button, job detail with formatted JSON payload
- Decision form: Verdict dropdown, Action multi-select, Policy multi-select, Reason textarea

**`nest-ui/pages/text_banks.py`** (~90 lines):
- Bank list: Name, Description, Entry Count
- Entry list with Add Entry form
- Delete entry per row

**`nest-ui/pages/users.py`** (~70 lines):
- Table: Name, Email, Role, Active, Created At
- Invite form: Email, Name, Role dropdown
- Edit role, deactivate user (ADMIN only)

**`nest-ui/pages/api_keys.py`** (~70 lines):
- Table: Name, Prefix, Created At, Revoked At
- Create form: show key ONCE in dialog with copy button
- Revoke with confirmation

**`nest-ui/pages/signals.py`** (~70 lines):
- Table: ID, Display Name, Description, Inputs, Cost
- Test panel: select signal, enter input, run test, display output

**`nest-ui/pages/settings.py`** (~45 lines):
- Org name display
- Signing keys: list, rotate button
- Links to Users and API Keys

**`nest-ui/main.py`** (~40 lines):
- Read env vars (NEST_API_URL, UI_PORT, UI_SECRET)
- Configure NiceGUI app
- Import all page modules
- Root redirect / -> /dashboard
- Start NiceGUI server

### Unit Tests
- Test each page renders without errors (mock NestClient responses)
- Test login page stores session on success and redirects
- Test login page shows error on failure
- Test rules page renders editor with templates
- Test rules page test panel calls test_rule and displays result
- Test MRT page assign button calls assign_next_job
- Test MRT page decision form calls record_decision
- Test api_keys page shows key once and hides it after dialog close
- Test page RBAC: ANALYST cannot see Users/API Keys/Settings pages
- Test page RBAC: MODERATOR cannot see Users/Settings
- Test error handling: 401 redirects to login, 403 shows warning, 422 shows detail

### Integration Tests
- Full UI flow against running backend:
  1. Login as admin
  2. Create item type
  3. Create action
  4. Create policy
  5. Create rule (using Starlark editor)
  6. Test rule via test panel
  7. Verify rule appears in list
  8. Create API key, verify shown once
  9. Navigate to MRT, verify queues displayed
  10. Navigate to Settings, rotate signing key

### Invariants to Enforce
- **UI Invariant 1**: No business logic in the UI (every action calls API)
- **UI Invariant 2**: No page imports from another page
- **UI Invariant 5**: Every page function is async
- **UI Invariant 7**: No JavaScript written or maintained
- **UI Invariant 8**: No build step (python main.py starts the app)
- **UI Invariant 11**: Sidebar reflects user role

### Exit Criteria
1. All 12 pages implemented and rendering
2. All routes registered (matching NEST_UI.md section 9 route table)
3. No page imports from another page
4. Every page function is async
5. Login/logout flow works end-to-end
6. Rules page: Starlark editor, test panel, template dropdown all functional
7. MRT page: queue list, job assignment, decision recording all functional
8. RBAC sidebar filtering works for all 3 roles
9. Error handling follows standard pattern for all HTTP status codes
10. `python main.py` starts the app with no errors
11. All unit tests pass
12. All integration tests pass
13. `ruff check` reports zero errors
14. `pyright` reports zero errors
15. Total UI codebase approximately 1,665 lines

### Review Checklist
- [ ] No page imports from another page
- [ ] Every page uses `with layout('Title'):` for consistent app shell
- [ ] Every page uses `@require_auth` decorator (except login)
- [ ] Error handling follows standard pattern from NEST_UI.md section 8
- [ ] Rules page has template dropdown with 5 starter templates
- [ ] Rules page test panel shows verdict, reason, actions, logs, latency
- [ ] MRT decision form includes Verdict, Actions, Policies, and Reason fields
- [ ] API Keys page shows plaintext key ONCE in dialog, never again
- [ ] Login page creates httpx.AsyncClient and stores in app.storage.user
- [ ] Logout closes httpx.AsyncClient and clears app.storage.user
- [ ] main.py imports all page modules (triggering route registration)
- [ ] No JavaScript in any file

---

## Cross-Stage Dependency Graph

```
Stage 1: domain + config + migrations
    |
    v
Stage 2: store
    |
    v
Stage 3: auth + signal (PARALLEL)
    |
    v
Stage 4: engine
    |
    v
Stage 5: service
    |
    v
Stage 6: worker + handler (PARALLEL)
    |
    v
Stage 7: cmd/server + cmd/migrate + cmd/seed
    |
    v
Stage 8: UI api + auth + components
    |
    v
Stage 9: UI pages + main
```

## UI Invariants (from NEST_UI.md section 19)

These invariants govern the Python frontend (Stages 8-9). They are referenced as "UI Invariant N" in the stage definitions above.

| # | UI Invariant |
|---|-------------|
| 1 | **No business logic in the UI.** Every action calls a Nest REST endpoint. The UI never computes, validates, or transforms data beyond display formatting. |
| 2 | **No page imports from another page.** Pages are independent modules. Shared functionality lives in `components/` or `api/`. |
| 3 | **The API client is the only module that makes HTTP calls.** No page constructs raw HTTP requests. |
| 4 | **RBAC is advisory in the UI, authoritative in the backend.** The UI hides buttons and sidebar links but does not enforce access. The backend rejects unauthorized requests. |
| 5 | **Every page function is async.** All API calls use `await`. No blocking I/O. |
| 6 | **Two production dependencies maximum.** `nicegui` and `httpx`. Nothing else. |
| 7 | **No JavaScript written or maintained.** All UI logic is Python. |
| 8 | **No build step.** `python main.py` starts the app. No compilation, no bundling, no transpilation. |
| 9 | **One httpx.AsyncClient per session.** Created at login, stored in `app.storage.user`, closed at logout. Never created per page load. |
| 10 | **No `**kwargs` in NestClient.** All public API methods have fully typed parameters. |
| 11 | **Sidebar reflects user role.** Nav items are filtered by the RBAC table. An ANALYST never sees Users, API Keys, MRT, Item Types, Text Banks, or Settings links. |

## Invariant Enforcement Matrix

| Invariant | Stages Where Enforced |
|-----------|----------------------|
| 1. org_id in every WHERE clause | 2, 3, 4, 5, 6 |
| 2. No circular dependencies | All stages |
| 3. Zero sync.Mutex on hot path | 4 |
| 4. Starlark evaluation never panics | 4 |
| 5. Rules compile before activation | 4, 5 |
| 6. Atomic snapshot swap | 4 |
| 7. domain has zero internal imports | 1 |
| 8. ActionPublisher/MRTService no direct dependency | 5, 6 |
| 9. Entity history append-only | 2, 5 |
| 10. API keys never plaintext | 3, 5 |
| 11. context.Context first parameter on I/O | 2, 3, 4, 5, 6 |
| 12. No error swallowing | All stages |
| 13. Signal results cached per event | 4 |
| 14. In-memory counters eventually consistent | 4 |
| 15. Starlark source is single source of truth | 4, 5 |
| 16. No JSONB condition trees | 1 |
| 17. CGO not required | 1, 7 |
| UI-1. No business logic in UI | 8, 9 |
| UI-2. No page imports from another page | 9 |
| UI-3. API client is only HTTP caller | 8 |
| UI-4. RBAC advisory in UI, authoritative in backend | 8, 9 |
| UI-5. Every page function is async | 9 |
| UI-6. Two production dependencies maximum | 8, 9 |
| UI-7. No JavaScript written or maintained | 8, 9 |
| UI-8. No build step | 9 |
| UI-9. One httpx.AsyncClient per session | 8, 9 |
| UI-10. No **kwargs in NestClient | 8 |
| UI-11. Sidebar reflects user role | 8, 9 |

## Estimated Total Size

| Component | Lines |
|-----------|-------|
| Go production code | ~7,100 |
| Go test code | ~7,100 |
| SQL migrations | ~250 |
| Python UI | ~1,665 |
| **Grand total** | **~16,115** |
