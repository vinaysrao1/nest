"""User management page: list, invite, change role, deactivate (ADMIN only)."""

from __future__ import annotations

import dataclasses

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout


@ui.page("/users")
@require_auth
async def users_page() -> None:
    """Render the users management page.

    Pre-conditions: valid session in app.storage.user.
    Post-conditions: page rendered with user table and invite form.
    """
    client = NestClient(get_http_client())

    try:
        result = await client.list_users(page_size=100)
        rows: list[dict] = [dataclasses.asdict(u) for u in result.items]
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        ui.label(f"Error loading users: {e.response.text}")
        return
    except httpx.ConnectError:
        ui.label("Cannot reach API server")
        return

    with layout("Users"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Users").classes("text-h5")

        table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "email", "label": "Email", "field": "email", "sortable": True},
                {"name": "role", "label": "Role", "field": "role"},
                {"name": "is_active", "label": "Active", "field": "is_active"},
                {"name": "created_at", "label": "Created At", "field": "created_at", "sortable": True},
            ],
            rows=rows,
            row_key="id",
            pagination=25,
        ).classes("w-full")

        if can_edit("users"):
            with ui.expansion("Invite User", icon="person_add").classes("w-full mt-4"):
                form: dict[str, str] = {"email": "", "name": "", "role": "MODERATOR"}
                ui.input("Email").bind_value(form, "email").classes("w-full")
                ui.input("Name").bind_value(form, "name").classes("w-full")
                ui.select(["ADMIN", "MODERATOR", "ANALYST"], label="Role").bind_value(form, "role")

                async def invite() -> None:
                    try:
                        await client.invite_user(
                            email=form["email"],
                            name=form["name"],
                            role=form["role"],
                        )
                        ui.notify("User invited", type="positive")
                        refreshed = await client.list_users(page_size=100)
                        table.rows = [dataclasses.asdict(u) for u in refreshed.items]
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

                ui.button("Invite", on_click=invite, icon="send").classes("mt-2")

            with ui.expansion("Edit / Deactivate User", icon="edit").classes("w-full mt-2"):
                edit_form: dict[str, str] = {"user_id": "", "role": "MODERATOR"}
                user_options = {u["id"]: u["name"] for u in rows}
                ui.select(user_options, label="User").bind_value(edit_form, "user_id")
                ui.select(["ADMIN", "MODERATOR", "ANALYST"], label="New Role").bind_value(edit_form, "role")

                async def change_role() -> None:
                    if not edit_form["user_id"]:
                        ui.notify("Select a user", type="warning")
                        return
                    try:
                        await client.update_user(edit_form["user_id"], role=edit_form["role"])
                        ui.notify("Role updated", type="positive")
                        refreshed = await client.list_users(page_size=100)
                        table.rows = [dataclasses.asdict(u) for u in refreshed.items]
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

                async def deactivate() -> None:
                    if not edit_form["user_id"]:
                        ui.notify("Select a user", type="warning")
                        return
                    ok = await confirm("Deactivate this user?", "Confirm Deactivation")
                    if not ok:
                        return
                    try:
                        await client.deactivate_user(edit_form["user_id"])
                        ui.notify("User deactivated", type="positive")
                        refreshed = await client.list_users(page_size=100)
                        table.rows = [dataclasses.asdict(u) for u in refreshed.items]
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

                with ui.row().classes("gap-2 mt-2"):
                    ui.button("Change Role", on_click=change_role, icon="manage_accounts")
                    ui.button("Deactivate", on_click=deactivate, icon="person_off").props("color=negative")
