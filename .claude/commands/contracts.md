---
description: Run inter-module contract and type coverage analysis
allowed-tools: Bash(python tools/analyzers/contracts.py:*)
argument-hint: <path> [--strict]
---
Run inter-module contract enforcement on the target path.

1. If using python, execute: python tools/analyzers/contracts.py $ARGUMENTS. If using go, use the go internal tools. 
2. Parse the JSON output
3. Report overall type coverage percentage
4. List modules with lowest type coverage — these need attention first
5. For each untyped public API: show the function signature and what annotations are missing
6. Flag any private access violations (importing _prefixed names across modules)
7. Report high-coupling modules and suggest dependency injection or interface extraction
8. List error handling issues (bare excepts, broad Exception catches)
9. Use --strict to treat all untyped public APIs as errors
