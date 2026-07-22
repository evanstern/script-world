# Recipe & Tuning Contract: Resources, Food, and Crafting v1

Human-readable mirror of `internal/sim/recipes.go` + the agents.go tuning block.
This table is the single tuning surface; implementation must match it or update it
(and the spec's Assumptions) in the same commit.

## Gathering

| Action | Site | Duration (ticks) | Yield | Depletion |
|---|---|---|---|---|
| forage | adjacent/on Forage tile | 120 | 2 FoodRaw | regrows 12 game-h (unchanged) |
| chop | adjacent Tree | 300 | 2 Wood | cleared (unchanged) |
| hunt (bare) | ready den | 900 | 8 FoodRaw | den cooldown 6 game-h (unchanged) |
| hunt (spear) | ready den | 600 | 12 FoodRaw | + spends 1 spear use |
| quarry | adjacent Rock | 400 | 2 Stone | permanent (no regrow) |
| collect_water | adjacent Water | 60 | 1 Water | none (inexhaustible) |

## Hand-crafts (anywhere, planner-only)

| Goal | Inputs | Outputs | Duration |
|---|---|---|---|
| craft_planks | 1 Wood | 4 Planks | 180 |
| craft_stone | 1 Stone | 1 RefinedStone | 180 |
| craft_spear | 1 Wood + 1 RefinedStone | 1 Spear (3 uses) | 240 |

## Builds (on-site; fire also reflex, rest planner-only)

| Goal | Inputs | Result | Duration |
|---|---|---|---|
| build_fire | 2 Wood | fire, FuelUntil = now + 8 game-h | 600 |
| build_shelter | 8 Planks (was 5 Wood) | shelter | 1200 |
| build_oven | 4 RefinedStone + 2 Planks | oven | 900 |

## Station actions

| Goal | Site | Inputs | Effect | Duration |
|---|---|---|---|---|
| cook @ fire | lit fire | up to 8 FoodRaw | → same count FoodCooked | 240 |
| cook @ oven | oven | 1 Wood + up to 8 FoodRaw | → same count Meals | 360 |
| bathe | oven | 1 Water + 1 Wood | Morale +150, Warmth +300 (capped, absolute in event) | 240 |
| refuel_fire | any fire | 1 Wood | FuelUntil += 4 game-h, cap now + 12 game-h; relights | instant on arrival |

## Food & needs constants

| Constant | Value | Note |
|---|---|---|
| FoodRaw restore | +40 / unit | eating stops at need ≥ 900 |
| FoodCooked restore | +80 / unit | fire-cooked |
| Meals restore | +100 / unit | oven-cooked; eat order Meals → Cooked → Raw |
| hungryAt | 350 | unchanged |
| restRegenShelter | 6 / game-min | sleeping on shelter (else 4) |
| fireBurnPerWood | 4 game-h | build with 2 wood = 8 game-h |
| fireFuelCap | now + 12 game-h | refuel truncates to cap |
| spearDurability | 3 hunts | per spear |
| Rock coverage | ~6% of dry land | worldmap tuning const |

## Degraded-mode guarantee (must hold at these numbers)

Reflex vocabulary after this feature: eat, forage, hunt (bare), chop, build_fire,
refuel_fire, goto_warmth, sleep, build_shelter REMOVED from reflex (planks are
planner-territory), wander. A planner-less village of 8 must survive 3+ game days —
covered by a required regression test. Sizing sanity: forage 2×40 = 80 need/action raw,
hunt 8×40 = 320 need/action — comfortably above pre-feature nutrition per action
(forage 1×350÷~3.5 effective), and fires cost the same wood as today.
