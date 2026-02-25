"""Tests for page modules -- structural invariants."""

from __future__ import annotations

import ast
import pathlib

PAGES_DIR = pathlib.Path(__file__).parent.parent / "pages"


class TestPageInvariants:
    def test_no_cross_page_imports(self) -> None:
        """No page module imports from another page module."""
        for f in PAGES_DIR.glob("*.py"):
            if f.name == "__init__.py":
                continue
            source = f.read_text()
            tree = ast.parse(source)
            for node in ast.walk(tree):
                if isinstance(node, ast.ImportFrom) and node.module and node.module.startswith("pages."):
                    assert False, f"{f.name} imports from {node.module}"

    def test_no_javascript(self) -> None:
        """No raw JavaScript in any page module."""
        for f in PAGES_DIR.glob("*.py"):
            if f.name == "__init__.py":
                continue
            source = f.read_text()
            assert "run_javascript" not in source, f"{f.name} uses run_javascript"
            assert "<script>" not in source, f"{f.name} has <script> tag"

    def test_no_vue_templates(self) -> None:
        """No add_slot Vue/Quasar templates in any page module."""
        for f in PAGES_DIR.glob("*.py"):
            if f.name == "__init__.py":
                continue
            source = f.read_text()
            assert "add_slot" not in source, f"{f.name} uses add_slot (Vue template)"

    def test_login_does_not_use_require_auth(self) -> None:
        """Login page must NOT use @require_auth."""
        login = (PAGES_DIR / "login.py").read_text()
        assert "require_auth" not in login

    def test_all_non_login_pages_use_require_auth(self) -> None:
        """All pages except login must use @require_auth."""
        for f in PAGES_DIR.glob("*.py"):
            if f.name in ("__init__.py", "login.py"):
                continue
            source = f.read_text()
            assert "require_auth" in source, f"{f.name} does not use require_auth"

    def test_httpx_client_only_created_in_login(self) -> None:
        """httpx.AsyncClient() constructor only appears in login.py."""
        for f in PAGES_DIR.glob("*.py"):
            if f.name in ("__init__.py", "login.py"):
                continue
            source = f.read_text()
            assert "AsyncClient(" not in source, f"{f.name} creates AsyncClient"

    def test_future_annotations_in_updated_pages(self) -> None:
        """Pages updated in Stage 9 must have from __future__ import annotations."""
        required_pages = [
            "users.py",
            "api_keys.py",
            "signals.py",
            "settings.py",
            "actions.py",
            "policies.py",
            "item_types.py",
            "text_banks.py",
        ]
        for name in required_pages:
            source = (PAGES_DIR / name).read_text()
            assert "from __future__ import annotations" in source, (
                f"{name} missing 'from __future__ import annotations'"
            )


class TestMainImports:
    def test_main_imports_all_pages(self) -> None:
        """main.py must import all 12 page modules."""
        main_path = PAGES_DIR.parent / "main.py"
        source = main_path.read_text()
        expected_pages = [
            "login",
            "dashboard",
            "rules",
            "mrt",
            "actions",
            "policies",
            "item_types",
            "text_banks",
            "users",
            "api_keys",
            "signals",
            "settings",
        ]
        for page in expected_pages:
            assert f"import pages.{page}" in source, f"main.py missing import pages.{page}"

    def test_rules_has_templates(self) -> None:
        """rules.py must have Starlark templates."""
        rules = (PAGES_DIR / "rules.py").read_text()
        assert "TEMPLATES" in rules, "rules.py missing TEMPLATES constant"
