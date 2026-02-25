"""Item type CRUD pages: list, create, edit, delete.

Three routes:
  /item-types         -- list table with create button (RBAC gated)
  /item-types/new     -- create form with JSON schema editor
  /item-types/{id}    -- edit form with delete button
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


@ui.page("/item-types")
@require_auth
async def item_types_list_page() -> None:
    """Render the item types list table.

    Loads all item types and displays Name, Kind, Updated At.
    Row click navigates to the edit page.
    Create button visible to ADMIN only.
    """
    client = NestClient(get_http_client())
    try:
        result = await client.list_item_types()
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        with layout("Item Types"):
            ui.label(f"Error loading item types: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Item Types"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("Item Types"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Item Types").classes("text-h5")
            if can_edit("item_types"):
                ui.button(
                    "Create Item Type",
                    icon="add",
                    on_click=lambda: ui.navigate.to("/item-types/new"),
                )

        item_types_table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "kind", "label": "Kind", "field": "kind", "sortable": True},
                {"name": "updated_at", "label": "Updated", "field": "updated_at", "sortable": True},
            ],
            rows=[dataclasses.asdict(it) for it in result.items],
            row_key="id",
            pagination=25,
        ).classes("w-full")
        item_types_table.on("row-click", lambda e: ui.navigate.to(f"/item-types/{e.args[1]['id']}"))


@ui.page("/item-types/new")
@require_auth
async def item_types_new_page() -> None:
    """Render the item type creation form.

    Fields: Name, Kind (CONTENT | USER | THREAD), Schema JSON, Field Roles JSON.
    On save: POST /api/v1/item-types then navigate to /item-types.
    """
    client = NestClient(get_http_client())

    with layout("New Item Type"):
        form: dict[str, str] = {
            "name": "",
            "kind": "CONTENT",
            "schema": "{}",
            "field_roles": "{}",
        }

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.select(["CONTENT", "USER", "THREAD"], label="Kind").bind_value(form, "kind")
        ui.textarea("Schema (JSON)").bind_value(form, "schema").classes("w-full font-mono")
        ui.textarea("Field Roles (JSON)").bind_value(form, "field_roles").classes("w-full font-mono")

        async def save() -> None:
            try:
                schema = json.loads(form["schema"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in schema", type="negative")
                return
            try:
                field_roles = json.loads(form["field_roles"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in field roles", type="negative")
                return
            try:
                await client.create_item_type(
                    name=form["name"],
                    kind=form["kind"],
                    schema=schema,
                    field_roles=field_roles,
                )
                ui.notify("Item type created", type="positive")
                ui.navigate.to("/item-types")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        ui.button("Save", on_click=save, icon="save").classes("mt-4")


@ui.page("/item-types/{item_type_id}")
@require_auth
async def item_types_edit_page(item_type_id: str) -> None:
    """Render the item type edit form.

    Pre-populates fields from the loaded item type.
    On save: PUT /api/v1/item-types/{id}.
    Delete button shows confirm dialog then DELETE /api/v1/item-types/{id}.

    Args:
        item_type_id: The UUID of the item type to edit.
    """
    client = NestClient(get_http_client())
    try:
        item_type = await client.get_item_type(item_type_id)
    except httpx.HTTPStatusError as e:
        with layout("Item Type"):
            ui.label(f"Error loading item type: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Item Type"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout(f"Item Type: {item_type.name}"):
        form: dict[str, str] = {
            "name": item_type.name,
            "kind": item_type.kind,
            "schema": json.dumps(item_type.schema, indent=2),
            "field_roles": json.dumps(item_type.field_roles, indent=2),
        }

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.select(["CONTENT", "USER", "THREAD"], label="Kind").bind_value(form, "kind")
        ui.textarea("Schema (JSON)").bind_value(form, "schema").classes("w-full font-mono")
        ui.textarea("Field Roles (JSON)").bind_value(form, "field_roles").classes("w-full font-mono")

        async def save() -> None:
            try:
                schema = json.loads(form["schema"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in schema", type="negative")
                return
            try:
                field_roles = json.loads(form["field_roles"])
            except json.JSONDecodeError:
                ui.notify("Invalid JSON in field roles", type="negative")
                return
            try:
                await client.update_item_type(
                    item_type_id,
                    name=form["name"],
                    kind=form["kind"],
                    schema=schema,
                    field_roles=field_roles,
                )
                ui.notify("Item type updated", type="positive")
                ui.navigate.to("/item-types")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        async def delete() -> None:
            if await confirm("Delete this item type? This cannot be undone."):
                try:
                    await client.delete_item_type(item_type_id)
                    ui.notify("Item type deleted", type="positive")
                    ui.navigate.to("/item-types")
                except httpx.HTTPStatusError as e:
                    ui.notify(f"Error: {e.response.text}", type="negative")
                except httpx.ConnectError:
                    ui.notify("Cannot reach API server", type="negative")

        with ui.row().classes("mt-4 gap-2"):
            ui.button("Save", on_click=save, icon="save")
            if can_edit("item_types"):
                ui.button("Delete", on_click=delete, icon="delete").props("color=negative")
