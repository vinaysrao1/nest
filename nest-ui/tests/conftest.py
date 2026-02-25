"""Shared pytest fixtures for nest-ui tests."""

from __future__ import annotations

import pytest


@pytest.fixture
def rule_dict() -> dict[str, object]:
    """Minimal dict representing a Rule API response."""
    return {
        "id": "rule-1",
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


@pytest.fixture
def user_dict() -> dict[str, object]:
    """Minimal dict representing a User API response."""
    return {
        "id": "user-1",
        "org_id": "org-1",
        "email": "alice@example.com",
        "name": "Alice",
        "role": "ADMIN",
        "is_active": True,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z",
    }
