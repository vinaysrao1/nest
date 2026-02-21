---
description: Run scaling and concurrency bottleneck analysis
allowed-tools: Bash(python tools/analyzers/scaling.py:*)
argument-hint: <path>
---
Run scaling and concurrency bottleneck analysis on the target path.

1. if python, execute: python tools/analyzers/scaling.py $ARGUMENTS. For go use all possible thread safety and correctness features.
2. Parse the JSON output
3. Group findings by category:
   - sync_in_async: List each blocking call and its async replacement
   - n_plus_1: Show the loop + I/O call and suggest batching strategy
   - shared_mutable_state: Identify the variable and recommend thread-safe alternative
   - unbounded_concurrency: Show the pattern and recommend semaphore/gather approach
   - missing_backpressure: Identify queues needing maxsize
4. For each error-severity finding, provide a concrete code fix
5. For warning-severity findings, explain the risk and when it matters
