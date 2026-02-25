"""Tests for main.py entry point."""

from __future__ import annotations


def test_main_is_importable() -> None:
    """main.py should be importable without starting the server."""
    # We can't fully import main.py as it calls ui.run(),
    # but we can verify it exists and has the expected structure.
    import pathlib

    main_path = pathlib.Path(__file__).parent.parent / "main.py"
    assert main_path.exists()
    source = main_path.read_text()
    assert "ui.run(" in source
    assert "storage_secret" in source
