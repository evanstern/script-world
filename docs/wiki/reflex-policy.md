---
name: reflex-policy
description: Deterministic survival decisions for idle agents (eat → food → night warmth/fire-refuel → rest → prep → wander) plus BFS pathfinding with fixed neighbor order; resolveGoal resolves the full spec-012 planner goal vocabulary (quarry, water, crafting, stations, refuel, cook, bathe) plus spec-013's storage goals (build_chest, drop, pick_up, deposit, withdraw) to coordinates
kind: component
sources:
  - internal/sim/policy.go
  - internal/sim/path.go
verified_against: 8be4440aae8d108884080cb6476782d2f11ad165
---

# Reflex policy & pathfinding

`decideIntent` is the deterministic pure function that gives idle, awake agents
something to do — since TASK-7, only agents idle past the 120-tick grace (the
planner's injection window). It is the permanent degraded mode: when no planner
thoughts arrive, this keeps bodies alive. `resolveGoal` (same file) is the shared
target resolver: planner goals from [[agent-mind]] resolve to coordinates through
the exact same nearest-X helpers the reflex uses. Spec 012 widened `resolveGoal`'s
goal set considerably (quarrying, water, crafting, an oven, refueling, cooking,
bathing) while trimming the reflex ladder itself down to one addition — refueling
a dying fire — and one removal — shelter-building dropped out of the reflex
entirely once it was re-costed in planks. Spec 013 (inventory & storage v1) widened
`resolveGoal` again — a chest to build, goods to drop/pick up/deposit/withdraw —
and left the reflex ladder itself completely untouched: all five new goals are
planner/plan-only (FR-014), added to `resolveGoal` and `planGoals`
([[executor]]'s guarded-plan validation) but never reachable from `decideIntent`.

## How it works

Priority ladder (first match wins):

1. **Eat** — hungry (`Food < hungryAt`, 350) and carrying any edible unit
   (`hasAnyFood`: `Inv.Meals + Inv.FoodCooked + Inv.FoodRaw > 0`) → instant
   `agent.ate`. The triplet check replaces the old raw-food-only check (T018) so an
   agent carrying only meals or only cooked food still eats reflexively.
2. **Get food** — hungry, nothing carried → `foodIntent`: nearest reachable
   effective-forage tile, else nearest ready den (`hunt`).
3. **Night, cold** (`!warmAt`) — reach existing warmth (`goto_warmth`) if
   reachable; else `reflexRefuelIntent`, the reflex's one new rule (T020,
   FR-012): if carrying any wood and a nearby fire is cold or has less than
   `refuelDyingBelow` (3600 ticks, one game-hour) of fuel left, relight or top it
   up (`refuel_fire`) — cheaper than a fresh build; else build a fire if carrying
   `fireWoodCost` (2) wood; else chop the nearest tree (yes, chopping in the cold
   dark — that's the day-1 drama working as designed).
4. **Night, warm** — sleep where you stand.
5. **Exhausted by day** (`Rest < tiredAt`, 250) — nap, preferring a warm tile.
6. **Prep** — no fire in the village (`!hasStructure("fire")`) → build/chop
   toward one; then `reflexRefuelIntent` again, unconditionally, to keep an
   existing fire from dying down; then stock the larder to `stockFoodRawTo` (8)
   units of raw food (`Inv.FoodRaw`). Shelter-building is gone from this ladder
   (T020): since spec 012 re-costed it in `Planks` (`shelterPlankCost`, 8) instead
   of raw wood, it joined the crafting economy and became planner-only — the
   reflex never enters `resolveGoal`'s `build_shelter`, `craft_*`, or `build_oven`
   cases.
7. **Wander** — a seeded short stroll (`rngAt(seed, "wander", tick, idx)`).

Waking (`wakeReason` in executor.go) mirrors this: day + decent rest
(`Rest >= 600`), or a hunger emergency the agent can act on — `Food < 150` and
`hasAnyFood`, the same triplet check as the eat rule above. Fully-rested agents
sleep through the night — the live-run sleep/wake churn bug is documented in the
TASK-5 notes. Actually eating the food (most-nutritious form first — `Meals` then
`FoodCooked` then `FoodRaw`, stopping at `satietyAt`) is the executor's
`eatOutcome`, detailed in [[executor]].

## resolveGoal's goal vocabulary

`resolveGoal` grew from the original handful (`eat`, `forage`, `hunt`, `chop`,
`build_fire`, `build_shelter`, `sleep`, `goto_warmth`, `wander`, `talk_to`/`seek`)
to cover spec 012's full economy, still resolving every goal to a concrete
`Intent` or an error through the same `nearest`/`nearestAdjacentTo` helpers the
reflex uses:

- **`eat`** now refuses on two grounds — nothing to eat (`!hasAnyFood`) or already
  sated (`Needs.Food >= satietyAt`, 900) — so a planner-chosen eat is never wasted
  at the ceiling.
- **`quarry`** and **`collect_water`** are planner-only (never in the reflex
  ladder): both resolve via `nearestAdjacentTo`, the same beside-the-resource
  pattern `chop` uses, matching a `worldmap.Rock` or `worldmap.Water` tile
  instead of a tree.
- **`build_fire`** is unchanged: gated on `fireWoodCost` wood, resolved to the
  nearest `buildSite`.
- **`build_shelter`** is re-costed to `Planks` (`shelterPlankCost`, 8, was wood)
  and is planner-only now that the reflex dropped it.
- **`build_oven`** is new: gated on `recipeFor("build_oven")`'s inputs (refined
  stone plus planks, checked via `hasItems`) and resolved to a `buildSite` the
  same way as fire and shelter.
- **`craft_planks`**, **`craft_stone`**, and **`craft_spear`** are new hand-crafts
  that need no travel — each resolves to the agent's own tile once
  `recipeFor(goal)`'s inputs are satisfied.
- **`refuel_fire`** is the one goal both the reflex (`reflexRefuelIntent`) and the
  planner can choose (FR-020): it targets the nearest fire structure regardless of
  lit state — a cold fire is relit on arrival, a dying one topped up. See
  [[executor]] for the fuel window (`fireBurnPerWood`, `fireFuelCap`) the
  completion applies.
- **`cook`** targets the nearest station where `litFireAt` or a `oven`
  structure stands — a lit fire and an oven are equally valid, and the fixed BFS
  neighbor order makes the tie-break deterministic; the station reached
  determines the output and duration (`food_cooked` vs. `meals`) at the executor.
- **`bathe`** is new and oven-only, gated on `recipeFor("bathe")`'s water/wood
  inputs — water's only v1 consumer.
- **`build_chest`** (spec 013 US3) is planner/plan-only, gated on
  `chestPlankCost` (6) planks and resolved to the nearest `buildSite` — the same
  pattern as `build_fire`/`build_oven` (the pile-tile exclusion, FR-007, already
  lives in `buildSite`).
- **`drop`**, **`pick_up`**, **`deposit`**, and **`withdraw`** (spec 013 US2/US3)
  are the storage goals, all planner/plan-only and instant-on-arrival (like
  `eat`): `drop` targets the agent's own tile; `pick_up` targets the nearest
  tile holding a pile; `deposit` targets the nearest chest (any owner — deposit
  has no ownership gate); `withdraw` targets the nearest chest whose `Store`
  holds `Kind` (or, with `Kind` "", the nearest chest holding anything). All
  four carry `Kind`/`Qty` (`Qty` 0 = all of kind, or as much as fits) onto the
  resolved `Intent`, threaded through to the completion at [[executor]] — see
  there for the truncation/re-validation rules and the theft consequences of a
  non-owner `withdraw`.
- `sleep`, `goto_warmth`, `wander`, and `talk_to`/`seek` are unchanged.

Pathfinding (`path.go`, unchanged by spec 012): breadth-first search with **fixed
neighbor order (N, E, S, W)** and FIFO frontier, so shortest paths and
nearest-match searches are identical on every run. `nextStep` re-derives one hop
per move from the shortest path (paths are never stored in state — movement
outcomes are evented, so replay needs no path data). `nearest` finds the closest
reachable tile matching a predicate in BFS order; `nearestAdjacentTo` finds a
standing tile beside a resource — chopping a tree, quarrying rock, and drawing
water all resolve through it. The escape clause lets an agent standing on
impassable terrain (pre-terrain saves) step out.

## Connections

[[executor]] invokes decisions on a staggered cadence and executes the resulting
intents, including the fire-fuel and cooking/crafting mechanics several of the new
goals above key on; passability comes from [[executor]]'s terrain overlays over
[[worldmap-generation]]; randomness only via [[deterministic-rng]] purpose tags
(`wander`, plus `genesis` placement in [[sim-state-reducer]]).

## Operational notes

BFS over a 64×64 map per decision/move is the current throughput ceiling — the
executor still clears >200k ticks/sec in the test harness, and auto-slow
([[sim-loop]]) degrades honestly under load. TASK-7 replaces this ladder with
planner-chosen goals; the ladder itself must remain reachable as the fallback.
