"""API key management page: list, create (show once), revoke."""

from __future__ import annotations

import dataclasses

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout


@ui.page("/api-keys")
@require_auth
async def api_keys_page() -> None:
    """Render the API keys management page.

    Pre-conditions: valid session in app.storage.user.
    Post-conditions: page rendered with API key table, create form, revoke buttons.
    """
    client = NestClient(get_http_client())

    try:
        keys = await client.list_api_keys()
        rows: list[dict] = [dataclasses.asdict(k) for k in keys]
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        ui.label(f"Error loading API keys: {e.response.text}")
        return
    except httpx.ConnectError:
        ui.label("Cannot reach API server")
        return

    with layout("API Keys"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("API Keys").classes("text-h5")

        table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "prefix", "label": "Prefix", "field": "prefix"},
                {"name": "created_at", "label": "Created At", "field": "created_at", "sortable": True},
                {"name": "revoked_at", "label": "Revoked At", "field": "revoked_at"},
            ],
            rows=rows,
            row_key="id",
            pagination=25,
        ).classes("w-full")

        async def refresh_table() -> None:
            refreshed = await client.list_api_keys()
            table.rows = [dataclasses.asdict(k) for k in refreshed]

        if can_edit("api_keys"):
            with ui.expansion("Create API Key", icon="add").classes("w-full mt-4"):
                name_input = ui.input("Key Name").classes("w-full")

                async def create_key() -> None:
                    if not name_input.value:
                        ui.notify("Enter a key name", type="warning")
                        return
                    try:
                        result = await client.create_api_key(name=name_input.value)
                        key = result.get("key", "")
                        with ui.dialog() as dialog, ui.card():
                            ui.label("API Key Created").classes("text-h6")
                            ui.label(
                                "Copy this key now. It will not be shown again."
                            ).classes("text-negative mb-2")
                            ui.input(value=key).classes("w-full font-mono").props("readonly outlined")
                            ui.button("Close", on_click=dialog.close).classes("mt-2")
                        dialog.open()
                        name_input.value = ""
                        await refresh_table()
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

                ui.button("Create", on_click=create_key, icon="key").classes("mt-2")

            with ui.expansion("Revoke API Key", icon="block").classes("w-full mt-2"):
                revoke_form: dict[str, str] = {"key_id": ""}
                active_options = {k["id"]: k["name"] for k in rows if k.get("revoked_at") is None}
                key_select = ui.select(active_options, label="Select Key").bind_value(revoke_form, "key_id")

                async def revoke_key() -> None:
                    if not revoke_form["key_id"]:
                        ui.notify("Select a key to revoke", type="warning")
                        return
                    ok = await confirm("Revoke this API key? This cannot be undone.", "Confirm Revocation")
                    if not ok:
                        return
                    try:
                        await client.revoke_api_key(revoke_form["key_id"])
                        ui.notify("API key revoked", type="positive")
                        await refresh_table()
                        refreshed_active = {k["id"]: k["name"] for k in table.rows if k.get("revoked_at") is None}
                        key_select.options = refreshed_active
                        revoke_form["key_id"] = ""
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

                ui.button("Revoke", on_click=revoke_key, icon="block").props("color=negative").classes("mt-2")
