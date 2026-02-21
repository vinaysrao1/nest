#!/usr/bin/env python3
"""
Static Rigor Analyzer
Runs pyright (strict mode) and ruff on a Python codebase.
Outputs structured JSON combining all findings.

Usage:
    python static_rigor.py <path> [--fix] [--severity=error|warning|all]
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path


def run_pyright(target: str) -> list[dict]:
    """Run pyright in strict mode, return structured diagnostics."""
    cmd = ["pyright", "--outputjson", "--pythonversion", "3.12", target]
    result = subprocess.run(cmd, capture_output=True, text=True)

    findings = []
    try:
        data = json.loads(result.stdout)
        for diag in data.get("generalDiagnostics", []):
            findings.append({
                "tool": "pyright",
                "file": diag.get("file", ""),
                "line": diag.get("range", {}).get("start", {}).get("line", 0),
                "severity": diag.get("severity", "error"),
                "code": diag.get("rule", ""),
                "message": diag.get("message", ""),
            })
    except json.JSONDecodeError:
        # pyright may not be installed or failed
        findings.append({
            "tool": "pyright",
            "file": "",
            "line": 0,
            "severity": "error",
            "code": "TOOL_ERROR",
            "message": f"pyright failed: {result.stderr.strip() or 'unknown error'}",
        })
    return findings


def run_ruff(target: str, fix: bool = False) -> list[dict]:
    """Run ruff linter, return structured diagnostics."""
    cmd = ["ruff", "check", "--output-format=json"]
    if fix:
        cmd.append("--fix")
    cmd.append(target)

    result = subprocess.run(cmd, capture_output=True, text=True)

    findings = []
    try:
        diagnostics = json.loads(result.stdout) if result.stdout.strip() else []
        for diag in diagnostics:
            loc = diag.get("location", {})
            findings.append({
                "tool": "ruff",
                "file": diag.get("filename", ""),
                "line": loc.get("row", 0),
                "severity": "warning" if diag.get("fix") else "error",
                "code": diag.get("code", ""),
                "message": diag.get("message", ""),
            })
    except json.JSONDecodeError:
        findings.append({
            "tool": "ruff",
            "file": "",
            "line": 0,
            "severity": "error",
            "code": "TOOL_ERROR",
            "message": f"ruff failed: {result.stderr.strip() or 'unknown error'}",
        })
    return findings


def summarize(findings: list[dict]) -> dict:
    """Produce a summary of findings."""
    by_severity = {}
    by_tool = {}
    by_file = {}
    for f in findings:
        sev = f["severity"]
        tool = f["tool"]
        file = f["file"]
        by_severity[sev] = by_severity.get(sev, 0) + 1
        by_tool[tool] = by_tool.get(tool, 0) + 1
        by_file[file] = by_file.get(file, 0) + 1

    # Top 10 files by issue count
    top_files = sorted(by_file.items(), key=lambda x: x[1], reverse=True)[:10]

    return {
        "total": len(findings),
        "by_severity": by_severity,
        "by_tool": by_tool,
        "top_files": [{"file": f, "count": c} for f, c in top_files],
    }


def main():
    parser = argparse.ArgumentParser(description="Static rigor analysis for Python")
    parser.add_argument("path", help="File or directory to analyze")
    parser.add_argument("--fix", action="store_true", help="Auto-fix ruff issues")
    parser.add_argument(
        "--severity",
        choices=["error", "warning", "all"],
        default="all",
        help="Filter by severity",
    )
    args = parser.parse_args()

    target = args.path
    if not Path(target).exists():
        print(json.dumps({"error": f"Path not found: {target}"}), file=sys.stderr)
        sys.exit(1)

    findings = []
    findings.extend(run_pyright(target))
    findings.extend(run_ruff(target, fix=args.fix))

    # Filter by severity
    if args.severity != "all":
        findings = [f for f in findings if f["severity"] == args.severity]

    output = {
        "summary": summarize(findings),
        "findings": findings,
    }

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
