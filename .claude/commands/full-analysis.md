---
description: Run all four analyzers and produce a prioritized report
allowed-tools: Bash(python tools/analyzers/static_rigor.py:*), Bash(python tools/analyzers/callgraph.py:*), Bash(python tools/analyzers/scaling.py:*), Bash(python tools/analyzers/contracts.py:*)
argument-hint: <path>
---
Run ALL analyzers on the target path and produce a comprehensive report.

1. For python run all four analyzers in sequence:
   - python tools/analyzers/static_rigor.py $ARGUMENTS
   - python tools/analyzers/callgraph.py $ARGUMENTS --format=summary
   - python tools/analyzers/scaling.py $ARGUMENTS
   - python tools/analyzers/contracts.py $ARGUMENTS --strict
   For go, use built in commands with similar functionalities.

2. Produce a unified report:
   a. Overall health score: count total errors across all tools
   b. Type coverage: from contracts analyzer
   c. Structural issues: from callgraph (circular deps, god modules)
   d. Scaling risks: from scaling analyzer
   e. Lint/type errors: from static_rigor

3. Prioritize findings:
   - P0 (fix now): circular deps, sync-in-async errors, N+1 patterns, parse errors
   - P1 (fix before merge): untyped public APIs, bare excepts, shared mutable state
   - P2 (fix soon): high coupling, high fan-out, missing backpressure, warnings

4. For each P0 finding, provide a concrete fix or code change.
