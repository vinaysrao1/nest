"""Tests for api/types.py dataclasses.

Validates that every dataclass can be instantiated from a dict (simulating API
JSON responses) and that optional fields default correctly.
"""

from __future__ import annotations

import dataclasses
from typing import Any

import pytest

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

# ---------------------------------------------------------------------------
# User
# ---------------------------------------------------------------------------


def test_user_from_dict() -> None:
    """User.from_dict populates all fields from a complete dict."""
    data: dict[str, Any] = {
        "id": "u-1",
        "org_id": "org-1",
        "email": "alice@example.com",
        "name": "Alice",
        "role": "ADMIN",
        "is_active": True,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-02T00:00:00Z",
    }
    user = User.from_dict(data)
    assert user.id == "u-1"
    assert user.org_id == "org-1"
    assert user.email == "alice@example.com"
    assert user.name == "Alice"
    assert user.role == "ADMIN"
    assert user.is_active is True
    assert user.created_at == "2024-01-01T00:00:00Z"
    assert user.updated_at == "2024-01-02T00:00:00Z"


def test_user_from_dict_minimal() -> None:
    """User.from_dict uses safe defaults for all optional fields."""
    user = User.from_dict({"id": "u-2"})
    assert user.id == "u-2"
    assert user.org_id == ""
    assert user.email == ""
    assert user.name == ""
    assert user.role == ""
    assert user.is_active is True
    assert user.created_at == ""
    assert user.updated_at == ""


# ---------------------------------------------------------------------------
# Rule
# ---------------------------------------------------------------------------


def test_rule_from_dict() -> None:
    """Rule.from_dict populates all fields including lists."""
    data: dict[str, Any] = {
        "id": "r-1",
        "org_id": "org-1",
        "name": "Spam Rule",
        "status": "LIVE",
        "source": 'verdict("block")',
        "event_types": ["post.create", "comment.create"],
        "priority": 100,
        "tags": ["spam", "auto"],
        "version": 3,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-03T00:00:00Z",
    }
    rule = Rule.from_dict(data)
    assert rule.id == "r-1"
    assert rule.status == "LIVE"
    assert rule.event_types == ["post.create", "comment.create"]
    assert rule.priority == 100
    assert rule.tags == ["spam", "auto"]
    assert rule.version == 3


# ---------------------------------------------------------------------------
# Action
# ---------------------------------------------------------------------------


def test_action_from_dict() -> None:
    """Action.from_dict handles nested config dict."""
    data: dict[str, Any] = {
        "id": "a-1",
        "org_id": "org-1",
        "name": "Webhook Action",
        "action_type": "WEBHOOK",
        "config": {"url": "https://example.com/hook", "timeout_ms": 3000},
        "version": 1,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    action = Action.from_dict(data)
    assert action.id == "a-1"
    assert action.action_type == "WEBHOOK"
    assert action.config["url"] == "https://example.com/hook"


# ---------------------------------------------------------------------------
# Policy
# ---------------------------------------------------------------------------


def test_policy_from_dict_with_parent() -> None:
    """Policy.from_dict populates parent_id when present."""
    data: dict[str, Any] = {
        "id": "p-1",
        "org_id": "org-1",
        "name": "Child Policy",
        "description": "A child policy",
        "parent_id": "p-0",
        "strike_penalty": 5,
        "version": 2,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    policy = Policy.from_dict(data)
    assert policy.parent_id == "p-0"
    assert policy.strike_penalty == 5


def test_policy_from_dict_no_parent() -> None:
    """Policy.from_dict leaves parent_id as None when absent or null."""
    data: dict[str, Any] = {"id": "p-2", "name": "Root Policy", "parent_id": None}
    policy = Policy.from_dict(data)
    assert policy.parent_id is None


# ---------------------------------------------------------------------------
# ItemType
# ---------------------------------------------------------------------------


def test_item_type_from_dict() -> None:
    """ItemType.from_dict handles schema and field_roles dicts."""
    data: dict[str, Any] = {
        "id": "it-1",
        "org_id": "org-1",
        "name": "Post",
        "kind": "CONTENT",
        "schema": {"type": "object", "properties": {"body": {"type": "string"}}},
        "field_roles": {"body": "text"},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    item_type = ItemType.from_dict(data)
    assert item_type.id == "it-1"
    assert item_type.kind == "CONTENT"
    assert item_type.schema["type"] == "object"
    assert item_type.field_roles["body"] == "text"


# ---------------------------------------------------------------------------
# MRTQueue
# ---------------------------------------------------------------------------


def test_mrt_queue_from_dict() -> None:
    """MRTQueue.from_dict sets all fields correctly including archived_at."""
    data: dict[str, Any] = {
        "id": "q-1",
        "org_id": "org-1",
        "name": "Default Queue",
        "description": "The default review queue",
        "is_default": True,
        "archived_at": "2024-06-01T00:00:00Z",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    queue = MRTQueue.from_dict(data)
    assert queue.id == "q-1"
    assert queue.is_default is True
    assert queue.archived_at == "2024-06-01T00:00:00Z"


def test_mrt_queue_from_dict_no_archived_at() -> None:
    """MRTQueue.from_dict defaults archived_at to None when absent."""
    data: dict[str, Any] = {
        "id": "q-2",
        "org_id": "org-1",
        "name": "Active Queue",
        "description": "",
        "is_default": False,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    queue = MRTQueue.from_dict(data)
    assert queue.id == "q-2"
    assert queue.archived_at is None


# ---------------------------------------------------------------------------
# MRTJob
# ---------------------------------------------------------------------------


def test_mrt_job_from_dict() -> None:
    """MRTJob.from_dict handles nullable assigned_to."""
    data: dict[str, Any] = {
        "id": "j-1",
        "org_id": "org-1",
        "queue_id": "q-1",
        "item_id": "item-1",
        "item_type_id": "it-1",
        "payload": {"body": "hello"},
        "status": "ASSIGNED",
        "assigned_to": "u-1",
        "policy_ids": ["p-1"],
        "enqueue_source": "engine",
        "source_info": {"rule_id": "r-1"},
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    job = MRTJob.from_dict(data)
    assert job.id == "j-1"
    assert job.assigned_to == "u-1"
    assert job.status == "ASSIGNED"

    data_unassigned = {**data, "id": "j-2", "assigned_to": None}
    job2 = MRTJob.from_dict(data_unassigned)
    assert job2.assigned_to is None


# ---------------------------------------------------------------------------
# MRTDecision
# ---------------------------------------------------------------------------


def test_mrt_decision_from_dict() -> None:
    """MRTDecision.from_dict populates all fields."""
    data: dict[str, Any] = {
        "id": "d-1",
        "org_id": "org-1",
        "job_id": "j-1",
        "user_id": "u-1",
        "verdict": "block",
        "action_ids": ["a-1", "a-2"],
        "policy_ids": ["p-1"],
        "reason": "Spam content",
        "created_at": "2024-01-01T00:00:00Z",
    }
    decision = MRTDecision.from_dict(data)
    assert decision.id == "d-1"
    assert decision.verdict == "block"
    assert decision.action_ids == ["a-1", "a-2"]
    assert decision.reason == "Spam content"
    assert decision.target_queue_id is None


def test_mrt_decision_from_dict_target_queue_id() -> None:
    """MRTDecision.from_dict parses target_queue_id when present and defaults to None."""
    data_with_target: dict[str, Any] = {
        "id": "d-2",
        "org_id": "org-1",
        "job_id": "j-2",
        "user_id": "u-1",
        "verdict": "ROUTE",
        "action_ids": [],
        "policy_ids": [],
        "reason": "Wrong queue",
        "target_queue_id": "q-99",
        "created_at": "2024-01-01T00:00:00Z",
    }
    decision = MRTDecision.from_dict(data_with_target)
    assert decision.target_queue_id == "q-99"
    assert decision.verdict == "ROUTE"

    data_without_target: dict[str, Any] = {
        "id": "d-3",
        "org_id": "org-1",
        "job_id": "j-3",
        "user_id": "u-1",
        "verdict": "APPROVE",
        "action_ids": [],
        "policy_ids": [],
        "reason": "",
        "created_at": "2024-01-01T00:00:00Z",
    }
    decision_no_target = MRTDecision.from_dict(data_without_target)
    assert decision_no_target.target_queue_id is None


# ---------------------------------------------------------------------------
# ApiKey
# ---------------------------------------------------------------------------


def test_api_key_from_dict() -> None:
    """ApiKey.from_dict handles nullable revoked_at."""
    data: dict[str, Any] = {
        "id": "k-1",
        "org_id": "org-1",
        "name": "My Key",
        "prefix": "nst_abc",
        "created_at": "2024-01-01T00:00:00Z",
        "revoked_at": None,
    }
    key = ApiKey.from_dict(data)
    assert key.id == "k-1"
    assert key.revoked_at is None

    revoked = {**data, "revoked_at": "2024-06-01T00:00:00Z"}
    key2 = ApiKey.from_dict(revoked)
    assert key2.revoked_at == "2024-06-01T00:00:00Z"


# ---------------------------------------------------------------------------
# SigningKey
# ---------------------------------------------------------------------------


def test_signing_key_from_dict() -> None:
    """SigningKey.from_dict sets public_key and is_active."""
    data: dict[str, Any] = {
        "id": "sk-1",
        "org_id": "org-1",
        "public_key": "-----BEGIN PUBLIC KEY-----\nMIIB...\n-----END PUBLIC KEY-----",
        "is_active": True,
        "created_at": "2024-01-01T00:00:00Z",
    }
    sk = SigningKey.from_dict(data)
    assert sk.id == "sk-1"
    assert sk.is_active is True
    assert sk.public_key.startswith("-----BEGIN")


# ---------------------------------------------------------------------------
# TextBank
# ---------------------------------------------------------------------------


def test_text_bank_from_dict() -> None:
    """TextBank.from_dict sets all string fields."""
    data: dict[str, Any] = {
        "id": "tb-1",
        "org_id": "org-1",
        "name": "Slur List",
        "description": "Known slurs",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
    bank = TextBank.from_dict(data)
    assert bank.id == "tb-1"
    assert bank.name == "Slur List"


# ---------------------------------------------------------------------------
# TextBankEntry
# ---------------------------------------------------------------------------


def test_text_bank_entry_from_dict() -> None:
    """TextBankEntry.from_dict sets value, is_regex, text_bank_id."""
    data: dict[str, Any] = {
        "id": "tbe-1",
        "text_bank_id": "tb-1",
        "value": r"\bspam\b",
        "is_regex": True,
        "created_at": "2024-01-01T00:00:00Z",
    }
    entry = TextBankEntry.from_dict(data)
    assert entry.id == "tbe-1"
    assert entry.is_regex is True
    assert entry.text_bank_id == "tb-1"


# ---------------------------------------------------------------------------
# Signal
# ---------------------------------------------------------------------------


def test_signal_from_dict() -> None:
    """Signal.from_dict populates eligible_inputs list and cost."""
    data: dict[str, Any] = {
        "id": "sig-text-regex",
        "display_name": "Text Regex",
        "description": "Regex match on text",
        "eligible_inputs": ["text"],
        "cost": 0,
    }
    signal = Signal.from_dict(data)
    assert signal.id == "sig-text-regex"
    assert signal.eligible_inputs == ["text"]
    assert signal.cost == 0


# ---------------------------------------------------------------------------
# UDF
# ---------------------------------------------------------------------------


def test_udf_from_dict() -> None:
    """UDF.from_dict populates name, signature, description, example."""
    data: dict[str, Any] = {
        "name": "contains_url",
        "signature": "contains_url(text: str) -> bool",
        "description": "Returns True if text contains a URL.",
        "example": "contains_url(item.body)",
    }
    udf = UDF.from_dict(data)
    assert udf.name == "contains_url"
    assert "URL" in udf.description


# ---------------------------------------------------------------------------
# TestResult
# ---------------------------------------------------------------------------


def test_test_result_from_dict() -> None:
    """TestResult.from_dict populates verdict, actions, logs, latency."""
    data: dict[str, Any] = {
        "verdict": "block",
        "reason": "matched spam rule",
        "rule_id": "r-1",
        "actions": ["webhook-1"],
        "logs": ["rule executed"],
        "latency_us": 512,
    }
    result = TestResult.from_dict(data)
    assert result.verdict == "block"
    assert result.actions == ["webhook-1"]
    assert result.latency_us == 512


# ---------------------------------------------------------------------------
# PaginatedResult
# ---------------------------------------------------------------------------


def test_paginated_result_with_rules(rule_dict: dict[str, Any]) -> None:
    """PaginatedResult can hold Rule items."""
    rule = Rule.from_dict(rule_dict)
    pr: PaginatedResult[Rule] = PaginatedResult(
        items=[rule],
        total=1,
        page=1,
        page_size=50,
        total_pages=1,
    )
    assert pr.total == 1
    assert pr.items[0].id == rule_dict["id"]


def test_paginated_result_with_users(user_dict: dict[str, Any]) -> None:
    """PaginatedResult can hold User items."""
    user = User.from_dict(user_dict)
    pr: PaginatedResult[User] = PaginatedResult(
        items=[user],
        total=1,
        page=1,
        page_size=50,
        total_pages=1,
    )
    assert pr.items[0].email == user_dict["email"]


def test_paginated_result_empty() -> None:
    """PaginatedResult handles empty items list."""
    pr: PaginatedResult[Rule] = PaginatedResult(
        items=[],
        total=0,
        page=1,
        page_size=50,
        total_pages=0,
    )
    assert pr.items == []
    assert pr.total == 0


# ---------------------------------------------------------------------------
# Frozen invariant
# ---------------------------------------------------------------------------


def test_dataclasses_are_frozen() -> None:
    """Assigning a field on a frozen dataclass raises FrozenInstanceError."""
    user = User.from_dict({"id": "u-frozen"})
    with pytest.raises(dataclasses.FrozenInstanceError):
        # Use object.__setattr__ to bypass the frozen check enforcement and
        # trigger the underlying error from the dataclass machinery.
        user.__class__.__setattr__(user, "email", "hack@example.com")  # type: ignore[misc]

    rule = Rule.from_dict({"id": "r-frozen"})
    with pytest.raises(dataclasses.FrozenInstanceError):
        rule.__class__.__setattr__(rule, "status", "LIVE")  # type: ignore[misc]
