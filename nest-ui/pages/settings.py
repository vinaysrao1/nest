"""Settings page: org settings display, signing keys list and rotation."""

from __future__ import annotations

import dataclasses
from typing import Any

import httpx
from nicegui import app, ui

from api.client import NestClient
from api.types import SigningKey
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout

_KEY_TRUNCATE_LEN: int = 40


def _truncate_public_key(row: dict[str, Any]) -> dict[str, Any]:
    """Return a copy of row with public_key truncated to _KEY_TRUNCATE_LEN chars.

    Pre-conditions: row is a dict, may contain 'public_key'.
    Post-conditions: returns new dict with truncated public_key value.

    Args:
        row: signing key row dict from dataclasses.asdict.
    """
    pk = str(row.get("public_key", ""))
    truncated = pk[:_KEY_TRUNCATE_LEN] + "..." if len(pk) > _KEY_TRUNCATE_LEN else pk
    return {**row, "public_key": truncated}


def _build_key_rows(keys: list[SigningKey]) -> list[dict[str, Any]]:
    """Convert SigningKey list to display rows with truncated public keys.

    Pre-conditions: keys is a list of SigningKey dataclasses.
    Post-conditions: returns list of dicts ready for ui.table rows.

    Args:
        keys: list of SigningKey instances.
    """
    return [_truncate_public_key(dataclasses.asdict(k)) for k in keys]


@ui.page("/settings")
@require_auth
async def settings_page() -> None:
    """Render the settings page with org settings and signing key management.

    Pre-conditions: valid session in app.storage.user.
    Post-conditions: page rendered with org settings, signing keys table, rotation button.
    """
    client = NestClient(get_http_client())

    org_settings: dict[str, Any] = {}
    try:
        org_settings = await client.get_org_settings()
    except httpx.HTTPStatusError:
        pass
    except httpx.ConnectError:
        pass

    try:
        signing_keys = await client.list_signing_keys()
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        ui.label(f"Error loading signing keys: {e.response.text}")
        return
    except httpx.ConnectError:
        ui.label("Cannot reach API server")
        return

    with layout("Settings"):
        ui.label("Settings").classes("text-h5 mb-4")

        with ui.card().classes("w-full mb-4 p-4"):
            ui.label("Organisation Settings").classes("text-subtitle1 font-bold mb-2")
            if org_settings:
                for setting_key, setting_val in org_settings.items():
                    ui.label(f"{setting_key}: {setting_val}").classes("text-body2")
            else:
                ui.label("No settings configured.").classes("text-grey-7")

        ui.label("Signing Keys").classes("text-subtitle1 font-bold mb-2")

        key_table = ui.table(
            columns=[
                {"name": "id", "label": "ID", "field": "id", "align": "left"},
                {"name": "public_key", "label": "Public Key (truncated)", "field": "public_key"},
                {"name": "is_active", "label": "Active", "field": "is_active"},
                {"name": "created_at", "label": "Created At", "field": "created_at", "sortable": True},
            ],
            rows=_build_key_rows(signing_keys),
            row_key="id",
            pagination=10,
        ).classes("w-full mb-4")

        if can_edit("settings"):
            async def rotate_key() -> None:
                ok = await confirm(
                    "Rotate the signing key? A new key will be created and the old one deactivated.",
                    "Confirm Key Rotation",
                )
                if not ok:
                    return
                try:
                    await client.rotate_signing_key()
                    ui.notify("Signing key rotated", type="positive")
                    refreshed = await client.list_signing_keys()
                    key_table.rows = _build_key_rows(refreshed)
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

            ui.button("Rotate Signing Key", on_click=rotate_key, icon="refresh").props("color=negative")

        ui.separator().classes("my-4")
        ui.label("Quick Links").classes("text-subtitle1 font-bold mb-2")
        with ui.row().classes("gap-2"):
            ui.button("Users", on_click=lambda: ui.navigate.to("/users"), icon="people")
            ui.button("API Keys", on_click=lambda: ui.navigate.to("/api-keys"), icon="key")
