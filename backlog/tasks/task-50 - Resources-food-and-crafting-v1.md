---
id: TASK-50
title: 'Resources, food, and crafting v1'
status: In Progress
assignee: []
created_date: '2026-07-21 20:54'
updated_date: '2026-07-22 02:03'
labels: []
dependencies: []
ordinal: 44000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Implement the resource-economy layer specced in specs/012-resources-food-crafting (from TASK-25's design session): stone via rock-outcrop terrain + quarrying, water collection (ingredient only, no thirst), fine-grained raw/cooked food units, fire fuel + refuel, and the crafting chain (planks, refined stone, spear w/ durability, plank shelter w/ rest bonus, oven w/ meals + baths). Reflex keeps the survival raw-loop only; crafting/cooking are planner-initiated. Storage/carry caps deferred to TASK-26.

Spec: specs/012-resources-food-crafting
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec phase: Setup
- [ ] #2 Spec phase: Foundational (Blocking Prerequisites)
- [ ] #3 Spec phase: User Story 1 — Stone and water enter the world (P1) 🎯 MVP
- [ ] #4 Spec phase: User Story 2 — Fine-grained food and cooking at the fire (P2)
- [ ] #5 Spec phase: User Story 3 — Crafting chain: planks, refined stone, spear (P3)
- [ ] #6 Spec phase: User Story 4 — The oven: meals and baths (P4)
- [ ] #7 Spec phase: User Story 5 — Shelter joins the plank economy (P5)
- [ ] #8 Spec phase: Polish & Cross-Cutting
- [ ] #9 Spec phase: User Story 6 — An old world's people survive the new world (P6)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 0/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 0/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 0/4 · User Story 4 — The oven: meals and baths (P4): 0/6 · User Story 5 — Shelter joins the plank economy (P5): 0/2 · Polish & Cross-Cutting: 0/5

Planning complete (Fable 5, per constitution V): plan.md (Constitution Check PASS pre+post design), research.md R1-R9 (outcrop placement via elevation percentile, fixed-field Inventory + sorted Spears slice, absolute-outcome payloads, FormatVersion 1→2 refuse-don't-migrate, recipes.go single source, model-tier map), data-model.md, contracts/events.md + contracts/recipes.md, quickstart.md, tasks.md (42 tasks, 8 phases, US1 MVP). Tier recommendation recorded: Opus 4.8 for Phase 2 (cross-package substrate) + Phase 4 (degraded-mode doctrine slice); Sonnet for Phases 3/5/6/7/8. Ready for /speckit-implement via spec-implementer in .worktrees/task-50.

spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 0/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 0/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 0/4 · User Story 4 — The oven: meals and baths (P4): 0/6 · User Story 5 — Shelter joins the plank economy (P5): 0/2 · User Story 6 — An old world's people survive the new world (P6): 0/5 · Polish & Cross-Cutting: 0/5

Migration design added (user decision 2026-07-22: full map reset OK, agents NOT reset — myworld-01 is the reference target at 107k events / tick 257,400 / ~day 3). Spec US6 + FR-023..027 + SC-007; research R7 revised + R10 (snapshot-cut migration: clean-shutdown snapshot required, v1 events never replayed under v2; people-state carried with tick continuity, map-bound state reset, agents re-placed genesis-style; wood 1:1, legacy food ×3 → Meals; world.db archived as world.v1.db; world.migrated event carries full v2 state so the log alone is sufficient truth — snapshots stay discardable). tasks.md: Phase 8 US6 (T038-T042, Opus tier — determinism-critical cross-package), Polish renumbered to Phase 9 (T043-T047). Board phase AC synced.
<!-- SECTION:NOTES:END -->
