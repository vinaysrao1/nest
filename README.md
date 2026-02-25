# Nest

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Nest is a content moderation rules engine that evaluates user-submitted items against Starlark-based rules, producing verdicts (approve, block, review) with optional webhook actions and manual review routing. It is a single Go binary backed by PostgreSQL.

## Prerequisites

- **Go 1.25+**
- **PostgreSQL 16+**
- **Python 3.11+** (for the admin UI only)

## Quick Start

### 1. Database Setup

Create a PostgreSQL database:

```bash
createdb nest
```

### 2. Run Migrations

```bash
DATABASE_URL="postgres://user:pass@localhost:5432/nest?sslmode=disable" make migrate
```

Check migration status:

```bash
DATABASE_URL="..." go run ./cmd/migrate/ status
```

### 3. Seed Default Data (Development)

The seed tool creates a default org, admin user, and MRT queues for development/testing:

```bash
DATABASE_URL="..." go run ./cmd/seed/
```

Or with make:

```bash
DATABASE_URL="..." make seed
```

This creates:

| Entity | ID | Details |
|--------|----|---------|
| Org | `org-default` | "Default Org" |
| Admin User | `usr-admin-default` | Email: `admin@nest.local`, Password: `admin123`, Role: ADMIN |
| MRT Queue | `mrtq-default` | "default" |
| MRT Queue | `mrtq-urgent` | "urgent" |
| MRT Queue | `mrtq-escalation` | "escalation" |

**WARNING:** Do not use these credentials in production.

All seed operations are idempotent (safe to run multiple times).

### 4. Set Environment Variables

Required:

```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/nest?sslmode=disable"
export SESSION_SECRET="your-secret-key-at-least-32-chars"
```

Optional (with defaults):

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `WORKER_COUNT` | `runtime.NumCPU()` | Starlark worker pool size |
| `RIVER_WORKER_COUNT` | `100` | Background job worker count |
| `RULE_TIMEOUT` | `1s` | Per-rule evaluation timeout |
| `EVENT_TIMEOUT` | `5s` | Per-event total evaluation timeout |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `DEV_MODE` | `false` | Enable development mode |
| `COUNTER_BACKEND` | `memory` | Counter storage: `memory` or `postgres` |
| `OPENAI_API_KEY` | _(empty)_ | OpenAI API key (enables moderation signal adapter, empty = disabled) |
| `OPENAI_MODERATION_MODEL` | `omni-moderation-latest` | OpenAI moderation model name |
| `OPENAI_MODERATION_TIMEOUT` | `5s` | HTTP timeout for OpenAI moderation requests |
| `OPENAI_MODERATION_MAX_INPUT` | `102400` | Max input bytes for OpenAI moderation |

### 5. Build and Run the Server

```bash
make build
./nest
```

Or run directly:

```bash
go run ./cmd/server/
```

### 6. Run the Admin UI (Optional)

```bash
cd nest-ui
pip install nicegui httpx
NEST_API_URL="http://localhost:8080" python main.py
```

The UI runs on `http://localhost:8090` by default. Optional environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `UI_PORT` | `8090` | UI server port |
| `UI_SECRET` | `change-me-in-production` | NiceGUI storage secret |

## Key Commands

```bash
make build      # Compile binary (CGO_ENABLED=0)
make test       # Run full test suite
make vet        # Run go vet
make lint       # Run go vet (alias for vet)
make docker     # Build Docker image
make migrate    # Apply database migrations
make seed       # Seed default org, admin user, and MRT queues
make run        # Run server directly with go run
make clean      # Remove compiled binary
```

Run a single test:

```bash
go test ./internal/engine/... -run TestCompiler -v
```

## Architecture

Nine Go packages under `internal/`:

```
domain   -- Pure types, zero imports
config   -- Environment configuration
store    -- PostgreSQL data access (pgx)
auth     -- Sessions, API keys, RBAC, password hashing, webhook signing
signal   -- Signal adapter framework (TextRegex, TextBank, HTTP)
engine   -- Starlark rule evaluation: Pool, Workers, Snapshots
service  -- Business logic orchestration
worker   -- Background jobs via river (item processing, snapshot rebuild)
handler  -- HTTP handlers and chi routing
```

Dependency flow: `domain` -> `store` -> `auth`/`signal` -> `engine` -> `service` -> `worker`/`handler`. No cycles.

## Documentation

- [Rules Engine](docs/RULES_ENGINE.md) -- Rule syntax, verdict types, MRT routing, and examples
- [Writing UDFs](docs/WRITING_UDFS.md) -- Built-in functions available in rules
- [Admin Guide](docs/ADMIN_GUIDE.md) -- Org, user, and RBAC management
- [Integration Guide](docs/INTEGRATION_GUIDE.md) -- Submitting items and receiving verdicts
- [Analytics Queries](docs/ANALYTICS_QUERIES.md) -- SQL queries for reporting
- [Design Document](docs/NEST_DESIGN.md) -- Full architecture specification
- [Module Contracts](docs/NEST_MODULES.md) -- Package interfaces and invariants

## License

Copyright 2026 Vinay Rao. Licensed under the [Apache License, Version 2.0](LICENSE).
