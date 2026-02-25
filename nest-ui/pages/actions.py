"""Action CRUD pages: list, create, edit, delete.

Three routes:
  /actions         -- list table with create button (RBAC gated)
  /actions/new     -- create form
  /actions/{id}    -- edit form with delete button
"""

from __future__ import annotations

import dataclasses
import json

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout


@ui.page("/actions")
@require_auth
async def actions_list_page() -> None:
    """Render the actions list table.

    Loads all actions and displays them in a sortable table.
    Row click navigates to the edit page.
    Create button visible to ADMIN only.
    """
    client = NestClient(get_http_client())
    try:
        result = await client.list_actions()
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        with layout("Actions"):
            ui.label(f"Error loading actions: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Actions"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("Actions"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Actions").classes("text-h5")
            if can_edit("actions"):
                ui.button(
                    "Create Action",
                    icon="add",
                    on_click=lambda: ui.navigate.to("/actions/new"),
                )

        actions_table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "action_type", "label": "Type", "field": "action_type", "sortable": True},
                {"name": "updated_at", "label": "Updated", "field": "updated_at", "sortable": True},
            ],
            rows=[dataclasses.asdict(a) for a in result.items],
            row_key="id",
            pagination=25,
        ).classes("w-full")
        actions_table.on("row-click", lambda e: ui.navigate.to(f"/actions/{e.args[1]['id']}"))


@ui.page("/actions/new")
@require_auth
async def actions_new_page() -> None:
    """Render the action creation form.

    Fields: Name, Type (WEBHOOK | ENQUEUE_TO_MRT), Config JSON.
    On save: POST /api/v1/actions then navigate to /actions.
    """
    client = NestClient(get_http_client())

    with layout("New Action"):
        form: dict[str, str] = {"name": "", "action_type": "WEBHOOK", "config": "{}"}

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.select(
            ["WEBHOOK", "ENQUEUE_TO_MRT"],
            label="Type",
        ).bind_value(form, "action_type")
        ui.textarea("Config (JSON)").bind_value(form, "config").classes("w-full font-mono")

        async def save() -> None:
            try:
                config = json.loads(form["config"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in config", type="negative")
                return
            try:
                await client.create_action(
                    name=form["name"],
                    action_type=form["action_type"],
                    config=config,
                )
                ui.notify("Action created", type="positive")
                ui.navigate.to("/actions")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        ui.button("Save", on_click=save, icon="save").classes("mt-4")


@ui.page("/actions/{action_id}")
@require_auth
async def actions_edit_page(action_id: str) -> None:
    """Render the action edit form.

    Pre-populates fields from the loaded action.
    On save: PUT /api/v1/actions/{id}.
    Delete button shows confirm dialog then DELETE /api/v1/actions/{id}.

    Args:
        action_id: The UUID of the action to edit.
    """
    client = NestClient(get_http_client())
    try:
        action = await client.get_action(action_id)
    except httpx.HTTPStatusError as e:
        with layout("Action"):
            ui.label(f"Error loading action: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Action"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout(f"Action: {action.name}"):
        form: dict[str, str] = {
            "name": action.name,
            "action_type": action.action_type,
            "config": json.dumps(action.config, indent=2),
        }

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.select(
            ["WEBHOOK", "ENQUEUE_TO_MRT"],
            label="Type",
        ).bind_value(form, "action_type")
        ui.textarea("Config (JSON)").bind_value(form, "config").classes("w-full font-mono")

        async def save() -> None:
            try:
                config = json.loads(form["config"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in config", type="negative")
                return
            try:
                await client.update_action(
                    action_id,
                    name=form["name"],
                    action_type=form["action_type"],
                    config=config,
                )
                ui.notify("Action updated", type="positive")
                ui.navigate.to("/actions")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        async def delete() -> None:
            if await confirm("Delete this action? This cannot be undone."):
                try:
                    await client.delete_action(action_id)
                    ui.notify("Action deleted", type="positive")
                    ui.navigate.to("/actions")
                except httpx.HTTPStatusError as e:
                    ui.notify(f"Error: {e.response.text}", type="negative")
                except httpx.ConnectError:
                    ui.notify("Cannot reach API server", type="negative")

        with ui.row().classes("mt-4 gap-2"):
            ui.button("Save", on_click=save, icon="save")
            if can_edit("actions"):
                ui.button("Delete", on_click=delete, icon="delete").props("color=negative")
