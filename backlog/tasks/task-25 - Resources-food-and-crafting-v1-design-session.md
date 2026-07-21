---
id: TASK-25
title: 'Resources, food, and crafting v1: design session'
status: In Progress
assignee: []
created_date: '2026-07-20 13:18'
updated_date: '2026-07-21 20:50'
labels:
  - design
dependencies:
  - TASK-26
ordinal: 1000
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

Design session round 1 (foundations) decided: (1) Stone = new rock-outcrop TileKind, noise-placed on dry land, quarried adjacently like chopping (worldmap change, format-versioned). (2) Water = gatherable crafting/usage ingredient only; NO thirst need in v1. (3) Food = fine-grained abstract units (berry~1, rabbit~10) WITH raw/cooked distinction; cooking multiplies value. (4) Crafting = two intermediates: planks (wood side) + a refined stone form (stone side).

Design session round 2 (per-item) decided: FIRE = needs fuel (burns out after N game-hours, refuel-with-wood interaction), cooks raw food at modest multiplier. SHELTER = re-costed in planks, keeps warmth, adds faster rest regen when sleeping there, communal (no ownership in v1). OVEN = placed station (refined stone + planks), consumes wood fuel per batch from day one; cooking does NOT require water in v1, but oven can heat water (bath/flavor use) — water intended for future recipes. SPEAR = optional carried tool: hunting works bare-handed at modest yield, spear raises yield/speed, breaks after N hunts (durability re-craft loop).

Design session round 3 (cross-cutting) decided: EXECUTION = hand-craft portable things (planks, refined stone, spear) anywhere as timed work intents; structures (fire/shelter/oven) stay build-on-site intents with new recipe costs; no place-item mechanic. FOOD NUMBERS = berry 1 unit, rabbit ~8 units; eat raw +40/unit, fire-cooked +80, oven-cooked +100; agents eat units to satiety. BATH = heat water at oven (1 water + fuel share) gives bather +morale and +warmth. REFLEX (degraded mode) = survival raw-loop only: forage/hunt bare-handed, eat raw, chop, build+refuel fire; crafting/cooking/oven/spear/shelter are planner-initiated only — civilization requires minds.
<!-- SECTION:NOTES:END -->
