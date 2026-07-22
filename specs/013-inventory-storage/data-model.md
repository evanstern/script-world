# Data Model: Inventory & Storage v1

State shapes only — behavior lives in contracts/events.md; decisions in
research.md. All fields integer, canonical-JSON structs, fixed iteration orders
(determinism rules). Format version 2→3 (research R3).

## sim.Pile (new) & sim.State.Piles

```go
type FoodBatch struct {
    Kind    string `json:"kind"`     // "food_raw" | "food_cooked" | "meals"
    N       int    `json:"n"`
    SpoilAt int64  `json:"spoil_at"` // drop/death tick + rotWindowTicks
}

type Pile struct {
    X            int         `json:"x"`
    Y            int         `json:"y"`
    Wood         int         `json:"wood,omitempty"`
    Stone        int         `json:"stone,omitempty"`
    Water        int         `json:"water,omitempty"`
    Planks       int         `json:"planks,omitempty"`
    RefinedStone int         `json:"refined_stone,omitempty"`
    Spears       []int       `json:"spears,omitempty"` // remaining uses, sorted ascending
    Food         []FoodBatch `json:"food,omitempty"`   // drop order; same (Kind,SpoilAt) merges
}
```

- `State.Piles []Pile` (`json:"piles,omitempty"`): append order = creation order;
  **one pile per tile** (reducer merges drops onto an existing pile); a pile whose
  contents reach zero is removed in the same reducer application.
- Piles are event-sourced overlay state (like `Quarried`) — never a tile mutation.
- Stockpile "zones" have **no state entity**: adjacency grouping is computed at
  render time (TUI).

## sim.Structure (extended)

```go
type Structure struct {
    Kind      string     `json:"kind"` // "fire" | "shelter" | "oven" | "chest"
    X         int        `json:"x"`
    Y         int        `json:"y"`
    FuelUntil int64      `json:"fuel_until,omitempty"` // fires only
    Owner     int        `json:"owner,omitempty"`      // chests only: builder agent index, permanent
    Store     *Inventory `json:"store,omitempty"`      // chests only: contents (no rot inside)
}
```

- `Owner` zero-value round-trips to agent 0 unambiguously: every chest has an
  owner and non-chests never read the field. No transfer, no inheritance in v1;
  an owner's death changes nothing on the struct.
- `Store` bulk is capped at `chestCap` (48) via the same derived `bulk()` used
  for agents; capacity is per-kind-constant, never stored.
- Chest food needs no batches — chests preserve food indefinitely (FR-010).

## Bulk (derived, never stored)

```go
func bulk(inv Inventory) int // wood+stone+water+planks+refined_stone+food_raw+food_cooked+meals+len(spears)
```

- `bulkCap = 24` per villager (spec assumption; > largest single yield, spear
  hunt 12). Chest capacity uses the same function over `*Store`.
- Every acquisition edge clamps (full audit table: research R2).

## sim.Intent / IntentSetPayload / PlanStep (extended)

```go
// added to all three, additive & omitempty:
Kind string `json:"kind,omitempty"` // inventory item key; "" = all kinds (pick_up/withdraw)
Qty  int    `json:"qty,omitempty"`  // 0 = all of kind / as much as fits
```

- Spears transfer most-worn-first (`Spears[0]`), re-sorted on both sides.
- "All kinds" order is canonical inventory field order (wood, stone, water,
  planks, refined_stone, food_raw, food_cooked, meals, spears); food leaves
  piles oldest-batch-first.

## Goal vocabulary (extended)

```
existing: forage, chop, hunt, build_fire, build_shelter, build_oven, eat, sleep,
          wander, goto_warmth, talk_to, quarry, collect_water, craft_planks,
          craft_stone, craft_spear, cook, bathe, refuel_fire
new:      drop, pick_up, deposit, withdraw, build_chest
```

- All five are planner/plan-only (`planGoals` + inject_intent validation);
  **none** enter the reflex ladder (FR-014). Reflex code is untouched.
- `resolveGoal` targets: drop → current tile; pick_up → nearest pile on/adjacent;
  deposit → nearest chest; withdraw → nearest chest containing Kind (nearest
  chest when Kind = ""); build_chest → nearest buildable tile (build_shelter
  pattern, pile tiles excluded).
- drop/pick_up/deposit/withdraw are instant-on-arrival (duration 0, eat
  pattern); build_chest is timed work (fire-comparable duration).

## Recipes (recipes.go, one new row)

```
build_chest: Inputs {planks 6} → Structure "chest", Site on_site, Duration ~buildFireTicks
```

- Build-site validation (all `build_*`) additionally rejects tiles holding a
  pile (FR-007: goods aren't buried).

## Tuning constants (agents.go tuning block)

| Const | Value | Meaning |
|---|---|---|
| `bulkCap` | 24 | per-villager carried bulk ceiling |
| `chestCap` | 48 | per-chest stored bulk ceiling |
| `chestPlankCost` | 6 | build_chest recipe input |
| `rotWindowTicks` | 172800 (2 game days) | ground-pile food batch lifetime |
| `theftTrustDelta` | −120 | owner→taker trust on a taking |
| `theftAffectionDelta` | −40 | owner→taker affection on a taking |
| `theftMemoryTone` | −60 | owner/witness memory tone (gossip seed) |

Witness range reuses `witnessRadius` (8). No new needs constants; movement speed
is unaffected by carried bulk (out of scope).

## world.Manifest

`FormatVersion`: 2 → 3. Existing rejection path text continues to name
`promptworld migrate`. Migration artifacts: `world.v2.db` archive beside
`world.db`; fresh log `world.created` → `world.migrated{from_format: 2, state}`.
Transform (pure, in `internal/sim/migrate.go`): everything carries verbatim —
**no land reset** (no map inputs changed) — except carried bulk over `bulkCap`
spills to a pile at the agent's tile (food batches stamped `migration tick +
rotWindowTicks`). A v1 world chains 1→2→3 in one `migrate` run.
