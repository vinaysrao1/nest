# Nest UI Guide

The Nest UI is a lightweight Python frontend (NiceGUI + httpx) for managing all aspects of the Nest content moderation platform. It is an internal admin tool -- no JavaScript, no build step, just `python main.py`.

This guide explains every page, what it does, and what each role can see.

---

## Roles

Three roles control what you can see and do:

| Role | Access |
|------|--------|
| **ADMIN** | Everything. Full CRUD on all resources. |
| **MODERATOR** | Dashboard, Rules (view), MRT Queues (review jobs), Actions (view), Policies (view), Item Types (view), Text Banks (view), Signals. |
| **ANALYST** | Dashboard, Rules (view), Actions (view), Policies (view), Signals. |

The sidebar only shows pages your role can access. The backend enforces permissions independently -- the UI hides buttons, but the server rejects unauthorized requests.

---

## Login (`/login`)

A standalone page with email and password fields. No sidebar or header.

- Enter your email and password, click **Login**.
- On success, you are redirected to the Dashboard.
- On failure, a notification shows "Invalid email or password."
- If the backend is unreachable, a notification says "Cannot reach API server."

There is no self-service registration. New accounts are created by an admin via the Users page or the CLI seed tool.

---

## Dashboard (`/dashboard`)

The landing page after login. Shows:

- **Welcome message** with your name and role.
- **Entity counts**: total rules, total users (fetched best-effort; silently skipped if unavailable).
- **Quick links**: buttons to Rules, MRT Queues, Actions, Policies.

This page is read-only for all roles.

---

## Rules (`/rules`, `/rules/new`, `/rules/{id}`)

The most complex page. This is where Starlark moderation rules are authored, tested, and managed.

### List View (`/rules`)

- A sortable table showing: Name, Status, Event Types, Priority, Tags, Updated At.
- **Filter by status**: dropdown to show only LIVE, BACKGROUND, or DISABLED rules.
- Click any row to open the edit page.
- **Create Rule** button (ADMIN only) navigates to `/rules/new`.

### Editor View (`/rules/new` or `/rules/{id}`)

The rule editor has these sections:

**Template selector** (top): A dropdown with starter templates:
- Blank rule -- minimal `evaluate` skeleton.
- Signal threshold -- single signal check with block/approve.
- Rate limiter -- counter-based rate limiting.
- Multi-signal -- combine two signals with AND logic.
- MRT routing -- enqueue borderline items for review.

Selecting a template replaces the editor content.

**Metadata fields**: Name, Status (LIVE/BACKGROUND/DISABLED), Tags (comma-separated), and Policy associations (multi-select dropdown).

**Starlark code editor**: A syntax-highlighted code editor (Python mode, closest to Starlark) with a side panel showing:
- **Built-in UDFs**: expandable list with signatures and descriptions (verdict, signal, counter, enqueue, memo, log, now, hash, regex_match).
- **Available Signals**: expandable list with usage examples (text-regex, text-bank, plus any custom HTTP signals).

**Test panel** (collapsible): Enter a JSON event, click **Run Test**, and see the verdict, reason, actions, logs, and latency without saving the rule.

**Save** button creates or updates the rule. **Delete** button (ADMIN, edit page only) with confirmation dialog.

See [RULES_ENGINE.md](RULES_ENGINE.md) for how to write rules for auto-action (approve/block) vs. enqueue (route to human review).

---

## MRT -- Manual Review Tool (`/mrt`, `/mrt/queues/{id}`, `/mrt/jobs/{id}`)

This is where moderators review flagged content and record decisions. Three sub-pages:

### Queue List (`/mrt`)

- Shows all MRT queues: Name, Description, Default flag.
- Click a queue name to see its jobs.
- **ADMIN only**: "Create Queue" button opens a dialog (Name, Description, Is Default). "Archive" button on each row with confirmation ("No new jobs will be enqueued. Pending jobs can still be assigned.").
- Non-admins see a read-only table.

### Job List (`/mrt/queues/{id}`)

- Table of jobs in the queue: ID, Status, Item, Source, Created At.
- **Get Next Job** button: atomically claims the next PENDING job and navigates to its detail page. If the queue is empty, shows "No pending jobs."
- Click any row to view the job detail.

### Job Detail and Decision (`/mrt/jobs/{id}`)

This is the core review workflow. The page shows:

> **Auto-claim behavior**: When you navigate directly to `/mrt/jobs/{id}` for a PENDING job (e.g., by clicking a row in the job list), the UI automatically claims the job for your account. The job status transitions from PENDING to ASSIGNED before the page renders. If the job is already ASSIGNED to another user, the decision form is hidden and a message shows the current status.

**Job metadata**: Status, Source (which rule enqueued it), Queue ID, Item ID.

**Item Payload**: The full content submitted for moderation, displayed as formatted JSON. This is the content the moderator reads and judges.

**Decision form** (only for ASSIGNED jobs): Four action buttons and an optional notes field.

| Button | Color | What It Does |
|--------|-------|--------------|
| **Approve** | Green | Records an APPROVE verdict. The item passes moderation. |
| **Block** | Red | Records a BLOCK verdict. The item is blocked. |
| **Skip** | Gray | Records a SKIP verdict. The item is returned to the queue or deferred. |
| **Route** | Blue | Shows a queue selector dropdown. Choose a target queue and click **Confirm Route** to move the job to a different queue for another team. |

**Notes / Reason**: Optional free-text field attached to every decision. Good practice for audit trails ("blocked because the image contains graphic violence").

After any action, you are navigated back to the queue's job list.

If the job is not ASSIGNED (e.g., already DECIDED or still PENDING), the decision form is hidden and a message explains the status.

**Typical moderator workflow:**
1. Navigate to your queue (`/mrt`).
2. Click **Get Next Job** to claim a pending item.
3. Read the item payload.
4. Click Approve, Block, Skip, or Route.
5. Repeat.

---

## Actions (`/actions`, `/actions/new`, `/actions/{id}`)

Actions are side-effects triggered when a rule fires. Two types:

| Type | What It Does |
|------|-------------|
| **WEBHOOK** | Sends an HTTP POST to a configured URL with the event payload. Used for notifications, logging to external systems, etc. |
| **ENQUEUE_TO_MRT** | Inserts the item into a named MRT queue. Config contains `{"queue_name": "..."}`. |

### List View (`/actions`)

- Table: Name, Type, Updated At.
- Click to edit. **Create Action** button (ADMIN only).

### Create/Edit (`/actions/new`, `/actions/{id}`)

- Fields: Name, Type dropdown, Config (JSON textarea).
- For WEBHOOK: config is `{"url": "https://...", "headers": {...}}`.
- For ENQUEUE_TO_MRT: config is `{"queue_name": "default"}`.
- **Delete** button (ADMIN, edit page only) with confirmation.

---

## Policies (`/policies`, `/policies/new`, `/policies/{id}`)

Policies are organizational rules that group moderation behavior. Rules can be associated with policies.

### List View (`/policies`)

- Table: Name, Description, Parent ID, Strike Penalty, Version.
- Click to edit. **Create Policy** button (ADMIN only).

### Create/Edit (`/policies/new`, `/policies/{id}`)

- Fields: Name, Description (textarea), Parent Policy (dropdown of existing policies -- supports hierarchy), Strike Penalty (number).
- **Delete** button (ADMIN, edit page only) with confirmation.

---

## Item Types (`/item-types`, `/item-types/new`, `/item-types/{id}`)

Item types define the schema for content submitted to Nest (posts, comments, user profiles, etc.).

### List View (`/item-types`)

- Table: Name, Kind, Updated At.
- Click to edit. **Create Item Type** button (ADMIN only).

### Create/Edit (`/item-types/new`, `/item-types/{id}`)

- Fields: Name, Kind (CONTENT/USER/THREAD), Schema (JSON textarea), Field Roles (JSON textarea).
- Schema defines the expected structure of submitted payloads.
- Field Roles maps fields to semantic roles (e.g., `{"body": "text", "author": "user_id"}`).
- **Delete** button (ADMIN, edit page only) with confirmation.

---

## Text Banks (`/text-banks`, `/text-banks/{id}`)

Text banks are lists of words/phrases/patterns used by the `text-bank` signal adapter. Rules check content against text banks to detect violations.

### List View (`/text-banks`)

- Table: Name, Description.
- Click to view entries. **Create Text Bank** button (ADMIN only) opens an inline dialog.

### Detail View (`/text-banks/{id}`)

- Bank name and description at the top.
- **Entries table**: Value, Is Regex, Created At.
- **Add Entry** (ADMIN, collapsible): text input + "Is Regex" checkbox. Regex entries use RE2 syntax.
- **Delete Entry** (ADMIN, collapsible): select an entry from a dropdown, confirm deletion.

---

## Signals (`/signals`)

Signals are the detection adapters available to rules. This page is read-only (no CRUD) -- signals are registered at server startup.

- **Table**: ID, Display Name, Description, Eligible Inputs, Cost.
- **Test Panel**: Select a signal, choose input type (text/url/image_url), enter input, click **Run Test**. Output shows the raw JSON response (score, label, metadata).

Useful for verifying that a signal adapter is working before writing a rule that depends on it.

---

## Users (`/users`) -- ADMIN only

Nest uses an invite-only model. No self-service registration.

- **Table**: Name, Email, Role, Active, Created At.
- **Invite User** (collapsible): Email, Name, Role dropdown. Click **Invite** to create the account.
- **Edit / Deactivate User** (collapsible): Select a user, change their role with **Change Role**, or click **Deactivate** (with confirmation) to disable their account.

---

## API Keys (`/api-keys`) -- ADMIN only

API keys authenticate programmatic access to the Nest REST API (e.g., for submitting items from your application).

- **Table**: Name, Prefix, Created At, Revoked At.
- **Create API Key** (collapsible): Enter a name, click **Create**. The full key is shown **once** in a dialog -- copy it immediately. It will not be shown again.
- **Revoke API Key** (collapsible): Select an active key, click **Revoke** (with confirmation). Revoked keys cannot be used and cannot be un-revoked.

---

## Settings (`/settings`) -- ADMIN only

Organization-level configuration.

- **Organisation Settings**: Displays current org configuration key-value pairs.
- **Signing Keys**: Table of RSA-PSS signing keys (ID, Public Key truncated, Active, Created At). Used for signing webhook payloads so receivers can verify authenticity.
- **Rotate Signing Key** button (with confirmation): Creates a new key and deactivates the old one.
- **Quick Links**: Buttons to Users and API Keys pages.
