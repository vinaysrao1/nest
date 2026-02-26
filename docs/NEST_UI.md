# Nest UI -- Lightweight Python Client

A minimal Python-based UI for the Nest content moderation platform. One framework. One API client. Zero JavaScript.

---

## 1. Problem Statement

Nest has a complete REST API (documented in NEST_DESIGN.md) but no frontend. The original Coop frontend is a React 18 + TypeScript + Apollo + Ant Design + Tailwind + Radix monolith with 92 production dependencies and ~30,000 lines of code. That frontend is tightly coupled to Coop's GraphQL API and cannot be reused.

Nest needs a UI that:
- Lets admins manage rules (Starlark source), actions, policies, item types, text banks, users, API keys, and signing keys
- Lets moderators review MRT queues and record decisions
- Provides a Starlark code editor with live testing
- Shows available signals and UDFs for rule authoring
- Is small enough for a single developer to understand and modify in an afternoon

The UI does NOT need to be flashy. It needs to be correct, fast to build, and trivial to extend.

---

## 2. Framework Choice: NiceGUI

### Decision

Use **NiceGUI** as the sole UI framework.

### Alternatives Considered

| Framework | Verdict | Why |
|-----------|---------|-----|
| **Streamlit** | Rejected | Re-runs entire script on every interaction. No real routing. No persistent state without hacks. Fine for dashboards, terrible for CRUD apps with forms, editors, and multi-step workflows. |
| **Reflex** | Rejected | Compiles Python to React under the hood. Heavy build step, complex state management, large footprint (~150MB install). Defeats the "lightweight" goal. |
| **Gradio** | Rejected | ML demo tool. No routing, no layout control, no table components suitable for CRUD. |
| **Dash** | Rejected | Callback-based reactivity is verbose for forms and CRUD. Heavy Plotly dependency. |
| **Panel (HoloViz)** | Rejected | Designed for data exploration dashboards, not CRUD apps. Weak form/table components. Complex reactive model (param-based). Heavyweight dependency chain (Bokeh, pandas). |
| **NiceGUI** | Selected | Lightweight (~20MB). Real Python -- no transpilation. Built-in routing, persistent state, tables, forms, code editors (via Codemirror/Monaco integration), tabs, dialogs. WebSocket-based reactivity. FastAPI under the hood. Single `pip install nicegui`. |

### Why NiceGUI Wins

1. **Real routing**: `@ui.page('/rules')` gives URL-based navigation. Each page is a Python function.
2. **Persistent state**: Server-side Python objects persist across interactions. No script re-execution.
3. **Built-in code editor**: `ui.codemirror()` or `ui.code()` for the Starlark rule editor. No JavaScript integration needed.
4. **Built-in table**: `ui.table()` with sorting, filtering, pagination, and row selection.
5. **Built-in forms**: `ui.input()`, `ui.select()`, `ui.textarea()`, `ui.switch()` -- standard form elements.
6. **FastAPI underneath**: Can add custom API endpoints if needed. Familiar Python ecosystem.
7. **Single dependency**: `pip install nicegui` and `pip install httpx` (async HTTP client). Two dependencies total.
8. **Hot reload**: `ui.run(reload=True)` during development.

### Constraints

- NiceGUI is server-rendered via WebSockets. Each browser tab maintains a WebSocket connection to the Python server. This is fine for an internal admin tool with <100 concurrent users. It is not suitable for a public-facing app with thousands of concurrent sessions.
- NiceGUI uses Quasar (Vue-based) components under the hood. We never touch them directly -- the Python API abstracts everything.

---

## 3. Production Dependencies (2)

| Package | Why |
|---------|-----|
| `nicegui` | UI framework. Includes routing, components, WebSocket reactivity. |
| `httpx` | Async HTTP client for Nest API calls. Replaces `requests` with async support. |

Dev dependencies: `ruff` (linter), `pyright` (type checker).

**Total install footprint: ~25MB.**

---

## 4. Directory Structure

```
nest-ui/
|-- pyproject.toml              # Project config, dependencies
|-- main.py                     # Entry point: configure app, mount modules, run
|
|-- api/
|   |-- __init__.py
|   |-- client.py               # NestClient: typed async HTTP client for all Nest endpoints
|   |-- types.py                # Response/request dataclasses (mirrors Nest domain types)
|
|-- auth/
|   |-- __init__.py
|   |-- state.py                # AuthState: current user, session token, org context, RBAC helpers
|   |-- middleware.py            # Auth guard: redirect to login if not authenticated
|
|-- pages/
|   |-- __init__.py
|   |-- login.py                # Login page
|   |-- dashboard.py            # Overview / home page
|   |-- rules.py                # Rule list + create/edit (Starlark editor + test)
|   |-- actions.py              # Action list + create/edit
|   |-- policies.py             # Policy list + create/edit
|   |-- item_types.py           # Item type list + create/edit
|   |-- mrt.py                  # MRT queues + job review + decisions
|   |-- text_banks.py           # Text bank list + entry management
|   |-- users.py                # User list + invite + role management
|   |-- api_keys.py             # API key list + create + revoke
|   |-- signals.py              # Available signals + test
|   |-- settings.py             # Org settings + signing keys
|
|-- components/
|   |-- __init__.py
|   |-- layout.py               # App shell: sidebar nav (RBAC-filtered) + header + confirm()
|   |-- starlark_editor.py      # Starlark code editor with UDF autocomplete hints
```

**File count: ~18 Python files.**

Components reduced to 2 files: `layout.py` (app shell, navigation, confirmation dialog) and `starlark_editor.py` (the Starlark editor is the one component complex enough to justify its own file). Everything else (`data_table.py`, `form_helpers.py`, `json_viewer.py`, `confirm_dialog.py`) has been eliminated. Pages call `ui.table()`, `ui.notify()`, `ui.code()`, and `ui.button()` directly -- wrapping one-liner NiceGUI calls in custom abstractions adds indirection without value.

---

## 5. Architecture

### Data Flow

```
Browser (WebSocket)
    |
    v
NiceGUI Server (Python)
    |
    +-- pages/*.py (UI logic, renders components)
    |       |
    |       v
    +-- api/client.py (NestClient)
    |       |
    |       v (HTTP)
    +-- Nest Backend (Go, :8080)
            |
            v
        PostgreSQL
```

The UI is a **thin presentation layer**. All business logic lives in the Nest backend. The UI:
1. Calls Nest REST endpoints via `NestClient`
2. Renders the response data as tables, forms, and editors
3. Submits user input back to Nest REST endpoints

No business logic in the UI. No local database. No client-side caching. Every page re-fetches data from the API on navigation -- this is intentional. An internal tool with <100 users and a same-network Go backend does not benefit from cache complexity.

### State Management

NiceGUI maintains server-side state per browser session via `app.storage.user` (persisted) and `app.storage.tab` (per-tab). We use:

- `app.storage.user['session_token']` -- Nest session cookie, persisted across page navigations
- `app.storage.user['user']` -- Current user object (id, name, email, role, org_id)
- `app.storage.user['http_client']` -- Shared `httpx.AsyncClient` instance (see NestClient Lifecycle below)
- Per-page state is local Python variables in the page function -- NiceGUI handles reactivity via bindings

There is no global state store, no Redux equivalent, no state management library. Each page function fetches what it needs from the API when it loads.

### Authentication Flow

```
Browser navigates to any page
    |
    v
auth/middleware.py checks app.storage.user['session_token']
    |
    +-- No token -> redirect to /login
    |
    +-- Has token -> GET /api/v1/auth/me (validate session)
        |
        +-- 401 -> clear token, close http_client, redirect to /login
        |
        +-- 200 -> store user, proceed to page
```

Login flow:
```
POST /api/v1/auth/login { email, password }
    |
    +-- 200 -> store session token + user in app.storage.user
    |          create httpx.AsyncClient, store in app.storage.user['http_client']
    |          redirect to /dashboard
    |
    +-- 401 -> show error message
```

---

## 6. NestClient Lifecycle

### Problem

`httpx.AsyncClient` manages a connection pool internally. Creating a new client per page load wastes connections and prevents connection reuse.

### Design

The `httpx.AsyncClient` is created once at login and stored in `app.storage.user`. All `NestClient` instances for that session share the same underlying HTTP client.

```python
# On login success:
http_client = httpx.AsyncClient(
    base_url=NEST_API_URL,
    cookies={"session": session_token},
    timeout=30.0,
)
app.storage.user['http_client'] = http_client

# On each page load:
client = NestClient(app.storage.user['http_client'])

# On logout:
http_client = app.storage.user.pop('http_client', None)
if http_client:
    await http_client.aclose()
app.storage.user.clear()
```

NestClient becomes a thin typed wrapper around an existing `httpx.AsyncClient`, not the owner of one:

```python
class NestClient:
    def __init__(self, http: httpx.AsyncClient) -> None:
        self._http = http
```

---

## 7. API Client Layer

A single `NestClient` class wraps all Nest REST endpoints with typed methods. Every page imports `NestClient` and calls methods on it. No page ever constructs raw HTTP requests.

```python
# api/client.py

from dataclasses import dataclass
import httpx
from api.types import (
    Rule, Action, Policy, ItemType, User, ApiKey,
    MRTQueue, MRTJob, MRTDecision, TextBank, Signal,
    PaginatedResult, SigningKey,
    RuleUpdate, ActionUpdate, PolicyUpdate, ItemTypeUpdate, UserUpdate,
)


class NestClient:
    """Typed async HTTP client for Nest REST API.

    Every method corresponds to one Nest endpoint.
    All methods include org_id via the session -- Nest infers it from the session token.

    The httpx.AsyncClient is NOT owned by this class. It is created once per
    session (at login) and shared across page loads. See section 6.
    """

    def __init__(self, http: httpx.AsyncClient) -> None:
        self._http = http

    # -- Auth --
    async def login(self, email: str, password: str) -> dict:
        """POST /api/v1/auth/login"""
        ...

    async def logout(self) -> None:
        """POST /api/v1/auth/logout"""
        ...

    async def me(self) -> User:
        """GET /api/v1/auth/me"""
        ...

    # -- Rules --
    async def list_rules(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Rule]:
        """GET /api/v1/rules"""
        ...

    async def get_rule(self, rule_id: str) -> Rule:
        """GET /api/v1/rules/{id}"""
        ...

    async def create_rule(self, name: str, status: str, source: str,
                          tags: list[str] | None = None,
                          policy_ids: list[str] | None = None) -> Rule:
        """POST /api/v1/rules"""
        ...

    async def update_rule(self, rule_id: str, *,
                          name: str | None = None,
                          status: str | None = None,
                          source: str | None = None,
                          tags: list[str] | None = None,
                          policy_ids: list[str] | None = None) -> Rule:
        """PUT /api/v1/rules/{id}

        All parameters are optional. Only non-None values are sent.
        """
        ...

    async def delete_rule(self, rule_id: str) -> None:
        """DELETE /api/v1/rules/{id}"""
        ...

    async def test_rule(self, source: str, event: dict) -> dict:
        """POST /api/v1/rules/test"""
        ...

    async def test_existing_rule(self, rule_id: str, event: dict) -> dict:
        """POST /api/v1/rules/{id}/test"""
        ...

    # -- Actions --
    async def list_actions(self) -> PaginatedResult[Action]:
        """GET /api/v1/actions"""
        ...

    async def get_action(self, action_id: str) -> Action:
        """GET /api/v1/actions/{id}"""
        ...

    async def create_action(self, name: str, action_type: str, config: dict) -> Action:
        """POST /api/v1/actions"""
        ...

    async def update_action(self, action_id: str, *,
                            name: str | None = None,
                            action_type: str | None = None,
                            config: dict | None = None) -> Action:
        """PUT /api/v1/actions/{id}

        All parameters are optional. Only non-None values are sent.
        """
        ...

    async def delete_action(self, action_id: str) -> None:
        """DELETE /api/v1/actions/{id}"""
        ...

    # -- Policies --
    async def list_policies(self) -> PaginatedResult[Policy]:
        """GET /api/v1/policies"""
        ...

    async def get_policy(self, policy_id: str) -> Policy:
        """GET /api/v1/policies/{id}

        NOTE: Backend gap -- NEST_DESIGN.md section 10 does not list
        GET /api/v1/policies/{id}. This endpoint must be added to the
        backend before the UI edit page can function. See section 16.
        """
        ...

    async def create_policy(self, name: str, description: str | None = None,
                            parent_id: str | None = None,
                            strike_penalty: int = 0) -> Policy:
        """POST /api/v1/policies"""
        ...

    async def update_policy(self, policy_id: str, *,
                            name: str | None = None,
                            description: str | None = None,
                            parent_id: str | None = None,
                            strike_penalty: int | None = None) -> Policy:
        """PUT /api/v1/policies/{id}

        All parameters are optional. Only non-None values are sent.
        """
        ...

    async def delete_policy(self, policy_id: str) -> None:
        """DELETE /api/v1/policies/{id}"""
        ...

    # -- Item Types --
    async def list_item_types(self) -> PaginatedResult[ItemType]:
        """GET /api/v1/item-types"""
        ...

    async def get_item_type(self, item_type_id: str) -> ItemType:
        """GET /api/v1/item-types/{id}

        NOTE: Backend gap -- NEST_DESIGN.md section 10 does not list
        GET /api/v1/item-types/{id}. This endpoint must be added to the
        backend before the UI edit page can function. See section 16.
        """
        ...

    async def create_item_type(self, name: str, kind: str, schema: dict,
                               field_roles: dict | None = None) -> ItemType:
        """POST /api/v1/item-types"""
        ...

    async def update_item_type(self, item_type_id: str, *,
                               name: str | None = None,
                               kind: str | None = None,
                               schema: dict | None = None,
                               field_roles: dict | None = None) -> ItemType:
        """PUT /api/v1/item-types/{id}

        All parameters are optional. Only non-None values are sent.
        """
        ...

    async def delete_item_type(self, item_type_id: str) -> None:
        """DELETE /api/v1/item-types/{id}"""
        ...

    # -- MRT --
    async def list_mrt_queues(self) -> list[MRTQueue]:
        """GET /api/v1/mrt/queues"""
        ...

    async def create_mrt_queue(self, name: str, description: str = "",
                               is_default: bool = False) -> MRTQueue:
        """POST /api/v1/mrt/queues"""
        ...

    async def archive_mrt_queue(self, queue_id: str) -> None:
        """DELETE /api/v1/mrt/queues/{id} (soft-delete)"""
        ...

    async def list_mrt_jobs(self, queue_id: str, status: str | None = None,
                            page: int = 1) -> PaginatedResult[MRTJob]:
        """GET /api/v1/mrt/queues/{id}/jobs"""
        ...

    async def assign_next_job(self, queue_id: str) -> MRTJob | None:
        """POST /api/v1/mrt/queues/{id}/assign"""
        ...

    async def get_mrt_job(self, job_id: str) -> MRTJob:
        """GET /api/v1/mrt/jobs/{id}"""
        ...

    async def record_decision(self, job_id: str, verdict: str,
                              action_ids: list[str] | None = None,
                              policy_ids: list[str] | None = None,
                              reason: str | None = None) -> MRTDecision:
        """POST /api/v1/mrt/decisions"""
        ...

    # -- Users --
    async def list_users(self) -> PaginatedResult[User]:
        """GET /api/v1/users"""
        ...

    async def invite_user(self, email: str, name: str, role: str) -> User:
        """POST /api/v1/users/invite"""
        ...

    async def update_user(self, user_id: str, *,
                          name: str | None = None,
                          role: str | None = None,
                          is_active: bool | None = None) -> User:
        """PUT /api/v1/users/{id}

        All parameters are optional. Only non-None values are sent.
        """
        ...

    async def deactivate_user(self, user_id: str) -> None:
        """DELETE /api/v1/users/{id}"""
        ...

    # -- API Keys --
    async def list_api_keys(self) -> list[ApiKey]:
        """GET /api/v1/api-keys"""
        ...

    async def create_api_key(self, name: str) -> dict:
        """POST /api/v1/api-keys (returns key once)"""
        ...

    async def revoke_api_key(self, key_id: str) -> None:
        """DELETE /api/v1/api-keys/{id}"""
        ...

    # -- Text Banks --
    async def list_text_banks(self) -> PaginatedResult[TextBank]:
        """GET /api/v1/text-banks"""
        ...

    async def create_text_bank(self, name: str, description: str | None = None) -> TextBank:
        """POST /api/v1/text-banks"""
        ...

    async def get_text_bank(self, bank_id: str) -> TextBank:
        """GET /api/v1/text-banks/{id}"""
        ...

    async def add_text_bank_entry(self, bank_id: str, value: str,
                                  is_regex: bool = False) -> dict:
        """POST /api/v1/text-banks/{id}/entries"""
        ...

    async def delete_text_bank_entry(self, bank_id: str, entry_id: str) -> None:
        """DELETE /api/v1/text-banks/{id}/entries/{entryId}"""
        ...

    # -- Signals --
    async def list_signals(self) -> list[Signal]:
        """GET /api/v1/signals"""
        ...

    async def test_signal(self, signal_id: str, input_value: str) -> dict:
        """POST /api/v1/signals/test"""
        ...

    # -- UDFs --
    async def list_udfs(self) -> list[dict]:
        """GET /api/v1/udfs"""
        ...

    # -- Signing Keys --
    async def list_signing_keys(self) -> list[SigningKey]:
        """GET /api/v1/signing-keys"""
        ...

    async def rotate_signing_key(self) -> SigningKey:
        """POST /api/v1/signing-keys/rotate"""
        ...

    # -- Health --
    async def health(self) -> dict:
        """GET /api/v1/health"""
        ...
```

Every method is async, returns a typed dataclass, and raises `httpx.HTTPStatusError` on failure. Error handling is centralized -- see section 13.

---

## 8. Error Handling

### HTTP Error Strategy

All API calls go through `NestClient`, which raises `httpx.HTTPStatusError` on non-2xx responses. Pages handle errors using a shared pattern:

```python
# Common error handling pattern used by all pages:

async def safe_api_call(coro, error_prefix: str = "Error"):
    """Execute an API call with standard error handling.

    This is NOT a shared function -- it is the pattern each page follows inline.
    Shown here for documentation purposes.
    """
    try:
        return await coro
    except httpx.HTTPStatusError as e:
        status = e.response.status_code
        if status == 401:
            app.storage.user.clear()
            ui.navigate.to('/login')
        elif status == 403:
            ui.notify("You do not have permission for this action.", type='warning')
        elif status == 409:
            ui.notify("Conflict: this resource was modified by another user. "
                      "Refresh and try again.", type='warning')
        elif status == 422:
            detail = e.response.json().get('detail', 'Validation error')
            ui.notify(f"{error_prefix}: {detail}", type='negative')
        else:
            ui.notify(f"{error_prefix}: {e.response.text}", type='negative')
        return None
    except httpx.ConnectError:
        ui.notify("Cannot reach the Nest API. Is the backend running?", type='negative')
        return None
```

### Status Code Behavior

| HTTP Status | UI Behavior |
|-------------|-------------|
| 200-299 | Success. Proceed normally. |
| 401 | Session expired. Clear `app.storage.user`, close `http_client`, redirect to `/login`. |
| 403 | Forbidden. Show warning notification. Do not redirect. |
| 409 | Conflict (concurrent edit). Show warning, suggest refresh. |
| 422 | Validation error. Show detail from response body. |
| 500+ | Server error. Show error notification with response text. |
| Connection refused | Backend unreachable. Show "cannot reach API" notification. |

### Loading States

Pages show a `ui.spinner()` while API calls are in flight. For tables, the spinner replaces the table content area. For forms, the submit button is disabled during the request.

---

## 9. Module Breakdown

Each page module is self-contained. It imports `NestClient`, `layout`, and optionally `starlark_editor`. Pages do not import from each other.

### 9.1 Login (`pages/login.py`)

- Email + password form
- Calls `NestClient.login()`
- Creates `httpx.AsyncClient`, stores in `app.storage.user['http_client']`
- Stores session token and user in `app.storage.user`
- Redirects to `/dashboard`
- No sidebar, no header -- standalone page

### 9.2 Dashboard (`pages/dashboard.py`)

v1.0 is a simple landing page:
- Welcome message with user name and role
- Quick links to Rules, MRT Queues, Actions, Policies
- Counts: total rules, active rules, pending MRT jobs, total users

v1.1 adds charts (rule execution counts over time, action counts) when analytics endpoints are available.

### 9.3 Rules (`pages/rules.py`)

The most complex page. Contains:

**List view** (`/rules`):
- Table with columns: Name, Status, Event Types, Priority, Tags, Updated At
- Filter by status (LIVE/BACKGROUND/DISABLED)
- Create button -> opens editor
- Click row -> opens detail/edit view

**Editor view** (`/rules/{id}` or `/rules/new`):
- **Starlark code editor** (the primary interaction surface)
  - Uses `ui.codemirror()` with Python syntax highlighting (closest to Starlark)
  - Sidebar panel listing available UDFs (from `GET /api/v1/udfs`) with signatures and descriptions
  - Sidebar panel listing available signals (from `GET /api/v1/signals`) with usage examples
- **Metadata fields**: Name, Status (dropdown), Tags (chip input)
- **Policy associations**: Multi-select from available policies
- **"New from template" dropdown** (on create page only): Provides 3-5 starter Starlark templates:
  - "Blank rule" (minimal `evaluate` skeleton)
  - "Signal threshold" (single signal check with block/approve)
  - "Rate limiter" (counter-based rate limiting)
  - "Multi-signal" (combine two signals with AND logic)
  - "MRT routing" (enqueue to review queue based on score)
- **Test panel**: JSON input for a sample event + "Test" button -> calls `POST /api/v1/rules/test` -> displays verdict, reason, actions, logs, latency
- **Save button**: Validates via test endpoint first, then creates/updates

This page is ~200-250 lines of Python. The Starlark editor is the key differentiator from Coop's condition-tree builder.

### 9.4 Actions (`pages/actions.py`)

- Table: Name, Type (WEBHOOK/ENQUEUE_TO_MRT), Updated At
- Create/edit form: Name, Type dropdown, Config (JSON editor for webhook URL/headers or MRT queue selection)
- Type dropdown includes: `WEBHOOK`, `ENQUEUE_TO_MRT`
- `ENQUEUE_AUTHOR_TO_MRT` is NOT supported. Nest does not have this action type. In Coop, this action enqueued the content author (user) for review. In Nest, this behavior is achieved directly in Starlark rules by calling `enqueue("queue-name")` with the author's user ID in the payload. The Starlark model subsumes this special-case action type.
- Delete with confirmation

### 9.5 Policies (`pages/policies.py`)

- Table: Name, Description, Parent, Strike Penalty, Version
- Create/edit form: Name, Description, Parent (dropdown of existing policies), Strike Penalty (number)
- Hierarchical display (indented rows or tree view)

### 9.6 Item Types (`pages/item_types.py`)

- Table: Name, Kind (CONTENT/USER/THREAD), Field Count, Updated At
- Create/edit form: Name, Kind dropdown, Schema editor (JSON or dynamic field builder), Field Roles
- Schema preview

### 9.7 MRT -- Manual Review Tool (`pages/mrt.py`)

Two sub-views:

**Queue list** (`/mrt`):
- Table of queues with columns: Name, Description, Default
- ADMIN users see a "Create Queue" button in the header and an "Archive" button on each queue row
- Create dialog has Name (required), Description (textarea), and Is Default (checkbox) fields
- Archive uses a confirmation dialog explaining soft-delete semantics ("No new jobs will be enqueued. Pending jobs can still be assigned.")
- Non-admin users see a read-only table with row-click navigation
- No PUT (update) endpoint exists yet -- queue editing is not supported in v1.0
- Click queue name -> shows jobs

**Job review** (`/mrt/queues/{id}` and `/mrt/jobs/{id}`):
- "Get Next Job" button -> calls `POST /api/v1/mrt/queues/{id}/assign`
- Job detail: item payload (rendered as formatted JSON via `ui.code(json.dumps(data, indent=2), language='json')`), enqueue source, source info, associated policies
- Decision form: Verdict dropdown (APPROVE/REJECT/ESCALATE/IGNORE), Action multi-select, Policy multi-select, Reason textarea
- Submit decision -> calls `POST /api/v1/mrt/decisions`

### 9.8 Text Banks (`pages/text_banks.py`)

- Bank list table: Name, Description, Entry Count
- Click bank -> shows entries
- Entry list with "Add Entry" form (value text input + is_regex checkbox)
- Delete entry button per row

### 9.9 Users (`pages/users.py`)

- Table: Name, Email, Role, Active, Created At
- Invite form: Email, Name, Role dropdown
- Edit role, deactivate user
- RBAC: only ADMIN role sees this page
- Note: Nest uses an invite-only model. There is no self-service signup/registration page. New users are created by an admin via the invite form or CLI seed.

### 9.10 API Keys (`pages/api_keys.py`)

- Table: Name, Prefix, Created At, Revoked At
- Create form: Name input -> shows key ONCE in a dialog (with copy button)
- Revoke button with confirmation

### 9.11 Signals (`pages/signals.py`)

- Table: ID, Display Name, Description, Eligible Inputs, Cost
- Test panel: select signal, enter input text, run test, display output (score, label, metadata)

### 9.12 Settings (`pages/settings.py`)

- Org name display
- Signing keys: list, rotate button
- Links to Users and API Keys pages
- This page is intentionally kept standalone rather than absorbed into other pages. It serves as the single entry point for org-level configuration. If it remains under ~40 lines, it could be absorbed into the dashboard in a future revision, but there is no urgency.
- Favicon: serve `static/favicon.ico` via NiceGUI's `app.add_static_files()`. Use a simple Nest logo or text icon.

---

## 10. Shared Components

### 10.1 Layout (`components/layout.py`)

The app shell wrapping every page (except login). Includes RBAC-aware sidebar filtering:

```python
# components/layout.py

from nicegui import ui, app


# Nav items with RBAC visibility rules.
# Format: (label, path, icon, min_role)
# min_role: 'ANALYST' = everyone, 'MODERATOR' = mod+admin, 'ADMIN' = admin only
_NAV_ITEMS = [
    ('Dashboard',   '/dashboard',   'home',        'ANALYST'),
    ('Rules',       '/rules',       'rule',        'ANALYST'),
    ('MRT Queues',  '/mrt',         'inbox',       'MODERATOR'),
    ('Actions',     '/actions',     'bolt',        'ANALYST'),
    ('Policies',    '/policies',    'policy',      'ANALYST'),
    ('Item Types',  '/item-types',  'category',    'MODERATOR'),
    ('Text Banks',  '/text-banks',  'text_fields', 'MODERATOR'),
    ('Signals',     '/signals',     'sensors',     'ANALYST'),
    ('Users',       '/users',       'people',      'ADMIN'),
    ('API Keys',    '/api-keys',    'key',         'ADMIN'),
    ('Settings',    '/settings',    'settings',    'ADMIN'),
]

_ROLE_RANK = {'ANALYST': 0, 'MODERATOR': 1, 'ADMIN': 2}


def _user_can_see(min_role: str) -> bool:
    """Check if current user's role meets the minimum role requirement."""
    user_role = app.storage.user.get('user', {}).get('role', '')
    return _ROLE_RANK.get(user_role, -1) >= _ROLE_RANK.get(min_role, 99)


async def confirm(message: str, title: str = 'Confirm') -> bool:
    """Show a confirmation dialog. Returns True if confirmed.

    This is the one justified shared utility. Confirmation dialogs require
    async await on a dialog result, which is non-trivial boilerplate.
    """
    with ui.dialog() as dialog, ui.card():
        ui.label(title).classes('text-h6')
        ui.label(message)
        with ui.row():
            ui.button('Cancel', on_click=lambda: dialog.submit(False))
            ui.button('Confirm', on_click=lambda: dialog.submit(True)) \
                .props('color=negative')
    return await dialog


def layout(title: str):
    """App shell with sidebar navigation and header.

    Usage:
        @ui.page('/rules')
        async def rules_page():
            with layout('Rules'):
                # page content here
                ui.label('Hello')
    """

    # Header
    with ui.header().classes('bg-primary text-white'):
        ui.label('Nest').classes('text-h6')
        ui.space()
        ui.label(app.storage.user.get('user', {}).get('name', ''))
        ui.button('Logout', on_click=logout)

    # Sidebar -- filtered by current user's role
    with ui.left_drawer().classes('bg-grey-2'):
        for label, path, icon, min_role in _NAV_ITEMS:
            if _user_can_see(min_role):
                ui.link(label, path).classes('block py-2 px-4')

    # Content area -- caller fills this via context manager
    return ui.column().classes('w-full p-4')
```

~60 lines. Every page uses it via `with layout('Page Title'):`.

### 10.2 Starlark Editor (`components/starlark_editor.py`)

Wraps NiceGUI's code editor with Starlark-specific features:

```python
# components/starlark_editor.py

from nicegui import ui


def starlark_editor(
    value: str = '',
    on_change=None,
    udfs: list[dict] | None = None,
    signals: list[dict] | None = None,
) -> ui.codemirror:
    """Starlark code editor with UDF and signal reference sidebar.

    Args:
        value: Initial Starlark source code.
        on_change: Callback when source changes.
        udfs: List of UDF definitions from GET /api/v1/udfs.
        signals: List of signal definitions from GET /api/v1/signals.

    Returns:
        The codemirror element for binding.
    """
    with ui.splitter(value=75).classes('w-full h-96') as splitter:
        with splitter.before:
            editor = ui.codemirror(value, language='python', theme='dark') \
                .classes('w-full h-full')
            if on_change:
                editor.on_change(on_change)

        with splitter.after:
            with ui.scroll_area().classes('h-full'):
                if udfs:
                    ui.label('Built-in UDFs').classes('text-bold')
                    for udf in udfs:
                        with ui.expansion(udf['name']).classes('w-full'):
                            ui.code(udf.get('signature', ''), language='python')
                            ui.label(udf.get('description', ''))

                if signals:
                    ui.label('Available Signals').classes('text-bold mt-4')
                    for sig in signals:
                        with ui.expansion(sig['display_name']).classes('w-full'):
                            ui.code(f'signal("{sig["id"]}", text)', language='python')
                            ui.label(sig.get('description', ''))

    return editor
```

~40 lines. Used only by `pages/rules.py`.

---

## 11. Entry Point

```python
# main.py

from nicegui import app, ui
from auth.middleware import require_auth
from pages import (
    login, dashboard, rules, actions, policies,
    item_types, mrt, text_banks, users, api_keys,
    signals, settings,
)
import os

# Configuration
NEST_API_URL = os.environ.get('NEST_API_URL', 'http://localhost:8080')
UI_PORT = int(os.environ.get('UI_PORT', '3000'))
UI_SECRET = os.environ.get('UI_SECRET', 'change-me-in-production')

# Storage secret for session persistence
app.storage_secret = UI_SECRET

# Static files (favicon)
app.add_static_files('/static', 'static')

# Mount all page modules
# Each module registers its own @ui.page routes
# login.py   -> /login
# dashboard.py -> /dashboard
# rules.py   -> /rules, /rules/new, /rules/{id}
# actions.py -> /actions, /actions/new, /actions/{id}
# policies.py -> /policies, /policies/new, /policies/{id}
# item_types.py -> /item-types, /item-types/new, /item-types/{id}
# mrt.py     -> /mrt, /mrt/queues/{id}, /mrt/jobs/{id}
# text_banks.py -> /text-banks, /text-banks/{id}
# users.py   -> /users
# api_keys.py -> /api-keys
# signals.py -> /signals
# settings.py -> /settings

# Root redirect
@ui.page('/')
async def root():
    ui.navigate.to('/dashboard')

ui.run(
    title='Nest',
    port=UI_PORT,
    favicon='static/favicon.ico',
    reload=os.environ.get('DEV', '') == '1',
    storage_secret=UI_SECRET,
)
```

~40 lines.

---

## 12. RBAC in the UI

The UI enforces role-based visibility using the stored user role:

| Page | ADMIN | MODERATOR | ANALYST |
|------|-------|-----------|---------|
| Dashboard | Yes | Yes | Yes |
| Rules | Full CRUD | View only | View only |
| MRT Queues | Yes | Yes | No |
| Actions | Full CRUD | View only | View only |
| Policies | Full CRUD | View only | View only |
| Item Types | Full CRUD | View only | No |
| Text Banks | Full CRUD | View only | No |
| Users | Full CRUD | No | No |
| API Keys | Full CRUD | No | No |
| Signals | Yes | Yes | Yes |
| Settings | Yes | No | No |

Enforcement is two layers:
1. **UI layer**: Hide create/edit/delete buttons based on role. Filter sidebar links based on role (see `layout.py` section 10.1). Hide entire pages for unauthorized roles.
2. **Backend layer**: Nest RBAC middleware rejects unauthorized requests with 403. The UI is advisory; the backend is authoritative.

```python
# auth/state.py

from nicegui import app


def user_role() -> str:
    """Return current user's role."""
    return app.storage.user.get('user', {}).get('role', '')


def can_edit(resource: str) -> bool:
    """Check if current user can edit a resource type."""
    return user_role() == 'ADMIN'


def is_moderator_or_above() -> bool:
    return user_role() in ('ADMIN', 'MODERATOR')
```

---

## 13. Coop Feature Parity -- v1.0 and Deferred

### v1.0 (Ships with Nest backend v1.0)

| Feature | Page | Endpoint Coverage |
|---------|------|-------------------|
| Login/Logout | `login.py` | `/auth/login`, `/auth/logout`, `/auth/me` |
| Dashboard (basic) | `dashboard.py` | Aggregated from other endpoints |
| Rule management (Starlark editor + test + templates) | `rules.py` | `/rules/*`, `/rules/test` |
| Action management | `actions.py` | `/actions/*` |
| Policy management | `policies.py` | `/policies/*` |
| Item type management | `item_types.py` | `/item-types/*` |
| MRT queues (create + archive + list) + job review + decisions | `mrt.py` | `/mrt/*` |
| Text bank management | `text_banks.py` | `/text-banks/*` |
| User management (invite-only) | `users.py` | `/users/*` |
| API key management | `api_keys.py` | `/api-keys/*` |
| Signal listing + testing | `signals.py` | `/signals/*` |
| Org settings + signing keys | `settings.py` | `/signing-keys/*` |
| UDF reference sidebar | `starlark_editor.py` | `/udfs` |

### Deferred to v1.1+ (when Nest backend v1.1 ships)

| Feature | Depends On | Rationale for Deferral |
|---------|-----------|------------------------|
| Analytics dashboard (charts) | `/analytics/*` endpoints | Backend analytics service is deferred to v1.1. No endpoints to call yet. |
| Investigation tool | `/investigation/*` endpoints | Backend investigation service is deferred to v1.1. |
| Reports management | `/reports` endpoint | Backend reports service is deferred to v1.1. |
| User strikes display | `user_strikes` backend | Backend user strikes table and service deferred to v1.1. |
| Bulk MRT actions | Backend bulk endpoints | Requires backend support for batch decision submission. |
| CSV export | Backend export endpoints | Requires backend to support CSV serialization or streaming. |
| Password reset flow | Full reset flow in backend | Requires email sending infrastructure not yet specified. |
| Location banks | Backend location bank CRUD | Nest v1.0 only supports text banks. Location banks (geographic coordinates/polygons) require new domain types, storage, and matching logic. |
| Hash banks | Backend hash bank CRUD | Nest v1.0 only supports text banks. Hash banks (perceptual hashing for images/video) require new domain types and hashing infrastructure. |
| SSO configuration UI | Backend SSO/OIDC support | Nest v1.0 uses email/password auth only. SSO requires OIDC provider integration in the backend. |
| Signup/registration page | N/A | Nest uses an invite-only model. No self-service registration. If public signup is needed later, it requires a new backend endpoint and email verification flow. |
| MRT analytics | `/analytics/queue-throughput` | Deferred with the analytics service. Queue throughput metrics depend on the same analytics pipeline. |
| Rule execution history view | `/analytics/rule-executions` | Requires analytics endpoints to query partitioned execution log tables. Partially available via direct SQL in v1.0. |
| Appeals workflow | Backend appeals service | Not present in Nest v1.0 scope. Requires new domain types (Appeal, AppealDecision) and queue integration. |
| Org management (multi-org) | Backend org CRUD for super-admins | Nest v1.0 supports multi-tenancy but org creation is a seed/CLI operation. A super-admin UI for managing multiple orgs is deferred. |
| MRT queue update UI | `PUT /api/v1/mrt/queues/{id}` | Create (`POST`) and archive (`DELETE`) are implemented. Update (rename, change description) requires a PUT endpoint that does not exist yet. |

---

## 14. Estimated Size

| Component | Lines of Code |
|-----------|--------------|
| `main.py` | ~40 |
| `api/client.py` | ~250 |
| `api/types.py` | ~120 |
| `auth/state.py` + `auth/middleware.py` | ~50 |
| `components/layout.py` | ~60 |
| `components/starlark_editor.py` | ~40 |
| `pages/login.py` | ~35 |
| `pages/dashboard.py` | ~45 |
| `pages/rules.py` (most complex) | ~250 |
| `pages/mrt.py` | ~160 |
| `pages/actions.py` | ~90 |
| `pages/policies.py` | ~90 |
| `pages/item_types.py` | ~90 |
| `pages/text_banks.py` | ~90 |
| `pages/users.py` | ~70 |
| `pages/api_keys.py` | ~70 |
| `pages/signals.py` | ~70 |
| `pages/settings.py` | ~45 |
| **Total** | **~1,665** |

For reference:
- Original Coop React frontend: ~30,000 lines
- Coop-lite proposed React frontend: ~5,600 lines
- **Nest UI (NiceGUI): ~1,665 lines**

That is 6% of the original and 30% of the lite version, while covering the same functional surface area for v1.0. The increase from the original ~1,470 estimate reflects typed update parameters (replacing `**kwargs`), RBAC sidebar filtering, error handling, template support, and the `confirm()` utility -- all of which were underspecified before.

---

## 15. How to Add a New Page

Adding a new feature/page requires exactly 3 steps:

**Step 1**: Add API methods to `api/client.py` (if new endpoints).

```python
# api/client.py -- add methods
async def list_reports(self) -> PaginatedResult[Report]:
    """GET /api/v1/reports"""
    resp = await self._http.get('/api/v1/reports')
    resp.raise_for_status()
    return PaginatedResult[Report](**resp.json())
```

**Step 2**: Create `pages/reports.py`.

```python
# pages/reports.py
from nicegui import ui, app
from components.layout import layout
from api.client import NestClient
from auth.middleware import require_auth

NEST_API_URL = os.environ.get('NEST_API_URL', 'http://localhost:8080')


@ui.page('/reports')
@require_auth
async def reports_page():
    client = NestClient(app.storage.user['http_client'])
    result = await client.list_reports()

    with layout('Reports'):
        ui.table(
            columns=[
                {'name': 'id', 'label': 'ID', 'field': 'id'},
                {'name': 'reason', 'label': 'Reason', 'field': 'reason'},
                {'name': 'created_at', 'label': 'Created', 'field': 'created_at'},
            ],
            rows=[vars(r) for r in result.items],
            row_key='id',
            pagination=25,
        ).classes('w-full')
```

**Step 3**: Import the module in `main.py`.

```python
from pages import reports  # add this line
```

Done. The page is live at `/reports`. No routing config, no component registration, no state management boilerplate.

---

## 16. Backend Gaps

The following backend endpoints are assumed by this UI design but are NOT listed in NEST_DESIGN.md section 10 (API Design). They must be added to the backend before the corresponding UI pages can function:

| Missing Endpoint | Needed By | Notes |
|------------------|-----------|-------|
| `GET /api/v1/policies/{id}` | `pages/policies.py` edit view | List endpoint exists but individual GET is missing. Required for edit page to load a single policy. |
| `GET /api/v1/item-types/{id}` | `pages/item_types.py` edit view | Same as above. Required for edit page to load a single item type. |

These are trivial additions to `internal/handler/config.go` -- they follow the exact same pattern as `GET /api/v1/rules/{id}` and `GET /api/v1/actions/{id}` which already exist.

---

## 17. Design Decisions

### Decision 1: Server-Rendered, Not SPA

**Decision**: NiceGUI renders UI server-side over WebSockets. No JavaScript bundle, no build step, no node_modules.

**Alternatives considered**:
- **React SPA with TypeScript types from OpenAPI**: Best DX for large teams. Rejected: requires JavaScript ecosystem, build tooling, and a separate deployment. Violates "Python as primary language" and "very lightweight" requirements.
- **HTMX + Jinja2**: Lightweight, no JS framework. Rejected: manual HTML templating is verbose for complex forms (rule editor, MRT decision panel). No built-in code editor component.

**Constraints**: The UI is an internal admin tool, not a public-facing app. <100 concurrent users. WebSocket overhead is negligible.

### Decision 2: No Local State Store

**Decision**: Each page fetches data from the API when it loads. No client-side cache, no global store. Every navigation triggers a fresh API call. This is intentional -- it guarantees the UI always shows current data without cache invalidation complexity.

**Alternatives considered**:
- **In-memory cache with TTL**: Cache API responses to reduce backend calls. Rejected: adds complexity (cache invalidation) for negligible benefit. An internal tool with <100 users does not need to optimize API call count.

**Constraints**: API calls are fast (Go backend, PostgreSQL). Network latency is minimal (same-machine or same-network deployment). Simplicity wins.

### Decision 3: One File Per Page

**Decision**: Each page is a single Python file in `pages/`. No sub-modules, no page-specific components directory, no page-specific types.

**Alternatives considered**:
- **Page directories** (`pages/rules/__init__.py`, `pages/rules/editor.py`, etc.): Rejected: premature structure. If a page grows beyond ~300 lines, split it then. Most pages are 40-90 lines.

**Constraints**: The entire UI is ~1,665 lines. Organizing 70-line files into directories adds navigational overhead without benefit.

### Decision 4: httpx Over requests

**Decision**: Use `httpx` for API calls.

**Alternatives considered**:
- **requests**: Synchronous. Rejected: NiceGUI is async-native. Blocking HTTP calls in an async framework cause latency.
- **aiohttp**: Viable. Rejected: `httpx` has a cleaner API, `requests`-like interface, and is the modern standard for async Python HTTP.

### Decision 5: Two Component Files, Not Six

**Decision**: `components/` contains only `layout.py` and `starlark_editor.py`. The four eliminated files (`data_table.py`, `form_helpers.py`, `json_viewer.py`, `confirm_dialog.py`) wrapped single NiceGUI calls in unnecessary abstractions.

**Alternatives considered**:
- **Keep all wrappers**: Provides uniform API. Rejected: `data_table()` wraps `ui.table()` with near-identical arguments. `json_viewer()` is two lines (`json.dumps` + `ui.code`). `form_helpers.py` wraps `ui.notify()` and `ui.button()`. These wrappers obscure what NiceGUI call is actually being made, harm searchability, and add files without adding value.
- **Zero component files**: Inline everything. Rejected: `starlark_editor.py` is ~40 lines of non-trivial layout (splitter, UDF panel, signal panel) used by one page. It earns its own file. `layout.py` defines the app shell used by every page. `confirm()` is kept in `layout.py` because async dialog submission is genuinely non-obvious boilerplate.

**Constraints**: Fewer files means less navigation, fewer imports, and a flatter learning curve. A new developer reads 2 component files instead of 6.

### Decision 6: Typed Update Parameters

**Decision**: All `update_*` methods use explicit keyword-only optional parameters instead of `**kwargs`.

**Alternatives considered**:
- **`**kwargs`**: Concise. Rejected: no type checking, no IDE autocomplete, no documentation of valid fields. A typo in a keyword silently sends wrong data to the API.
- **Typed dataclass**: e.g., `RuleUpdate(name=..., status=...)`. Viable but heavier. The keyword-only pattern achieves the same type safety with less boilerplate for 3-5 optional fields.

**Constraints**: `pyright --strict` must pass. `**kwargs` defeats static analysis.

---

## 18. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| NiceGUI WebSocket overhead at scale | Low | Low | This is an admin tool. If >100 concurrent moderators are a concern, deploy multiple UI instances behind a load balancer. NiceGUI supports this. |
| Starlark syntax highlighting imperfect | Medium | Low | NiceGUI Codemirror uses Python mode, which is close enough to Starlark. The differences (no classes, no imports, no generators) do not cause false errors in highlighting. |
| NiceGUI is a young framework | Medium | Medium | NiceGUI is actively maintained (1.4k+ GitHub stars, weekly releases). The API surface we use is small and stable (ui.page, ui.table, ui.codemirror, ui.input, ui.dialog). Migration to another framework would be straightforward given the ~1,665 line codebase. |
| No offline support | Low | Low | Admin tools require network access. Not a concern. |
| Mobile experience is poor | Low | Low | Admin tools are used on desktops. NiceGUI Quasar components are responsive but not optimized for mobile. Not a concern for v1.0. |
| Session token in server memory | Low | Medium | NiceGUI's `app.storage.user` is server-side, keyed by a browser cookie. The session token is never exposed to client-side JavaScript. This is more secure than a typical SPA storing tokens in localStorage. |
| httpx.AsyncClient not cleaned up | Low | Medium | The client is stored in `app.storage.user` and explicitly closed on logout. If the browser disconnects without logout, NiceGUI's session cleanup will eventually garbage-collect it. For defense-in-depth, the middleware validates sessions and clears stale storage on 401. |
| Backend missing GET endpoints for policies/item-types | High | Medium | Documented in section 16. These are trivial to add (same pattern as existing GET endpoints). Must be resolved before UI development begins. |

---

## 19. Invariants

1. **No business logic in the UI.** Every action calls a Nest REST endpoint. The UI never computes, validates, or transforms data beyond display formatting.
2. **No page imports from another page.** Pages are independent modules. Shared functionality lives in `components/` or `api/`.
3. **The API client is the only module that makes HTTP calls.** No page constructs raw HTTP requests.
4. **RBAC is advisory in the UI, authoritative in the backend.** The UI hides buttons and sidebar links but does not enforce access. The backend rejects unauthorized requests.
5. **Every page function is async.** All API calls use `await`. No blocking I/O.
6. **Two production dependencies maximum.** `nicegui` and `httpx`. Nothing else.
7. **No JavaScript written or maintained.** All UI logic is Python.
8. **No build step.** `python main.py` starts the app. No compilation, no bundling, no transpilation.
9. **One httpx.AsyncClient per session.** Created at login, stored in `app.storage.user`, closed at logout. Never created per page load.
10. **No `**kwargs` in NestClient.** All public API methods have fully typed parameters.
11. **Sidebar reflects user role.** Nav items are filtered by the RBAC table in section 12. An ANALYST never sees Users, API Keys, MRT, Item Types, Text Banks, or Settings links.

---

## 20. Development Workflow

### Setup

```bash
cd nest-ui
python -m venv .venv
source .venv/bin/activate
pip install nicegui httpx
```

### Run (development)

```bash
NEST_API_URL=http://localhost:8080 DEV=1 python main.py
```

Opens browser at `http://localhost:3000`. Hot reload on file changes.

### Run (production)

```bash
NEST_API_URL=http://nest-backend:8080 UI_SECRET=<random-secret> python main.py
```

Or via Docker:

```dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY pyproject.toml .
RUN pip install --no-cache-dir nicegui httpx
COPY . .
CMD ["python", "main.py"]
```

Image size: ~150MB (Python slim base + 25MB deps).

---

## 21. Validation Criteria

1. **Dependency count**: `pip list` shows exactly 2 direct dependencies (nicegui, httpx) plus their transitive deps.
2. **File count**: ~18 Python source files (down from ~20).
3. **Component file count**: Exactly 2 files in `components/` (`layout.py`, `starlark_editor.py`).
4. **Line count**: Total source under 1,800 lines.
5. **Startup**: `python main.py` serves the UI without any build step.
6. **Every Nest v1.0 endpoint is reachable**: Every endpoint listed in NEST_DESIGN.md section 10 (Internal API) has a corresponding `NestClient` method and a UI surface.
7. **No `**kwargs` in NestClient**: `grep '**kwargs' api/client.py` returns zero matches.
8. **Rule editor works**: A user can write Starlark source, see UDF/signal references, select a template, test against a sample event, and save -- all from the rules page.
9. **MRT workflow works**: A moderator can view queues, assign a job, see item details, and record a decision with verdict + actions + policies.
10. **No page imports another page**: `grep -r "from pages" pages/` shows only `__init__.py` imports.
11. **Auth guard works**: Navigating to any page without a session redirects to `/login`.
12. **RBAC sidebar filtering**: A MODERATOR user does not see Users, API Keys, or Settings in the sidebar. An ANALYST does not see MRT, Item Types, Text Banks, Users, API Keys, or Settings.
13. **Error handling**: A 401 response from any API call redirects to login. A 403 shows a warning notification. A 409 shows a conflict message.
14. **NestClient lifecycle**: `httpx.AsyncClient` is created once at login and closed at logout. `grep 'AsyncClient(' pages/` returns zero matches (client creation happens only in login flow).
15. **GET endpoints exist for all edit pages**: `get_action()`, `get_policy()`, `get_item_type()` methods exist in NestClient (pending backend gaps in section 16 being resolved).
