---
id: TASK-73
title: Villager system-prompt quality pass (behavior-affecting; eval-gated)
status: In Progress
assignee: []
created_date: '2026-07-23 06:34'
updated_date: '2026-07-23 18:41'
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
- [ ] #1 New prompt: single identity statement, no redundant name repetition, doctrine framing preserved
- [ ] #2 Exemplar included or explicitly rejected with the measured reason
- [ ] #3 Before/after eval recorded on the task: rejected_malformed + rejected_cardinality rates and token counts; no regression
- [ ] #4 Prompt-cache prefix boundaries unchanged or consciously re-drawn; scripted-stub tests updated and passing
- [ ] #5 Spec phase: Setup
- [ ] #6 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #7 Spec phase: User Story 2 — The prompt reads as exemplary craft (Priority: P2, built first)
- [ ] #8 Spec phase: User Story 3 — The exemplar question gets evidence (Priority: P3, variant build)
- [ ] #9 Spec phase: User Story 1 — Villager decisions keep landing (Priority: P1) 🎯 the ship gate
- [ ] #10 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit: specs/027-villager-prompt-quality (spec+plan+research+contracts+tasks on main; implementation on branch task-73-villager-prompt-quality, worktree .worktrees/task-73).
Build order: red-first contract tests (C1-C5) -> rewrite (variant `new`) -> exemplar commit (variant `new+exemplar`) -> eval gate: scripted-stub suite + serial live soaks (seed 4242, 6 game-hours, local tier) over refs origin/main / new / new+exemplar; tally rejected_malformed + rejected_cardinality rates, tool distribution, token counts from cog.tool_call events; ship gate = no rejection-rate regression (SC-001), exemplar decided by numbers (FR-004).
Implementation tier: Opus 4.8 via spec-implementer (rubric: doctrine-adjacent behavior change in internal/mind, eval-gated behavior-affecting slice — constitution Principle V). Planning/gating on Fable 5.
<!-- SECTION:PLAN:END -->
