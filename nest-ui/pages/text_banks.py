"""Text bank pages: list and detail with entry management.

Two routes:
  /text-banks        -- list table with create bank button (RBAC gated)
  /text-banks/{id}   -- bank detail with entries table, add/delete entry
"""

from __future__ import annotations

import dataclasses

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import confirm, layout


@ui.page("/text-banks")
@require_auth
async def text_banks_list_page() -> None:
    """Render the text banks list table.

    Loads all text banks and displays Name, Description.
    Row click navigates to the detail page.
    Create button visible to ADMIN only.
    """
    client = NestClient(get_http_client())
    try:
        banks = await client.list_text_banks()
    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            await remove_http_client()
            app.storage.user.clear()
            ui.navigate.to("/login")
            return
        with layout("Text Banks"):
            ui.label(f"Error loading text banks: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Text Banks"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("Text Banks"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Text Banks").classes("text-h5")
            if can_edit("text_banks"):
                _render_create_bank_button(client)

        text_banks_table = ui.table(
            columns=[
                {"name": "name", "label": "Name", "field": "name", "sortable": True, "align": "left"},
                {"name": "description", "label": "Description", "field": "description"},
            ],
            rows=[dataclasses.asdict(b) for b in banks],
            row_key="id",
            pagination=25,
        ).classes("w-full")
        text_banks_table.on("row-click", lambda e: ui.navigate.to(f"/text-banks/{e.args[1]['id']}"))


def _render_create_bank_button(client: NestClient) -> None:
    """Render an inline create-bank dialog button.

    Args:
        client: Authenticated NestClient instance.
    """
    create_form: dict[str, str] = {"name": "", "description": ""}

    async def do_create() -> None:
        try:
            await client.create_text_bank(
                name=create_form["name"],
                description=create_form["description"],
            )
            ui.notify("Text bank created", type="positive")
            ui.navigate.to("/text-banks")
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 422:
                detail = e.response.json().get("detail", "Validation error")
                ui.notify(f"Error: {detail}", type="negative")
            else:
                ui.notify(f"Error: {e.response.text}", type="negative")
        except httpx.ConnectError:
            ui.notify("Cannot reach API server", type="negative")

    with ui.dialog() as dialog, ui.card().classes("w-96"):
        ui.label("Create Text Bank").classes("text-h6")
        ui.input("Name").bind_value(create_form, "name").classes("w-full")
        ui.textarea("Description").bind_value(create_form, "description").classes("w-full")
        with ui.row().classes("gap-2 mt-2"):
            ui.button("Cancel", on_click=dialog.close)
            ui.button("Create", on_click=do_create).props("color=primary")

    ui.button("Create Text Bank", icon="add", on_click=dialog.open)


@ui.page("/text-banks/{bank_id}")
@require_auth
async def text_bank_detail_page(bank_id: str) -> None:
    """Render the text bank detail page.

    Shows bank metadata, lists entries in a table, and provides
    an add-entry form. Each entry row has a delete button.

    Args:
        bank_id: The UUID of the text bank to display.
    """
    client = NestClient(get_http_client())
    try:
        bank = await client.get_text_bank(bank_id)
        entries = await client.list_text_bank_entries(bank_id)
    except httpx.HTTPStatusError as e:
        with layout("Text Bank"):
            ui.label(f"Error loading text bank: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Text Bank"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout(f"Text Bank: {bank.name}"):
        ui.label(bank.description).classes("text-subtitle1 text-grey-7 mb-4")

        ui.label("Entries").classes("text-h6 mt-2 mb-2")

        entry_rows = [dataclasses.asdict(e) for e in entries]

        ui.table(
            columns=[
                {"name": "value", "label": "Value", "field": "value", "align": "left"},
                {"name": "is_regex", "label": "Regex", "field": "is_regex"},
                {"name": "created_at", "label": "Created", "field": "created_at", "sortable": True},
            ],
            rows=entry_rows,
            row_key="id",
            pagination=50,
        ).classes("w-full")

        if can_edit("text_banks"):
            ui.separator().classes("mt-4 mb-4")

            with ui.expansion("Add Entry", icon="add").classes("w-full mt-2"):
                add_form: dict[str, object] = {"value": "", "is_regex": False}

                ui.input("Value").bind_value(add_form, "value").classes("w-full")
                ui.checkbox("Is Regex").bind_value(add_form, "is_regex")

                async def add_entry() -> None:
                    try:
                        await client.add_text_bank_entry(
                            bank_id=bank_id,
                            value=str(add_form["value"]),
                            is_regex=bool(add_form["is_regex"]),
                        )
                        ui.notify("Entry added", type="positive")
                        ui.navigate.to(f"/text-banks/{bank_id}")
                    except httpx.HTTPStatusError as e:
                        if e.response.status_code == 422:
                            detail = e.response.json().get("detail", "Validation error")
                            ui.notify(f"Error: {detail}", type="negative")
                        else:
                            ui.notify(f"Error: {e.response.text}", type="negative")
                    except httpx.ConnectError:
                        ui.notify("Cannot reach API server", type="negative")

                ui.button("Add Entry", on_click=add_entry, icon="add").classes("mt-2")

            with ui.expansion("Delete Entry", icon="delete").classes("w-full mt-2"):
                delete_form: dict[str, str] = {"entry_id": ""}
                entry_options = {e["id"]: e["value"] for e in entry_rows}
                ui.select(entry_options, label="Select Entry").bind_value(delete_form, "entry_id")

                async def delete_entry() -> None:
                    if not delete_form["entry_id"]:
                        ui.notify("Select an entry to delete", type="warning")
                        return
                    entry_label = entry_options.get(delete_form["entry_id"], delete_form["entry_id"])
                    ok = await confirm(f"Delete entry '{entry_label}'?", "Confirm Deletion")
                    if not ok:
                        return
                    try:
                        await client.delete_text_bank_entry(bank_id, delete_form["entry_id"])
                        ui.notify("Entry deleted", type="positive")
                        ui.navigate.to(f"/text-banks/{bank_id}")
                    except httpx.HTTPStatusError as exc:
                        if exc.response.status_code == 401:
                            await remove_http_client()
                            app.storage.user.clear()
                            ui.navigate.to("/login")
                        else:
                            ui.notify(f"Error: {exc.response.text}", type="negative")
                    except httpx.ConnectError:
                        ui.notify("Cannot reach API server", type="negative")

                ui.button("Delete", on_click=delete_entry, icon="delete").props(
                    "color=negative"
                ).classes("mt-2")
