"""RBAC state helpers reading from NiceGUI app.storage.user."""

from nicegui import app


def user_role() -> str:
    """Return current user's role from session storage.

    Pre-conditions: called within a NiceGUI page context.
    Post-conditions: returns 'ADMIN', 'MODERATOR', 'ANALYST', or '' if not logged in.
    """
    return app.storage.user.get("user", {}).get("role", "")  # type: ignore[no-any-return]


def can_edit(resource: str) -> bool:
    """Check if current user can create/edit/delete a resource type.

    Pre-conditions: called within a NiceGUI page context.
    Post-conditions: returns True only for ADMIN role.

    Args:
        resource: The resource type being accessed (e.g. 'rules', 'users').
    """
    return user_role() == "ADMIN"


def is_moderator_or_above() -> bool:
    """Check if current user is MODERATOR or ADMIN.

    Pre-conditions: called within a NiceGUI page context.
    Post-conditions: returns True for ADMIN or MODERATOR.
    """
    return user_role() in ("ADMIN", "MODERATOR")
