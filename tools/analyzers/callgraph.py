#!/usr/bin/env python3
"""
Call Graph Analyzer
Builds a static call graph from a Python codebase using AST analysis.
Identifies: circular dependencies, high fan-out, god modules, coupling metrics,
dead code (unreachable functions, unused classes, orphan modules).

Usage:
    python callgraph.py <path> [--depth=2] [--format=summary|full]
"""

import argparse
import ast
import json
import sys
from collections import defaultdict
from pathlib import Path


class CallGraphBuilder(ast.NodeVisitor):
    """Walks a single file's AST to extract function definitions and calls."""

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.current_class: str | None = None
        self.current_func: str | None = None
        self.definitions: list[dict] = []
        self.class_definitions: list[dict] = []
        self.calls: list[dict] = []
        self.imports: list[dict] = []
        self.name_references: set[str] = set()  # all Name/Attribute refs, not just calls
        self.has_main_guard: bool = False  # if __name__ == "__main__"

    def _qualified_name(self, name: str) -> str:
        parts = [self.module_name]
        if self.current_class:
            parts.append(self.current_class)
        parts.append(name)
        return ".".join(parts)

    def visit_Import(self, node: ast.Import):
        for alias in node.names:
            self.imports.append({
                "module": alias.name,
                "alias": alias.asname,
                "line": node.lineno,
            })
        self.generic_visit(node)

    def visit_ImportFrom(self, node: ast.ImportFrom):
        module = node.module or ""
        for alias in node.names:
            self.imports.append({
                "module": module,
                "name": alias.name,
                "alias": alias.asname,
                "line": node.lineno,
            })
        self.generic_visit(node)

    def visit_ClassDef(self, node: ast.ClassDef):
        qname = self._qualified_name(node.name)
        bases = []
        for base in node.bases:
            resolved = self._resolve_call(base)
            if resolved:
                bases.append(resolved)
                self.name_references.add(resolved)

        self.class_definitions.append({
            "name": qname,
            "short_name": node.name,
            "line": node.lineno,
            "end_line": node.end_lineno or node.lineno,
            "bases": bases,
            "decorators": [self._decorator_name(d) for d in node.decorator_list],
        })

        # Track decorator references
        for dec in node.decorator_list:
            ref = self._resolve_call(dec.func if isinstance(dec, ast.Call) else dec)
            if ref:
                self.name_references.add(ref)

        prev_class = self.current_class
        self.current_class = node.name
        self.generic_visit(node)
        self.current_class = prev_class

    def visit_FunctionDef(self, node: ast.FunctionDef):
        is_async = isinstance(node, ast.AsyncFunctionDef)
        qname = self._qualified_name(node.name)

        # Count parameters
        args = node.args
        param_count = (
            len(args.args)
            + len(args.posonlyargs)
            + len(args.kwonlyargs)
            + (1 if args.vararg else 0)
            + (1 if args.kwarg else 0)
        )

        # Check for type annotations
        has_return_annotation = node.returns is not None
        annotated_params = sum(1 for a in args.args if a.annotation is not None)
        total_params = len(args.args)

        self.definitions.append({
            "name": qname,
            "short_name": node.name,
            "line": node.lineno,
            "end_line": node.end_lineno or node.lineno,
            "is_async": is_async,
            "is_method": self.current_class is not None,
            "param_count": param_count,
            "has_return_type": has_return_annotation,
            "typed_params_ratio": (
                f"{annotated_params}/{total_params}" if total_params > 0 else "n/a"
            ),
            "decorators": [self._decorator_name(d) for d in node.decorator_list],
        })

        # Track decorator references
        for dec in node.decorator_list:
            ref = self._resolve_call(dec.func if isinstance(dec, ast.Call) else dec)
            if ref:
                self.name_references.add(ref)

        prev_func = self.current_func
        self.current_func = qname
        self.generic_visit(node)
        self.current_func = prev_func

    visit_AsyncFunctionDef = visit_FunctionDef

    def visit_Call(self, node: ast.Call):
        caller = self.current_func or f"{self.module_name}.<module>"
        callee = self._resolve_call(node.func)
        if callee:
            self.calls.append({
                "caller": caller,
                "callee": callee,
                "line": node.lineno,
            })
        self.generic_visit(node)

    def visit_If(self, node: ast.If):
        """Detect if __name__ == '__main__' guard."""
        if (
            isinstance(node.test, ast.Compare)
            and isinstance(node.test.left, ast.Name)
            and node.test.left.id == "__name__"
            and any(
                isinstance(c, ast.Constant) and c.value == "__main__"
                for c in node.test.comparators
            )
        ):
            self.has_main_guard = True
        self.generic_visit(node)

    def visit_Name(self, node: ast.Name):
        """Track all name references — catches functions used as callbacks, assigned to vars, etc."""
        self.name_references.add(node.id)
        self.generic_visit(node)

    def visit_Attribute(self, node: ast.Attribute):
        """Track attribute references — catches obj.method used without calling."""
        self.name_references.add(node.attr)
        resolved = self._resolve_call(node)
        if resolved:
            self.name_references.add(resolved)
        self.generic_visit(node)

    def _resolve_call(self, node: ast.expr) -> str | None:
        if isinstance(node, ast.Name):
            return node.id
        elif isinstance(node, ast.Attribute):
            value = self._resolve_call(node.value)
            if value:
                return f"{value}.{node.attr}"
            return node.attr
        return None

    def _decorator_name(self, node: ast.expr) -> str:
        if isinstance(node, ast.Name):
            return node.id
        elif isinstance(node, ast.Attribute):
            return self._resolve_call(node) or "<unknown>"
        elif isinstance(node, ast.Call):
            return self._decorator_name(node.func)
        return "<unknown>"


def find_python_files(path: Path) -> list[Path]:
    """Find all .py files, excluding common non-source dirs."""
    exclude = {
        "__pycache__", ".git", ".venv", "venv", "node_modules",
        ".mypy_cache", ".pytest_cache", ".ruff_cache", "dist", "build",
        ".eggs", "*.egg-info",
    }
    files = []
    if path.is_file():
        return [path] if path.suffix == ".py" else []

    for f in path.rglob("*.py"):
        parts = set(f.parts)
        if not parts & exclude:
            files.append(f)
    return sorted(files)


def path_to_module(filepath: Path, root: Path) -> str:
    """Convert file path to Python module name."""
    rel = filepath.relative_to(root)
    parts = list(rel.parts)
    if parts[-1] == "__init__.py":
        parts = parts[:-1]
    else:
        parts[-1] = parts[-1].replace(".py", "")
    return ".".join(parts) if parts else "<root>"


def detect_circular_deps(import_graph: dict[str, set[str]]) -> list[list[str]]:
    """Find circular dependency chains in the import graph."""
    cycles = []
    visited = set()
    rec_stack = set()

    def dfs(node: str, path: list[str]):
        visited.add(node)
        rec_stack.add(node)
        path.append(node)

        for neighbor in import_graph.get(node, set()):
            if neighbor not in visited:
                dfs(neighbor, path)
            elif neighbor in rec_stack:
                cycle_start = path.index(neighbor)
                cycle = path[cycle_start:] + [neighbor]
                cycles.append(cycle)

        path.pop()
        rec_stack.discard(node)

    for node in import_graph:
        if node not in visited:
            dfs(node, [])

    return cycles


# Names that are entry points or framework hooks — never flag as dead code.
_EXEMPT_NAMES: set[str] = {
    # Python entry points
    "main", "__init__", "__new__", "__del__", "__repr__", "__str__",
    "__hash__", "__eq__", "__ne__", "__lt__", "__le__", "__gt__", "__ge__",
    "__bool__", "__len__", "__getitem__", "__setitem__", "__delitem__",
    "__iter__", "__next__", "__contains__", "__call__", "__enter__",
    "__exit__", "__aenter__", "__aexit__", "__aiter__", "__anext__",
    "__add__", "__radd__", "__sub__", "__mul__", "__truediv__",
    "__floordiv__", "__mod__", "__pow__", "__and__", "__or__", "__xor__",
    "__neg__", "__pos__", "__abs__", "__invert__", "__index__",
    "__get__", "__set__", "__delete__", "__set_name__",
    "__init_subclass__", "__class_getitem__", "__missing__",
    "__format__", "__bytes__", "__fspath__", "__reduce__", "__reduce_ex__",
    "__getattr__", "__setattr__", "__delattr__", "__getattribute__",
    # Testing
    "setUp", "tearDown", "setUpClass", "tearDownClass", "setUpModule",
    "tearDownModule",
    # Framework hooks (Django, Flask, FastAPI, Click, etc.)
    "ready", "get_queryset", "get_context_data", "form_valid",
    "form_invalid", "dispatch", "get_object", "perform_create",
    "perform_update", "perform_destroy",
}

# Decorators that mark functions as entry points even if never directly called.
_ENTRY_POINT_DECORATORS: set[str] = {
    "app.route", "router.get", "router.post", "router.put", "router.delete",
    "router.patch", "app.get", "app.post", "app.put", "app.delete",
    "app.middleware", "app.exception_handler", "app.on_event",
    "pytest.fixture", "fixture",
    "staticmethod", "classmethod", "property", "abstractmethod",
    "overload", "override",
    "click.command", "click.group",
    "celery.task", "shared_task",
    "receiver", "register",
    "lru_cache", "cache", "cached_property",
}


# Method name prefixes that are dispatched by framework base classes, not called directly.
_DISPATCH_PREFIXES: tuple[str, ...] = (
    "visit_",       # ast.NodeVisitor, lxml, etc.
    "depart_",      # docutils/sphinx node visitors
    "handle_",      # http.server, logging handlers
    "do_",          # http.server (do_GET, do_POST)
    "test_",        # unittest/pytest
    "on_",          # event handlers (GUI, async frameworks)
    "process_",     # Django middleware
)


def _is_entry_point(defn: dict) -> bool:
    """Check if a definition is an entry point that should never be flagged."""
    name = defn["short_name"]
    if name in _EXEMPT_NAMES:
        return True
    if any(name.startswith(p) for p in _DISPATCH_PREFIXES):
        return True
    if name.startswith("Test"):
        return True
    for dec in defn.get("decorators", []):
        if dec in _ENTRY_POINT_DECORATORS:
            return True
        # Also match partial decorator names (e.g. "route" matches "app.route")
        for ep in _ENTRY_POINT_DECORATORS:
            if ep.endswith(dec) or dec.endswith(ep.split(".")[-1]):
                return True
    return False


def _is_class_entry_point(cls_defn: dict) -> bool:
    """Check if a class definition is an entry point."""
    name = cls_defn["short_name"]
    if name.startswith("Test"):
        return True
    for dec in cls_defn.get("decorators", []):
        if dec in _ENTRY_POINT_DECORATORS:
            return True
    return False


def detect_dead_code(
    modules: dict,
    all_calls: list[dict],
    all_name_refs: set[str],
    import_graph: dict[str, set[str]],
    imported_names: set[str],
) -> dict:
    """Detect unreachable functions, unused classes, and orphan modules."""

    # Build the set of all callee names (both short and qualified)
    called_names: set[str] = set()
    for call in all_calls:
        callee = call["callee"]
        called_names.add(callee)
        # Also add the short name (last segment)
        if "." in callee:
            called_names.add(callee.rsplit(".", 1)[-1])

    # Merge call-based refs with all name refs (callbacks, assignments, etc.)
    all_refs = called_names | all_name_refs | imported_names

    # --- Dead functions ---
    dead_functions: list[dict] = []
    for mod_name, mod_data in modules.items():
        if "error" in mod_data:
            continue
        for defn in mod_data.get("definitions", []):
            if _is_entry_point(defn):
                continue
            short = defn["short_name"]
            qualified = defn["name"]
            # A function is alive if its short name or qualified name appears
            # in any call, name reference, or import across the codebase.
            if short not in all_refs and qualified not in all_refs:
                dead_functions.append({
                    "module": mod_name,
                    "function": qualified,
                    "short_name": short,
                    "line": defn["line"],
                    "is_method": defn.get("is_method", False),
                    "severity": "warning",
                })

    # --- Dead classes ---
    dead_classes: list[dict] = []
    for mod_name, mod_data in modules.items():
        if "error" in mod_data:
            continue
        for cls_defn in mod_data.get("class_definitions", []):
            if _is_class_entry_point(cls_defn):
                continue
            short = cls_defn["short_name"]
            qualified = cls_defn["name"]
            if short not in all_refs and qualified not in all_refs:
                dead_classes.append({
                    "module": mod_name,
                    "class": qualified,
                    "short_name": short,
                    "line": cls_defn["line"],
                    "severity": "warning",
                })

    # --- Orphan modules (never imported by any other module in the project) ---
    all_imported_modules: set[str] = set()
    for deps in import_graph.values():
        all_imported_modules.update(deps)

    orphan_modules: list[dict] = []
    for mod_name, mod_data in modules.items():
        if "error" in mod_data:
            continue
        if mod_name in all_imported_modules:
            continue
        # Exempt __init__, __main__, and top-level entry scripts
        short = mod_name.rsplit(".", 1)[-1] if "." in mod_name else mod_name
        if short in ("__init__", "__main__", "conftest", "setup", "manage", "<root>"):
            continue
        # Also check the actual filename
        fname = Path(mod_data.get("file", "")).name
        if fname in ("__init__.py", "__main__.py", "conftest.py", "setup.py", "manage.py"):
            continue
        # Exempt modules that define a main() or have if __name__ == "__main__"
        has_main = any(
            d["short_name"] == "main"
            for d in mod_data.get("definitions", [])
        )
        if has_main or mod_data.get("has_main_guard", False):
            continue
        # Exempt single-module projects
        if len(modules) <= 1:
            continue
        orphan_modules.append({
            "module": mod_name,
            "file": mod_data.get("file", ""),
            "definition_count": mod_data.get("definition_count", 0),
            "severity": "info",
        })

    return {
        "dead_functions": dead_functions,
        "dead_classes": dead_classes,
        "orphan_modules": orphan_modules,
    }


def analyze_codebase(root: Path) -> dict:
    """Full analysis of a Python codebase."""
    files = find_python_files(root)

    modules = {}
    all_calls = []
    all_name_refs: set[str] = set()
    imported_names: set[str] = set()
    import_graph: dict[str, set[str]] = defaultdict(set)

    for filepath in files:
        module_name = path_to_module(filepath, root)
        try:
            source = filepath.read_text(encoding="utf-8", errors="replace")
            tree = ast.parse(source, filename=str(filepath))
        except SyntaxError as e:
            modules[module_name] = {
                "file": str(filepath.relative_to(root)),
                "error": f"SyntaxError: {e.msg} at line {e.lineno}",
            }
            continue

        builder = CallGraphBuilder(module_name)
        builder.visit(tree)

        for imp in builder.imports:
            imp_module = imp["module"]
            if imp_module:
                import_graph[module_name].add(imp_module)
            # Track specifically imported names (from X import Y)
            imp_name = imp.get("name")
            if imp_name:
                imported_names.add(imp_name)

        all_name_refs.update(builder.name_references)

        modules[module_name] = {
            "file": str(filepath.relative_to(root)),
            "definitions": builder.definitions,
            "class_definitions": builder.class_definitions,
            "imports": builder.imports,
            "call_count": len(builder.calls),
            "definition_count": len(builder.definitions),
            "class_count": len(builder.class_definitions),
            "has_main_guard": builder.has_main_guard,
        }
        all_calls.extend(builder.calls)

    fan_out = defaultdict(int)
    fan_in = defaultdict(int)
    for call in all_calls:
        fan_out[call["caller"]] += 1
        fan_in[call["callee"]] += 1

    circular_deps = detect_circular_deps(import_graph)

    god_modules = [
        {"module": name, "definitions": data["definition_count"]}
        for name, data in modules.items()
        if isinstance(data.get("definition_count"), int) and data["definition_count"] > 20
    ]
    god_modules.sort(key=lambda x: x["definitions"], reverse=True)

    high_fan_out = [
        {"function": fn, "outgoing_calls": count}
        for fn, count in fan_out.items()
        if count > 10
    ]
    high_fan_out.sort(key=lambda x: x["outgoing_calls"], reverse=True)

    high_fan_in = [
        {"function": fn, "incoming_calls": count}
        for fn, count in fan_in.items()
        if count > 5
    ]
    high_fan_in.sort(key=lambda x: x["incoming_calls"], reverse=True)

    untyped_apis = []
    for mod_name, mod_data in modules.items():
        if "error" in mod_data:
            continue
        for defn in mod_data.get("definitions", []):
            if (
                not defn["short_name"].startswith("_")
                and not defn["has_return_type"]
            ):
                untyped_apis.append({
                    "module": mod_name,
                    "function": defn["name"],
                    "line": defn["line"],
                })

    dead_code = detect_dead_code(
        modules, all_calls, all_name_refs, import_graph, imported_names,
    )

    return {
        "summary": {
            "total_modules": len(modules),
            "total_files": len(files),
            "total_definitions": sum(
                d.get("definition_count", 0)
                for d in modules.values()
                if isinstance(d.get("definition_count"), int)
            ),
            "total_calls_tracked": len(all_calls),
            "circular_dependencies": len(circular_deps),
            "god_modules": len(god_modules),
            "high_fan_out_functions": len(high_fan_out),
            "untyped_public_apis": len(untyped_apis),
            "dead_functions": len(dead_code["dead_functions"]),
            "dead_classes": len(dead_code["dead_classes"]),
            "orphan_modules": len(dead_code["orphan_modules"]),
        },
        "issues": {
            "circular_dependencies": circular_deps[:10],
            "god_modules": god_modules[:10],
            "high_fan_out": high_fan_out[:15],
            "high_fan_in": high_fan_in[:15],
            "untyped_public_apis": untyped_apis[:20],
            "dead_functions": dead_code["dead_functions"][:30],
            "dead_classes": dead_code["dead_classes"][:20],
            "orphan_modules": dead_code["orphan_modules"][:10],
        },
        "modules": modules,
        "import_graph": {k: sorted(v) for k, v in import_graph.items()},
    }


def main():
    parser = argparse.ArgumentParser(description="Python call graph analyzer")
    parser.add_argument("path", help="File or directory to analyze")
    parser.add_argument(
        "--format",
        choices=["summary", "full"],
        default="summary",
        help="Output format (summary=issues only, full=everything)",
    )
    args = parser.parse_args()

    root = Path(args.path).resolve()
    if not root.exists():
        print(json.dumps({"error": f"Path not found: {root}"}), file=sys.stderr)
        sys.exit(1)

    result = analyze_codebase(root)

    if args.format == "summary":
        output = {
            "summary": result["summary"],
            "issues": result["issues"],
        }
    else:
        output = result

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
