---
id: TASK-47
title: >-
  Hail protocol: 'let's chat' hail pauses the target so talk_to survives
  movement at speed
status: In Progress
assignee: []
created_date: '2026-07-21 15:47'
updated_date: '2026-07-21 17:03'
labels:
  - bug
  - feature
  - sim
dependencies: []
priority: medium
ordinal: 41000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## Problem

Frequent warnings / LLM thought rejections when one agent decides to talk to another but the target has moved away by the time the intent lands. Seen live in myworld-01: `mind: Hazel goal "talk_to" rejected: rejected-guard: Rowan is gone (distance 35)`.

Root cause chain:
- `talk_to` goals carry a `target_present` guard checked at landing: target must be within `presentRadius = 16` Manhattan (internal/sim/guard.go:36-72).
- Planner LLM calls take wall-clock seconds; at 8x+ game speed those seconds span many game ticks, so the target routinely walks beyond the radius before the intent lands. **Most common at 8x+ speed on the local tier** (slow local model + fast clock = widest gap).
- Result: the thought is rejected outright (internal/mind/mind.go:419) and the LLM spend is wasted; agents rarely manage to open conversations at speed.

## Proposed feature

A "hail" — a cheap, deterministic (no-LLM) sim-level message an agent emits when it forms a talk_to intent: "let's chat". On receipt, the target pauses in place for a short window ("hears the other out") instead of walking off, giving the hailer time to close distance and the conversation time to start. Rough shape:

- Emitted when a talk_to intent is set (or when the planner reply is parsed), targeting the named agent.
- Target enters a brief `hailed` pause state (a few game-minutes, tunable; should scale with or be robust to game speed) unless engaged in something un-interruptible.
- Guard/adapt path treats a hailed-and-waiting target as present; existing adapt rung re-resolves position as it does today.
- Pause expires if the hailer never arrives; target resumes prior intent/plan.
- Events for observability (e.g. `social.hailed`, `social.hail_expired`) so tail/TUI show hails.

## Non-goals

- No LLM call in the hail path — this is a sim-level courtesy protocol.
- Not a fix for cognition-horizon calibration generally (TASK-40 covers warning/auto-suggest).

Spec: specs/010-hail-protocol
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 talk_to rejections with reason 'is gone' are substantially reduced at 8x+ speed in a local-tier world (before/after measurement on a test world recorded in task notes)
- [ ] #2 Hailed target pauses without abandoning its plan: pause expires safely and prior intent/plan resumes if the hailer never arrives
- [ ] #3 Hail path is deterministic sim logic — zero LLM calls; hail and expiry are emitted as events visible in scriptworld tail
- [ ] #4 Un-interruptible states (e.g. sleeping, mid-conversation) are exempt from being paused by a hail
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (Blocking Prerequisites)
- [x] #7 Spec phase: User Story 1 - A talk_to decision survives target movement at speed (Priority: P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 - A stood-up target resumes its life safely (Priority: P2)
- [x] #9 Spec phase: User Story 3 - Un-interruptible villagers are left alone (Priority: P2)
- [x] #10 Spec phase: User Story 4 - Hails are visible to the observer (Priority: P3)
- [ ] #11 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Baseline evidence (myworld-01, 2026-07-21, local tier at speed): in ~30 min of wall time only ONE conversation landed (conv 2190, 11:31, 4 turns). Afterwards zero conversations in 45+ min while talk_to attempts kept failing the target_present guard — Birch→Sage rejected 3× at distances 47, 50, 36 (11:42–11:53). 4× agent.intent_rejected vs 1× social.conversation in the event log. Use this world/config shape for the AC #1 before/after measurement.

spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 0/4 · User Story 1 - A talk_to decision survives target movement at speed (Priority: P1) 🎯 MVP: 0/6 · User Story 2 - A stood-up target resumes its life safely (Priority: P2): 0/2 · User Story 3 - Un-interruptible villagers are left alone (Priority: P2): 0/2 · User Story 4 - Hails are visible to the observer (Priority: P3): 0/1 · Polish & Cross-Cutting Concerns: 0/4

Tier decision (constitution V rubric): implementation delegated to spec-implementer on **Opus 4.8** — the slice changes the cognition-horizon landing ladder in internal/sim/loop.go (doctrine-adjacent behavior change: guard-failure outcomes for talk_to) and executor scheduling behavior (pause suppression in the per-agent step). Planning/spec/tasks authored on Fable 5.

Implementation complete on branch task-47-hail-protocol (2 commits, T002-T018 all checked in spec tasks.md on the branch). Full gate green incl. e2e. PR #28 open: https://github.com/evanstern/script-world/pull/28. Diff reviewed by orchestrator: rung order matches contracts/events.md, talkEvents extraction byte-order-preserving, no double-emit in sweep. Known acceptable race: meeting pin set while paused delays attendance <=8 game-min (research D7 priority holds). Remaining: T019 live before/after measurement, merge, T020 wiki re-ground.

spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 4/4 · User Story 1 - A talk_to decision survives target movement at speed (Priority: P1) 🎯 MVP: 6/6 · User Story 2 - A stood-up target resumes its life safely (Priority: P2): 2/2 · User Story 3 - Un-interruptible villagers are left alone (Priority: P2): 2/2 · User Story 4 - Hails are visible to the observer (Priority: P3): 1/1 · Polish & Cross-Cutting Concerns: 2/4

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 4/4 · User Story 1 - A talk_to decision survives target movement at speed (Priority: P1) 🎯 MVP: 6/6 · User Story 2 - A stood-up target resumes its life safely (Priority: P2): 2/2 · User Story 3 - Un-interruptible villagers are left alone (Priority: P2): 2/2 · User Story 4 - Hails are visible to the observer (Priority: P3): 1/1 · Polish & Cross-Cutting Concerns: 2/4
<!-- SECTION:NOTES:END -->
