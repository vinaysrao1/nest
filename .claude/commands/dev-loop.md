---
description: End-to-end development loop — architect plans, SWE agents implement in parallel, reviewer reviews, validation tests
allowed-tools: Bash(python -m pytest:*), Bash(python tools/analyzers/*:*), Read, Write, Edit, Glob, Grep, Task
argument-hint: "<task description>" <filename>
---
# Dev Loop

**Input:** $ARGUMENTS must be a quoted task description followed by a file path.
Example: `/dev-loop "add retry logic to all API calls" src/api/`

Parse arguments:
- `TASK` = the quoted string
- `FILENAME` = the file or directory path after the quoted string

---

## Phase 1: Architect + SWE (parallel)

Do both of these simultaneously:

**A) Architect — Analyze FILENAME**

Spawn a Task using the **architect** agent (`.claude/agents/architect.md`):
```
Read and analyze the file(s) at FILENAME.
Run all four analyzers to establish baseline metrics.
Produce a design document for this task: TASK

In your Module Plan, mark which work items are INDEPENDENT (can be done in parallel)
vs SEQUENTIAL (depends on another item). Group independent items into parallel batches.
```

**B) SWE agents — Implement TASK in parallel**

Based on the task description, spawn **swe** agent Tasks (`.claude/agents/swe.md`) in parallel to implement TASK against the code in FILENAME.

Each SWE agent should:
1. Read the relevant source files.
2. Run baseline tools on files it will change.
3. Implement its portion of the task.
4. Run tools after. Error counts must not increase.
5. **If there are errors during implementation: do not hack around them.** Find the root cause using all four analyzers, `callgraph --format=full`, grep, reading the code — whatever it takes. Fix the actual cause, not the symptom.
6. Report: files changed, baseline vs post tool results, tests added.

After both A and B complete, proceed to Phase 2.

---

## Phase 2: Reviewer — Thorough Review

Spawn a Task using the **reviewer** agent (`.claude/agents/reviewer.md`):
```
Review this implementation thoroughly. Use every tool and skill at your disposal.

Task: TASK
Design document: ARCHITECT'S DESIGN FROM PHASE 1A
Files changed: LIST FROM ALL SWE AGENTS
Tool results (baseline): ARCHITECT'S BASELINE FROM PHASE 1A
Tool results (current): LATEST POST-IMPLEMENTATION TOOL RESULTS

Run all four analyzers yourself. Read every changed file. Check the full
Implementation Review Checklist — critical, important, and advisory items.
Cross-reference the implementation against the design. Check for regressions
in files that were NOT changed but might be affected.
```

**If verdict is APPROVED:** → Go to Phase 4 (Validation).

**If verdict is REVISE or REJECTED:** → Go to Phase 3.

---

## Phase 3: Architect Plans Fix, SWE Agents Fix

**A) Architect — Plan fixes**

Spawn a Task using the **architect** agent:
```
The reviewer found these issues:

REVIEWER'S FULL ISSUE LIST (critical, important, advisory)

Original design: DESIGN DOCUMENT
Files changed: FILE LIST

Produce a Fix Plan:
- Root cause of each issue
- Exact fix: file, function, change
- Invariants preserved
Mark independent fixes for parallel execution.
```

**B) SWE agents — Apply fixes**

Spawn SWE agent Tasks (parallel where independent) to implement the architect's fix plan. Same rules as Phase 1B: no hacking, find root causes, use all tools.

After fixes complete, re-submit to the **reviewer** (repeat Phase 2).

**Max Phase 2↔3 cycles: 2.** If the reviewer still has critical issues after 2 rounds, stop and report to the user.

---

## Phase 4: Validation — Test

Spawn a Task using the **validation** agent (`.claude/agents/validation.md`):
```
Validate this implementation.

Task: TASK
Design document: ARCHITECT'S DESIGN
Files changed: FULL LIST
Architect's baseline: BASELINE TOOL NUMBERS

Follow your full 4-phase validation protocol:
1. Tool verification (compare against baseline)
2. Invariant verification (design section 7)
3. Test verification (run pytest)
4. Design conformance (signatures, module plan)
```

**If verdict is PASS or PASS WITH WARNINGS:**
→ Report success. Include: summary of changes, files changed, final metrics vs baseline, test results.

**If verdict is FAIL:**
→ Produce a failure summary and stop.

---

## Failure Summary

On validation failure, compile and report:

```
## Dev Loop Result: TASK

**Status: INCOMPLETE — validation failed**

### What was implemented
- Completed items

### Validation failures
- Each failure from validation report

### Tool metrics
| Metric | Baseline | Final | Delta |
|--------|----------|-------|-------|
| static_rigor errors | N | N | +/-N |
| circular deps | N | N | +/-N |
| untyped public APIs | N | N | +/-N |
| scaling errors | N | N | +/-N |
| contract violations | N | N | +/-N |

### Files changed
- file — summary

### Suggested next steps
- Specific actions to resolve remaining failures
- Consider `/debug-loop` for iterative test fixing
```

Stop and let the user decide next steps.

---

## Limits

| Boundary | Max | On exceed |
|----------|-----|-----------|
| Review↔Fix cycles (Phase 2↔3) | 2 | Stop, report to user |
| Validation | 1 attempt | On fail, summary and stop |
