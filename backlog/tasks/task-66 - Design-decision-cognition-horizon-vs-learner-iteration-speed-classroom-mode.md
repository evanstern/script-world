---
id: TASK-66
title: 'Design decision: cognition horizon vs learner iteration speed (classroom mode)'
status: Done
assignee: []
created_date: '2026-07-23 03:27'
updated_date: '2026-07-23 17:02'
labels:
  - review-2026-07-22
  - teaching-game
  - design-decision
dependencies: []
priority: medium
ordinal: 59000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (new-ideas item 4); client agreed 2026-07-22 this should be discussed. The tension: a learner's loop is "tweak the charter -> speed the world up -> watch the effect", but the cognition horizon deterministically SUPPRESSES model calls whose answers would land too stale at high speed (route.go) — so at 16x/32x the very planner/metatron activity the learner is iterating on degrades to reflex floors. The mechanism protecting determinism directly opposes fast pedagogical feedback. This is a design decision, not a bug.

Options to weigh (deliverable is a decision artifact, not code): (a) Paused authoring sandbox — pause is already zero-staleness by doctrine (in-flight minds land at the frozen tick); an "authoring mode" where the player edits, triggers a thought, and single-steps could give instant feedback with no horizon conflict. (b) Classroom/learner speed cap — worlds created in teaching mode cap at the speed the calibrated host affords for planner-class thoughts ("planner suppressed above 16x" is already computed by calibrate). (c) Per-class staleness-budget overrides in teaching worlds — accept more drift for faster iteration, recorded as a world-config posture. (d) Some combination staged by curriculum level. Each option must be argued WITH the horizon arithmetic (the router prints it), not against vibes. Output: a decision doc under docs/design/ (or a spec input), reviewed with the client, plus follow-up implementation task(s) created for the chosen option.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Options enumerated with concrete horizon arithmetic examples (costs, budgets, speeds) for each
- [x] #2 Interaction with the curriculum ladder considered (does the answer differ per learning stage?)
- [x] #3 Decision recorded in a durable artifact under docs/design/ and discussed with the client
- [x] #4 Follow-up implementation task(s) created on the board for the chosen option
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Design-session pattern (per TASK-32/35): 1) worktree .worktrees/task-66 (branch task-66-horizon-vs-iteration) from origin/main. 2) Ground in route.go horizon arithmetic, decision-4 doctrine, calibrate output, pause semantics, speed ladder. 3) Author options doc under docs/design/ weighing (a) paused authoring sandbox (b) classroom speed cap (c) per-class staleness-budget overrides (d) staged combination — each argued WITH the router's printed arithmetic; consider curriculum-ladder (TASK-68) interaction per stage. 4) Include a recommendation; commit + PR from the worktree. 5) Client review gates the decision (AC#3); follow-up tasks (AC#4) cut after the client picks. Ticks/board edits always from repo root.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Options doc authored on branch task-66-horizon-vs-iteration (PR #50): docs/design/horizon-vs-learner-iteration-speed.md. AC1 proven — all four options argued with route.go arithmetic (registry table + max-speed-per-class at 20/17/12.3 s/pt; calibrate horizonSummary cross-checked against calibrate_test.go's 17s/pt fixture). AC2 proven — curriculum interaction resolved per stage: stage 1 (conversational Metatron) has NO conflict since the metatron class never suppresses at watchable speeds (5pt×20×32=3,200 « 86,400 budget); the tension only bites stages 2-3 via the planner/conversation rows. Recommendation on the PR: staging (d) carried by (a) paused authoring sandbox + (b) calibration-derived classroom speed cap; reject (c) budget overrides (loosens router AND landing door — teaches on a degraded sim). Cross-cutting: horizon legibility (TASK-41) is prerequisite either way. AC3 awaits client review of PR #50; AC4 (follow-up tasks) cut after the client picks.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Design session complete, decision merged (PR #50, 0b7a2a5). Deliverables: docs/design/horizon-vs-learner-iteration-speed.md (all four options argued with route.go/registry.go arithmetic at 20/17/12.3 s/pt; calibrate horizonSummary cross-checked) + decision-6 (accepted). Client decisions 2026-07-23: mechanisms (a)+(b) staged per curriculum (d); (c) budget overrides REJECTED (loosens router AND stale-landing door — teaches on a degraded sim); (a) scoped during review to chain-completion only — code-grounding showed metatron_chat already has no pause gate (ipc/server.go:312) and nudges already land at the frozen tick, so the sandbox is exactly two fixes: nudge arms the nudged villager's one bounded round (mind.go absorb list) + pause-aware routing (routeVerdict computes at set speed while frozen; paused ⇒ drift 0). Single-stepping explicitly deferred. (b) is a SOFT calibrated cap, warn-with-override — the override surfaces the horizon arithmetic and is itself a lesson. Key finding: the metatron class (5pt/86,400t) never suppresses at watchable speeds — the tension bites only stages 2-3 via planner/conversation. Follow-ups cut: TASK-77 (chain-completion, doctrine-adjacent, Opus rubric tier, spec required), TASK-78 (teaching-world speed posture, consumed by TASK-68 presets, interacts TASK-40); legibility folded into TASK-41 (note appended). Curriculum interaction (AC#2): stage 1 conversational-Metatron needs no mechanism.
<!-- SECTION:FINAL_SUMMARY:END -->
