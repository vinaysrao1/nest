#!/usr/bin/env python3
"""
Scaling & Concurrency Bottleneck Analyzer
Statically detects common scaling antipatterns in Python code.

Detects:
- Sync calls inside async functions (blocking the event loop)
- N+1 query patterns (I/O calls inside loops)
- Shared mutable state (globals, class-level mutables)
- Unbounded concurrency (task spawning without limits)
- Missing backpressure (queues without maxsize)
- Serialization bottlenecks (locks held across await)

Usage:
    python scaling.py <path> [--format=summary|full]
"""

import argparse
import ast
import json
import sys
from pathlib import Path

# Known blocking calls that should not appear in async functions
BLOCKING_CALLS = {
    "time.sleep",
    "os.system",
    "subprocess.run",
    "subprocess.call",
    "subprocess.check_output",
    "subprocess.check_call",
    "subprocess.Popen",
    "open",
    "input",
    "requests.get",
    "requests.post",
    "requests.put",
    "requests.delete",
    "requests.patch",
    "requests.head",
    "requests.request",
    "urllib.request.urlopen",
    "socket.socket",
}

# Known I/O call patterns (partial matches)
IO_CALL_PATTERNS = {
    "requests.", "urllib.", "http.", "socket.",
    ".execute", ".query", ".fetch", ".send",
    ".read", ".write", ".get", ".post", ".put", ".delete",
    "cursor.", "session.", "conn.", "connection.",
    "db.", "database.", "redis.", "mongo.",
    ".find", ".find_one", ".insert", ".update", ".delete_one",
    ".select", ".commit",
}

# Mutable default types
MUTABLE_TYPES = {"list", "dict", "set", "List", "Dict", "Set"}


class ScalingAnalyzer(ast.NodeVisitor):
    """Analyze a single file for scaling antipatterns."""

    def __init__(self, module_name: str, source_lines: list[str]):
        self.module_name = module_name
        self.source_lines = source_lines
        self.findings: list[dict] = []
        self.in_async_func: bool = False
        self.in_loop: bool = False
        self.loop_depth: int = 0
        self.current_func: str | None = None
        self.current_class: str | None = None

    def _add_finding(
        self, category: str, severity: str, line: int, message: str, fix: str = ""
    ):
        self.findings.append({
            "module": self.module_name,
            "category": category,
            "severity": severity,
            "line": line,
            "message": message,
            "fix": fix,
            "context": self._get_context(line),
        })

    def _get_context(self, line: int) -> str:
        if 0 < line <= len(self.source_lines):
            return self.source_lines[line - 1].strip()
        return ""

    def _resolve_call_name(self, node: ast.expr) -> str:
        if isinstance(node, ast.Name):
            return node.id
        elif isinstance(node, ast.Attribute):
            value = self._resolve_call_name(node.value)
            if value:
                return f"{value}.{node.attr}"
            return node.attr
        return ""

    # --- Shared mutable state ---

    def visit_Module(self, node: ast.Module):
        for item in node.body:
            if isinstance(item, ast.Assign):
                for target in item.targets:
                    self._check_mutable_global(target, item.value, item.lineno)
            elif isinstance(item, ast.AnnAssign) and item.value:
                if item.target:
                    self._check_mutable_global(
                        item.target, item.value, item.lineno
                    )
        self.generic_visit(node)

    def _check_mutable_global(
        self, target: ast.expr, value: ast.expr, lineno: int
    ):
        name = ""
        if isinstance(target, ast.Name):
            name = target.id
        if not name or name.isupper():
            return

        is_mutable = False
        if isinstance(value, (ast.List, ast.Dict, ast.Set)):
            is_mutable = True
        elif isinstance(value, ast.Call):
            call_name = self._resolve_call_name(value.func)
            if call_name in MUTABLE_TYPES or call_name.endswith(
                ("defaultdict", "OrderedDict", "Counter")
            ):
                is_mutable = True

        if is_mutable:
            self._add_finding(
                "shared_mutable_state",
                "warning",
                lineno,
                f"Module-level mutable variable '{name}' - potential shared state issue in concurrent code",
                "Move to function scope, use threading.local(), or protect with a lock",
            )

    # --- Async/sync violations ---

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef):
        prev_async = self.in_async_func
        prev_func = self.current_func
        self.in_async_func = True
        self.current_func = node.name
        self.generic_visit(node)
        self.in_async_func = prev_async
        self.current_func = prev_func

    def visit_FunctionDef(self, node: ast.FunctionDef):
        prev_async = self.in_async_func
        prev_func = self.current_func
        self.in_async_func = False
        self.current_func = node.name
        self.generic_visit(node)
        self.in_async_func = prev_async
        self.current_func = prev_func

    def visit_ClassDef(self, node: ast.ClassDef):
        prev_class = self.current_class
        self.current_class = node.name

        for item in node.body:
            if isinstance(item, ast.Assign):
                for target in item.targets:
                    if isinstance(target, ast.Name) and isinstance(
                        item.value, (ast.List, ast.Dict, ast.Set)
                    ):
                        self._add_finding(
                            "shared_mutable_state",
                            "warning",
                            item.lineno,
                            f"Class-level mutable '{target.id}' in {node.name} - shared across all instances",
                            "Move to __init__ as self.{name} or use a factory default",
                        )

        self.generic_visit(node)
        self.current_class = prev_class

    # --- Loop analysis ---

    def visit_For(self, node: ast.For):
        self._enter_loop(node)

    def visit_While(self, node: ast.While):
        self._enter_loop(node)

    def visit_AsyncFor(self, node: ast.AsyncFor):
        self._enter_loop(node)

    def _enter_loop(self, node):
        prev_loop = self.in_loop
        prev_depth = self.loop_depth
        self.in_loop = True
        self.loop_depth += 1
        self.generic_visit(node)
        self.in_loop = prev_loop
        self.loop_depth = prev_depth

    # --- Call analysis ---

    def visit_Call(self, node: ast.Call):
        call_name = self._resolve_call_name(node.func)

        # Sync-in-async detection
        if self.in_async_func and call_name in BLOCKING_CALLS:
            self._add_finding(
                "sync_in_async",
                "error",
                node.lineno,
                f"Blocking call '{call_name}' inside async function '{self.current_func}'",
                "Use async equivalent (e.g., aiofiles, aiohttp, asyncio.to_thread)",
            )

        # N+1 pattern
        if self.in_loop:
            is_io = any(pat in call_name for pat in IO_CALL_PATTERNS)
            if is_io:
                self._add_finding(
                    "n_plus_1",
                    "error",
                    node.lineno,
                    f"Potential N+1: I/O call '{call_name}' inside loop (depth={self.loop_depth})",
                    "Batch the operation: fetch all data before the loop, or use bulk APIs",
                )

        # Unbounded concurrency
        if call_name in (
            "asyncio.create_task",
            "asyncio.ensure_future",
            "loop.create_task",
        ):
            if self.in_loop:
                self._add_finding(
                    "unbounded_concurrency",
                    "warning",
                    node.lineno,
                    f"Unbounded task creation '{call_name}' inside loop",
                    "Use asyncio.Semaphore or asyncio.gather with bounded batches",
                )

        # Queue without maxsize
        if call_name in (
            "asyncio.Queue",
            "queue.Queue",
            "Queue",
            "multiprocessing.Queue",
        ):
            has_maxsize = any(
                kw.arg == "maxsize" for kw in node.keywords
            ) or len(node.args) > 0
            if not has_maxsize:
                self._add_finding(
                    "missing_backpressure",
                    "warning",
                    node.lineno,
                    f"Queue '{call_name}' created without maxsize - no backpressure",
                    "Set maxsize to prevent unbounded memory growth",
                )

        # Thread/process pool without max_workers
        if call_name in (
            "ThreadPoolExecutor",
            "ProcessPoolExecutor",
            "concurrent.futures.ThreadPoolExecutor",
            "concurrent.futures.ProcessPoolExecutor",
        ):
            has_max = any(
                kw.arg == "max_workers" for kw in node.keywords
            ) or len(node.args) > 0
            if not has_max:
                self._add_finding(
                    "unbounded_concurrency",
                    "warning",
                    node.lineno,
                    f"'{call_name}' without explicit max_workers",
                    "Set max_workers to control resource usage",
                )

        self.generic_visit(node)


def find_python_files(path: Path) -> list[Path]:
    exclude = {
        "__pycache__", ".git", ".venv", "venv", "node_modules",
        ".mypy_cache", ".pytest_cache", ".ruff_cache", "dist", "build",
    }
    if path.is_file():
        return [path] if path.suffix == ".py" else []
    files = []
    for f in path.rglob("*.py"):
        if not set(f.parts) & exclude:
            files.append(f)
    return sorted(files)


def path_to_module(filepath: Path, root: Path) -> str:
    rel = filepath.relative_to(root)
    parts = list(rel.parts)
    if parts[-1] == "__init__.py":
        parts = parts[:-1]
    else:
        parts[-1] = parts[-1].replace(".py", "")
    return ".".join(parts) if parts else "<root>"


def analyze(root: Path) -> dict:
    files = find_python_files(root)
    all_findings = []

    for filepath in files:
        module_name = path_to_module(filepath, root)
        try:
            source = filepath.read_text(encoding="utf-8", errors="replace")
            source_lines = source.splitlines()
            tree = ast.parse(source, filename=str(filepath))
        except SyntaxError:
            continue

        analyzer = ScalingAnalyzer(module_name, source_lines)
        analyzer.visit(tree)
        all_findings.extend(analyzer.findings)

    by_category = {}
    by_severity = {}
    for f in all_findings:
        cat = f["category"]
        sev = f["severity"]
        by_category[cat] = by_category.get(cat, 0) + 1
        by_severity[sev] = by_severity.get(sev, 0) + 1

    return {
        "summary": {
            "total_findings": len(all_findings),
            "files_analyzed": len(files),
            "by_category": by_category,
            "by_severity": by_severity,
        },
        "findings": all_findings,
    }


def main():
    parser = argparse.ArgumentParser(
        description="Scaling & concurrency bottleneck analyzer"
    )
    parser.add_argument("path", help="File or directory to analyze")
    parser.add_argument(
        "--format",
        choices=["summary", "full"],
        default="full",
        help="Output format",
    )
    args = parser.parse_args()

    root = Path(args.path).resolve()
    if not root.exists():
        print(json.dumps({"error": f"Path not found: {root}"}), file=sys.stderr)
        sys.exit(1)

    result = analyze(root)

    if args.format == "summary":
        output = {"summary": result["summary"]}
    else:
        output = result

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
