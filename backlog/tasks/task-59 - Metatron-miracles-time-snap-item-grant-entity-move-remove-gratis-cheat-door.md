---
id: TASK-59
title: >-
  Metatron miracles: time-snap, item grant, entity move/remove + gratis (cheat)
  door
status: To Do
assignee: []
created_date: '2026-07-22 16:58'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Four new metatron.* event types are whitelisted with reducer arms; each dry-runs at the injection door and replays deterministically
- [ ] #2 Time snap implements shift semantics via a single re-base helper; a drift test proves world behavior is identical modulo tick offset
- [ ] #3 gratis:true bypasses only the charge decrement, is settable only from the player console/IPC path, and is stripped/rejected from model-originated batches
- [ ] #4 Angel turn surface exposes the miracles charge-gated; a player console command lands the same events with --force
- [ ] #5 entity_removed cannot target agents in v1
- [ ] #6 Spec Kit spec authored and linked to this task via spec-bridge:link before implementation starts
<!-- AC:END -->
