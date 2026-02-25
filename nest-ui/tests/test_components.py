"""Tests for components.layout and components.starlark_editor modules."""

from unittest.mock import patch


class TestLayoutImports:
    """Verify the layout module can be imported and exports the expected symbols."""

    def test_layout_function_exists(self) -> None:
        """layout() function is importable from components.layout."""
        from components.layout import layout

        assert callable(layout)

    def test_confirm_function_exists(self) -> None:
        """confirm() function is importable from components.layout."""
        from components.layout import confirm

        assert callable(confirm)

    def test_nav_items_is_list(self) -> None:
        """_NAV_ITEMS is a non-empty list of tuples."""
        from components.layout import _NAV_ITEMS

        assert isinstance(_NAV_ITEMS, list)
        assert len(_NAV_ITEMS) > 0

    def test_nav_items_tuple_structure(self) -> None:
        """Each _NAV_ITEMS entry is a 4-tuple of strings."""
        from components.layout import _NAV_ITEMS

        for item in _NAV_ITEMS:
            assert len(item) == 4
            label, path, icon, min_role = item
            assert isinstance(label, str)
            assert isinstance(path, str)
            assert isinstance(icon, str)
            assert isinstance(min_role, str)

    def test_role_rank_constant_exists(self) -> None:
        """_ROLE_RANK dict is importable and contains all three roles."""
        from components.layout import _ROLE_RANK

        assert "ANALYST" in _ROLE_RANK
        assert "MODERATOR" in _ROLE_RANK
        assert "ADMIN" in _ROLE_RANK

    def test_role_rank_ordering(self) -> None:
        """ADMIN > MODERATOR > ANALYST in _ROLE_RANK."""
        from components.layout import _ROLE_RANK

        assert _ROLE_RANK["ADMIN"] > _ROLE_RANK["MODERATOR"]
        assert _ROLE_RANK["MODERATOR"] > _ROLE_RANK["ANALYST"]


class TestUserCanSee:
    """Tests for components.layout._user_can_see()."""

    def test_admin_can_see_all(self) -> None:
        """ADMIN role passes all min_role checks."""
        mock_storage: dict[str, object] = {"user": {"role": "ADMIN"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from components.layout import _user_can_see

            assert _user_can_see("ANALYST") is True
            assert _user_can_see("MODERATOR") is True
            assert _user_can_see("ADMIN") is True

    def test_moderator_limited(self) -> None:
        """MODERATOR can see ANALYST and MODERATOR items but not ADMIN."""
        mock_storage: dict[str, object] = {"user": {"role": "MODERATOR"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from components.layout import _user_can_see

            assert _user_can_see("ANALYST") is True
            assert _user_can_see("MODERATOR") is True
            assert _user_can_see("ADMIN") is False

    def test_analyst_limited(self) -> None:
        """ANALYST can only see ANALYST items."""
        mock_storage: dict[str, object] = {"user": {"role": "ANALYST"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from components.layout import _user_can_see

            assert _user_can_see("ANALYST") is True
            assert _user_can_see("MODERATOR") is False
            assert _user_can_see("ADMIN") is False

    def test_no_role_sees_nothing(self) -> None:
        """Unauthenticated user sees nothing."""
        mock_storage: dict[str, object] = {}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from components.layout import _user_can_see

            assert _user_can_see("ANALYST") is False
            assert _user_can_see("MODERATOR") is False
            assert _user_can_see("ADMIN") is False


class TestNavItemsMatchRbacTable:
    """Verify _NAV_ITEMS entries match the RBAC table from NEST_UI.md section 12."""

    def test_dashboard_visible_to_analyst(self) -> None:
        """Dashboard is visible to ANALYST (min_role=ANALYST)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Dashboard")
        assert item[3] == "ANALYST"

    def test_rules_visible_to_analyst(self) -> None:
        """Rules is visible to ANALYST (min_role=ANALYST)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Rules")
        assert item[3] == "ANALYST"

    def test_mrt_queues_visible_to_moderator(self) -> None:
        """MRT Queues is visible to MODERATOR and above (min_role=MODERATOR)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "MRT Queues")
        assert item[3] == "MODERATOR"

    def test_users_visible_to_admin_only(self) -> None:
        """Users is visible to ADMIN only (min_role=ADMIN)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Users")
        assert item[3] == "ADMIN"

    def test_api_keys_visible_to_admin_only(self) -> None:
        """API Keys is visible to ADMIN only (min_role=ADMIN)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "API Keys")
        assert item[3] == "ADMIN"

    def test_settings_visible_to_admin_only(self) -> None:
        """Settings is visible to ADMIN only (min_role=ADMIN)."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Settings")
        assert item[3] == "ADMIN"

    def test_text_banks_visible_to_moderator(self) -> None:
        """Text Banks is visible to MODERATOR and above."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Text Banks")
        assert item[3] == "MODERATOR"

    def test_item_types_visible_to_moderator(self) -> None:
        """Item Types is visible to MODERATOR and above."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Item Types")
        assert item[3] == "MODERATOR"

    def test_signals_visible_to_analyst(self) -> None:
        """Signals is visible to ANALYST and above."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Signals")
        assert item[3] == "ANALYST"

    def test_actions_visible_to_analyst(self) -> None:
        """Actions is visible to ANALYST and above."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Actions")
        assert item[3] == "ANALYST"

    def test_policies_visible_to_analyst(self) -> None:
        """Policies is visible to ANALYST and above."""
        from components.layout import _NAV_ITEMS

        item = next(i for i in _NAV_ITEMS if i[0] == "Policies")
        assert item[3] == "ANALYST"

    def test_all_required_nav_items_present(self) -> None:
        """All 11 nav items from the design doc are present."""
        from components.layout import _NAV_ITEMS

        labels = {item[0] for item in _NAV_ITEMS}
        expected = {
            "Dashboard", "Rules", "MRT Queues", "Actions", "Policies",
            "Item Types", "Text Banks", "Signals", "Users", "API Keys", "Settings",
        }
        assert labels == expected


class TestStarlarkEditorImports:
    """Verify starlark_editor can be imported and has the correct signature."""

    def test_starlark_editor_is_importable(self) -> None:
        """starlark_editor function is importable from components.starlark_editor."""
        from components.starlark_editor import starlark_editor

        assert callable(starlark_editor)

    def test_starlark_editor_accepts_optional_args(self) -> None:
        """starlark_editor has correct default parameter values."""
        import inspect

        from components.starlark_editor import starlark_editor

        sig = inspect.signature(starlark_editor)
        params = sig.parameters

        assert "value" in params
        assert "on_change" in params
        assert "udfs" in params
        assert "signals" in params

        assert params["value"].default == ""
        assert params["on_change"].default is None
        assert params["udfs"].default is None
        assert params["signals"].default is None
