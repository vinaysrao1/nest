"""MRT (Manual Review Tool) pages for Nest UI.

Three routes:
- /mrt              Queue list
- /mrt/queues/{id}  Job list for a queue with assign-next-job
- /mrt/jobs/{id}    Job detail with decision form
"""

from __future__ import annotations

import dataclasses
import json
import urllib.parse
from typing import Any

import httpx
from nicegui import app, ui

from api.client import NestClient
from auth.middleware import get_http_client, remove_http_client, require_auth
from auth.state import can_edit
from components.layout import layout

# ---------------------------------------------------------------------------
# Queue list page
# ---------------------------------------------------------------------------


@ui.page("/mrt")
@require_auth
async def mrt_queues_page() -> None:
    """List all MRT queues. Clicking a row navigates to that queue's job list.

    ADMIN users see a Create Queue button and an Archive button on each row.
    """
    client = NestClient(get_http_client())

    try:
        queues = await client.list_mrt_queues()
    except httpx.HTTPStatusError as e:
        with layout("MRT Queues"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            else:
                ui.label(f"Error loading queues: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("MRT Queues"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    is_admin = can_edit("mrt_queues")

    with layout("MRT Queues"):
        if not is_admin:
            # Non-admin: simple table with row-click navigation
            ui.label("MRT Queues").classes("text-h5 mb-4")
            if not queues:
                ui.label("No queues configured.").classes("text-grey-7")
                return
            queues_table = ui.table(
                columns=[
                    {"name": "name", "label": "Name", "field": "name", "align": "left", "sortable": True},
                    {"name": "description", "label": "Description", "field": "description"},
                    {"name": "is_default", "label": "Default", "field": "is_default"},
                ],
                rows=[dataclasses.asdict(q) for q in queues],
                row_key="id",
            ).classes("w-full")
            queues_table.on("row-click", lambda e: ui.navigate.to(f"/mrt/queues/{e.args[1]['id']}"))
            return

        # ------------------------------------------------------------------
        # Admin view: Create Queue dialog
        # ------------------------------------------------------------------
        with ui.dialog() as create_dialog, ui.card().classes("w-96"):
            ui.label("Create Queue").classes("text-h6 mb-2")
            name_input = ui.input("Name", placeholder="Queue name").classes("w-full")
            desc_input = ui.textarea("Description").classes("w-full")
            default_check = ui.checkbox("Is Default", value=False)

            async def do_create() -> None:
                name_val = name_input.value or ""
                if not name_val.strip():
                    ui.notify("Queue name is required", type="warning")
                    return
                try:
                    await client.create_mrt_queue(
                        name=name_val.strip(),
                        description=desc_input.value or "",
                        is_default=default_check.value,
                    )
                    ui.notify("Queue created", type="positive")
                    create_dialog.close()
                    ui.navigate.to("/mrt")
                except httpx.HTTPStatusError as exc:
                    if exc.response.status_code == 401:
                        await remove_http_client()
                        app.storage.user.clear()
                        ui.navigate.to("/login")
                    elif exc.response.status_code == 409:
                        ui.notify("A queue with that name already exists", type="warning")
                    elif exc.response.status_code == 403:
                        ui.notify("Permission denied", type="warning")
                    else:
                        ui.notify(f"Error: {exc.response.text}", type="negative")
                except httpx.ConnectError:
                    ui.notify("Cannot reach API server", type="negative")

            with ui.row().classes("w-full justify-end gap-2 mt-4"):
                ui.button("Cancel", on_click=create_dialog.close).props("flat")
                ui.button("Create", on_click=do_create)

        # Header with Create button
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("MRT Queues").classes("text-h5")
            ui.button("Create Queue", icon="add", on_click=create_dialog.open)

        if not queues:
            ui.label("No queues configured.").classes("text-grey-7")
            return

        # ------------------------------------------------------------------
        # Archive confirmation dialog
        # ------------------------------------------------------------------
        archive_target: dict[str, str] = {"id": "", "name": ""}

        with ui.dialog() as archive_dialog, ui.card().classes("w-96"):
            ui.label("Archive Queue").classes("text-h6 mb-2")
            confirm_label = ui.label("")

            async def do_archive() -> None:
                try:
                    await client.archive_mrt_queue(archive_target["id"])
                    ui.notify("Queue archived", type="positive")
                    archive_dialog.close()
                    ui.navigate.to("/mrt")
                except httpx.HTTPStatusError as exc:
                    if exc.response.status_code == 401:
                        await remove_http_client()
                        app.storage.user.clear()
                        ui.navigate.to("/login")
                    elif exc.response.status_code == 404:
                        ui.notify("Queue not found or already archived", type="warning")
                    elif exc.response.status_code == 403:
                        ui.notify("Permission denied", type="warning")
                    else:
                        ui.notify(f"Error: {exc.response.text}", type="negative")
                except httpx.ConnectError:
                    ui.notify("Cannot reach API server", type="negative")

            with ui.row().classes("w-full justify-end gap-2 mt-4"):
                ui.button("Cancel", on_click=archive_dialog.close).props("flat")
                ui.button("Archive", on_click=do_archive).props("color=negative")

        def open_archive(queue_id: str, queue_name: str) -> None:
            archive_target["id"] = queue_id
            archive_target["name"] = queue_name
            confirm_label.set_text(
                f'Archive queue "{queue_name}"? No new jobs will be enqueued. '
                "Pending jobs can still be assigned."
            )
            archive_dialog.open()

        # Queue rows with archive buttons
        with ui.row().classes("w-full items-center border-b pb-1 mb-1"):
            ui.label("Name").classes("font-bold w-48")
            ui.label("Description").classes("font-bold flex-1")
            ui.label("Default").classes("font-bold w-20 text-center")
            ui.label("Actions").classes("font-bold w-24 text-right")

        for queue in queues:
            with ui.row().classes("w-full items-center border-b py-2 hover:bg-grey-1"):
                ui.label(queue.name).classes("w-48 cursor-pointer text-primary").on(
                    "click", lambda _q=queue: ui.navigate.to(f"/mrt/queues/{_q.id}")
                )
                ui.label(queue.description).classes("flex-1")
                ui.label("Yes" if queue.is_default else "No").classes("w-20 text-center")
                ui.button(
                    icon="archive",
                    on_click=lambda _q=queue: open_archive(_q.id, _q.name),
                ).props("flat dense color=negative size=sm").classes("w-24")


# ---------------------------------------------------------------------------
# Queue jobs page
# ---------------------------------------------------------------------------


@ui.page("/mrt/queues/{queue_id}")
@require_auth
async def mrt_queue_jobs_page(queue_id: str) -> None:
    """List jobs in a queue, with a button to assign the next pending job.

    Args:
        queue_id: UUID of the MRT queue.
    """
    client = NestClient(get_http_client())

    try:
        jobs_result = await client.list_mrt_jobs(queue_id)
    except httpx.HTTPStatusError as e:
        with layout("Queue Jobs"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            else:
                ui.label(f"Error loading jobs: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Queue Jobs"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    with layout("Queue Jobs"):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label("Queue Jobs").classes("text-h5")
            ui.button(
                "Back to Queues",
                icon="arrow_back",
                on_click=lambda: ui.navigate.to("/mrt"),
            ).props("flat")

        async def get_next() -> None:
            try:
                job = await client.assign_next_job(queue_id)
                if job is None:
                    ui.notify("No pending jobs", type="info")
                else:
                    ui.navigate.to(f"/mrt/jobs/{urllib.parse.quote(job.id, safe='')}")
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

        ui.button("Get Next Job", on_click=get_next, icon="assignment").classes("mb-4")

        def _serialize_job_row(j: object) -> dict[str, Any]:
            """Convert a job dataclass to a table-safe dict.

            Lists and dicts are not valid Quasar table cell values and cause
            rendering warnings, so policy_ids and source_info are stringified.
            """
            row: dict[str, Any] = dataclasses.asdict(j)  # type: ignore[arg-type]
            row["policy_ids"] = ", ".join(row.get("policy_ids", []) or [])
            if isinstance(row.get("source_info"), dict):
                row["source_info"] = json.dumps(row["source_info"])
            return row

        jobs_table = ui.table(
            columns=[
                {"name": "id", "label": "ID", "field": "id", "align": "left"},
                {"name": "status", "label": "Status", "field": "status", "sortable": True},
                {"name": "item_id", "label": "Item", "field": "item_id"},
                {"name": "enqueue_source", "label": "Source", "field": "enqueue_source"},
                {"name": "created_at", "label": "Created", "field": "created_at", "sortable": True},
            ],
            rows=[_serialize_job_row(j) for j in jobs_result.items],
            row_key="id",
            pagination=25,
        ).classes("w-full")
        jobs_table.on(
            "row-click",
            lambda e: ui.navigate.to(f"/mrt/jobs/{urllib.parse.quote(e.args[1]['id'], safe='')}"),
        )


# ---------------------------------------------------------------------------
# Job detail + decision page
# ---------------------------------------------------------------------------


@ui.page("/mrt/jobs/{job_id:path}")
@require_auth
async def mrt_job_page(job_id: str) -> None:
    """Display job payload and four action buttons for ASSIGNED jobs.

    Actions: Approve (green), Block (red), Skip (gray), Route (blue).
    Route requires selecting a target queue from a dropdown.
    After any action the user is navigated back to the queue jobs list.

    Args:
        job_id: UUID of the MRT job.
    """
    client = NestClient(get_http_client())

    try:
        job = await client.get_mrt_job(job_id)
        all_queues = await client.list_mrt_queues()
    except httpx.HTTPStatusError as e:
        with layout("Job Detail"):
            if e.response.status_code == 401:
                await remove_http_client()
                app.storage.user.clear()
                ui.navigate.to("/login")
            elif e.response.status_code == 404:
                ui.label("Job not found").classes("text-negative")
            else:
                ui.label(f"Error: {e.response.text}").classes("text-negative")
        return
    except httpx.ConnectError:
        with layout("Job Detail"):
            ui.label("Cannot reach API server").classes("text-negative")
        return

    short_id = job_id[:8]

    # Auto-claim: if job is PENDING, attempt to claim it for the current user.
    if job.status == "PENDING":
        try:
            job = await client.claim_mrt_job(job_id)
        except httpx.HTTPStatusError as claim_err:
            with layout(f"Job: {short_id}..."):
                if claim_err.response.status_code == 401:
                    await remove_http_client()
                    app.storage.user.clear()
                    ui.navigate.to("/login")
                elif claim_err.response.status_code == 409:
                    ui.label(f"Job: {short_id}...").classes("text-h5 mb-4")
                    ui.label("This job has already been claimed by another moderator.").classes(
                        "text-negative"
                    )
                    ui.button(
                        "Back to Queue",
                        icon="arrow_back",
                        on_click=lambda: ui.navigate.to(f"/mrt/queues/{job.queue_id}"),
                    ).props("flat")
                else:
                    ui.label(f"Error claiming job: {claim_err.response.text}").classes(
                        "text-negative"
                    )
            return
        except httpx.ConnectError:
            with layout(f"Job: {short_id}..."):
                ui.label("Cannot reach API server").classes("text-negative")
            return

    # Queues available for routing: active (not archived), excluding the current queue.
    route_queue_options: dict[str, str] = {
        q.id: q.name
        for q in all_queues
        if q.archived_at is None and q.id != job.queue_id
    }

    with layout(f"Job: {short_id}..."):
        with ui.row().classes("w-full justify-between items-center mb-4"):
            ui.label(f"Job: {short_id}...").classes("text-h5")
            ui.button(
                "Back to Queue",
                icon="arrow_back",
                on_click=lambda: ui.navigate.to(f"/mrt/queues/{job.queue_id}"),
            ).props("flat")

        with ui.row().classes("gap-4 mb-4 flex-wrap"):
            ui.label(f"Status: {job.status}").classes("text-subtitle2")
            ui.label(f"Source: {job.enqueue_source}").classes("text-subtitle2")
            ui.label(f"Queue: {job.queue_id[:8]}...").classes("text-subtitle2")
            ui.label(f"Item: {job.item_id[:8]}...").classes("text-subtitle2")

        ui.label("Item Payload").classes("text-subtitle1 font-bold mb-2")
        payload_json = json.dumps(job.payload, indent=2)
        ui.code(payload_json, language="json").classes("w-full")

        if job.status != "ASSIGNED":
            ui.separator().classes("my-4")
            ui.label(f"Job is {job.status} -- no decision can be recorded.").classes("text-grey-7")
            return

        ui.separator().classes("my-4")
        ui.label("Record Decision").classes("text-h6 mb-2")

        form_state: dict[str, Any] = {
            "reason": "",
            "target_queue_id": None,
        }

        notes_input = ui.textarea("Notes / Reason (optional)").classes("w-full mb-4")
        notes_input.bind_value(form_state, "reason")

        # Route queue selector -- only shown when Route is clicked (always rendered
        # but hidden until needed so bind_value works without deferred lookup).
        route_row = ui.row().classes("w-full mb-4 items-center gap-4")
        with route_row:
            if route_queue_options:
                queue_select = ui.select(
                    route_queue_options,
                    label="Target Queue",
                ).classes("flex-1")
                queue_select.bind_value(form_state, "target_queue_id")
            else:
                ui.label("No other active queues available for routing.").classes("text-grey-7")
        route_row.set_visibility(False)

        async def _submit(verdict: str) -> None:
            """Send the decision and navigate back to queue on success."""
            target: str | None = None
            if verdict == "ROUTE":
                target = form_state.get("target_queue_id")
                if not target:
                    ui.notify("Select a target queue before routing", type="warning")
                    return
            try:
                await client.record_decision(
                    job_id=job_id,
                    verdict=verdict,
                    reason=form_state["reason"],
                    target_queue_id=target,
                )
                ui.notify(f"{verdict.capitalize()} recorded", type="positive")
                ui.navigate.to(f"/mrt/queues/{job.queue_id}")
            except httpx.HTTPStatusError as exc:
                if exc.response.status_code == 401:
                    await remove_http_client()
                    app.storage.user.clear()
                    ui.navigate.to("/login")
                elif exc.response.status_code == 403:
                    ui.notify("Permission denied", type="warning")
                elif exc.response.status_code == 422:
                    detail = exc.response.json().get("detail", "Validation error")
                    ui.notify(f"Error: {detail}", type="negative")
                else:
                    ui.notify(f"Error: {exc.response.text}", type="negative")
            except httpx.ConnectError:
                ui.notify("Cannot reach API server", type="negative")

        async def _approve() -> None:
            await _submit("APPROVE")

        async def _block() -> None:
            await _submit("BLOCK")

        async def _skip() -> None:
            await _submit("SKIP")

        async def _confirm_route() -> None:
            await _submit("ROUTE")

        def _toggle_route_row() -> None:
            route_row.set_visibility(not route_row.visible)

        with ui.row().classes("gap-3 mt-2"):
            ui.button(
                "Approve",
                icon="check_circle",
                on_click=_approve,
            ).props("color=positive")
            ui.button(
                "Block",
                icon="block",
                on_click=_block,
            ).props("color=negative")
            ui.button(
                "Skip",
                icon="skip_next",
                on_click=_skip,
            ).props("color=grey-7")
            ui.button(
                "Route",
                icon="alt_route",
                on_click=_toggle_route_row,
            ).props("color=info")

        # Confirm route button appears below the queue selector row.
        with ui.row().classes("w-full mt-2"):
            confirm_route_btn = ui.button(
                "Confirm Route",
                icon="send",
                on_click=_confirm_route,
            ).props("color=info")
        confirm_route_btn.bind_visibility_from(route_row, "visible")
