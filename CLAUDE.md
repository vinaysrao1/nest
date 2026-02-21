# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nest is a content moderation rules engine merging Fruitfly (Starlark-based rule evaluation) with Coop-Lite-Go (multi-tenant CRUD, REST API, RBAC, MRT). Single Go binary, PostgreSQL-only infrastructure, no CGO.

## Key Design Documents

- `docs/NEST_DESIGN.md` — Full architecture specification (20 sections, authoritative)
- `docs/NEST_MODULES.md` — Module contracts with strict interfaces
- `docs/NEST_UI.md` — Python frontend design (NiceGUI)
- `docs/NEST_STAGING.md` — Implementation phases and exit criteria

Always consult these before making architectural decisions. The design docs are the source of truth. 

We will build the entire software based on the stages defined in docs/NEST_STAGING.md. For a new stage
- call architect agent to make a plan for a team of swe agents to develop. write the plan in working_docs/
- call a team of swe agents to implement the plan including the tests
- call validation agent to run the tests and if there are failures, call swe agents to find the root cause and fix
- call reviewer agent(s) thoroughly review for correctness, scalaing, module and data contract correctness, simple coding choices, no dead/duplicate code.
- Always mark the stage as done only after review and tests pass. Then work on the next stage. 
- Before starting the next stage always make sure the context window is sufficient for the full development of the stage. 

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.23+, chi v5 router, pgx/v5 |
| Rules | Starlark via `go.starlark.net` (pure Go, no CGO) |
| Job Queue | river (PostgreSQL-native) |
| Database | PostgreSQL 16 |
| Frontend | Python + NiceGUI + httpx |

Production dependencies are intentionally minimal: 5 Go modules, 2 Python packages.

## Build & Test Commands

```bash
make build        # Compile binary
make test         # Run full test suite
make docker       # Build Docker image
make migrate      # Run database migrations
```

Go tests use stdlib `testing`. Python tests use `pytest`. Run a single Go test:
```bash
go test ./internal/<package>/... -run TestName
```

## Static Analysis Tools

Four custom analyzers in `tools/analyzers/`:

```bash
python tools/analyzers/static_rigor.py   # pyright (strict) + ruff
python tools/analyzers/callgraph.py      # Circular deps, dead code, fan-out
python tools/analyzers/scaling.py        # Sync-in-async, N+1, unbounded concurrency
python tools/analyzers/contracts.py      # Module coupling, type coverage, API contracts
```

Slash commands: `/static-rigor`, `/callgraph`, `/scaling`, `/contracts`, `/full-analysis` (runs all four).

## Architecture (9 Go packages under `internal/`)

```
domain   ← Pure types, zero imports (dependency leaf)
config   ← Environment configuration
store    ← PostgreSQL data access (pgx)
auth     ← Sessions, API keys, RBAC, password hashing, RSA-PSS webhook signing
signal   ← Signal adapter framework (TextRegex, TextBank, HTTP adapters)
engine   ← Starlark rule evaluation: Pool → Workers → Snapshot (atomic.Pointer)
service  ← Business logic orchestration (rules, config, MRT, items, users, etc.)
worker   ← Background jobs via river (item processing, snapshot rebuild, maintenance)
handler  ← HTTP handlers and chi routing
```

**Dependency flow:** `domain` → `store` → `auth`/`signal` → `engine` → `service` → `worker`/`handler`. No cycles allowed.

## Critical Architectural Invariants

1. **Starlark source is the single source of truth.** Database columns like `event_types` and `priority` are derived values populated at compile time. No JSONB condition trees anywhere.
2. **No `sync.Mutex` on the hot path.** Concurrency uses `atomic.Pointer[Snapshot]`, `sync.Map` for per-org snapshots, and channels for worker communication.
3. **Actions are declared in Starlark** via `verdict("block", actions=["webhook-1"])`, not via join tables.
4. **PostgreSQL does everything** — jobs (river), sessions, analytics, storage. No Redis, no Kafka.
5. **CGO_ENABLED=0** — all dependencies are pure Go.

## Agent Pipeline

Defined in `.claude/agents/`. Four agents in strict sequence:

**Architect** (Opus) → **Reviewer** (Opus) → **SWE** (Sonnet) → **Reviewer** (Opus) → **Validation** (Sonnet)

- Trivial tasks can skip Architect design (Architect triages as "trivial, hand to SWE")
- Max 2 revision cycles per review stage before escalating to user
- All agents must run analyzer tools at their stage

Slash commands: `/dev-loop` (full pipeline), `/debug-loop <test_path> [source_path]` (test-driven fix cycle, max 5 iterations, state in `working_docs/BUG_TRACKER.md`).

## Implementation Stages

Stages are sequential — each depends on the prior stage being validated:

1. Domain Types + Config + SQL Migrations
2. Store (PostgreSQL Data Access)
3. Auth + Signal (parallel)
4. Engine (Starlark Rule Evaluation)
5. Service (Business Logic)
6. Worker (Background Jobs)
7. Handler (HTTP API)
8. Python Frontend
9. Integration + Docker

## Python Frontend Structure

| Module | Purpose |
|--------|---------|
| `api/client.py` | Typed async HTTP client for Nest API |
| `api/types.py` | Response/request dataclasses mirroring domain |
| `auth/state.py` | AuthState: user, session, org context, RBAC |
| `pages/` | Individual page modules (login, dashboard, rules, MRT, etc.) |
| `main.py` | Entry point |

Linting: `ruff`. Type checking: `pyright` in strict mode.
