"""Signals page: list all signals and test panel."""

from __future__ import annotations

import dataclasses
import json

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from components.layout import layout


@ui.page("/signals")
@require_auth
async def signals_page() -> None:
    """Render the signals page with a list table and an interactive test panel.

    Pre-conditions: valid session in app.storage.user.
    Post-conditions: page rendered with signal table and test panel.
    """
    client = NestClient(get_http_client())

    try:
        signals = await client.list_signals()
        rows: list[dict] = [dataclasses.asdict(s) for s in signals]
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        ui.label(f"Error loading signals: {e.response.text}")
        return
    except httpx.ConnectError:
        ui.label("Cannot reach API server")
        return

    with layout("Signals"):
        ui.label("Signals").classes("text-h5 mb-4")

        ui.table(
            columns=[
                {"name": "id", "label": "ID", "field": "id", "align": "left"},
                {"name": "display_name", "label": "Display Name", "field": "display_name", "sortable": True},
                {"name": "description", "label": "Description", "field": "description"},
                {"name": "eligible_inputs", "label": "Inputs", "field": "eligible_inputs"},
                {"name": "cost", "label": "Cost", "field": "cost", "sortable": True},
            ],
            rows=rows,
            row_key="id",
            pagination=25,
        ).classes("w-full mb-6")

        ui.label("Test Panel").classes("text-h6 mt-4 mb-2")

        signal_options: dict[str, str] = {s["id"]: s["display_name"] for s in rows}
        test_state: dict[str, str] = {
            "signal_id": signals[0].id if signals else "",
            "input_type": "text",
            "input_value": "",
        }

        signal_select = ui.select(
            signal_options,
            label="Signal",
            value=test_state["signal_id"],
        ).classes("w-full")
        signal_select.bind_value(test_state, "signal_id")

        input_type_select = ui.select(
            ["text", "url", "image_url"],
            label="Input Type",
            value="text",
        ).classes("w-full")
        input_type_select.bind_value(test_state, "input_type")

        input_value_area = ui.textarea("Input Value").classes("w-full font-mono")
        input_value_area.bind_value(test_state, "input_value")

        result_label = ui.label("").classes("font-mono whitespace-pre-wrap mt-2")

        async def run_test() -> None:
            if not test_state["signal_id"]:
                ui.notify("Select a signal", type="warning")
                return
            if not test_state["input_value"]:
                ui.notify("Enter an input value", type="warning")
                return
            try:
                output = await client.test_signal(
                    signal_id=test_state["signal_id"],
                    input_type=test_state["input_type"],
                    input_value=test_state["input_value"],
                )
                result_label.text = json.dumps(output, indent=2)
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

        ui.button("Run Test", on_click=run_test, icon="play_arrow").classes("mt-2")
