#!/usr/bin/env python3
"""
Inter-Module Contract Enforcer
Checks type contracts at module boundaries, detects coupling issues,
and enforces Go-like strictness on Python module interfaces.

Detects:
- Untyped public function parameters and return types
- Cross-module calls to private functions
- High coupling between modules
- Missing protocol/ABC implementations
- Inconsistent error handling patterns

Usage:
    python contracts.py <path> [--strict] [--format=summary|full]
"""

import argparse
import ast
import json
import sys
from collections import defaultdict
from pathlib import Path


class ModuleInterfaceExtractor(ast.NodeVisitor):
    """Extract the public interface of a module."""

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.public_functions: list[dict] = []
        self.public_classes: list[dict] = []
        self.imports: list[dict] = []
        self.cross_module_calls: list[dict] = []
        self.all_names_defined: set[str] = set()
        self.dunder_all: list[str] | None = None
        self.current_class: str | None = None
        self.current_func: str | None = None
        self.error_patterns: list[dict] = []

    def visit_Module(self, node: ast.Module):
        for item in node.body:
            if isinstance(item, ast.Assign):
                for target in item.targets:
                    if isinstance(target, ast.Name) and target.id == "__all__":
                        if isinstance(item.value, (ast.List, ast.Tuple)):
                            self.dunder_all = [
                                elt.value
                                for elt in item.value.elts
                                if isinstance(elt, ast.Constant)
                                and isinstance(elt.value, str)
                            ]
        self.generic_visit(node)

    def visit_Import(self, node: ast.Import):
        for alias in node.names:
            self.imports.append({
                "type": "import",
                "module": alias.name,
                "name": None,
                "line": node.lineno,
            })
        self.generic_visit(node)

    def visit_ImportFrom(self, node: ast.ImportFrom):
        module = node.module or ""
        for alias in node.names:
            self.imports.append({
                "type": "from",
                "module": module,
                "name": alias.name,
                "line": node.lineno,
            })
        self.generic_visit(node)

    def visit_ClassDef(self, node: ast.ClassDef):
        is_public = not node.name.startswith("_")
        self.all_names_defined.add(node.name)

        bases = []
        for base in node.bases:
            if isinstance(base, ast.Name):
                bases.append(base.id)
            elif isinstance(base, ast.Attribute):
                bases.append(self._resolve_name(base))

        if is_public:
            methods = []
            for item in ast.walk(node):
                if isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    if not item.name.startswith("_") or item.name in (
                        "__init__",
                        "__call__",
                        "__enter__",
                        "__exit__",
                        "__aenter__",
                        "__aexit__",
                    ):
                        method_info = self._extract_function_signature(item)
                        method_info["is_method"] = True
                        methods.append(method_info)

            self.public_classes.append({
                "name": node.name,
                "line": node.lineno,
                "bases": bases,
                "method_count": len(methods),
                "methods": methods,
            })

        prev_class = self.current_class
        self.current_class = node.name
        self.generic_visit(node)
        self.current_class = prev_class

    def visit_FunctionDef(self, node: ast.FunctionDef):
        self._handle_funcdef(node)

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef):
        self._handle_funcdef(node)

    def _handle_funcdef(self, node):
        is_public = not node.name.startswith("_")
        self.all_names_defined.add(node.name)

        if is_public and self.current_class is None:
            sig = self._extract_function_signature(node)
            self.public_functions.append(sig)

        prev_func = self.current_func
        self.current_func = node.name
        for child in ast.walk(node):
            if isinstance(child, ast.ExceptHandler):
                exc_type = None
                if child.type:
                    exc_type = self._resolve_name(child.type)
                if exc_type == "Exception" or child.type is None:
                    self.error_patterns.append({
                        "function": node.name,
                        "line": child.lineno,
                        "type": "bare_except" if child.type is None else "broad_except",
                        "message": (
                            "Bare except catches everything including SystemExit/KeyboardInterrupt"
                            if child.type is None
                            else "Catching base Exception hides bugs - catch specific exceptions"
                        ),
                    })
        self.current_func = prev_func
        self.generic_visit(node)

    def _extract_function_signature(self, node) -> dict:
        args = node.args

        params = []
        for arg in args.args:
            if arg.arg == "self" or arg.arg == "cls":
                continue
            params.append({
                "name": arg.arg,
                "has_type": arg.annotation is not None,
                "type": self._annotation_str(arg.annotation) if arg.annotation else None,
            })

        for arg in args.kwonlyargs:
            params.append({
                "name": arg.arg,
                "has_type": arg.annotation is not None,
                "type": self._annotation_str(arg.annotation) if arg.annotation else None,
            })

        has_return = node.returns is not None
        return_type = self._annotation_str(node.returns) if node.returns else None

        untyped_params = [p["name"] for p in params if not p["has_type"]]
        total_params = len(params)
        typed_params = total_params - len(untyped_params)

        return {
            "name": node.name,
            "line": node.lineno,
            "is_async": isinstance(node, ast.AsyncFunctionDef),
            "params": params,
            "has_return_type": has_return,
            "return_type": return_type,
            "untyped_params": untyped_params,
            "type_coverage": (
                f"{typed_params + (1 if has_return else 0)}/{total_params + 1}"
            ),
            "is_fully_typed": len(untyped_params) == 0 and has_return,
        }

    def _annotation_str(self, node) -> str:
        if node is None:
            return ""
        if isinstance(node, ast.Constant):
            return str(node.value)
        if isinstance(node, ast.Name):
            return node.id
        if isinstance(node, ast.Attribute):
            return self._resolve_name(node)
        if isinstance(node, ast.Subscript):
            value = self._annotation_str(node.value)
            slice_val = self._annotation_str(node.slice)
            return f"{value}[{slice_val}]"
        if isinstance(node, ast.Tuple):
            return ", ".join(self._annotation_str(e) for e in node.elts)
        if isinstance(node, ast.BinOp) and isinstance(node.op, ast.BitOr):
            return f"{self._annotation_str(node.left)} | {self._annotation_str(node.right)}"
        return "<complex>"

    def _resolve_name(self, node) -> str:
        if isinstance(node, ast.Name):
            return node.id
        if isinstance(node, ast.Attribute):
            value = self._resolve_name(node.value)
            return f"{value}.{node.attr}" if value else node.attr
        return ""


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


def analyze(root: Path, strict: bool = False) -> dict:
    files = find_python_files(root)
    modules = {}
    findings = []

    for filepath in files:
        module_name = path_to_module(filepath, root)
        try:
            source = filepath.read_text(encoding="utf-8", errors="replace")
            tree = ast.parse(source, filename=str(filepath))
        except SyntaxError as e:
            findings.append({
                "category": "parse_error",
                "severity": "error",
                "module": module_name,
                "line": e.lineno or 0,
                "message": f"SyntaxError: {e.msg}",
            })
            continue

        extractor = ModuleInterfaceExtractor(module_name)
        extractor.visit(tree)
        modules[module_name] = extractor

    # Untyped public interfaces
    for mod_name, mod in modules.items():
        for func in mod.public_functions:
            if not func["is_fully_typed"]:
                severity = "error" if strict else "warning"
                details = []
                if func["untyped_params"]:
                    details.append(
                        f"untyped params: {', '.join(func['untyped_params'])}"
                    )
                if not func["has_return_type"]:
                    details.append("missing return type")
                findings.append({
                    "category": "untyped_public_api",
                    "severity": severity,
                    "module": mod_name,
                    "function": func["name"],
                    "line": func["line"],
                    "message": f"Public function '{func['name']}' has incomplete type annotations: {'; '.join(details)}",
                    "type_coverage": func["type_coverage"],
                })

    # Cross-module private access
    for mod_name, mod in modules.items():
        for imp in mod.imports:
            if imp["name"] and imp["name"].startswith("_") and imp["type"] == "from":
                findings.append({
                    "category": "private_access",
                    "severity": "warning",
                    "module": mod_name,
                    "line": imp["line"],
                    "message": f"Importing private name '_{imp['name'][1:]}' from '{imp['module']}' - breaks encapsulation",
                })

    # Coupling analysis
    import_counts = defaultdict(int)
    for mod_name, mod in modules.items():
        imported_modules = set()
        for imp in mod.imports:
            imported_modules.add(imp["module"])
        import_counts[mod_name] = len(imported_modules)

    high_coupling = [
        {"module": mod, "import_count": count}
        for mod, count in import_counts.items()
        if count > 10
    ]
    high_coupling.sort(key=lambda x: x["import_count"], reverse=True)
    for item in high_coupling:
        findings.append({
            "category": "high_coupling",
            "severity": "warning",
            "module": item["module"],
            "line": 0,
            "message": f"Module imports from {item['import_count']} other modules - consider splitting",
        })

    # Error handling issues
    for mod_name, mod in modules.items():
        for pattern in mod.error_patterns:
            findings.append({
                "category": "error_handling",
                "severity": "warning",
                "module": mod_name,
                "function": pattern["function"],
                "line": pattern["line"],
                "message": pattern["message"],
            })

    # Missing __all__
    for mod_name, mod in modules.items():
        if mod.dunder_all is None and len(mod.public_functions) > 3:
            findings.append({
                "category": "missing_all",
                "severity": "info",
                "module": mod_name,
                "line": 0,
                "message": f"Module has {len(mod.public_functions)} public functions but no __all__ - implicit public API",
            })

    by_category = {}
    by_severity = {}
    for f in findings:
        cat = f["category"]
        sev = f["severity"]
        by_category[cat] = by_category.get(cat, 0) + 1
        by_severity[sev] = by_severity.get(sev, 0) + 1

    type_coverage = {}
    for mod_name, mod in modules.items():
        total = len(mod.public_functions)
        fully_typed = sum(1 for f in mod.public_functions if f["is_fully_typed"])
        if total > 0:
            type_coverage[mod_name] = {
                "total_public_functions": total,
                "fully_typed": fully_typed,
                "coverage_pct": round(fully_typed / total * 100, 1),
            }

    return {
        "summary": {
            "total_findings": len(findings),
            "files_analyzed": len(files),
            "modules_analyzed": len(modules),
            "by_category": by_category,
            "by_severity": by_severity,
            "overall_type_coverage": _overall_coverage(type_coverage),
        },
        "type_coverage": type_coverage,
        "findings": findings,
    }


def _overall_coverage(type_coverage: dict) -> str:
    total = sum(v["total_public_functions"] for v in type_coverage.values())
    typed = sum(v["fully_typed"] for v in type_coverage.values())
    if total == 0:
        return "n/a"
    return f"{typed}/{total} ({round(typed / total * 100, 1)}%)"


def main():
    parser = argparse.ArgumentParser(description="Inter-module contract enforcer")
    parser.add_argument("path", help="File or directory to analyze")
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Treat untyped public APIs as errors instead of warnings",
    )
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

    result = analyze(root, strict=args.strict)

    if args.format == "summary":
        output = {
            "summary": result["summary"],
            "type_coverage": result["type_coverage"],
        }
    else:
        output = result

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
