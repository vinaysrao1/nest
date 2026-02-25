"""App shell with RBAC-filtered sidebar, header, logout, and confirm dialog."""

from collections.abc import Generator
from contextlib import contextmanager

from nicegui import app, ui

from auth import state as auth_state
from auth.middleware import get_http_client, remove_http_client

# Nav items: (label, path, icon, min_role)
# min_role uses RBAC rank: ANALYST=0, MODERATOR=1, ADMIN=2
_NAV_ITEMS: list[tuple[str, str, str, str]] = [
    ("Dashboard", "/dashboard", "home", "ANALYST"),
    ("Rules", "/rules", "rule", "ANALYST"),
    ("MRT Queues", "/mrt", "inbox", "MODERATOR"),
    ("Actions", "/actions", "bolt", "ANALYST"),
    ("Policies", "/policies", "policy", "ANALYST"),
    ("Item Types", "/item-types", "category", "MODERATOR"),
    ("Text Banks", "/text-banks", "text_fields", "MODERATOR"),
    ("Signals", "/signals", "sensors", "ANALYST"),
    ("Users", "/users", "people", "ADMIN"),
    ("API Keys", "/api-keys", "key", "ADMIN"),
    ("Settings", "/settings", "settings", "ADMIN"),
]

_ROLE_RANK: dict[str, int] = {"ANALYST": 0, "MODERATOR": 1, "ADMIN": 2}


def _user_can_see(min_role: str) -> bool:
    """Check if current user's role meets the minimum role requirement.

    Pre-conditions: min_role is a valid role string.
    Post-conditions: returns True if user role rank >= min_role rank.

    Args:
        min_role: The minimum role required to see this nav item.
    """
    user_role = auth_state.user_role()
    return _ROLE_RANK.get(user_role, -1) >= _ROLE_RANK.get(min_role, 99)


async def confirm(message: str, title: str = "Confirm") -> bool:
    """Show a confirmation dialog. Returns True if user confirms.

    Pre-conditions: called within a NiceGUI page context.
    Post-conditions: returns True on confirm, False on cancel.

    Args:
        message: The confirmation message to display.
        title: The dialog title.
    """
    with ui.dialog() as dialog, ui.card():
        ui.label(title).classes("text-h6")
        ui.label(message)
        with ui.row():
            ui.button("Cancel", on_click=lambda: dialog.submit(False))
            ui.button("Confirm", on_click=lambda: dialog.submit(True)).props("color=negative")
    result: bool = await dialog
    return result


async def _logout() -> None:
    """Logout handler: close http_client, clear storage, redirect.

    Post-conditions: http_client closed, storage cleared, redirected to /login.
    """
    http_client = get_http_client()
    if http_client is not None:
        try:
            await http_client.post("/api/v1/auth/logout")
        except Exception:
            pass
    await remove_http_client()
    app.storage.user.clear()
    ui.navigate.to("/login")


@contextmanager
def layout(title: str) -> Generator[ui.column, None, None]:
    """App shell with sidebar navigation and header.

    Renders: header with app name, page title, and logout button.
    Sidebar: RBAC-filtered nav items.
    Yields: ui.column context manager for page content.

    Pre-conditions: called within a NiceGUI page context with active session.
    Post-conditions: app shell rendered, yields content column.

    Usage:
        @ui.page('/rules')
        @require_auth
        async def rules_page():
            with layout('Rules'):
                ui.label('Rules content')

    Args:
        title: The page title displayed in the header.
    """
    user_info: dict[str, object] = app.storage.user.get("user", {})  # type: ignore[assignment]
    user_name = str(user_info.get("name", "")) if user_info else ""

    with ui.header().classes("items-center justify-between bg-primary text-white px-4 py-2"):
        ui.label("Nest").classes("text-h6 font-bold")
        ui.label(title).classes("text-subtitle1")
        with ui.row().classes("items-center gap-2"):
            if user_name:
                ui.label(user_name).classes("text-caption")
            ui.button("Logout", on_click=_logout).props("flat color=white")

    with ui.left_drawer().classes("bg-grey-2 pt-4"):
        for label, path, icon, min_role in _NAV_ITEMS:
            if _user_can_see(min_role):
                with ui.item(on_click=lambda p=path: ui.navigate.to(p)).classes("cursor-pointer"):
                    with ui.item_section().props("avatar"):
                        ui.icon(icon)
                    with ui.item_section():
                        ui.item_label(label)

    with ui.column().classes("w-full p-4") as content:
        yield content
