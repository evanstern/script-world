# Event Contract: Resources, Food, and Crafting v1

Extends the catalog in `docs/wiki/event-types.md`. Conventions inherited: namespaced
types, canonical-JSON payload structs (field order below is canonical), outcome-only
payloads (absolute values, no dice rolls), unknown types no-op in old reducers,
`sim.*`/`agent.*` are pure world happenings. All new payload structs live in
`internal/sim` (agents.go unless noted).

| Type | Payload struct (canonical field order) | Emitted by | Reducer effect |
|---|---|---|---|
| `agent.quarried` | `HarvestPayload{agent, x, y}` (reused) | executor, quarry completion | `Inv.Stone += quarryYield`; `(x,y)` appended to `Quarried`; intent cleared |
| `agent.collected_water` | `HarvestPayload{agent, x, y}` (reused) | executor, collect_water completion | `Inv.Water += 1`; intent cleared |
| `agent.crafted` | `CraftedPayload{agent, kind}` — kind ∈ `planks`, `refined_stone`, `spear` | executor, craft_* completion (inputs re-validated at completion; insufficient ⇒ no event, intent cleared via `agent.intent_done`) | recipe delta from the table: inputs −, outputs + (spear appends 3 to `Spears`, re-sorted); intent cleared |
| `agent.built` | `BuiltPayload{agent, kind, x, y}` (existing; kind gains `oven`) | executor, build completion | oven: `RefinedStone −4, Planks −2`, structure added. shelter: cost becomes `Planks −8` (was Wood −5). fire: unchanged cost, sets `FuelUntil` |
| `agent.ate` | `AtePayload{agent, meals, cooked, raw, food_after}` — REPLACES the old empty `AgentPayload` shape (format bump shields old logs) | reflex/planner eat (instant) | `Meals/FoodCooked/FoodRaw` decremented by counts; `Needs.Food = food_after` (absolute) |
| `agent.cooked` | `CookedPayload{agent, station, consumed, produced, kind}` — station ∈ `fire`, `oven`; kind ∈ `food_cooked`, `meals` | executor, cook completion (station re-validated lit/present; oven also consumes fuel) | `FoodRaw −= consumed`; kind field `+= produced`; oven: `Wood −= 1`; intent cleared |
| `agent.bathed` | `BathedPayload{agent, morale_after, warmth_after}` (absolute, gru-pattern) | executor, bathe completion at oven | `Water −1, Wood −1`; `Morale/Warmth` set absolutely; intent cleared; memory companion event |
| `agent.refueled` | `RefueledPayload{agent, x, y, fuel_until}` (absolute deadline) | executor, refuel_fire completion (planner or reflex) | `Wood −1`; fire's `FuelUntil = fuel_until`; relights if cold; intent cleared |
| `agent.spear_broke` | `SpearBrokePayload{agent}` | executor, alongside the hunt completion that spent the last use | `Spears[0]` removed (post-decrement zero); companion `agent.memory_added` in same batch |
| `sim.fire_burned_out` | `FireBurnedOutPayload{x, y}` | executor fuel sweep, once per burnout transition | none (lit-ness is derived from `FuelUntil`; event is chronicle/TUI signal) |
| `world.migrated` | `WorldMigratedPayload{from_format, source_events, source_tick, state}` — `state` is the full canonical v2 `sim.State` JSON (struct-embedded, ~1–2 MB, once per world lifetime) | `promptworld migrate` command (offline, appended to the fresh log right after `world.created`) | state replaced wholesale after validating name/seed match — the log alone reproduces the migrated world with zero snapshots (research R10) |

## Changed semantics under format v2 (no new types)

- `agent.foraged`: yield 1 → **2 FoodRaw** (was 1 legacy Food).
- `agent.hunted`: yield 3 → **8 FoodRaw** bare-handed, **12 FoodRaw** when a spear is
  carried; spear hunts also decrement `Spears[0]` (and may co-emit
  `agent.spear_broke`); hunt duration 900 → **600 ticks** with spear (duration is
  encoded in `WorkStart` + completion timing, not the payload).
- `agent.built{kind: shelter}`: consumes Planks, not Wood.
- Reducer-internal `eatFoodValue` (350) is deleted in favor of the three per-kind
  restore constants.

## Emission rules

- All completion events re-validate their resource/station at completion tick
  (contested-resource pattern): vanished outcrop, cold fire, missing oven, missing
  inputs ⇒ `agent.intent_done` only, no effect event.
- `sim.fire_burned_out` fires on the transition `tick−1 < FuelUntil ≤ tick` — exactly
  once per burnout, re-armed by refuel.
- New memorable moments (salience table, memory.go): spear broke (high), first bath
  (medium, tone positive), oven built (high, village-visible), fire burned out while
  agents nearby (low).
- None of the new types are model-injectable; all are world-emitted (executor). The
  planner only ever injects intents (`agent.intent_set` source `planner`/`plan`).

## Determinism notes

- Payload structs, never maps; field order above is the canonical serialization order.
- `fuel_until`, `food_after`, `morale_after`, `warmth_after` are absolute — replay
  applies recorded outcomes, never recomputes arithmetic that could drift.
- Yield/cost constants live in `internal/sim/recipes.go` + agents.go tuning block;
  changing any is format-versioned behavior (the v2 gate is the shield).
