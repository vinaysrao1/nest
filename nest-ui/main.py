"""Entry point for the Nest Admin UI.

Reads environment variables, configures NiceGUI storage, imports all page
modules to trigger route registration, and starts the server.
"""

import os

from nicegui import app, ui

import pages.actions  # noqa: F401
import pages.api_keys  # noqa: F401
import pages.dashboard  # noqa: F401
import pages.item_types  # noqa: F401
import pages.login  # noqa: F401
import pages.mrt  # noqa: F401
import pages.policies  # noqa: F401
import pages.rules  # noqa: F401
import pages.settings  # noqa: F401
import pages.signals  # noqa: F401
import pages.text_banks  # noqa: F401
import pages.users  # noqa: F401

NEST_API_URL: str = os.environ.get("NEST_API_URL", "http://localhost:8080")
UI_PORT: int = int(os.environ.get("UI_PORT", "8090"))
UI_SECRET: str = os.environ.get("UI_SECRET", "change-me-in-production")

app.storage_secret = UI_SECRET


@ui.page("/")
async def root() -> None:
    """Root route: redirect to dashboard.

    Pre-conditions: none.
    Post-conditions: browser navigated to /dashboard.
    """
    ui.navigate.to("/dashboard")


ui.run(
    port=UI_PORT,
    title="Nest Admin",
    storage_secret=UI_SECRET,
    reload=os.environ.get("DEV", "") == "1",
)
