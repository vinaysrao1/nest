---
description: Run call graph, dead code, and structural analysis
allowed-tools: Bash(python tools/analyzers/callgraph.py:*)
argument-hint: <path> [--format=summary|full]
---
Run call graph analysis on the target path.

1. If using python, execute: python tools/analyzers/callgraph.py $ARGUMENTS, if using go, us the built in function to understand the call graph
2. Parse the JSON output
3. Report the summary: total modules, definitions, calls tracked
4. If circular dependencies found: list each cycle and recommend how to break it
5. If god modules found: list them and suggest how to split
6. If high fan-out functions found: identify them as refactoring candidates
7. List untyped public APIs that need type annotations
8. If dead functions found: list each with module and line, confirm they are truly unused before removing
9. If dead classes found: list each, check if they are used via dynamic dispatch or reflection before removing
10. If orphan modules found: list each, verify they are not entry-point scripts before flagging
11. Use --format=full if you need the import graph to understand module relationships

Note on dead code: The analyzer exempts dunder methods, test functions, framework hooks
(visit_*, handle_*, do_*, on_*, process_*), decorated entry points (routes, fixtures, etc.),
and modules with `if __name__ == "__main__"`. False positives are possible for dynamically
dispatched code — verify before deleting.
