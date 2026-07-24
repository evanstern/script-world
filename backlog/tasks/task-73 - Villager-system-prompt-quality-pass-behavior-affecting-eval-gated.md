---
id: TASK-73
title: Villager system-prompt quality pass (behavior-affecting; eval-gated)
status: Done
assignee: []
created_date: '2026-07-23 06:34'
updated_date: '2026-07-23 23:22'
labels:
  - review-2026-07-22
  - code-quality
  - teaching-game
dependencies: []
priority: medium
ordinal: 66000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (improvement 2, villager half — the Metatron half shipped in TASK-64), re-verified 2026-07-23: systemPrompt (internal/mind/prompt.go:23-38) uses the agent name five times in one short paragraph and provides no output exemplar. Functional but weak — ironic for a prompt-engineering teaching game, and villager prompts run thousands of times per day on the local tier, so quality here is leverage.

Scope: rewrite for one clear identity statement, the persona block, tight task framing, and (evaluate) one worked exemplar of good tool selection. Constraints: doctrine unchanged — acting-tool-only contract, muse-is-an-action framing, no free-text action path; prompt stays the cacheable prefix (mind the cache_control block boundaries).

THIS IS BEHAVIOR-AFFECTING, NOT A PURE REFACTOR — it must be eval-gated, not vibes-gated: compare before/after on the scripted-stub suite AND a live soak (same seed, N game-hours) measuring rejected_malformed and rejected_cardinality rates, tool-selection distribution sanity, and prompt token count. Ship only if rejection rates do not regress. Record the eval numbers on this task.

Spec: specs/027-villager-prompt-quality
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 New prompt: single identity statement, no redundant name repetition, doctrine framing preserved
- [x] #2 Exemplar included or explicitly rejected with the measured reason
- [x] #3 Before/after eval recorded on the task: rejected_malformed + rejected_cardinality rates and token counts; no regression
- [x] #4 Prompt-cache prefix boundaries unchanged or consciously re-drawn; scripted-stub tests updated and passing
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 2 — The prompt reads as exemplary craft (Priority: P2, built first)
- [x] #8 Spec phase: User Story 3 — The exemplar question gets evidence (Priority: P3, variant build)
- [x] #9 Spec phase: User Story 1 — Villager decisions keep landing (Priority: P1) 🎯 the ship gate
- [x] #10 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit: specs/027-villager-prompt-quality (spec+plan+research+contracts+tasks on main; implementation on branch task-73-villager-prompt-quality, worktree .worktrees/task-73).
Build order: red-first contract tests (C1-C5) -> rewrite (variant `new`) -> exemplar commit (variant `new+exemplar`) -> eval gate: scripted-stub suite + serial live soaks (seed 4242, 6 game-hours, local tier) over refs origin/main / new / new+exemplar; tally rejected_malformed + rejected_cardinality rates, tool distribution, token counts from cog.tool_call events; ship gate = no rejection-rate regression (SC-001), exemplar decided by numbers (FR-004).
Implementation tier: Opus 4.8 via spec-implementer (rubric: doctrine-adjacent behavior change in internal/mind, eval-gated behavior-affecting slice — constitution Principle V). Planning/gating on Fable 5.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation complete (Opus 4.8 spec-implementer per Principle V rubric: doctrine-adjacent behavior change in internal/mind). PR #54 open from .worktrees/task-73 (branch task-73-villager-prompt-quality, tip a99edc1 = shipped `new` variant + wiki re-pin).

EVAL (ship gate) — three serial soaks, seed 4242, 8 game-hours each, local cogito:3b tool_mode json, 16x; villager-planner cog.tool_call verdicts from the event log (full records: specs/027-villager-prompt-quality/eval/ on the branch):

| variant | denom | rejected_malformed | rejected_cardinality | ~tokens |
| old (main b96c028) | 789 | 121 = 15.34% | 0 = 0.00% | 189 |
| new (SHIPPED) | 896 | 103 = 11.50% | 0 = 0.00% | 193 |
| new+exemplar | 982 | 147 = 14.97% | 0 = 0.00% | 251 |

SC-001 PASS: malformed -3.84pp (25% relative), cardinality stays 0. SC-003 PASS: no collapse; muse share halved 34.37%->18.73% (healthiest movement — less ruminating, more doing); every >=5% old tool keeps nonzero share, no major tool >2x.

EXEMPLAR DECISION (AC#2): REJECTED on measurement — worked exemplar pushed malformed back up to 14.97%, cost +58 approx tokens (+30%), and anchored the example's verb (cook share 2.50%->3.45%->5.52% monotone across variants). Commit reverted; numbers and reasoning in eval/decision.md.

Eval caveats (recorded in decision.md): window extended 6->8 game-hours for margin (identical for all variants, >=200 decision floor cleared: 789/896/982); old built from b96c028 (main had advanced via TASK-82 docs-only merge — no code delta vs merge-base); eval worlds needed local model cogito:3b + tool_mode json (default llm.json declares uninstalled gemma4:12b-mlx) — applied identically to all variants; 16x used because 32x suppresses all planner calls via the cognition horizon; three unrelated user daemons were running during all soaks (identical conditions, serial runs).

Wiki re-pinned on the branch: agent-mind.md verified_against 642a75d (Principle IV). Awaiting PR merge for Done.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 2 — The prompt reads as exemplary craft (Priority: P2, built first): 2/2 · User Story 3 — The exemplar question gets evidence (Priority: P3, variant build): 1/1 · User Story 1 — Villager decisions keep landing (Priority: P1) 🎯 the ship gate: 6/6 · Polish & Cross-Cutting Concerns: 3/3 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 1/1 · Foundational (Blocking Prerequisites): 2/2 · User Story 2 — The prompt reads as exemplary craft (Priority: P2, built first): 2/2 · User Story 3 — The exemplar question gets evidence (Priority: P3, variant build): 1/1 · User Story 1 — Villager decisions keep landing (Priority: P1) 🎯 the ship gate: 6/6 · Polish & Cross-Cutting Concerns: 3/3). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
