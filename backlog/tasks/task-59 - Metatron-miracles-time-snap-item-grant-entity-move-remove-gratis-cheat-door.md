---
id: TASK-59
title: >-
  Metatron miracles: time-snap, item grant, entity move/remove + gratis (cheat)
  door
status: In Progress
assignee: []
created_date: '2026-07-22 16:58'
updated_date: '2026-07-22 20:28'
labels: []
dependencies: []
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Surfaced by a live incident (2026-07-22): villager Ash was stuck and had to be moved by hand-appending an agent.moved event with the daemon stopped. Metatron is the only designed 'hands' in the system — no God panel — so the angel needs a small miracle vocabulary, plus a player-side force door that bypasses the charge economy but not validation or the log.

Design decisions (agreed 2026-07-22):
- New whitelisted event types with reducer arms, following the metatron.nudged pattern (dry-run at the InjectSocial door, validate-not-clamp, charge spend enforced in the reducer): metatron.time_snapped, metatron.item_granted, metatron.entity_moved, metatron.entity_removed.
- Time snap uses SHIFT semantics: the snap re-bases every absolute-tick field in state (intent WorkStart, IdleSince, fire/den/rot/conversation stamps, charge-regen anchors) by the delta. The world is frozen; only the clock label changes. Requires a single re-base helper plus a drift test (world behavior byte-identical modulo tick offset); doctrine note that every future absolute-tick field must join the helper.
- Invocation: angel + player force. LLM Metatron can spend charges on all miracles in normal play; the player console/IPC path can land the same events with gratis:true (skips the charge decrement only). The model-mediated turn path MUST strip/reject gratis from LLM output — only the console path may set it. Every gratis use still lands in the append-only log.
- Remove excludes agents in v1: entity_removed covers structures, piles, and terrain features (terrain routed through existing cleared/harvested/quarried overlays). Agents can be moved but not removed; despawn is its own future spec.
- Move validates destination with existing passable/buildSite helpers; item grant validates item kind + bulk cap (reject over cap, consistent with validate-not-clamp).

Non-trivial: requires full Spec Kit (specify -> clarify -> plan -> tasks) linked via spec-bridge:link before implementation. Doctrine-adjacent (whitelist/isolation boundary, charge economy) -> Opus tier per constitution Principle V rubric.

Clarified 2026-07-22 (5 Qs, see spec Clarifications): tiered pricing (snap 2, rest 1); chest removal spills contents to ground pile; villager move cancels intent (replan); miracles land memories for affected villagers; operator surface is a dedicated `promptworld miracle` subcommand.

Spec: specs/016-metatron-miracles
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Four new metatron.* event types are whitelisted with reducer arms; each dry-runs at the injection door and replays deterministically
- [ ] #2 Time snap implements shift semantics via a single re-base helper; a drift test proves world behavior is identical modulo tick offset
- [ ] #3 gratis:true bypasses only the charge decrement, is settable only from the player console/IPC path, and is stripped/rejected from model-originated batches
- [ ] #4 Angel turn surface exposes the miracles charge-gated; a player console command lands the same events with --force
- [ ] #5 entity_removed cannot target agents in v1
- [ ] #6 Spec Kit spec authored and linked to this task via spec-bridge:link before implementation starts
- [ ] #7 Spec phase: Setup
- [ ] #8 Spec phase: Foundational (blocking prerequisites for all stories)
- [ ] #9 Spec phase: User Story 1 — Rescue a stuck villager: entity move/remove (P1) 🎯 MVP
- [ ] #10 Spec phase: User Story 2 — Operator force door: gratis miracles (P2)
- [ ] #11 Spec phase: User Story 3 — Time snap with shift semantics (P2)
- [ ] #12 Spec phase: User Story 4 — Item grant (P3)
- [ ] #13 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Execute specs/016-metatron-miracles/tasks.md in .worktrees/task-59 on branch task-59-metatron-miracles via spec-implementer (Opus 4.8), phase-gated: (1) T001 worktree [orchestrator]; (2) T002–T011 foundation + US1 MVP; (3) T012–T019 US2 gratis + US3 time snap; (4) T020–T025 US4 + polish; orchestrator gates each batch with build/test/review, then spec-bridge:sync, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Tier decision (constitution V rubric): Opus 4.8 for implementation — doctrine-adjacent (injectSocialWhitelist isolation boundary + charge economy in reducer), cross-package (internal/sim, internal/metatron, internal/ipc, cmd/promptworld), and rebaseTicks touches replay determinism. Spec Kit complete: spec (5 clarifications) → plan (Constitution Check PASS ×2) → tasks (25, T001–T025). MVP = Phases 1–3 (US1 move/remove).

Implementation complete: T001–T024 done on branch task-59-metatron-miracles (13 commits, rebased onto origin/main post tool-registry merge; miracles deliberately registry-free — coverage doctrine is directional, roster is nudge-form-scoped). Full gate green (build/vet/test, 18 pkgs); quickstart A–E validated live, F covered by unit tests (SC-005 adversarial gratis strip). PR #38 opened: https://github.com/evanstern/promptworld/pull/38. Phase ACs sync to Done post-merge when root spec artifacts prove it; then /grounding-wiki:wiki-update (sim, metatron, ipc, clock, cmd).
<!-- SECTION:NOTES:END -->
