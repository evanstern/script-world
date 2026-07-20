---
id: TASK-25
title: 'Resources, food, and crafting v1: design session'
status: To Do
assignee: []
created_date: '2026-07-20 13:18'
updated_date: '2026-07-20 13:24'
labels:
  - design
dependencies:
  - TASK-26
ordinal: 21000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The resource economy needs a ground-up design session like TASK-23's (user, 2026-07-20). Today the executor has only wood + food with forage/chop/hunt and wood-spending builds (see docs/wiki/executor.md, event-types.md); terrain already generates water, woods, forage patches, and animal dens (worldmap-generation.md). Socratic/spec session covering three interlocking mechanics: (1) Resources — wood, water, stone as gatherable base resources. (2) Food sources — foraging and animals, with food as an abstract unit rather than distinct items (e.g. 1 rabbit = 10 food, 1 berry = 1 food). (3) Items & crafting — Minecraft-ish recipe style ('1 wood = 4 planks', '6 planks = 1 stair', '2 stone + 1 wood = stove'); needs a list of basic resources, recipes, and item-usage rules. User wants discussion before committing. Scope cap: ~5 items for v1 — Oven, Basic Shelter, Fire, a spear/weapon for hunting, and Food as the placeholder for all forage/hunt yields. Output: a spec under specs/ linked to the board via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for resources/food/crafting v1, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) Intermediate items MUST exist — crafting chains like wood → planks → stairs are in scope, not just direct raw→item recipes. (2) Each of the ~5 v1 items gets its own detailed discussion in the session (mechanics/usage rules per item). (3) Crafted items live in agent inventory. Storage/carry-capacity questions split out into their own design session task (inventory & storage) — that spec is an input to this one.
<!-- SECTION:NOTES:END -->
