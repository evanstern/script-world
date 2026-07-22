---
id: TASK-30
title: 'The survival labor budget: effort, healing, and health retune — design session'
status: To Do
assignee: []
created_date: '2026-07-20 19:54'
updated_date: '2026-07-22 04:34'
labels:
  - design
dependencies:
  - TASK-28
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Roguelike survival design (user, 2026-07-20). This is the calibration task that makes decision-3 real: retune every effort/yield/decay constant so that surviving a day costs most of a day. Current vs target, in game-time (current numbers from docs/wiki/executor.md and internal/sim/agents.go tuning constants): FOOD — need drains 1440/day and one food unit restores 350, so an agent eats ~4 units/day; today a forage trip is 2 min per unit (~30 units/hour). Target: ~1 unit per hour of effort, so ~8h of foraging or hunting feeds the worker plus one other. Foraging becomes the slow reliable trickle (~60 min per unit); hunting becomes a long chunky gamble (e.g. a 6h expedition with meaningful failure odds and a multi-unit carcass on success, den cooldowns per season) so hunters structurally overfeed and must share — instant debt fodder for the social fabric. WOOD — chop goes from 5 min for 2 wood to ~30 min for 1 wood (12x scarcer), against the ~8 wood/night fire bill from TASK-29. BUILDING — a hut is ~2 person-days of total effort (e.g. ~12 wood to fell plus ~10h of construction; today a shelter is 20 min + 5 wood). Candidate: cooperative construction — multiple agents advance the same build site so a hut is a village project, not a hermit hobby. HEALTH and HEALING — health depletion stays (starvation/exposure/wounds) but healing becomes sleep-only: regen only while asleep AND fed AND warm (today agents heal +1/min merely by being awake, fed, and rested — remove that). Sleep becomes load-bearing and every wound costs labor-hours, so injuring someone or nursing them both matter. DELIVERABLE — besides the spec, a calibration worksheet proving the doctrine arithmetic: solo break-even ~8h/day, village-of-8 with shared hearth and hunts ~4-5h/agent-day, cold season pushes solo play into structural deficit. Depends on TASK-28 (ambient temperature drives the warmth/fuel demand side) and TASK-29 (fuel burn is the wood sink); must coordinate with TASK-25 (food/crafting yields must not undercut these targets) and TASK-26 (carry caps make stockpiles physical and stealable). Output: a spec under specs/ linked to the board via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for the survival labor budget retune, linked on the board via spec-bridge
- [ ] #2 The spec includes a calibration worksheet demonstrating solo ~8h/day break-even and cooperative surplus per decision-3
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) Feeding self + one other costs ~8h of hunting or foraging. (2) Chop a tree ~30 min = 1 wood. (3) One hut ~2 days of effort from one person; a fire ~1h. (4) Healing must exist and stay simple — sleep is the preferred mechanic. (5) Health can deplete; permadeath (already permanent in code) makes that final.

Re-grounding 2026-07-22: cited constants hold (eatFoodValue=350 agents.go:144, healthRegen=1 agents.go:142, chopTicks=300 / chopWood=2 agents.go:153/159). WARNING: spec 012 (TASK-50, in progress) re-denominates food (raw +40 / cooked +80 / meal +100 replacing the single +350; forage=2, hunt=8/12) and explicitly defers the labor-budget retune to this task — rewrite the food-side baselines against spec 012's pins once it merges. Absorbed from TASK-29 (archived): raising fire build cost (~1 game-hour of labor) folds into this retune. Coordinate carry caps with spec 013.
<!-- SECTION:NOTES:END -->
