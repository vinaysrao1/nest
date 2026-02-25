"""Auth guard middleware for NiceGUI page functions."""

import functools
from collections.abc import Callable, Coroutine
from typing import Any

import httpx
from nicegui import app, ui

# Module-level dict keyed by browser ID to store non-serializable AsyncClient objects.
# NiceGUI's app.storage.user is JSON-serialized; httpx.AsyncClient cannot be stored there.
_clients: dict[str, httpx.AsyncClient] = {}


def get_http_client() -> httpx.AsyncClient | None:
    """Return the AsyncClient for the current browser session, or None.

    Pre-conditions: called within a NiceGUI request context.
    Post-conditions: returns the client associated with app.storage.browser['id'], or None.
    """
    browser_id: str | None = app.storage.browser.get("id")
    if browser_id:
        return _clients.get(browser_id)
    return None


def set_http_client(client: httpx.AsyncClient) -> None:
    """Store an AsyncClient associated with the current browser session.

    Pre-conditions: called within a NiceGUI request context.
    Post-conditions: client is stored under app.storage.browser['id'].

    Args:
        client: The AsyncClient instance to store.
    """
    browser_id: str | None = app.storage.browser.get("id")
    if browser_id:
        _clients[browser_id] = client


async def remove_http_client() -> None:
    """Close and remove the AsyncClient for the current browser session.

    Pre-conditions: called within a NiceGUI request context.
    Post-conditions: client is closed and removed from _clients.
    """
    browser_id: str | None = app.storage.browser.get("id")
    if browser_id and browser_id in _clients:
        client = _clients.pop(browser_id)
        await client.aclose()


def require_auth(
    page_func: Callable[..., Coroutine[Any, Any, None]],
) -> Callable[..., Coroutine[Any, Any, None]]:
    """Auth guard decorator for NiceGUI page functions.

    Validates session on every page load by checking the module-level _clients
    dict for an http_client and calling GET /api/v1/auth/me.

    Pre-conditions: page_func is an async NiceGUI page handler.
    Post-conditions:
      - If no http_client in _clients for this browser: redirects to /login.
      - If token invalid (401) or connection error: clears storage and _clients entry,
        redirects to /login.
      - If valid: proceeds to page_func.

    Args:
        page_func: The async NiceGUI page handler to guard.

    Returns:
        Wrapped async function with auth validation.
    """

    @functools.wraps(page_func)
    async def wrapper(*args: Any, **kwargs: Any) -> None:
        http_client = get_http_client()
        if http_client is None:
            ui.navigate.to("/login")
            return

        try:
            resp = await http_client.get("/api/v1/auth/me")
            resp.raise_for_status()
        except (httpx.HTTPStatusError, httpx.ConnectError):
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return

        await page_func(*args, **kwargs)

    return wrapper
