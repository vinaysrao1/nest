"""Response dataclasses mirroring Go domain types.

All dataclasses are frozen and slotted for immutability and memory efficiency.
Each provides a ``from_dict`` factory that handles missing/optional fields with
safe defaults, matching the JSON tags on the corresponding Go structs.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Generic, TypeVar

T = TypeVar("T")


@dataclass(frozen=True, slots=True)
class User:
    """Mirrors domain.User (Go). Password is never returned by API."""

    id: str
    org_id: str
    email: str
    name: str
    role: str  # "ADMIN" | "MODERATOR" | "ANALYST"
    is_active: bool
    created_at: str  # ISO 8601 string
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> User:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen User instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            email=data.get("email", ""),
            name=data.get("name", ""),
            role=data.get("role", ""),
            is_active=data.get("is_active", True),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class Rule:
    """Mirrors domain.Rule (Go). Source is Starlark code (single source of truth)."""

    id: str
    org_id: str
    name: str
    status: str  # "LIVE" | "BACKGROUND" | "DISABLED"
    source: str
    event_types: list[str]
    priority: int
    tags: list[str]
    version: int
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Rule:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen Rule instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            status=data.get("status", ""),
            source=data.get("source", ""),
            event_types=data.get("event_types", []),
            priority=data.get("priority", 0),
            tags=data.get("tags", []),
            version=data.get("version", 1),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class Action:
    """Mirrors domain.Action (Go)."""

    id: str
    org_id: str
    name: str
    action_type: str  # "WEBHOOK" | "ENQUEUE_TO_MRT"
    config: dict[str, Any]
    version: int
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Action:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen Action instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            action_type=data.get("action_type", ""),
            config=data.get("config", {}),
            version=data.get("version", 1),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class Policy:
    """Mirrors domain.Policy (Go)."""

    id: str
    org_id: str
    name: str
    description: str
    parent_id: str | None
    strike_penalty: int
    version: int
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Policy:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen Policy instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            description=data.get("description", ""),
            parent_id=data.get("parent_id"),
            strike_penalty=data.get("strike_penalty", 0),
            version=data.get("version", 1),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class ItemType:
    """Mirrors domain.ItemType (Go)."""

    id: str
    org_id: str
    name: str
    kind: str  # "CONTENT" | "USER" | "THREAD"
    schema: dict[str, Any]
    field_roles: dict[str, Any]
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ItemType:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen ItemType instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            kind=data.get("kind", ""),
            schema=data.get("schema", {}),
            field_roles=data.get("field_roles", {}),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class MRTQueue:
    """Mirrors domain.MRTQueue (Go)."""

    id: str
    org_id: str
    name: str
    description: str
    is_default: bool
    archived_at: str | None
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> MRTQueue:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen MRTQueue instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            description=data.get("description", ""),
            is_default=data.get("is_default", False),
            archived_at=data.get("archived_at"),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class MRTJob:
    """Mirrors domain.MRTJob (Go)."""

    id: str
    org_id: str
    queue_id: str
    item_id: str
    item_type_id: str
    payload: dict[str, Any]
    status: str  # "PENDING" | "ASSIGNED" | "DECIDED"
    assigned_to: str | None
    policy_ids: list[str]
    enqueue_source: str
    source_info: dict[str, Any]
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> MRTJob:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen MRTJob instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            queue_id=data.get("queue_id", ""),
            item_id=data.get("item_id", ""),
            item_type_id=data.get("item_type_id", ""),
            payload=data.get("payload", {}),
            status=data.get("status", ""),
            assigned_to=data.get("assigned_to"),
            policy_ids=data.get("policy_ids", []),
            enqueue_source=data.get("enqueue_source", ""),
            source_info=data.get("source_info", {}),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class MRTDecision:
    """Mirrors domain.MRTDecision (Go)."""

    id: str
    org_id: str
    job_id: str
    user_id: str
    verdict: str
    action_ids: list[str]
    policy_ids: list[str]
    reason: str
    target_queue_id: str | None  # set when verdict == "ROUTE"
    created_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> MRTDecision:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen MRTDecision instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            job_id=data.get("job_id", ""),
            user_id=data.get("user_id", ""),
            verdict=data.get("verdict", ""),
            action_ids=data.get("action_ids", []),
            policy_ids=data.get("policy_ids", []),
            reason=data.get("reason", ""),
            target_queue_id=data.get("target_queue_id"),
            created_at=data.get("created_at", ""),
        )


@dataclass(frozen=True, slots=True)
class ApiKey:
    """Mirrors domain.ApiKey (Go). Key hash is never returned."""

    id: str
    org_id: str
    name: str
    prefix: str
    created_at: str
    revoked_at: str | None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ApiKey:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen ApiKey instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            prefix=data.get("prefix", ""),
            created_at=data.get("created_at", ""),
            revoked_at=data.get("revoked_at"),
        )


@dataclass(frozen=True, slots=True)
class SigningKey:
    """Mirrors domain.SigningKey (Go). Private key is never returned."""

    id: str
    org_id: str
    public_key: str
    is_active: bool
    created_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> SigningKey:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen SigningKey instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            public_key=data.get("public_key", ""),
            is_active=data.get("is_active", True),
            created_at=data.get("created_at", ""),
        )


@dataclass(frozen=True, slots=True)
class TextBank:
    """Mirrors domain.TextBank (Go)."""

    id: str
    org_id: str
    name: str
    description: str
    created_at: str
    updated_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> TextBank:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen TextBank instance.
        """
        return cls(
            id=data["id"],
            org_id=data.get("org_id", ""),
            name=data.get("name", ""),
            description=data.get("description", ""),
            created_at=data.get("created_at", ""),
            updated_at=data.get("updated_at", ""),
        )


@dataclass(frozen=True, slots=True)
class TextBankEntry:
    """Mirrors domain.TextBankEntry (Go)."""

    id: str
    text_bank_id: str
    value: str
    is_regex: bool
    created_at: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> TextBankEntry:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen TextBankEntry instance.
        """
        return cls(
            id=data["id"],
            text_bank_id=data.get("text_bank_id", ""),
            value=data.get("value", ""),
            is_regex=data.get("is_regex", False),
            created_at=data.get("created_at", ""),
        )


@dataclass(frozen=True, slots=True)
class Signal:
    """Mirrors handler.signalSummary (Go). Not a domain type -- handler-level shape."""

    id: str
    display_name: str
    description: str
    eligible_inputs: list[str]
    cost: int

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Signal:
        """Construct from API response dict.

        Pre-conditions: data contains the ``id`` key.
        Post-conditions: returns a frozen Signal instance.
        """
        return cls(
            id=data["id"],
            display_name=data.get("display_name", ""),
            description=data.get("description", ""),
            eligible_inputs=data.get("eligible_inputs", []),
            cost=data.get("cost", 0),
        )


@dataclass(frozen=True, slots=True)
class UDF:
    """Mirrors handler.udfDescriptor (Go)."""

    name: str
    signature: str
    description: str
    example: str

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> UDF:
        """Construct from API response dict.

        Pre-conditions: data contains the ``name`` key.
        Post-conditions: returns a frozen UDF instance.
        """
        return cls(
            name=data["name"],
            signature=data.get("signature", ""),
            description=data.get("description", ""),
            example=data.get("example", ""),
        )


@dataclass(frozen=True, slots=True)
class TestResult:
    """Mirrors service.TestResult (Go). Returned by rule test endpoints."""

    verdict: str
    reason: str
    rule_id: str
    actions: list[str]
    logs: list[str]
    latency_us: int

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> TestResult:
        """Construct from API response dict.

        Pre-conditions: data is a dict from the test endpoint response.
        Post-conditions: returns a frozen TestResult instance.
        """
        return cls(
            verdict=data.get("verdict", ""),
            reason=data.get("reason", ""),
            rule_id=data.get("rule_id", ""),
            actions=data.get("actions", []),
            logs=data.get("logs", []),
            latency_us=data.get("latency_us", 0),
        )


@dataclass(frozen=True, slots=True)
class PaginatedResult(Generic[T]):
    """Mirrors domain.PaginatedResult[T] (Go).

    Generic pagination wrapper. The ``items`` field contains typed instances
    constructed by NestClient using the appropriate ``from_dict`` factory.
    """

    items: list[T]
    total: int
    page: int
    page_size: int
    total_pages: int
