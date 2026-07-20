---
id: TASK-26
title: 'Inventory and storage v1: design session'
status: To Do
assignee: []
created_date: '2026-07-20 13:23'
labels:
  - design
dependencies: []
ordinal: 22000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Companion design session to TASK-25 (user, 2026-07-20): crafted items and resources live in agent inventory, which today is just small per-agent carry caps for wood and meals (docs/wiki/executor.md, reflex-policy.md). Socratic/spec session covering: (1) Carry capacity — how much a person can carry, and whether items/resources share one capacity model (slots vs weight vs per-kind caps). (2) Overflow — what agents do with what they can't carry. Decided: BOTH chests (crafted container items) and stockpile zones exist. Decided: NO direct player zoning a-la Dwarf Fortress — agents place/organize storage themselves; the player is not in charge of zoning. Open: how agents decide where stockpiles/chests go, ownership (personal vs communal storage), how storage interacts with the social fabric (theft, sharing, debts), and what the chronicle/TUI show. Output: a spec under specs/ linked via spec-bridge; its decisions feed TASK-25's crafting spec (chests are themselves craftable items).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for inventory/storage v1, linked on the board via spec-bridge
<!-- AC:END -->
