"""Login page for Nest UI.

Standalone page -- no auth guard, no layout wrapper.
Creates httpx.AsyncClient on successful login and stores via set_http_client().
"""

from __future__ import annotations

import os

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, set_http_client

NEST_API_URL: str = os.environ.get("NEST_API_URL", "http://localhost:8080")


@ui.page("/login")
async def login_page() -> None:
    """Render the login form.

    If already authenticated, redirects to /dashboard.
    On successful login: stores http_client via set_http_client() and sets
    app.storage.user["authenticated"] = True.
    On failure: shows error notification.
    """
    if app.storage.user.get("authenticated") and get_http_client() is not None:
        ui.navigate.to("/dashboard")
        return
    # Stale flag from a previous server session — clear it
    app.storage.user.pop("authenticated", None)

    form: dict[str, str] = {"email": "", "password": ""}

    async def do_login() -> None:
        """Submit credentials, create persistent http_client on success."""
        http_client = httpx.AsyncClient(base_url=NEST_API_URL, timeout=30.0)
        client = NestClient(http_client)
        try:
            result = await client.login(form["email"], form["password"])
            csrf_token = result.get("csrf_token", "")
            app.storage.user["user"] = result.get("user", {})
            app.storage.user["csrf_token"] = csrf_token
            app.storage.user["authenticated"] = True
            http_client.headers["X-CSRF-Token"] = csrf_token
            set_http_client(http_client)
            ui.navigate.to("/dashboard")
        except httpx.HTTPStatusError:
            await http_client.aclose()
            ui.notify("Invalid email or password", type="negative")
        except httpx.ConnectError:
            await http_client.aclose()
            ui.notify("Cannot reach API server", type="negative")

    with ui.card().classes("absolute-center w-96"):
        ui.label("Nest").classes("text-h4 text-center w-full")
        ui.label("Content Moderation Platform").classes(
            "text-subtitle2 text-center w-full text-grey-7 mb-4"
        )
        ui.input("Email", placeholder="you@example.com").bind_value(form, "email").classes("w-full")
        ui.input("Password", password=True, password_toggle_button=True).bind_value(
            form, "password"
        ).classes("w-full")
        ui.button("Login", on_click=do_login).classes("w-full mt-4")
