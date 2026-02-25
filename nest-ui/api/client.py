"""Typed async HTTP client for the Nest REST API.

NestClient wraps every REST endpoint with a typed async method.  The caller
is responsible for creating and closing the underlying ``httpx.AsyncClient``;
this class never owns or closes it.
"""

from __future__ import annotations

import urllib.parse
from typing import Any, TypeVar

import httpx

from api.types import (
    UDF,
    Action,
    ApiKey,
    ItemType,
    MRTDecision,
    MRTJob,
    MRTQueue,
    PaginatedResult,
    Policy,
    Rule,
    Signal,
    SigningKey,
    TestResult,
    TextBank,
    TextBankEntry,
    User,
)

T = TypeVar("T")

# Protocol-compatible type alias for dataclasses with from_dict
_FromDict = Any


class NestClient:
    """Typed async HTTP client for the Nest REST API.

    Every method corresponds to one Nest endpoint.  The ``httpx.AsyncClient``
    is NOT owned by this class -- it is passed via constructor and shared
    across the session.

    Pre-conditions: ``http`` must be a configured ``httpx.AsyncClient`` with
        ``base_url`` set and session cookie attached.
    Post-conditions: All methods raise ``httpx.HTTPStatusError`` on non-2xx.

    Invariants:
    - No keyword-catch-all parameters anywhere.
    - All methods are async.
    - All return typed dataclasses (not raw dicts) except the six raw-dict methods
      documented below.
    - NestClient never creates or closes the ``httpx.AsyncClient``.
    """

    def __init__(self, http: httpx.AsyncClient) -> None:
        self._http = http

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _parse_paginated(self, data: dict[str, Any], item_cls: type[_FromDict]) -> PaginatedResult[Any]:
        """Parse a standard paginated API response envelope.

        Expected envelope shape:
        ``{"items": [...], "total": N, "page": N, "page_size": N, "total_pages": N}``

        Pre-conditions: ``item_cls`` has a ``from_dict`` classmethod.
        Post-conditions: returns a ``PaginatedResult`` with typed items.
        """
        items = [item_cls.from_dict(item) for item in data.get("items", [])]
        return PaginatedResult(
            items=items,
            total=data.get("total", 0),
            page=data.get("page", 1),
            page_size=data.get("page_size", 0),
            total_pages=data.get("total_pages", 0),
        )

    # ------------------------------------------------------------------
    # Auth
    # ------------------------------------------------------------------

    async def login(self, email: str, password: str) -> dict[str, Any]:
        """POST /api/v1/auth/login.

        Returns raw dict with ``user`` and ``csrf_token`` keys.  This is the
        one method that returns a raw dict because the login response is
        consumed by the login page to set up session state, not to render a
        typed domain object.

        Pre-conditions: ``email`` and ``password`` non-empty.
        Post-conditions: session cookie set by backend (httpx stores it).
        Raises: ``httpx.HTTPStatusError`` on 401 (bad credentials).
        """
        resp = await self._http.post(
            "/api/v1/auth/login",
            json={"email": email, "password": password},
        )
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    async def logout(self) -> None:
        """POST /api/v1/auth/logout.

        Pre-conditions: session must be active.
        Post-conditions: server session deleted, cookie cleared.
        """
        resp = await self._http.post("/api/v1/auth/logout")
        resp.raise_for_status()

    async def me(self) -> dict[str, Any]:
        """GET /api/v1/auth/me.

        Returns raw dict with ``user_id``, ``org_id``, ``role`` keys.
        Used by auth middleware for session validation.

        Pre-conditions: valid session.
        Post-conditions: returns identity dict.
        Raises: ``httpx.HTTPStatusError`` on 401.
        """
        resp = await self._http.get("/api/v1/auth/me")
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    # ------------------------------------------------------------------
    # Rules
    # ------------------------------------------------------------------

    async def list_rules(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Rule]:
        """GET /api/v1/rules?page={page}&page_size={page_size}.

        Pre-conditions: valid session.
        Post-conditions: returns paginated list of rules.
        """
        resp = await self._http.get("/api/v1/rules", params={"page": page, "page_size": page_size})
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), Rule)

    async def get_rule(self, rule_id: str) -> Rule:
        """GET /api/v1/rules/{rule_id}.

        Pre-conditions: ``rule_id`` non-empty.
        Post-conditions: returns Rule or raises 404.
        """
        resp = await self._http.get(f"/api/v1/rules/{rule_id}")
        resp.raise_for_status()
        return Rule.from_dict(resp.json())

    async def create_rule(
        self,
        name: str,
        status: str,
        source: str,
        tags: list[str] | None = None,
        policy_ids: list[str] | None = None,
    ) -> Rule:
        """POST /api/v1/rules.

        Pre-conditions: ``name``, ``status``, ``source`` non-empty.
        Post-conditions: rule created, returns Rule with generated ID.
        Raises: ``httpx.HTTPStatusError`` on 422 (compile error), 400 (validation).
        """
        body: dict[str, Any] = {"name": name, "status": status, "source": source}
        if tags is not None:
            body["tags"] = tags
        if policy_ids is not None:
            body["policy_ids"] = policy_ids
        resp = await self._http.post("/api/v1/rules", json=body)
        resp.raise_for_status()
        return Rule.from_dict(resp.json())

    async def update_rule(
        self,
        rule_id: str,
        *,
        name: str | None = None,
        status: str | None = None,
        source: str | None = None,
        tags: list[str] | None = None,
        policy_ids: list[str] | None = None,
    ) -> Rule:
        """PUT /api/v1/rules/{rule_id}.

        All keyword parameters are optional.  Only non-None values are sent.

        Pre-conditions: ``rule_id`` non-empty, at least one field non-None.
        Post-conditions: rule updated, returns updated Rule.
        """
        body: dict[str, Any] = {}
        if name is not None:
            body["name"] = name
        if status is not None:
            body["status"] = status
        if source is not None:
            body["source"] = source
        if tags is not None:
            body["tags"] = tags
        if policy_ids is not None:
            body["policy_ids"] = policy_ids
        resp = await self._http.put(f"/api/v1/rules/{rule_id}", json=body)
        resp.raise_for_status()
        return Rule.from_dict(resp.json())

    async def delete_rule(self, rule_id: str) -> None:
        """DELETE /api/v1/rules/{rule_id}.

        Pre-conditions: ``rule_id`` non-empty.
        Post-conditions: rule deleted. Returns None (204).
        """
        resp = await self._http.delete(f"/api/v1/rules/{rule_id}")
        resp.raise_for_status()

    async def test_rule(self, source: str, event: dict[str, Any]) -> TestResult:
        """POST /api/v1/rules/test.

        Pre-conditions: ``source`` non-empty, ``event`` has event_type,
            item_type, payload.
        Post-conditions: returns TestResult with verdict, actions, logs, latency.
        Raises: ``httpx.HTTPStatusError`` on 422 (compile error).
        """
        resp = await self._http.post("/api/v1/rules/test", json={"source": source, "event": event})
        resp.raise_for_status()
        return TestResult.from_dict(resp.json())

    async def test_existing_rule(self, rule_id: str, event: dict[str, Any]) -> TestResult:
        """POST /api/v1/rules/{rule_id}/test.

        Pre-conditions: ``rule_id`` non-empty, ``event`` dict with
            event_type/item_type/payload.
        Post-conditions: returns TestResult.
        """
        resp = await self._http.post(f"/api/v1/rules/{rule_id}/test", json={"event": event})
        resp.raise_for_status()
        return TestResult.from_dict(resp.json())

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    async def list_actions(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Action]:
        """GET /api/v1/actions?page={page}&page_size={page_size}."""
        resp = await self._http.get("/api/v1/actions", params={"page": page, "page_size": page_size})
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), Action)

    async def get_action(self, action_id: str) -> Action:
        """GET /api/v1/actions/{action_id}."""
        resp = await self._http.get(f"/api/v1/actions/{action_id}")
        resp.raise_for_status()
        return Action.from_dict(resp.json())

    async def create_action(
        self,
        name: str,
        action_type: str,
        config: dict[str, Any],
        item_type_ids: list[str] | None = None,
    ) -> Action:
        """POST /api/v1/actions."""
        body: dict[str, Any] = {"name": name, "action_type": action_type, "config": config}
        if item_type_ids is not None:
            body["item_type_ids"] = item_type_ids
        resp = await self._http.post("/api/v1/actions", json=body)
        resp.raise_for_status()
        return Action.from_dict(resp.json())

    async def update_action(
        self,
        action_id: str,
        *,
        name: str | None = None,
        action_type: str | None = None,
        config: dict[str, Any] | None = None,
        item_type_ids: list[str] | None = None,
    ) -> Action:
        """PUT /api/v1/actions/{action_id}.

        Only non-None values are sent.
        """
        body: dict[str, Any] = {}
        if name is not None:
            body["name"] = name
        if action_type is not None:
            body["action_type"] = action_type
        if config is not None:
            body["config"] = config
        if item_type_ids is not None:
            body["item_type_ids"] = item_type_ids
        resp = await self._http.put(f"/api/v1/actions/{action_id}", json=body)
        resp.raise_for_status()
        return Action.from_dict(resp.json())

    async def delete_action(self, action_id: str) -> None:
        """DELETE /api/v1/actions/{action_id}."""
        resp = await self._http.delete(f"/api/v1/actions/{action_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # Policies
    # ------------------------------------------------------------------

    async def list_policies(self, page: int = 1, page_size: int = 50) -> PaginatedResult[Policy]:
        """GET /api/v1/policies?page={page}&page_size={page_size}."""
        resp = await self._http.get("/api/v1/policies", params={"page": page, "page_size": page_size})
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), Policy)

    async def get_policy(self, policy_id: str) -> Policy:
        """GET /api/v1/policies/{policy_id}."""
        resp = await self._http.get(f"/api/v1/policies/{policy_id}")
        resp.raise_for_status()
        return Policy.from_dict(resp.json())

    async def create_policy(
        self,
        name: str,
        description: str | None = None,
        parent_id: str | None = None,
        strike_penalty: int = 0,
    ) -> Policy:
        """POST /api/v1/policies."""
        body: dict[str, Any] = {"name": name, "strike_penalty": strike_penalty}
        if description is not None:
            body["description"] = description
        if parent_id is not None:
            body["parent_id"] = parent_id
        resp = await self._http.post("/api/v1/policies", json=body)
        resp.raise_for_status()
        return Policy.from_dict(resp.json())

    async def update_policy(
        self,
        policy_id: str,
        *,
        name: str | None = None,
        description: str | None = None,
        parent_id: str | None = None,
        strike_penalty: int | None = None,
    ) -> Policy:
        """PUT /api/v1/policies/{policy_id}.

        Only non-None values are sent.
        """
        body: dict[str, Any] = {}
        if name is not None:
            body["name"] = name
        if description is not None:
            body["description"] = description
        if parent_id is not None:
            body["parent_id"] = parent_id
        if strike_penalty is not None:
            body["strike_penalty"] = strike_penalty
        resp = await self._http.put(f"/api/v1/policies/{policy_id}", json=body)
        resp.raise_for_status()
        return Policy.from_dict(resp.json())

    async def delete_policy(self, policy_id: str) -> None:
        """DELETE /api/v1/policies/{policy_id}."""
        resp = await self._http.delete(f"/api/v1/policies/{policy_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # Item Types
    # ------------------------------------------------------------------

    async def list_item_types(self, page: int = 1, page_size: int = 50) -> PaginatedResult[ItemType]:
        """GET /api/v1/item-types?page={page}&page_size={page_size}."""
        resp = await self._http.get("/api/v1/item-types", params={"page": page, "page_size": page_size})
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), ItemType)

    async def get_item_type(self, item_type_id: str) -> ItemType:
        """GET /api/v1/item-types/{item_type_id}."""
        resp = await self._http.get(f"/api/v1/item-types/{item_type_id}")
        resp.raise_for_status()
        return ItemType.from_dict(resp.json())

    async def create_item_type(
        self,
        name: str,
        kind: str,
        schema: dict[str, Any],
        field_roles: dict[str, Any] | None = None,
    ) -> ItemType:
        """POST /api/v1/item-types."""
        body: dict[str, Any] = {"name": name, "kind": kind, "schema": schema}
        if field_roles is not None:
            body["field_roles"] = field_roles
        resp = await self._http.post("/api/v1/item-types", json=body)
        resp.raise_for_status()
        return ItemType.from_dict(resp.json())

    async def update_item_type(
        self,
        item_type_id: str,
        *,
        name: str | None = None,
        kind: str | None = None,
        schema: dict[str, Any] | None = None,
        field_roles: dict[str, Any] | None = None,
    ) -> ItemType:
        """PUT /api/v1/item-types/{item_type_id}.

        Only non-None values are sent.
        """
        body: dict[str, Any] = {}
        if name is not None:
            body["name"] = name
        if kind is not None:
            body["kind"] = kind
        if schema is not None:
            body["schema"] = schema
        if field_roles is not None:
            body["field_roles"] = field_roles
        resp = await self._http.put(f"/api/v1/item-types/{item_type_id}", json=body)
        resp.raise_for_status()
        return ItemType.from_dict(resp.json())

    async def delete_item_type(self, item_type_id: str) -> None:
        """DELETE /api/v1/item-types/{item_type_id}."""
        resp = await self._http.delete(f"/api/v1/item-types/{item_type_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # MRT
    # ------------------------------------------------------------------

    async def list_mrt_queues(self) -> list[MRTQueue]:
        """GET /api/v1/mrt/queues.

        Response is wrapped in ``{"queues": [...]}``.
        """
        resp = await self._http.get("/api/v1/mrt/queues")
        resp.raise_for_status()
        return [MRTQueue.from_dict(q) for q in resp.json().get("queues", [])]

    async def create_mrt_queue(
        self,
        name: str,
        description: str = "",
        is_default: bool = False,
    ) -> MRTQueue:
        """POST /api/v1/mrt/queues.

        Pre-conditions: ``name`` non-empty.
        Post-conditions: queue created, returns MRTQueue with generated ID.
        Raises: ``httpx.HTTPStatusError`` on 400 (validation), 409 (conflict), 403 (forbidden).
        """
        body: dict[str, Any] = {
            "name": name,
            "description": description,
            "is_default": is_default,
        }
        resp = await self._http.post("/api/v1/mrt/queues", json=body)
        resp.raise_for_status()
        return MRTQueue.from_dict(resp.json())

    async def archive_mrt_queue(self, queue_id: str) -> None:
        """DELETE /api/v1/mrt/queues/{queue_id}.

        Pre-conditions: ``queue_id`` non-empty.
        Post-conditions: queue archived (soft-deleted). Returns None (204).
        Raises: ``httpx.HTTPStatusError`` on 404 (not found/already archived), 403 (forbidden).
        """
        resp = await self._http.delete(f"/api/v1/mrt/queues/{queue_id}")
        resp.raise_for_status()

    async def list_mrt_jobs(
        self,
        queue_id: str,
        status: str | None = None,
        page: int = 1,
        page_size: int = 50,
    ) -> PaginatedResult[MRTJob]:
        """GET /api/v1/mrt/queues/{queue_id}/jobs?status={status}&page={page}&page_size={page_size}."""
        params: dict[str, Any] = {"page": page, "page_size": page_size}
        if status is not None:
            params["status"] = status
        resp = await self._http.get(f"/api/v1/mrt/queues/{queue_id}/jobs", params=params)
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), MRTJob)

    async def assign_next_job(self, queue_id: str) -> MRTJob | None:
        """POST /api/v1/mrt/queues/{queue_id}/assign.

        Returns None when no pending jobs (204).
        """
        resp = await self._http.post(f"/api/v1/mrt/queues/{queue_id}/assign")
        resp.raise_for_status()
        if resp.status_code == 204:
            return None
        return MRTJob.from_dict(resp.json())

    async def get_mrt_job(self, job_id: str) -> MRTJob:
        """GET /api/v1/mrt/jobs/{job_id}.

        job_id is URL-encoded to handle IDs that may contain forward slashes
        (e.g. those derived from AT Protocol URIs).
        """
        encoded_id = urllib.parse.quote(job_id, safe="")
        resp = await self._http.get(f"/api/v1/mrt/jobs/{encoded_id}")
        resp.raise_for_status()
        return MRTJob.from_dict(resp.json())

    async def claim_mrt_job(self, job_id: str) -> MRTJob:
        """POST /api/v1/mrt/jobs/claim.

        Claims a specific PENDING job for the current user.
        If already assigned to the current user, returns it as-is (idempotent).

        Pre-conditions: job_id non-empty, valid session.
        Post-conditions: returns MRTJob with status ASSIGNED.
        Raises: httpx.HTTPStatusError on 404 (not found), 409 (claimed by other).
        """
        resp = await self._http.post(
            "/api/v1/mrt/jobs/claim",
            json={"job_id": job_id},
        )
        resp.raise_for_status()
        return MRTJob.from_dict(resp.json())

    async def record_decision(
        self,
        job_id: str,
        verdict: str,
        reason: str = "",
        action_ids: list[str] | None = None,
        policy_ids: list[str] | None = None,
        target_queue_id: str | None = None,
    ) -> MRTDecision:
        """POST /api/v1/mrt/decisions.

        Pre-conditions: job_id and verdict non-empty.
            target_queue_id required when verdict == "ROUTE".
        Post-conditions: decision recorded, returns MRTDecision.
        """
        body: dict[str, Any] = {
            "job_id": job_id,
            "verdict": verdict,
            "reason": reason,
            "action_ids": action_ids if action_ids is not None else [],
            "policy_ids": policy_ids if policy_ids is not None else [],
        }
        if target_queue_id is not None:
            body["target_queue_id"] = target_queue_id
        resp = await self._http.post("/api/v1/mrt/decisions", json=body)
        resp.raise_for_status()
        return MRTDecision.from_dict(resp.json())

    # ------------------------------------------------------------------
    # Users
    # ------------------------------------------------------------------

    async def list_users(self, page: int = 1, page_size: int = 50) -> PaginatedResult[User]:
        """GET /api/v1/users?page={page}&page_size={page_size}."""
        resp = await self._http.get("/api/v1/users", params={"page": page, "page_size": page_size})
        resp.raise_for_status()
        return self._parse_paginated(resp.json(), User)

    async def invite_user(self, email: str, name: str, role: str) -> User:
        """POST /api/v1/users/invite."""
        resp = await self._http.post("/api/v1/users/invite", json={"email": email, "name": name, "role": role})
        resp.raise_for_status()
        return User.from_dict(resp.json())

    async def update_user(
        self,
        user_id: str,
        *,
        name: str | None = None,
        role: str | None = None,
        is_active: bool | None = None,
    ) -> User:
        """PUT /api/v1/users/{user_id}.

        Only non-None values are sent.
        """
        body: dict[str, Any] = {}
        if name is not None:
            body["name"] = name
        if role is not None:
            body["role"] = role
        if is_active is not None:
            body["is_active"] = is_active
        resp = await self._http.put(f"/api/v1/users/{user_id}", json=body)
        resp.raise_for_status()
        return User.from_dict(resp.json())

    async def deactivate_user(self, user_id: str) -> None:
        """DELETE /api/v1/users/{user_id}."""
        resp = await self._http.delete(f"/api/v1/users/{user_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # API Keys
    # ------------------------------------------------------------------

    async def list_api_keys(self) -> list[ApiKey]:
        """GET /api/v1/api-keys.

        Response is wrapped in ``{"api_keys": [...]}``.
        """
        resp = await self._http.get("/api/v1/api-keys")
        resp.raise_for_status()
        return [ApiKey.from_dict(k) for k in resp.json().get("api_keys", [])]

    async def create_api_key(self, name: str) -> dict[str, Any]:
        """POST /api/v1/api-keys.

        Returns raw dict with ``key`` (plaintext, shown once) and ``api_key`` (metadata).
        """
        resp = await self._http.post("/api/v1/api-keys", json={"name": name})
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    async def revoke_api_key(self, key_id: str) -> None:
        """DELETE /api/v1/api-keys/{key_id}."""
        resp = await self._http.delete(f"/api/v1/api-keys/{key_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # Text Banks
    # ------------------------------------------------------------------

    async def list_text_banks(self) -> list[TextBank]:
        """GET /api/v1/text-banks.

        Response is wrapped in ``{"text_banks": [...]}``.
        """
        resp = await self._http.get("/api/v1/text-banks")
        resp.raise_for_status()
        return [TextBank.from_dict(b) for b in resp.json().get("text_banks", [])]

    async def create_text_bank(self, name: str, description: str = "") -> TextBank:
        """POST /api/v1/text-banks."""
        resp = await self._http.post("/api/v1/text-banks", json={"name": name, "description": description})
        resp.raise_for_status()
        return TextBank.from_dict(resp.json())

    async def get_text_bank(self, bank_id: str) -> TextBank:
        """GET /api/v1/text-banks/{bank_id}."""
        resp = await self._http.get(f"/api/v1/text-banks/{bank_id}")
        resp.raise_for_status()
        return TextBank.from_dict(resp.json())

    async def list_text_bank_entries(self, bank_id: str) -> list[TextBankEntry]:
        """GET /api/v1/text-banks/{bank_id}/entries.

        Response is wrapped in {"entries": [...]}.
        """
        resp = await self._http.get(f"/api/v1/text-banks/{bank_id}/entries")
        resp.raise_for_status()
        data = resp.json()
        return [TextBankEntry.from_dict(e) for e in data["entries"]]

    async def add_text_bank_entry(
        self,
        bank_id: str,
        value: str,
        is_regex: bool = False,
    ) -> TextBankEntry:
        """POST /api/v1/text-banks/{bank_id}/entries."""
        resp = await self._http.post(
            f"/api/v1/text-banks/{bank_id}/entries",
            json={"value": value, "is_regex": is_regex},
        )
        resp.raise_for_status()
        return TextBankEntry.from_dict(resp.json())

    async def delete_text_bank_entry(self, bank_id: str, entry_id: str) -> None:
        """DELETE /api/v1/text-banks/{bank_id}/entries/{entry_id}."""
        resp = await self._http.delete(f"/api/v1/text-banks/{bank_id}/entries/{entry_id}")
        resp.raise_for_status()

    # ------------------------------------------------------------------
    # Signals
    # ------------------------------------------------------------------

    async def list_signals(self) -> list[Signal]:
        """GET /api/v1/signals.

        Response is wrapped in ``{"signals": [...]}``.
        """
        resp = await self._http.get("/api/v1/signals")
        resp.raise_for_status()
        return [Signal.from_dict(s) for s in resp.json().get("signals", [])]

    async def test_signal(self, signal_id: str, input_type: str, input_value: str) -> dict[str, Any]:
        """POST /api/v1/signals/test.

        Returns raw SignalOutput dict with score, label, metadata.
        """
        resp = await self._http.post(
            "/api/v1/signals/test",
            json={"signal_id": signal_id, "input_type": input_type, "input_value": input_value},
        )
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    # ------------------------------------------------------------------
    # UDFs
    # ------------------------------------------------------------------

    async def list_udfs(self) -> list[UDF]:
        """GET /api/v1/udfs.

        Response is wrapped in ``{"udfs": [...]}``.
        """
        resp = await self._http.get("/api/v1/udfs")
        resp.raise_for_status()
        return [UDF.from_dict(u) for u in resp.json().get("udfs", [])]

    # ------------------------------------------------------------------
    # Signing Keys
    # ------------------------------------------------------------------

    async def list_signing_keys(self) -> list[SigningKey]:
        """GET /api/v1/signing-keys.

        Response is wrapped in ``{"signing_keys": [...]}``.
        """
        resp = await self._http.get("/api/v1/signing-keys")
        resp.raise_for_status()
        return [SigningKey.from_dict(k) for k in resp.json().get("signing_keys", [])]

    async def rotate_signing_key(self) -> SigningKey:
        """POST /api/v1/signing-keys/rotate."""
        resp = await self._http.post("/api/v1/signing-keys/rotate")
        resp.raise_for_status()
        return SigningKey.from_dict(resp.json())

    # ------------------------------------------------------------------
    # Org Settings
    # ------------------------------------------------------------------

    async def get_org_settings(self) -> dict[str, Any]:
        """GET /api/v1/orgs/settings.

        Returns raw settings map.
        """
        resp = await self._http.get("/api/v1/orgs/settings")
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]

    # ------------------------------------------------------------------
    # Health
    # ------------------------------------------------------------------

    async def health(self) -> dict[str, Any]:
        """GET /api/v1/health (public, no auth required)."""
        resp = await self._http.get("/api/v1/health")
        resp.raise_for_status()
        return resp.json()  # type: ignore[no-any-return]
