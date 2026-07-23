---
id: TASK-66
title: 'Design decision: cognition horizon vs learner iteration speed (classroom mode)'
status: To Do
assignee: []
created_date: '2026-07-23 03:27'
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
- [ ] #1 Options enumerated with concrete horizon arithmetic examples (costs, budgets, speeds) for each
- [ ] #2 Interaction with the curriculum ladder considered (does the answer differ per learning stage?)
- [ ] #3 Decision recorded in a durable artifact under docs/design/ and discussed with the client
- [ ] #4 Follow-up implementation task(s) created on the board for the chosen option
<!-- AC:END -->
