---
description: Test-driven debug loop — cycles until all tests pass
allowed-tools: Bash(python -m pytest:*), Bash(python tools/analyzers/*:*), Read, Write, Edit, Glob, Grep, Task
argument-hint: <test_path> [source_path]
---
# Debug Loop

Run a test-driven debug loop that cycles until all tests pass.

**Input:** $ARGUMENTS should be the path to a test file or test directory, plus optionally the path to the source under test.
Example: `/debug-loop tests/test_parser.py src/parser/`

---

## Phase 0: Setup

1. Read `working_docs/BUG_TRACKER.md`. This is the persistent state file for this loop.
2. Identify the test file(s) and source path from the arguments.
3. If `working_docs/BUG_TRACKER.md` already has items in TO DO or IN PROGRESS, ask the user: "Resume existing debug session or start fresh?"

---

## Phase 1: Validation Agent — Run Tests

Use **Sonnet 4.5** (`claude-sonnet-4-5-20250929`) for this phase.

Follow the Validation Agent instructions from `.claude/agents/validation.md`, but focus specifically on the test execution loop:

1. Run the test suite:
   ```bash
   python -m pytest $TEST_PATH -v --tb=long 2>&1
   ```

2. If ALL tests pass, report **ALL PASS** and stop the loop. Done.

3. If any tests fail, produce a **Failure Summary** in this exact format:
   ```
   ## Test Failure Summary — Iteration N

   **Tests run:** X | **Passed:** Y | **Failed:** Z

   ### Failed Tests
   1. `test_name` — CORRECTNESS | PERFORMANCE | EDGE_CASE
      Expected: <what the test expected>
      Got: <what actually happened>
      Location: <file:line of the assertion that failed>
      Traceback hint: <the key line from the traceback, not the full dump>

   2. ...
   ```

   Classify each failure as:
   - **CORRECTNESS** — wrong output value
   - **PERFORMANCE** — timeout, complexity violation, or resource limit exceeded
   - **EDGE_CASE** — specific input that triggers incorrect behavior
   - **REGRESSION** — previously passing test now fails

4. Also run the four static analyzers on the source path:
   ```bash
   python tools/analyzers/static_rigor.py $SOURCE_PATH
   python tools/analyzers/callgraph.py $SOURCE_PATH --format=summary
   python tools/analyzers/scaling.py $SOURCE_PATH
   python tools/analyzers/contracts.py $SOURCE_PATH --strict
   ```
   Append a brief tool summary to the Failure Summary.

5. Hand the Failure Summary to Phase 2.

---

## Phase 2: Architect Agent — Diagnose and Plan

Use **Opus 4.5** (`claude-opus-4-5-20250514`) for this phase.

Follow the Architect Agent instructions from `.claude/agents/architect.md`, specifically the Root Cause Analysis Protocol. For each failed test:

1. **Read the failing test** to understand what behavior it asserts.
2. **Trace the code path** from the test's entry point through the source. Use callgraph output if available.
3. **Form a hypothesis** for why the test fails. Be specific:
   - CORRECTNESS: "Function X returns Y because condition Z is evaluated wrong on line N"
   - PERFORMANCE: "Loop in function X is O(n²) because of nested iteration over Y; needs to be restructured to O(n log n) using Z"
   - EDGE_CASE: "Function X does not handle input where Y is empty/negative/None"
4. **Write the fix plan** — what specifically needs to change in which file, at which function.

After diagnosing all failures:

5. **Update `working_docs/BUG_TRACKER.md`** — add one line per planned fix to the TO DO section:
   ```
   ## TO DO
   - [ ] Fix O(n²) in sort_results(): replace nested loop with heap-based merge (test_sort_large_input)
   - [ ] Handle empty input in parse_config(): add early return (test_parse_empty)
   ```
   Each line must reference the test it fixes.

6. **Produce an Implementation Plan** for the SWE Agent. This is a focused version of the Architect's design document, containing only:
   - The hypothesis for each failure (1-2 sentences)
   - The exact file + function to change
   - The specific change to make (pseudocode or precise description)
   - Any invariants the change must preserve

7. Hand the Implementation Plan to Phase 3 (Reviewer).

---

## Phase 3: Reviewer Agent — Review the Plan

Use **Opus 4.5** (`claude-opus-4-5-20250514`) for this phase.

Follow the Reviewer Agent instructions from `.claude/agents/reviewer.md`. Review the Architect's Implementation Plan:

- Does each hypothesis logically explain the test failure?
- Does each proposed fix actually address the root cause, or just the symptom?
- Could any proposed fix break other tests or violate existing contracts?
- For performance fixes: is the proposed complexity actually achievable? Is the approach correct?
- Are there any missing failure cases the Architect did not address?

**If the plan has issues:** send specific revision requests back to Phase 2. The Architect revises and resubmits. Max 2 revision cycles, then escalate to user.

**If the plan is approved:** hand it to Phase 4.

---

## Phase 4: SWE Agent — Implement the Fix

Use **Sonnet 4.5** (`claude-sonnet-4-5-20250929`) for this phase.

Follow the SWE Agent instructions from `.claude/agents/swe.md`:

1. Read the approved Implementation Plan.
2. Run baseline tools on the files to be changed.
3. Implement each fix exactly as specified.
4. Run the four analyzers on changed files. Error counts must not increase.
5. Hand the implementation to Phase 5 (Reviewer).

---

## Phase 5: Reviewer Agent — Review the Code

Use **Opus 4.5** (`claude-opus-4-5-20250514`) for this phase.

Follow the Reviewer Agent's Implementation Review Checklist from `.claude/agents/reviewer.md`:

- Does the code match the approved plan?
- No new analyzer errors?
- No circular dependencies introduced?
- Error handling correct?

**If code has issues:** send back to Phase 4 with specific fixes. Max 2 cycles.

**If code is approved:** proceed to Phase 6.

---

## Phase 6: Architect Agent — Update Bug Tracker

Use **Opus 4.5** (`claude-opus-4-5-20250514`) for this phase.

1. Open `working_docs/BUG_TRACKER.md`.
2. Move every implemented item from TO DO to DONE, adding the iteration number:
   ```
   ## DONE
   - [x] Fix O(n²) in sort_results(): replaced with heap merge (iteration 1)
   ```

---

## Phase 7: Validation Agent — Re-run Tests (Loop Back)

Use **Sonnet 4.5** (`claude-sonnet-4-5-20250929`) for this phase.

This is the same as Phase 1. Run the full test suite again.

- **ALL PASS:** Report success. Print final BUG_TRACKER.md state. Done.
- **Some still fail:** Produce a new Failure Summary. Note which tests are **newly passing** (progress) and which are **still failing** or **newly failing** (regression). Go back to Phase 2.

---

## Loop Controls

- **Max iterations: 5.** If after 5 full cycles tests still fail, stop and report to the user with the current BUG_TRACKER.md state and remaining failures. The user decides next steps.
- **Regression detection:** If a fix causes a previously-passing test to fail, that is a P0 issue. The Architect must address it in the next iteration before tackling other failures.
- **Progress tracking:** Each iteration's Failure Summary should note the delta from the previous iteration. If zero progress is made in an iteration (same tests fail with same errors), escalate to the user immediately — the loop is stuck.
- **Partial success:** If some tests are fixed but others remain, continue the loop. Only stop early on zero-progress or max iterations.

---

## File Layout

```
working_docs/
  BUG_TRACKER.md          — persistent TO DO / DONE tracker across iterations
```

The BUG_TRACKER.md is the single source of truth for what has been attempted and what remains.
