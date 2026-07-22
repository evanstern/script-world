---
id: TASK-55
title: >-
  planGoals drift: spec-012 verbs rejected as multi-step plan steps (violates
  spec 012 FR-020)
status: In Progress
assignee: []
created_date: '2026-07-22 05:32'
updated_date: '2026-07-22 17:57'
labels:
  - llm
dependencies: []
priority: medium
ordinal: 48000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found 2026-07-22 while grounding spec 014 (TASK-53). The three action-vocabulary maps have drifted in shipped code: goalVocabulary (internal/mind/prompt.go:15) and validGoals (internal/mind/parse.go:31) carry all 19 verbs, but planGoals (internal/sim/plan.go:33) still carries only the original 10. A model reply expressing quarry, collect_water, cook, refuel_fire, craft_planks, craft_stone, craft_spear, build_oven, or bathe as a guarded multi-step plan step is silently rejected at the sim door. Spec 012 FR-020 explicitly requires every new goal to be 'expressible as a guarded plan step', so this is a defect, not intent. Cure: TASK-53 / spec 014 derives all three surfaces from one registry; FR-012 of spec 014 records this as the sole permitted behavioral delta. If TASK-53 is delayed, the surgical fix is adding the 9 verbs to planGoals.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Multi-step plan steps naming any of the 9 spec-012 verbs are accepted at the sim door and execute (spec 012 FR-020 restored)
- [x] #2 Covered by a test that fails if the prompt vocabulary and the plan-step vocabulary ever diverge again
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Cured by TASK-53 / spec 014 on PR #36 (branch task-53-tool-registry): the plan-step accept set is now derived from the tool registry (tool.PlanStepGoals()), so all 9 spec-012 verbs are accepted at the sim door — TestPlanStepVocabulary (internal/sim/plan_test.go) pins the cure, and TestSingleWalkInvariant (internal/tool/registry_test.go) fails if prompt vocabulary and plan-step vocabulary ever diverge again (AC #2). Verified live: a multi-step plan naming collect_water landed in the T024 smoke world. Close as Done when PR #36 merges.
<!-- SECTION:NOTES:END -->
