---
id: TASK-50
title: 'Resources, food, and crafting v1'
status: Done
assignee: []
created_date: '2026-07-21 20:54'
updated_date: '2026-07-22 04:55'
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
- [x] #1 Spec phase: Setup
- [x] #2 Spec phase: Foundational (Blocking Prerequisites)
- [x] #3 Spec phase: User Story 1 — Stone and water enter the world (P1) 🎯 MVP
- [x] #4 Spec phase: User Story 2 — Fine-grained food and cooking at the fire (P2)
- [x] #5 Spec phase: User Story 3 — Crafting chain: planks, refined stone, spear (P3)
- [x] #6 Spec phase: User Story 4 — The oven: meals and baths (P4)
- [x] #7 Spec phase: User Story 5 — Shelter joins the plank economy (P5)
- [x] #8 Spec phase: Polish & Cross-Cutting
- [x] #9 Spec phase: User Story 6 — An old world's people survive the new world (P6)
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 0/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 0/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 0/4 · User Story 4 — The oven: meals and baths (P4): 0/6 · User Story 5 — Shelter joins the plank economy (P5): 0/2 · Polish & Cross-Cutting: 0/5

Planning complete (Fable 5, per constitution V): plan.md (Constitution Check PASS pre+post design), research.md R1-R9 (outcrop placement via elevation percentile, fixed-field Inventory + sorted Spears slice, absolute-outcome payloads, FormatVersion 1→2 refuse-don't-migrate, recipes.go single source, model-tier map), data-model.md, contracts/events.md + contracts/recipes.md, quickstart.md, tasks.md (42 tasks, 8 phases, US1 MVP). Tier recommendation recorded: Opus 4.8 for Phase 2 (cross-package substrate) + Phase 4 (degraded-mode doctrine slice); Sonnet for Phases 3/5/6/7/8. Ready for /speckit-implement via spec-implementer in .worktrees/task-50.

spec-bridge sync: Setup: 0/2 · Foundational (Blocking Prerequisites): 0/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 0/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 0/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 0/4 · User Story 4 — The oven: meals and baths (P4): 0/6 · User Story 5 — Shelter joins the plank economy (P5): 0/2 · User Story 6 — An old world's people survive the new world (P6): 0/5 · Polish & Cross-Cutting: 0/5

Migration design added (user decision 2026-07-22: full map reset OK, agents NOT reset — myworld-01 is the reference target at 107k events / tick 257,400 / ~day 3). Spec US6 + FR-023..027 + SC-007; research R7 revised + R10 (snapshot-cut migration: clean-shutdown snapshot required, v1 events never replayed under v2; people-state carried with tick continuity, map-bound state reset, agents re-placed genesis-style; wood 1:1, legacy food ×3 → Meals; world.db archived as world.v1.db; world.migrated event carries full v2 state so the log alone is sufficient truth — snapshots stay discardable). tasks.md: Phase 8 US6 (T038-T042, Opus tier — determinism-critical cross-package), Polish renumbered to Phase 9 (T043-T047). Board phase AC synced.

Implementation dispatch begun (constitution V). Phase 1 T001 done by orchestrator (worktree .worktrees/task-50 from origin/main d8f8f68). Phase 2 (T002-T008) dispatched to spec-implementer on OPUS 4.8 — rubric: cross-package substrate (sim state shapes + world format bump), determinism-critical payload/canonical-JSON work.

Phase 2 COMPLETE (Opus 4.8, commit 4927e5c): T002-T008 green — inventory v2 shape (legacy Food removed, byte-equivalent re-expression), Structure.FuelUntil, tuning block, recipes.go + mirror test, all payload structs, FormatVersion 1→2 with migrate-naming error, reducer no-op scaffolding. Full suite 180s green. Orchestrator accepted the one open call (recipe table Go shape follows existing constants idiom). Phase 3 (T009-T016, US1) dispatched on SONNET — rubric: single-package worldmap generation + established overlay/goal patterns, routine tier.

Phase 3 COMPLETE (Sonnet, commit 97cd7c0): T009-T016 green — Rock TileKind (elevation-percentile patches, ~6% of dry grass = ~3.8% of map, buildable ~55% >> 25% floor), Quarried overlay w/ Depleted effective kind, quarry/collect_water goals + events + reducer, contested-quarry + replay tests, TUI glyphs, vocabulary (incl. necessary parse.go validGoals addition — accepted in-slice call). US1 MVP checkpoint reached; AC #3 checked. Phase 4 (T017-T025, US2) dispatched on OPUS 4.8 — rubric: doctrine-adjacent degraded-mode contract (reflex ladder changes, eat rewrite, survival regression gate T023) + cross-cutting reducer semantics.

Phase 4 COMPLETE (Opus 4.8, commit 564b2a6): T017-T025 green — v2 yields, eat rewrite (most-nutritious-first, satiety 900, absolute AtePayload), fire fuel sweep + burnout-once + refuel (planner+reflex) + relight w/ cap, cook@fire, reflex loses shelter-building gains only refuel, TUI lit/cold + food triplet, vocabulary. DEGRADED-MODE GATE PASS: seeds 42/7/101, 8/8 alive 3 game days, zero crafted/cooked/bathed events (SC-002). Accepted deviations: warmAt(tick) signature (purity-correct); gru still wards on structural fire presence not lit-ness (KNOWN INCONSISTENCY — future design call); fire-burnout nearby-memory deferred to Phase 9. AC #4 checked. Phases 5-7 (T026-T037, US3+US4+US5) dispatched as one SONNET run — rubric: single-package sim work on now-established patterns (goal/event/reducer shape proven by phases 3-4).

Phases 5-7 COMPLETE (Sonnet, commit d5f7d51): T026-T037 green — hand-craft goals w/ recipe-table-driven reducer (agent.built now table-driven for fire/shelter/oven uniformly), spear hunts (600 ticks/12 yield, spend-most-worn, break event + salience-8 memory), oven (build, fueled meal batches, baths w/ absolute capped payload, witness-pattern village memory), shelter 8-planks + rest bonus 6/min. Doctrine gate re-verified green. Accepted calls: witness-radius broadcast for oven memory (only established bystander idiom), new saliences kept below GenerationBumpSalience, one combined commit (shared files). ACs #5-7 checked. Phase 8 (T038-T041, US6 migration code+fixtures) dispatched on OPUS 4.8 — rubric: determinism-critical cross-package (sim/store/world/cmd); T042 real-myworld-01 run deferred until Phase 9 T044 replay gate per tasks.md dependency note.

Phase 8 COMPLETE (Opus 4.8, commit e9ed88a): T038-T041 green — world.migrated wholesale-replace reducer (seed-guarded, reducer-total), typed legacy v1 decoder + MigrateState pure transform (genesisPlacement refactored+shared), scriptworld migrate command (OpenForMigration bypass, running-daemon/tail/second-run refusals, WAL-aware archive), fixture tests incl. zero-snapshot replay-from-genesis byte-identity (SC-007 determinism half) + all refusal cases. Accepted calls: seed-only reducer validation (State carries no name — only reducer-total option), Dead preserved (no resurrection-by-format-break), degradation flags reset, resolveWorldForMigrate bypass (worlds.Resolve is v2-gated). T042 still deferred behind T044. Phase 9 (T043-T045 + Phase-4 leftover fire-burnout memory) dispatched on SONNET — rubric: view/test/polish work.

Phase 9 COMPLETE (Sonnet, commit b5c6f72): T043-T045 green — fire-burnout witness memory (salience 3, deferred Phase-4 contract item closed), full TUI inventory surface + test, whole-feature replay test (every new event type asserted present, byte-identical from genesis, SC-004), quickstart §2 via real CLI/daemon e2e in isolated home (2 game days: 13 burnouts, 22-25 reflex refuels, 0 deaths, zero civilization events — doctrine live). Noted gap accepted: explicit unknown-type no-op assertion not added (pre-existing convention, format gate shields). §3 planner smoke deferred to post-merge. T042 (real myworld-01) now unblocked — running as orchestrator ops.

Discoverability verified (user request): v2 binary resolves myworld-01 via every verb — ps/ps --all (clean single row), status, start/stop (live-run proven), attach (dials correct socket), ui (same resolveWorld path; TTY-only failure in headless session). Ops hygiene: backup moved OUT of the scanned worlds home to ~/.scriptworld/backups/myworld-01.backup-pre-v2 (was polluting ps --all as 'unreadable'); stale TASK-49 registry entry removed. Pre-merge caveat: the current main (v1) binary shows myworld-01 as 'unreadable' and start fails — expected, closes when PR #31 merges.

T042 COMPLETE (orchestrator ops): myworld-01 migrated LIVE — precondition initially refused on a real-world shape (daemon.stopped appended after the shutdown snapshot; unsatisfiable exact-coverage), folded back as FR-024/R10 amendment (1576987) + implementation fix (e0a5eb4, Opus resume) with both tail-shape tests. Migration result: 8 souls carried at tick 269804/day 4 (Rowan 198 memories, 204 rumors, 51 relations, 64 conversations, chronicle intact; Fern's 107 legacy food → 321 meals; wood 1:1), structures/overlays reset, new log = world.created + world.migrated only, manifest v2, 114507 events archived in world.v1.db, full dir backup now at ~/.scriptworld/backups/myworld-01.backup-pre-v2. Live run under v2: ticks advance, needs decay, 0 deaths; stopped clean. PR #31 opened (T046) and MERGED. [Note re-appended on root after landing in the worktree's backlog copy by cwd accident.]

spec-bridge sync: Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 8/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 9/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 4/4 · User Story 4 — The oven: meals and baths (P4): 6/6 · User Story 5 — Shelter joins the plank economy (P5): 2/2 · User Story 6 — An old world's people survive the new world (P6): 5/5 · Polish & Cross-Cutting: 5/5 — status In Progress → Done

Quickstart §3 initial observation (live myworld-01, ~5 game-hours post-restart under merged main): planner minds are exercising the new economy unprompted — 2 completed agent.quarried (villagers mining the new outcrops), craft_planks chosen, 6 builds, quarry appearing in planner goal choices alongside seek/forage/chop. Full progression to a working oven left to ambient observation via TUI/chronicle.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 2/2 · Foundational (Blocking Prerequisites): 6/6 · User Story 1 — Stone and water enter the world (P1) 🎯 MVP: 8/8 · User Story 2 — Fine-grained food and cooking at the fire (P2): 9/9 · User Story 3 — Crafting chain: planks, refined stone, spear (P3): 4/4 · User Story 4 — The oven: meals and baths (P4): 6/6 · User Story 5 — Shelter joins the plank economy (P5): 2/2 · User Story 6 — An old world's people survive the new world (P6): 5/5 · Polish & Cross-Cutting: 5/5). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
