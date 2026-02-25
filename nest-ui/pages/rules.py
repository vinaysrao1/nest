"""Rules pages for Nest UI: list, create, and edit Starlark rules."""

from __future__ import annotations

import dataclasses
import json
from typing import Any

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import layout
from components.starlark_editor import starlark_editor

# ---------------------------------------------------------------------------
# Starlark rule templates
# ---------------------------------------------------------------------------

BLANK_TEMPLATE: str = '''def evaluate(event, signals):
    """Evaluate the event and return a verdict.

    Args:
        event: dict with event_type, item_type, payload
        signals: dict of signal results keyed by signal ID
    """
    return verdict("allow")
'''

SIGNAL_THRESHOLD_TEMPLATE: str = '''def evaluate(event, signals):
    """Block if a signal score exceeds a threshold."""
    score = signals.get("text-regex", {}).get("score", 0)
    if score > 0.8:
        return verdict("block", reason="Signal score too high: %s" % score)
    return verdict("allow")
'''

RATE_LIMITER_TEMPLATE: str = '''def evaluate(event, signals):
    """Rate-limit based on event count."""
    count = counter("user:%s" % event["payload"].get("user_id", ""),
                    window="1h")
    if count > 10:
        return verdict("block",
                       reason="Rate limit exceeded: %d events in 1h" % count)
    return verdict("allow")
'''

MULTI_SIGNAL_TEMPLATE: str = '''def evaluate(event, signals):
    """Combine two signals with AND logic."""
    regex_score = signals.get("text-regex", {}).get("score", 0)
    bank_score = signals.get("text-bank", {}).get("score", 0)

    if regex_score > 0.5 and bank_score > 0.5:
        return verdict("block",
                       reason="Multiple signals triggered",
                       actions=["webhook-1"])
    return verdict("allow")
'''

MRT_ROUTING_TEMPLATE: str = '''def evaluate(event, signals):
    """Route borderline items to manual review."""
    score = signals.get("text-regex", {}).get("score", 0)

    if score > 0.9:
        return verdict("block", reason="High confidence spam")
    elif score > 0.5:
        return verdict("enqueue",
                       reason="Borderline score: %s" % score,
                       actions=["enqueue-to-mrt"])
    return verdict("allow")
'''

TEMPLATES: dict[str, str] = {
    "Blank rule": BLANK_TEMPLATE,
    "Signal threshold": SIGNAL_THRESHOLD_TEMPLATE,
    "Rate limiter": RATE_LIMITER_TEMPLATE,
    "Multi-signal": MULTI_SIGNAL_TEMPLATE,
    "MRT routing": MRT_ROUTING_TEMPLATE,
}


# ---------------------------------------------------------------------------
# List page
# ---------------------------------------------------------------------------


@ui.page("/rules")
@require_auth
async def rules_list_page() -> None:
    """List all rules with status filter and create button."""
    client = NestClient(get_http_client())

    try:
        result = await client.list_rules()
    except httpx.HTTPStatusError as e:
        with layout("Rules"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            else:
                ui.label(f"Error loading rules: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Rules"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    all_rows: list[dict[str, Any]] = [dataclasses.asdict(r) for r in result.items]

    with layout("Rules"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Rules").classes("text-h5")
            if can_edit("rules"):
                ui.button(
                    "Create Rule",
                    icon="add",
                    on_click=lambda: ui.navigate.to("/rules/new"),
                )

        status_filter: dict[str, str] = {"value": ""}
        table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "status", "label": "Status", "field": "status", "sortable": True},
                {"name": "event_types", "label": "Event Types", "field": "event_types"},
                {"name": "priority", "label": "Priority", "field": "priority", "sortable": True},
                {"name": "tags", "label": "Tags", "field": "tags"},
                {"name": "updated_at", "label": "Updated", "field": "updated_at", "sortable": True},
            ],
            rows=all_rows,
            row_key="id",
            pagination=25,
        ).classes("w-full")
        table.on("row-click", lambda e: ui.navigate.to(f"/rules/{e.args[1]['id']}"))

        def apply_filter(e: Any) -> None:
            chosen = e.value if e.value else ""
            status_filter["value"] = chosen
            if chosen:
                table.rows = [r for r in all_rows if r.get("status") == chosen]
            else:
                table.rows = all_rows

        ui.select(
            ["", "LIVE", "BACKGROUND", "DISABLED"],
            label="Filter by status",
            on_change=apply_filter,
        ).classes("w-48 mt-2")


# ---------------------------------------------------------------------------
# New rule page
# ---------------------------------------------------------------------------


def _build_test_panel(client: NestClient, get_source: Any) -> None:
    """Render the test panel expansion section.

    Args:
        client: Active NestClient for API calls.
        get_source: Callable returning current editor source string.
    """
    with ui.expansion("Test Panel", icon="science").classes("w-full mt-4"):
        test_event = ui.textarea(
            "Test Event (JSON)",
            value='{\n  "event_type": "post.create",\n  "item_type": "Post",\n  "payload": {"body": "test content"}\n}',
        ).classes("w-full font-mono")
        test_output = ui.label("").classes("font-mono whitespace-pre-wrap")

        async def run_test() -> None:
            try:
                event = json.loads(test_event.value)
                test_result = await client.test_rule(source=get_source(), event=event)
                test_output.text = (
                    f"Verdict: {test_result.verdict}\n"
                    f"Reason: {test_result.reason}\n"
                    f"Actions: {test_result.actions}\n"
                    f"Logs: {test_result.logs}\n"
                    f"Latency: {test_result.latency_us}us"
                )
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in test event", type="negative")
            except httpx.HTTPStatusError as e:
                ui.notify(f"Test failed: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        ui.button("Run Test", on_click=run_test, icon="play_arrow")


@ui.page("/rules/new")
@require_auth
async def rules_new_page() -> None:
    """Render the create rule form with Starlark editor and test panel."""
    client = NestClient(get_http_client())

    udfs: list[Any] = []
    signals_list: list[Any] = []
    policies_result: Any = None

    try:
        udfs = await client.list_udfs()
        signals_list = await client.list_signals()
        policies_result = await client.list_policies()
    except httpx.HTTPStatusError as e:
        with layout("New Rule"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            else:
                ui.label(f"Error loading resources: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("New Rule"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("New Rule"):
        form: dict[str, Any] = {"name": "", "status": "LIVE", "tags": "", "policy_ids": []}

        editor_ref: list[Any] = []

        def on_template_change(e: Any) -> None:
            if e.value in TEMPLATES and editor_ref:
                editor_ref[0].value = TEMPLATES[e.value]

        ui.select(
            list(TEMPLATES.keys()),
            label="Start from template",
            on_change=on_template_change,
        ).classes("w-64 mb-4")

        with ui.row().classes("w-full gap-4 flex-wrap"):
            ui.input("Name").bind_value(form, "name").classes("flex-1 min-w-48")
            ui.select(
                ["LIVE", "BACKGROUND", "DISABLED"],
                label="Status",
            ).bind_value(form, "status")
            ui.input("Tags (comma-separated)").bind_value(form, "tags").classes("flex-1 min-w-48")

        policy_options: dict[str, str] = {}
        if policies_result:
            policy_options = {p.id: p.name for p in policies_result.items}
        if policy_options:
            ui.select(policy_options, label="Policies", multiple=True).bind_value(
                form, "policy_ids"
            ).classes("w-full")

        editor = starlark_editor(
            value=BLANK_TEMPLATE,
            udfs=[dataclasses.asdict(u) for u in udfs],
            signals=[dataclasses.asdict(s) for s in signals_list],
        )
        editor_ref.append(editor)

        _build_test_panel(client, lambda: editor.value)

        async def save() -> None:
            tags = [t.strip() for t in form["tags"].split(",") if t.strip()] if form["tags"] else None
            try:
                await client.create_rule(
                    name=form["name"],
                    status=form["status"],
                    source=editor.value,
                    tags=tags,
                    policy_ids=form["policy_ids"] or None,
                )
                ui.notify("Rule created", type="positive")
                ui.navigate.to("/rules")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 401:
                    await remove_http_client()
                    app.storage.user.clear()
                    ui.navigate.to("/login")
                elif e.response.status_code == 403:
                    ui.notify("Permission denied", type="warning")
                elif e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        ui.button("Save", on_click=save, icon="save").classes("mt-4")


# ---------------------------------------------------------------------------
# Edit rule page
# ---------------------------------------------------------------------------


@ui.page("/rules/{rule_id}")
@require_auth
async def rules_edit_page(rule_id: str) -> None:
    """Render the edit rule form pre-populated with existing rule data.

    Args:
        rule_id: UUID of the rule to edit.
    """
    client = NestClient(get_http_client())

    try:
        rule = await client.get_rule(rule_id)
        udfs = await client.list_udfs()
        signals_list = await client.list_signals()
        policies_result = await client.list_policies()
    except httpx.HTTPStatusError as e:
        with layout("Edit Rule"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            elif e.response.status_code == 404:
                ui.label("Rule not found").classes("text-negative")
            else:
                ui.label(f"Error: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Edit Rule"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout(f"Rule: {rule.name}"):
        form: dict[str, Any] = {
            "name": rule.name,
            "status": rule.status,
            "tags": ", ".join(rule.tags),
            "policy_ids": [],
        }

        editor_ref: list[Any] = []

        def on_template_change(e: Any) -> None:
            if e.value in TEMPLATES and editor_ref:
                editor_ref[0].value = TEMPLATES[e.value]

        ui.select(
            list(TEMPLATES.keys()),
            label="Load template",
            on_change=on_template_change,
        ).classes("w-64 mb-4")

        with ui.row().classes("w-full gap-4 flex-wrap"):
            ui.input("Name").bind_value(form, "name").classes("flex-1 min-w-48")
            ui.select(
                ["LIVE", "BACKGROUND", "DISABLED"],
                label="Status",
            ).bind_value(form, "status")
            ui.input("Tags (comma-separated)").bind_value(form, "tags").classes("flex-1 min-w-48")

        policy_options: dict[str, str] = {p.id: p.name for p in policies_result.items}
        if policy_options:
            ui.select(policy_options, label="Policies", multiple=True).bind_value(
                form, "policy_ids"
            ).classes("w-full")

        editor = starlark_editor(
            value=rule.source,
            udfs=[dataclasses.asdict(u) for u in udfs],
            signals=[dataclasses.asdict(s) for s in signals_list],
        )
        editor_ref.append(editor)

        _build_test_panel(client, lambda: editor.value)

        async def save() -> None:
            tags = [t.strip() for t in form["tags"].split(",") if t.strip()] if form["tags"] else None
            try:
                await client.update_rule(
                    rule_id,
                    name=form["name"],
                    status=form["status"],
                    source=editor.value,
                    tags=tags,
                    policy_ids=form["policy_ids"] or None,
                )
                ui.notify("Rule saved", type="positive")
                ui.navigate.to("/rules")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 401:
                    await remove_http_client()
                    app.storage.user.clear()
                    ui.navigate.to("/login")
                elif e.response.status_code == 403:
                    ui.notify("Permission denied", type="warning")
                elif e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        async def delete() -> None:
            from components.layout import confirm  # noqa: PLC0415

            confirmed = await confirm(
                f"Delete rule '{rule.name}'? This action cannot be undone.",
                title="Delete Rule",
            )
            if not confirmed:
                return
            try:
                await client.delete_rule(rule_id)
                ui.notify("Rule deleted", type="positive")
                ui.navigate.to("/rules")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 401:
                    await remove_http_client()
                    app.storage.user.clear()
                    ui.navigate.to("/login")
                elif e.response.status_code == 403:
                    ui.notify("Permission denied", type="warning")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        with ui.row().classes("mt-4 gap-2"):
            ui.button("Save", on_click=save, icon="save")
            if can_edit("rules"):
                ui.button("Delete", on_click=delete, icon="delete").props("color=negative")
