"""Dashboard page for Nest UI.

Shows a welcome message, entity counts, and quick navigation links.
"""

from __future__ import annotations

from typing import Any

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, require_auth
from components.layout import layout


@ui.page("/dashboard")
@require_auth
async def dashboard_page() -> None:
    """Render the dashboard with entity counts and quick links.

    Fetches rules and user counts best-effort; silently ignores failures.
    """
    client = NestClient(get_http_client())
    user: dict[str, Any] = app.storage.user.get("user", {})

    rules_count = 0
    users_count = 0

    try:
        rules_result = await client.list_rules(page=1, page_size=1)
        rules_count = rules_result.total
    except (httpx.HTTPStatusError, httpx.ConnectError):
        pass

    try:
        users_result = await client.list_users(page=1, page_size=1)
        users_count = users_result.total
    except (httpx.HTTPStatusError, httpx.ConnectError):
        pass

    with layout("Dashboard"):
        ui.label(f"Welcome, {user.get('name', '')}").classes("text-h5")
        ui.label(f"Role: {user.get('role', '')}").classes("text-subtitle1 text-grey-7 mb-4")

        with ui.row().classes("gap-4 w-full flex-wrap"):
            with ui.card().classes("p-4 min-w-32"):
                ui.label("Rules").classes("text-subtitle2 text-grey-7")
                ui.label(str(rules_count)).classes("text-h4")
            with ui.card().classes("p-4 min-w-32"):
                ui.label("Users").classes("text-subtitle2 text-grey-7")
                ui.label(str(users_count)).classes("text-h4")

        ui.label("Quick Links").classes("text-h6 mt-6 mb-2")
        with ui.row().classes("gap-2 flex-wrap"):
            ui.button("Rules", on_click=lambda: ui.navigate.to("/rules"))
            ui.button("MRT Queues", on_click=lambda: ui.navigate.to("/mrt"))
            ui.button("Actions", on_click=lambda: ui.navigate.to("/actions"))
            ui.button("Policies", on_click=lambda: ui.navigate.to("/policies"))
