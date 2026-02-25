"""Policy CRUD pages: list, create, edit, delete.

Three routes:
  /policies         -- list table with create button (RBAC gated)
  /policies/new     -- create form with parent dropdown
  /policies/{id}    -- edit form with delete button
"""

from __future__ import annotations

import dataclasses

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout


@ui.page("/policies")
@require_auth
async def policies_list_page() -> None:
    """Render the policies list table.

    Loads all policies and displays Name, Description, Parent, Strike Penalty, Version.
    Row click navigates to the edit page.
    Create button visible to ADMIN only.
    """
    client = NestClient(get_http_client())
    try:
        result = await client.list_policies()
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        with layout("Policies"):
            ui.label(f"Error loading policies: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Policies"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("Policies"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Policies").classes("text-h5")
            if can_edit("policies"):
                ui.button(
                    "Create Policy",
                    icon="add",
                    on_click=lambda: ui.navigate.to("/policies/new"),
                )

        policies_table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "description", "label": "Description", "field": "description"},
                {"name": "parent_id", "label": "Parent ID", "field": "parent_id"},
                {"name": "strike_penalty", "label": "Strike Penalty", "field": "strike_penalty", "sortable": True},
                {"name": "version", "label": "Version", "field": "version", "sortable": True},
            ],
            rows=[dataclasses.asdict(p) for p in result.items],
            row_key="id",
            pagination=25,
        ).classes("w-full")
        policies_table.on("row-click", lambda e: ui.navigate.to(f"/policies/{e.args[1]['id']}"))


@ui.page("/policies/new")
@require_auth
async def policies_new_page() -> None:
    """Render the policy creation form.

    Fields: Name, Description, Parent (dropdown), Strike Penalty.
    On save: POST /api/v1/policies then navigate to /policies.
    """
    client = NestClient(get_http_client())
    try:
        existing = await client.list_policies()
    except (httpx.HTTPStatusError, httpx.ConnectError):
        existing_items = []
    else:
        existing_items = existing.items

    with layout("New Policy"):
        form: dict[str, object] = {
            "name": "",
            "description": "",
            "parent_id": None,
            "strike_penalty": 0,
        }

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.textarea("Description").bind_value(form, "description").classes("w-full")

        parent_options: dict[str | None, str] = {None: "(none)"}
        for p in existing_items:
            parent_options[p.id] = p.name
        ui.select(parent_options, label="Parent Policy").bind_value(form, "parent_id")

        ui.number("Strike Penalty", value=0, min=0).bind_value(form, "strike_penalty")

        async def save() -> None:
            try:
                await client.create_policy(
                    name=str(form["name"]),
                    description=str(form["description"]) or None,
                    parent_id=form["parent_id"] if form["parent_id"] else None,  # type: ignore[arg-type]
                    strike_penalty=int(form["strike_penalty"]),  # type: ignore[arg-type]
                )
                ui.notify("Policy created", type="positive")
                ui.navigate.to("/policies")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        ui.button("Save", on_click=save, icon="save").classes("mt-4")


@ui.page("/policies/{policy_id}")
@require_auth
async def policies_edit_page(policy_id: str) -> None:
    """Render the policy edit form.

    Pre-populates fields from the loaded policy.
    On save: PUT /api/v1/policies/{id}.
    Delete button shows confirm dialog then DELETE /api/v1/policies/{id}.

    Args:
        policy_id: The UUID of the policy to edit.
    """
    client = NestClient(get_http_client())
    try:
        policy = await client.get_policy(policy_id)
        existing = await client.list_policies()
    except httpx.HTTPStatusError as e:
        with layout("Policy"):
            ui.label(f"Error loading policy: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Policy"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout(f"Policy: {policy.name}"):
        form: dict[str, object] = {
            "name": policy.name,
            "description": policy.description,
            "parent_id": policy.parent_id,
            "strike_penalty": policy.strike_penalty,
        }

        ui.input("Name").bind_value(form, "name").classes("w-full")
        ui.textarea("Description").bind_value(form, "description").classes("w-full")

        parent_options: dict[str | None, str] = {None: "(none)"}
        for p in existing.items:
            if p.id != policy_id:
                parent_options[p.id] = p.name
        ui.select(parent_options, label="Parent Policy").bind_value(form, "parent_id")

        ui.number("Strike Penalty", min=0).bind_value(form, "strike_penalty")

        async def save() -> None:
            try:
                await client.update_policy(
                    policy_id,
                    name=str(form["name"]),
                    description=str(form["description"]) or None,
                    parent_id=form["parent_id"] if form["parent_id"] else None,  # type: ignore[arg-type]
                    strike_penalty=int(form["strike_penalty"]),  # type: ignore[arg-type]
                )
                ui.notify("Policy updated", type="positive")
                ui.navigate.to("/policies")
            except httpx.HTTPStatusError as e:
                if e.response.status_code == 422:
                    detail = e.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {e.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        async def delete() -> None:
            if await confirm("Delete this policy? This cannot be undone."):
                try:
                    await client.delete_policy(policy_id)
                    ui.notify("Policy deleted", type="positive")
                    ui.navigate.to("/policies")
                except httpx.HTTPStatusError as e:
                    ui.notify(f"Error: {e.response.text}", type="negative")
                except httpx.ConnectError:
                    ui.notify("Cannot reach API server", type="negative")

        with ui.row().classes("mt-4 gap-2"):
            ui.button("Save", on_click=save, icon="save")
            if can_edit("policies"):
                ui.button("Delete", on_click=delete, icon="delete").props("color=negative")
