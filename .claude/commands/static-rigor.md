---
description: Run pyright + ruff static analysis
allowed-tools: Bash(python tools/analyzers/static_rigor.py:*)
argument-hint: <path> [--fix]
---
Run static rigor analysis (pyright + ruff) on the target path.

1. if python, execute: python tools/analyzers/static_rigor.py $ARGUMENTS. For go use built in static rigor analysis
2. Parse the JSON output
3. If there are errors: list each with file:line and the message
4. If there are warnings: summarize the count and top files
5. Suggest specific fixes for each error
6. If --fix was passed, confirm what ruff auto-fixed
