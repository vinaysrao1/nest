"""Starlark code editor component with UDF and signal reference sidebar."""

from collections.abc import Callable
from typing import Any

from nicegui import ui


def starlark_editor(
    value: str = "",
    on_change: Callable[[Any], None] | None = None,
    udfs: list[dict[str, str]] | None = None,
    signals: list[dict[str, str]] | None = None,
) -> ui.codemirror:
    """Starlark code editor with UDF and signal reference sidebar.

    Renders a split view: 75% code editor, 25% reference sidebar.
    Editor uses Python syntax highlighting (closest to Starlark).

    Pre-conditions: called within a NiceGUI page context.
    Post-conditions: returns the codemirror element for value binding.

    Args:
        value: Initial Starlark source code.
        on_change: Callback invoked when source changes.
        udfs: UDF descriptors from GET /api/v1/udfs (list of dicts with
              name, signature, description, example).
        signals: Signal descriptors from GET /api/v1/signals (list of dicts
                 with id, display_name, description).

    Returns:
        The codemirror editor element.
    """
    udfs_list = udfs or []
    signals_list = signals or []

    editor_element: ui.codemirror

    with ui.row().classes("w-full gap-0"):
        with ui.column().classes("w-3/4"):
            editor_element = ui.codemirror(
                value=value,
                language="python",
                on_change=on_change,
            ).classes("w-full h-96 font-mono text-sm border")

        with ui.column().classes("w-1/4 bg-grey-1 p-3 overflow-y-auto border-l"):
            if udfs_list:
                ui.label("Available UDFs").classes("text-subtitle2 font-bold mb-2")
                for udf in udfs_list:
                    with ui.expansion(udf.get("name", ""), icon="functions").classes("w-full mb-1"):
                        if udf.get("signature"):
                            ui.code(udf["signature"], language="python").classes("text-xs w-full")
                        if udf.get("description"):
                            ui.label(udf["description"]).classes("text-caption text-grey-7")
                        if udf.get("example"):
                            ui.code(udf["example"], language="python").classes("text-xs w-full mt-1")

            if signals_list:
                if udfs_list:
                    ui.separator().classes("my-2")
                ui.label("Available Signals").classes("text-subtitle2 font-bold mb-2")
                for signal in signals_list:
                    with ui.item().classes("w-full px-0"):
                        with ui.item_section().props("avatar"):
                            ui.icon("sensors").classes("text-sm")
                        with ui.item_section():
                            display = signal.get("display_name", signal.get("id", ""))
                            ui.item_label(display).classes("text-caption font-medium")
                            if signal.get("description"):
                                ui.item_label(signal["description"]).props("caption").classes("text-xs text-grey-6")

    return editor_element
