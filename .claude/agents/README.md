# Agent Orchestration

## Overview

Four agents operate in a strict pipeline. No stage can be skipped.

```
User Request
    │
    ▼
┌─────────────┐     design doc      ┌──────────────┐
│  Architect   │ ──────────────────► │   Reviewer    │
│  Agent       │ ◄────────────────── │   Agent       │
└─────────────┘   revise / approve   └──────────────┘
    │                                       │
    │ approved design                       │
    ▼                                       │
┌─────────────┐     implementation   ┌──────────────┐
│  SWE Agent   │ ──────────────────► │   Reviewer    │
│              │ ◄────────────────── │   Agent       │
└─────────────┘   revise / approve   └──────────────┘
    │                                       
    │ approved implementation               
    ▼                                       
┌─────────────┐                             
│  Validation  │ ──► PASS: task complete    
│  Agent       │ ──► FAIL: route to fixer   
└─────────────┘                             
```

## Pipeline Rules

1. **Architect → Reviewer → SWE → Reviewer → Validation.** This is the default flow.
2. **Trivial tasks** (single function fix, typo, config change) can skip Architect design. The Architect triages and says "trivial, hand to SWE" with a one-line description.
3. **Max 2 revision cycles** at any review stage. If something bounces back 3 times, escalate to the user for a decision.
4. **Tool runs are mandatory** at every stage. The Architect runs tools to understand current state. The SWE Agent runs tools before and after implementation. The Reviewer runs tools to verify. The Validation Agent runs tools as the final check.

## Agent Files

| Agent | Model | File | Purpose |
|-------|-------|------|---------|
| Architect | **Opus 4.5** | `architect.md` | Design, root-cause analysis, module planning |
| Reviewer | **Opus 4.5** | `reviewer.md` | Adversarial review of designs and implementations |
| SWE | **Sonnet 4.5** | `swe.md` | Code implementation following approved designs |
| Validation | **Sonnet 4.5** | `validation.md` | Final verification against design specs and tool baselines |

**Rationale:** Opus handles tasks requiring judgment under ambiguity (architecture, adversarial review). Sonnet handles tasks that are well-specified and execution-heavy (coding to a spec, running checklists).

## Tools

All agents share access to the same four analyzers in `tools/analyzers/`:

| Tool | Primary Users | Purpose |
|------|---------------|---------|
| `static_rigor.py` | SWE, Reviewer, Validation | Type errors, lint violations |
| `callgraph.py` | Architect, Reviewer, Validation | Module structure, circular deps |
| `scaling.py` | Architect, SWE, Validation | Concurrency and perf issues |
| `contracts.py` | Architect, Reviewer, Validation | API contracts, coupling |

## Debug Loop

The `/debug-loop` command runs a test-driven fix cycle. Invoke with:
```
/debug-loop <test_path> [source_path]
```

```
┌─────────────────────────────────────────────────────┐
│  Phase 1: Validation (Sonnet) — run tests       │
│  ALL PASS? ─── yes ─▶ DONE                        │
│       │ no                                        │
│       ▼                                             │
│  Phase 2: Architect (Opus) — diagnose + plan     │
│       │ update BUG_TRACKER.md TO DO                │
│       ▼                                             │
│  Phase 3: Reviewer (Opus) — review plan          │
│       │                                             │
│       ▼                                             │
│  Phase 4: SWE (Sonnet) — implement fix           │
│       │                                             │
│       ▼                                             │
│  Phase 5: Reviewer (Opus) — review code          │
│       │                                             │
│       ▼                                             │
│  Phase 6: Architect (Opus) — TO DO → DONE        │
│       │                                             │
│       ▼                                             │
│  Phase 7: Validation (Sonnet) — re-run tests     │
│  ALL PASS? ─── yes ─▶ DONE                        │
│       │ no ── loop back to Phase 2                  │
└─────────────────────────────────────────────────────┘
```

Max 5 iterations. Escalates to user on zero progress or max iterations reached.

State is tracked in `working_docs/BUG_TRACKER.md`.

## Quick Reference: Who Does What

| Activity | Agent |
|----------|-------|
| Understand the problem | Architect |
| Root-cause a bug | Architect |
| Decide module boundaries | Architect |
| Define function signatures | Architect |
| Approve a design | Reviewer |
| Write code | SWE |
| Write tests | SWE |
| Approve code | Reviewer |
| Verify invariants hold | Validation |
| Verify tests pass | Validation |
| Verify tools are clean | Validation |
