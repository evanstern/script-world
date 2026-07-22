---
id: TASK-26
title: 'Inventory and storage v1: design session'
status: Done
assignee: []
created_date: '2026-07-20 13:23'
updated_date: '2026-07-22 01:42'
labels:
  - design
dependencies: []
ordinal: 45000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Companion design session to TASK-25 (user, 2026-07-20): crafted items and resources live in agent inventory, which today is just small per-agent carry caps for wood and meals (docs/wiki/executor.md, reflex-policy.md). Socratic/spec session covering: (1) Carry capacity — how much a person can carry, and whether items/resources share one capacity model (slots vs weight vs per-kind caps). (2) Overflow — what agents do with what they can't carry. Decided: BOTH chests (crafted container items) and stockpile zones exist. Decided: NO direct player zoning a-la Dwarf Fortress — agents place/organize storage themselves; the player is not in charge of zoning. Open: how agents decide where stockpiles/chests go, ownership (personal vs communal storage), how storage interacts with the social fabric (theft, sharing, debts), and what the chronicle/TUI show. Output: a spec under specs/ linked via spec-bridge; its decisions feed TASK-25's crafting spec (chests are themselves craftable items).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A grounding/design session produces a spec directory for inventory/storage v1, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Design session round 1 decided: (1) CAPACITY = single integer bulk cap — every resource unit and item costs 1 bulk (one tunable number, ~24). (2) STOCKPILES = emergent from a new drop/deposit-on-ground action; adjacent ground piles cluster into zones; where agents drop IS the zoning (no player zoning, no governance machinery). (3) OWNERSHIP = chests remember their builder as owner (personal); ground piles are commons. (4) THEFT = taking from another's chest always works but is recorded: witnessed-style event, owner+witness memories/gossip seeds, trust drop via existing relation machinery; no permission gates — anti-theft norms can emerge later via governance.

Design session round 2 decided: (5) REFLEX ignores storage entirely — deposits/withdrawals are planner-only; bulk cap sized so the raw survival loop never jams (gather no-ops when full, eating frees space). (6) CHEST = build-on-site structure (consistent with 012's no craft-then-place), recipe ~6 planks, FINITE capacity ~48 bulk, builder = owner. (7) DEATH = carried bulk spills as a ground pile at the death site (lootable; reuses the pile mechanic). (8) DECAY = food in ground piles spoils after ~2 game days (event-sourced timer, like forage regrowth); chests preserve food; non-food never decays — chests get a mechanical job (the larder) beyond ownership.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Design session complete (2 Socratic rounds + 2 pre-decisions, all in notes): produced specs/013-inventory-storage (spec.md + passing requirements checklist) — single bulk cap (24), emergent drop-formed piles/stockpiles, builder-owned finite chests (6 planks / 48 bulk), theft recorded-not-prevented through existing social machinery, death drops, ground-only food rot. Reflex never touches storage. Linked on the board via spec-bridge as TASK-51 (depends on TASK-50). Decisions feed TASK-50's plan phase as designed.
<!-- SECTION:FINAL_SUMMARY:END -->
