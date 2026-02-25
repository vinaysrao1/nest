"""Tests for auth.state and auth.middleware modules."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# ---------------------------------------------------------------------------
# auth.state tests
# ---------------------------------------------------------------------------


class TestUserRole:
    """Tests for auth.state.user_role()."""

    def test_user_role_returns_role_for_admin(self) -> None:
        """user_role() returns 'ADMIN' when user dict has role=ADMIN."""
        mock_storage: dict[str, object] = {"user": {"role": "ADMIN", "name": "Alice"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import user_role

            assert user_role() == "ADMIN"

    def test_user_role_returns_role_for_moderator(self) -> None:
        """user_role() returns 'MODERATOR' when user dict has role=MODERATOR."""
        mock_storage: dict[str, object] = {"user": {"role": "MODERATOR"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import user_role

            assert user_role() == "MODERATOR"

    def test_user_role_returns_role_for_analyst(self) -> None:
        """user_role() returns 'ANALYST' when user dict has role=ANALYST."""
        mock_storage: dict[str, object] = {"user": {"role": "ANALYST"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import user_role

            assert user_role() == "ANALYST"

    def test_user_role_returns_empty_when_no_user(self) -> None:
        """user_role() returns '' when no 'user' key in storage."""
        mock_storage: dict[str, object] = {}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import user_role

            assert user_role() == ""

    def test_user_role_returns_empty_when_user_has_no_role(self) -> None:
        """user_role() returns '' when user dict exists but has no role key."""
        mock_storage: dict[str, object] = {"user": {"name": "Bob"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import user_role

            assert user_role() == ""


class TestCanEdit:
    """Tests for auth.state.can_edit()."""

    def test_can_edit_true_for_admin(self) -> None:
        """can_edit() returns True for ADMIN role."""
        mock_storage: dict[str, object] = {"user": {"role": "ADMIN"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import can_edit

            assert can_edit("rules") is True

    def test_can_edit_false_for_moderator(self) -> None:
        """can_edit() returns False for MODERATOR role."""
        mock_storage: dict[str, object] = {"user": {"role": "MODERATOR"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import can_edit

            assert can_edit("rules") is False

    def test_can_edit_false_for_analyst(self) -> None:
        """can_edit() returns False for ANALYST role."""
        mock_storage: dict[str, object] = {"user": {"role": "ANALYST"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import can_edit

            assert can_edit("rules") is False

    def test_can_edit_false_when_not_logged_in(self) -> None:
        """can_edit() returns False when user is not in storage."""
        mock_storage: dict[str, object] = {}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import can_edit

            assert can_edit("rules") is False

    def test_can_edit_resource_param_ignored(self) -> None:
        """can_edit() ignores resource param -- only ADMIN check matters."""
        mock_storage: dict[str, object] = {"user": {"role": "ADMIN"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import can_edit

            assert can_edit("users") is True
            assert can_edit("policies") is True
            assert can_edit("anything") is True


class TestIsModeratorOrAbove:
    """Tests for auth.state.is_moderator_or_above()."""

    def test_is_moderator_or_above_true_for_admin(self) -> None:
        """Returns True for ADMIN."""
        mock_storage: dict[str, object] = {"user": {"role": "ADMIN"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import is_moderator_or_above

            assert is_moderator_or_above() is True

    def test_is_moderator_or_above_true_for_moderator(self) -> None:
        """Returns True for MODERATOR."""
        mock_storage: dict[str, object] = {"user": {"role": "MODERATOR"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import is_moderator_or_above

            assert is_moderator_or_above() is True

    def test_is_moderator_or_above_false_for_analyst(self) -> None:
        """Returns False for ANALYST."""
        mock_storage: dict[str, object] = {"user": {"role": "ANALYST"}}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import is_moderator_or_above

            assert is_moderator_or_above() is False

    def test_is_moderator_or_above_false_when_not_logged_in(self) -> None:
        """Returns False when no user in storage."""
        mock_storage: dict[str, object] = {}
        with patch("auth.state.app") as mock_app:
            mock_app.storage.user = mock_storage
            from auth.state import is_moderator_or_above

            assert is_moderator_or_above() is False


# ---------------------------------------------------------------------------
# auth.middleware tests
# ---------------------------------------------------------------------------


class TestRequireAuth:
    """Tests for auth.middleware.require_auth decorator."""

    @pytest.mark.asyncio
    async def test_require_auth_redirects_when_no_http_client(self) -> None:
        """Redirects to /login when get_http_client() returns None."""
        page_called = False

        async def page_func() -> None:
            nonlocal page_called
            page_called = True

        with (
            patch("auth.middleware.get_http_client", return_value=None),
            patch("auth.middleware.ui") as mock_ui,
        ):
            from auth.middleware import require_auth

            wrapped = require_auth(page_func)
            await wrapped()

        assert not page_called
        mock_ui.navigate.to.assert_called_once_with("/login")

    @pytest.mark.asyncio
    async def test_require_auth_redirects_on_401(self) -> None:
        """Redirects to /login and clears storage on 401 HTTPStatusError."""
        import httpx

        mock_http = MagicMock()
        mock_resp = MagicMock()
        mock_resp.raise_for_status.side_effect = httpx.HTTPStatusError(
            "401", request=MagicMock(), response=MagicMock()
        )

        async def async_get(url: str) -> MagicMock:
            return mock_resp

        mock_http.get = async_get
        mock_storage: dict[str, object] = {"authenticated": True, "user": {"role": "ADMIN"}}
        page_called = False

        async def page_func() -> None:
            nonlocal page_called
            page_called = True

        remove_mock = AsyncMock()
        with (
            patch("auth.middleware.get_http_client", return_value=mock_http),
            patch("auth.middleware.remove_http_client", remove_mock),
            patch("auth.middleware.app") as mock_app,
            patch("auth.middleware.ui") as mock_ui,
        ):
            mock_app.storage.user = mock_storage
            from auth.middleware import require_auth

            wrapped = require_auth(page_func)
            await wrapped()

        assert not page_called
        mock_ui.navigate.to.assert_called_once_with("/login")
        remove_mock.assert_awaited_once()
        assert len(mock_storage) == 0

    @pytest.mark.asyncio
    async def test_require_auth_redirects_on_connect_error(self) -> None:
        """Redirects to /login and clears storage on ConnectError."""
        import httpx

        mock_http = MagicMock()

        async def async_get(url: str) -> None:
            raise httpx.ConnectError("Connection refused")

        mock_http.get = async_get
        mock_storage: dict[str, object] = {"authenticated": True, "user": {"role": "ADMIN"}}
        page_called = False

        async def page_func() -> None:
            nonlocal page_called
            page_called = True

        remove_mock = AsyncMock()
        with (
            patch("auth.middleware.get_http_client", return_value=mock_http),
            patch("auth.middleware.remove_http_client", remove_mock),
            patch("auth.middleware.app") as mock_app,
            patch("auth.middleware.ui") as mock_ui,
        ):
            mock_app.storage.user = mock_storage
            from auth.middleware import require_auth

            wrapped = require_auth(page_func)
            await wrapped()

        assert not page_called
        mock_ui.navigate.to.assert_called_once_with("/login")
        remove_mock.assert_awaited_once()
        assert len(mock_storage) == 0

    @pytest.mark.asyncio
    async def test_require_auth_calls_page_func_on_success(self) -> None:
        """Calls the wrapped page function when session is valid."""
        mock_http = MagicMock()
        mock_resp = MagicMock()
        mock_resp.raise_for_status.return_value = None

        async def async_get(url: str) -> MagicMock:
            return mock_resp

        mock_http.get = async_get
        page_called = False

        async def page_func() -> None:
            nonlocal page_called
            page_called = True

        with patch("auth.middleware.get_http_client", return_value=mock_http):
            from auth.middleware import require_auth

            wrapped = require_auth(page_func)
            await wrapped()

        assert page_called

    def test_require_auth_preserves_wrapped_function_name(self) -> None:
        """functools.wraps preserves the original function name."""

        async def my_page_func() -> None:
            pass

        from auth.middleware import require_auth

        wrapped = require_auth(my_page_func)
        assert wrapped.__name__ == "my_page_func"


class TestClientHelpers:
    """Tests for get_http_client, set_http_client, remove_http_client."""

    def test_get_http_client_returns_none_without_browser_id(self) -> None:
        """get_http_client() returns None when browser storage has no 'id'."""
        with patch("auth.middleware.app") as mock_app:
            mock_app.storage.browser = {}
            from auth.middleware import get_http_client

            result = get_http_client()
        assert result is None

    def test_set_and_get_http_client(self) -> None:
        """set_http_client() stores client; get_http_client() retrieves it."""
        import httpx

        from auth.middleware import _clients, get_http_client, set_http_client

        fake_client = MagicMock(spec=httpx.AsyncClient)
        browser_id = "test-browser-abc"

        with patch("auth.middleware.app") as mock_app:
            mock_app.storage.browser = {"id": browser_id}
            set_http_client(fake_client)
            result = get_http_client()

        assert result is fake_client
        # Clean up module-level dict
        _clients.pop(browser_id, None)

    @pytest.mark.asyncio
    async def test_remove_http_client_closes_and_removes(self) -> None:
        """remove_http_client() closes the client and removes it from _clients."""
        import httpx

        from auth.middleware import _clients, remove_http_client

        fake_client = AsyncMock(spec=httpx.AsyncClient)
        browser_id = "test-browser-xyz"
        _clients[browser_id] = fake_client

        with patch("auth.middleware.app") as mock_app:
            mock_app.storage.browser = {"id": browser_id}
            await remove_http_client()

        fake_client.aclose.assert_awaited_once()
        assert browser_id not in _clients

    @pytest.mark.asyncio
    async def test_remove_http_client_noop_without_browser_id(self) -> None:
        """remove_http_client() does nothing when browser storage has no 'id'."""
        with patch("auth.middleware.app") as mock_app:
            mock_app.storage.browser = {}
            from auth.middleware import remove_http_client

            # Should not raise
            await remove_http_client()
