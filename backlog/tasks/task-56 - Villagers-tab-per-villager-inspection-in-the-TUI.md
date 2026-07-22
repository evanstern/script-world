---
id: TASK-56
title: 'Villagers tab: per-villager inspection in the TUI'
status: In Progress
assignee: []
created_date: '2026-07-22 05:38'
updated_date: '2026-07-22 06:10'
labels: []
dependencies: []
ordinal: 49000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Rename the souls dock tab to villagers and upgrade it to a per-villager inspector: keyboard-select a villager, see identity/vitals, itemized inventory, soul (memories/beliefs/narrative), and the current or most recent objective (reducer-maintained LastGoal, survives idle and fresh attaches).

Spec: specs/015-villagers-tab
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec phase: Setup
- [ ] #2 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #3 Spec phase: User Story 1 — Select a villager and inspect them (P1) 🎯 MVP
- [ ] #4 Spec phase: User Story 2 — Most recent objective when idle (P2)
- [ ] #5 Spec phase: User Story 3 — Memories, beliefs, narrative (P3)
- [ ] #6 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete (specs/015-villagers-tab: spec, plan, research R1-R7, data-model, contracts/state-and-keys, quickstart, tasks T001-T023). Key decision (R1): reducer-maintained Agent.LastGoal/LastGoalTick (omitempty, set on agent.intent_set, never cleared) — snapshot-persisted so fresh attaches see history; no format bump, no new events. Tier decision (Principle V rubric): Sonnet — slice is dominated by single-package TUI view/rendering code with tests alongside; the internal/sim touch is two additive fields + one reducer assignment with prescribed tests, not concurrency/scheduling/governor logic; the cross-package footprint is mechanical, not architectural. Escalate to Opus 4.8 only if gates fail per rubric. Implementation delegated to spec-implementer in worktree .worktrees/task-56, branch task-56-villagers-tab, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation complete in worktree .worktrees/task-56 (spec-implementer, Sonnet tier): T001–T022 done across 3 commits, full suite green (build/vet/test incl. e2e), independently re-verified after rebase onto main. PR #32 open: https://github.com/evanstern/script-world/pull/32. Remaining: merge, then T023 (wiki re-pin via /grounding-wiki:wiki-update for tui-client.md + sim-state-reducer.md sources, worktree cleanup, root ff-pull) and board sync to Done. Phase ACs intentionally unchecked until the PR merges — the bridge derives from main, and the ticked tasks.md rides the PR branch.
<!-- SECTION:NOTES:END -->
