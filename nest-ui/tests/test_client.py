"""Tests for api/client.py NestClient.

Uses httpx MockTransport (HTTPX built-in transport mock) so no external
server is needed.  Each test constructs a minimal fake response and asserts
that NestClient sends the correct request and parses the response correctly.
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest

from api.client import NestClient
from api.types import (
    UDF,
    ApiKey,
    MRTDecision,
    MRTJob,
    MRTQueue,
    PaginatedResult,
    Rule,
    Signal,
    SigningKey,
    TestResult,
    TextBank,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_client(handler: httpx.MockTransport) -> NestClient:
    """Create a NestClient backed by an httpx mock transport."""
    http = httpx.AsyncClient(transport=handler, base_url="http://test")
    return NestClient(http)


def _json_response(data: Any, status_code: int = 200) -> httpx.Response:
    """Build an httpx.Response with JSON body."""
    return httpx.Response(
        status_code=status_code,
        headers={"content-type": "application/json"},
        content=json.dumps(data).encode(),
    )


def _empty_response(status_code: int = 204) -> httpx.Response:
    return httpx.Response(status_code=status_code)


_RULE_DATA: dict[str, Any] = {
    "id": "r-1",
    "org_id": "org-1",
    "name": "Test Rule",
    "status": "LIVE",
    "source": 'verdict("allow")',
    "event_types": ["post.create"],
    "priority": 10,
    "tags": ["spam"],
    "version": 2,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-02T00:00:00Z",
}

_USER_DATA: dict[str, Any] = {
    "id": "u-1",
    "org_id": "org-1",
    "email": "alice@example.com",
    "name": "Alice",
    "role": "ADMIN",
    "is_active": True,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z",
}

_PAGINATED_RULES: dict[str, Any] = {
    "items": [_RULE_DATA],
    "total": 1,
    "page": 1,
    "page_size": 50,
    "total_pages": 1,
}


# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_login_sends_correct_body() -> None:
    """login() sends email and password in POST body."""
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["method"] = request.method
        captured["url"] = str(request.url)
        captured["body"] = json.loads(request.content)
        return _json_response({"user": _USER_DATA, "csrf_token": "tok-abc"})

    client = _make_client(httpx.MockTransport(handler))
    await client.login("alice@example.com", "s3cret")
    assert captured["method"] == "POST"
    assert captured["url"].endswith("/api/v1/auth/login")
    assert captured["body"]["email"] == "alice@example.com"
    assert captured["body"]["password"] == "s3cret"


@pytest.mark.asyncio
async def test_login_returns_dict() -> None:
    """login() returns raw dict containing user and csrf_token keys."""
    payload = {"user": _USER_DATA, "csrf_token": "tok-xyz"}

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response(payload)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.login("alice@example.com", "pw")
    assert result["csrf_token"] == "tok-xyz"
    assert result["user"]["id"] == "u-1"


@pytest.mark.asyncio
async def test_logout_posts() -> None:
    """logout() sends POST to /api/v1/auth/logout."""
    captured: dict[str, str] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["method"] = request.method
        captured["path"] = request.url.path
        return _empty_response(204)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.logout()
    assert result is None
    assert captured["method"] == "POST"
    assert captured["path"] == "/api/v1/auth/logout"


@pytest.mark.asyncio
async def test_me_returns_dict() -> None:
    """me() returns dict with user_id, org_id, role."""
    identity = {"user_id": "u-1", "org_id": "org-1", "role": "ADMIN"}

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response(identity)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.me()
    assert result["user_id"] == "u-1"
    assert result["role"] == "ADMIN"


# ---------------------------------------------------------------------------
# Rules
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_rules_paginates() -> None:
    """list_rules() sends page and page_size params and returns PaginatedResult[Rule]."""
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["params"] = dict(request.url.params)
        return _json_response(_PAGINATED_RULES)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.list_rules(page=2, page_size=10)
    assert isinstance(result, PaginatedResult)
    assert captured["params"]["page"] == "2"
    assert captured["params"]["page_size"] == "10"
    assert len(result.items) == 1
    assert isinstance(result.items[0], Rule)
    assert result.items[0].id == "r-1"


@pytest.mark.asyncio
async def test_get_rule_by_id() -> None:
    """get_rule() issues GET /api/v1/rules/{id} and returns a Rule."""

    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/v1/rules/r-1"
        return _json_response(_RULE_DATA)

    client = _make_client(httpx.MockTransport(handler))
    rule = await client.get_rule("r-1")
    assert isinstance(rule, Rule)
    assert rule.id == "r-1"


@pytest.mark.asyncio
async def test_create_rule_sends_body() -> None:
    """create_rule() sends name, status, source, tags, policy_ids in POST body."""
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return _json_response(_RULE_DATA)

    client = _make_client(httpx.MockTransport(handler))
    await client.create_rule(
        name="Spam Rule",
        status="LIVE",
        source='verdict("block")',
        tags=["spam"],
        policy_ids=["p-1"],
    )
    assert captured["body"]["name"] == "Spam Rule"
    assert captured["body"]["status"] == "LIVE"
    assert captured["body"]["tags"] == ["spam"]
    assert captured["body"]["policy_ids"] == ["p-1"]


@pytest.mark.asyncio
async def test_update_rule_sends_only_non_none() -> None:
    """update_rule() omits None fields from the PUT body."""
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return _json_response(_RULE_DATA)

    client = _make_client(httpx.MockTransport(handler))
    await client.update_rule("r-1", name="New Name")
    assert "name" in captured["body"]
    assert "status" not in captured["body"]
    assert "source" not in captured["body"]
    assert "tags" not in captured["body"]


@pytest.mark.asyncio
async def test_delete_rule_returns_none() -> None:
    """delete_rule() returns None on 204."""

    def handler(request: httpx.Request) -> httpx.Response:
        return _empty_response(204)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.delete_rule("r-1")
    assert result is None


@pytest.mark.asyncio
async def test_test_rule_returns_test_result() -> None:
    """test_rule() POSTs source+event and returns TestResult."""
    tr_data: dict[str, Any] = {
        "verdict": "block",
        "reason": "matched",
        "rule_id": "",
        "actions": ["webhook-1"],
        "logs": [],
        "latency_us": 100,
    }
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return _json_response(tr_data)

    client = _make_client(httpx.MockTransport(handler))
    event = {"event_type": "post.create", "item_type": "Post", "payload": {"body": "hello"}}
    result = await client.test_rule(source='verdict("block")', event=event)
    assert isinstance(result, TestResult)
    assert result.verdict == "block"
    assert captured["body"]["source"] == 'verdict("block")'


# ---------------------------------------------------------------------------
# MRT
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_mrt_queues_unwraps() -> None:
    """list_mrt_queues() unwraps the ``queues`` key and returns list[MRTQueue]."""
    queue_data: dict[str, Any] = {
        "id": "q-1",
        "org_id": "org-1",
        "name": "Default",
        "description": "",
        "is_default": True,
        "archived_at": None,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"queues": [queue_data]})

    client = _make_client(httpx.MockTransport(handler))
    queues = await client.list_mrt_queues()
    assert len(queues) == 1
    assert isinstance(queues[0], MRTQueue)
    assert queues[0].id == "q-1"


@pytest.mark.asyncio
async def test_assign_next_job_returns_none_on_204() -> None:
    """assign_next_job() returns None when the server responds 204."""

    def handler(request: httpx.Request) -> httpx.Response:
        return _empty_response(204)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.assign_next_job("q-1")
    assert result is None


@pytest.mark.asyncio
async def test_assign_next_job_returns_job() -> None:
    """assign_next_job() returns MRTJob on 200."""
    job_data: dict[str, Any] = {
        "id": "j-1",
        "org_id": "org-1",
        "queue_id": "q-1",
        "item_id": "item-1",
        "item_type_id": "it-1",
        "payload": {},
        "status": "ASSIGNED",
        "assigned_to": "u-1",
        "policy_ids": [],
        "enqueue_source": "engine",
        "source_info": {},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response(job_data)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.assign_next_job("q-1")
    assert isinstance(result, MRTJob)
    assert result.id == "j-1"


@pytest.mark.asyncio
async def test_record_decision_sends_body() -> None:
    """record_decision() sends job_id, verdict, reason, action_ids, policy_ids."""
    captured: dict[str, Any] = {}
    decision_data: dict[str, Any] = {
        "id": "d-1",
        "org_id": "org-1",
        "job_id": "j-1",
        "user_id": "u-1",
        "verdict": "block",
        "action_ids": ["a-1"],
        "policy_ids": ["p-1"],
        "reason": "spam",
        "created_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return _json_response(decision_data)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.record_decision(
        job_id="j-1",
        verdict="block",
        reason="spam",
        action_ids=["a-1"],
        policy_ids=["p-1"],
    )
    assert isinstance(result, MRTDecision)
    assert captured["body"]["job_id"] == "j-1"
    assert captured["body"]["verdict"] == "block"
    assert captured["body"]["action_ids"] == ["a-1"]
    assert captured["body"]["policy_ids"] == ["p-1"]


@pytest.mark.asyncio
async def test_record_decision_with_target_queue_id() -> None:
    """record_decision() includes target_queue_id in the request body when provided."""
    captured: dict[str, Any] = {}
    decision_data: dict[str, Any] = {
        "id": "d-2",
        "org_id": "org-1",
        "job_id": "j-1",
        "user_id": "u-1",
        "verdict": "ROUTE",
        "action_ids": [],
        "policy_ids": [],
        "reason": "Wrong queue",
        "target_queue_id": "q-99",
        "created_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        return _json_response(decision_data)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.record_decision(
        job_id="j-1",
        verdict="ROUTE",
        reason="Wrong queue",
        target_queue_id="q-99",
    )
    assert isinstance(result, MRTDecision)
    assert captured["body"]["target_queue_id"] == "q-99"
    assert captured["body"]["verdict"] == "ROUTE"
    assert result.target_queue_id == "q-99"


@pytest.mark.asyncio
async def test_create_mrt_queue_sends_correct_body() -> None:
    """create_mrt_queue() sends POST with name/description/is_default and parses response."""
    captured: dict[str, Any] = {}
    queue_data: dict[str, Any] = {
        "id": "q-new",
        "org_id": "org-1",
        "name": "abuse-review",
        "description": "Queue for abuse reports",
        "is_default": False,
        "archived_at": None,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        captured["body"] = json.loads(request.content)
        captured["method"] = request.method
        captured["url"] = str(request.url)
        return _json_response(queue_data, status_code=201)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.create_mrt_queue(
        name="abuse-review",
        description="Queue for abuse reports",
        is_default=False,
    )
    assert isinstance(result, MRTQueue)
    assert result.id == "q-new"
    assert result.name == "abuse-review"
    assert result.archived_at is None
    assert captured["method"] == "POST"
    assert captured["url"].endswith("/api/v1/mrt/queues")
    assert captured["body"]["name"] == "abuse-review"
    assert captured["body"]["description"] == "Queue for abuse reports"
    assert captured["body"]["is_default"] is False


@pytest.mark.asyncio
async def test_archive_mrt_queue_sends_delete() -> None:
    """archive_mrt_queue() sends DELETE to correct URL and returns None on 204."""
    captured: dict[str, Any] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["method"] = request.method
        captured["url"] = str(request.url)
        return _empty_response(204)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.archive_mrt_queue("q-123")
    assert result is None
    assert captured["method"] == "DELETE"
    assert captured["url"].endswith("/api/v1/mrt/queues/q-123")


# ---------------------------------------------------------------------------
# API Keys
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_api_keys_unwraps() -> None:
    """list_api_keys() unwraps ``api_keys`` and returns list[ApiKey]."""
    key_data: dict[str, Any] = {
        "id": "k-1",
        "org_id": "org-1",
        "name": "My Key",
        "prefix": "nst_abc",
        "created_at": "2024-01-01T00:00:00Z",
        "revoked_at": None,
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"api_keys": [key_data]})

    client = _make_client(httpx.MockTransport(handler))
    keys = await client.list_api_keys()
    assert len(keys) == 1
    assert isinstance(keys[0], ApiKey)
    assert keys[0].id == "k-1"


@pytest.mark.asyncio
async def test_create_api_key_returns_dict() -> None:
    """create_api_key() returns raw dict with key and api_key fields."""
    key_data: dict[str, Any] = {
        "id": "k-1",
        "org_id": "org-1",
        "name": "New Key",
        "prefix": "nst_def",
        "created_at": "2024-01-01T00:00:00Z",
        "revoked_at": None,
    }
    response_payload = {"key": "nst_def_plaintext", "api_key": key_data}

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response(response_payload)

    client = _make_client(httpx.MockTransport(handler))
    result = await client.create_api_key("New Key")
    assert result["key"] == "nst_def_plaintext"
    assert result["api_key"]["id"] == "k-1"


# ---------------------------------------------------------------------------
# Text Banks
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_text_banks_unwraps() -> None:
    """list_text_banks() unwraps ``text_banks`` and returns list[TextBank]."""
    bank_data: dict[str, Any] = {
        "id": "tb-1",
        "org_id": "org-1",
        "name": "Slurs",
        "description": "",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"text_banks": [bank_data]})

    client = _make_client(httpx.MockTransport(handler))
    banks = await client.list_text_banks()
    assert len(banks) == 1
    assert isinstance(banks[0], TextBank)


# ---------------------------------------------------------------------------
# Signals
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_signals_unwraps() -> None:
    """list_signals() unwraps ``signals`` and returns list[Signal]."""
    sig_data: dict[str, Any] = {
        "id": "sig-regex",
        "display_name": "Regex",
        "description": "Text regex",
        "eligible_inputs": ["text"],
        "cost": 0,
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"signals": [sig_data]})

    client = _make_client(httpx.MockTransport(handler))
    signals = await client.list_signals()
    assert len(signals) == 1
    assert isinstance(signals[0], Signal)


# ---------------------------------------------------------------------------
# UDFs
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_udfs_unwraps() -> None:
    """list_udfs() unwraps ``udfs`` and returns list[UDF]."""
    udf_data: dict[str, Any] = {
        "name": "contains_url",
        "signature": "contains_url(text: str) -> bool",
        "description": "Check for URL",
        "example": "contains_url(item.body)",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"udfs": [udf_data]})

    client = _make_client(httpx.MockTransport(handler))
    udfs = await client.list_udfs()
    assert len(udfs) == 1
    assert isinstance(udfs[0], UDF)


# ---------------------------------------------------------------------------
# Signing Keys
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_list_signing_keys_unwraps() -> None:
    """list_signing_keys() unwraps ``signing_keys`` and returns list[SigningKey]."""
    sk_data: dict[str, Any] = {
        "id": "sk-1",
        "org_id": "org-1",
        "public_key": "-----BEGIN PUBLIC KEY-----\nMIIB...\n-----END PUBLIC KEY-----",
        "is_active": True,
        "created_at": "2024-01-01T00:00:00Z",
    }

    def handler(request: httpx.Request) -> httpx.Response:
        return _json_response({"signing_keys": [sk_data]})

    client = _make_client(httpx.MockTransport(handler))
    keys = await client.list_signing_keys()
    assert len(keys) == 1
    assert isinstance(keys[0], SigningKey)


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_http_error_propagates() -> None:
    """A 401 response raises httpx.HTTPStatusError."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(status_code=401, content=b'{"error":"unauthorized"}')

    client = _make_client(httpx.MockTransport(handler))
    with pytest.raises(httpx.HTTPStatusError):
        await client.me()


@pytest.mark.asyncio
async def test_http_error_propagates_on_list() -> None:
    """A 403 response on list_rules raises httpx.HTTPStatusError."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(status_code=403, content=b'{"error":"forbidden"}')

    client = _make_client(httpx.MockTransport(handler))
    with pytest.raises(httpx.HTTPStatusError):
        await client.list_rules()


# ---------------------------------------------------------------------------
# Static invariants (source inspection)
# ---------------------------------------------------------------------------


def test_no_kwargs_in_client() -> None:
    """NestClient source must not contain **kwargs."""
    import pathlib

    client_path = pathlib.Path(__file__).parent.parent / "api" / "client.py"
    source = client_path.read_text()
    assert "**kwargs" not in source, "client.py must not use **kwargs"


def test_no_asyncclient_creation_in_client() -> None:
    """NestClient source must not create AsyncClient (it receives one via constructor)."""
    import pathlib

    client_path = pathlib.Path(__file__).parent.parent / "api" / "client.py"
    source = client_path.read_text()
    # The only allowable AsyncClient reference is in the type annotation of __init__
    # which uses 'httpx.AsyncClient' without a '('. Check for constructor call pattern.
    lines = [ln for ln in source.splitlines() if "AsyncClient(" in ln]
    # Filter out comment lines
    non_comment = [ln for ln in lines if not ln.strip().startswith("#")]
    assert not non_comment, f"client.py must not instantiate AsyncClient; found: {non_comment}"
