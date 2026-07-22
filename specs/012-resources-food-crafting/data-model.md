# Data Model: Resources, Food, and Crafting v1

State shapes only — behavior lives in contracts/events.md; tuning in
contracts/recipes.md. All fields integer, canonical-JSON structs (determinism rules).

## worldmap.TileKind (extended enum)

```
Grass | Water | Tree | Forage | Rock   // Rock is new, uint8 value 4
```

- `Passable`: Rock is impassable while standing (like Tree).
- `Buildable`: unchanged (plain Grass only).
- Placement: highest-elevation ~6% of dry grass after trees, before forage (R1).
- Static map only — depletion is overlay state, never a tile mutation.

## sim.Inventory (extended)

```go
type Inventory struct {
    Wood         int   `json:"wood"`
    Stone        int   `json:"stone,omitempty"`
    Water        int   `json:"water,omitempty"`
    Planks       int   `json:"planks,omitempty"`
    RefinedStone int   `json:"refined_stone,omitempty"`
    FoodRaw      int   `json:"food_raw,omitempty"`
    FoodCooked   int   `json:"food_cooked,omitempty"`
    Meals        int   `json:"meals,omitempty"`
    Spears       []int `json:"spears,omitempty"` // remaining uses per spear, sorted ascending
}
```

- The legacy `Food int` field is removed (format bump shields old snapshots).
- `Spears` invariant: sorted ascending; hunts spend `Spears[0]` (most-worn first);
  a spear reaching 0 uses is removed in the same reducer application.
- Unbounded in this feature — the bulk cap is spec 013's.

## sim.Structure (extended)

```go
type Structure struct {
    Kind      string `json:"kind"` // "fire" | "shelter" | "oven"
    X, Y      int
    FuelUntil int64  `json:"fuel_until,omitempty"` // fires only: lit iff tick < FuelUntil
}
```

- Fire: `FuelUntil` set at build (build tick + 8 game-hours), extended by refuel
  (absolute value in the event, capped at now + 12 game-hours). Lit-ness is derived,
  never stored as a flag.
- Oven: no `FuelUntil` — fuel consumed per batch from the worker's carried wood.
- Shelter: unchanged shape; rest-bonus behavior keys on standing/sleeping position.

## sim.State overlays (extended)

```go
Quarried []Point `json:"quarried,omitempty"` // depleted outcrops (permanent in v1)
```

- `effectiveKind` merges: a Quarried rock tile renders/behaves as passable depleted
  ground (not Grass — distinct for TUI; buildable = no).
- Parallel to existing `Cleared` (trees) and `Harvested` (forage); no regrow entry.

## sim.State burnout bookkeeping

`sim.fire_burned_out` must be emitted exactly once per burnout. The sweep detects the
transition `tick-1 < FuelUntil ≤ tick` — pure function of (state, tick), no extra state
needed. Relight resets `FuelUntil` forward, re-arming the same detection.

## Needs interaction (constants, no shape change)

- Eating: most-nutritious-first (Meals → FoodCooked → FoodRaw), stop at need ≥ 900.
- Shelter sleep: rest regen `restRegenSleep` (4) → `restRegenShelter` (6) when asleep
  on a shelter tile.
- Bath: absolute post-bath `Morale = min(1000, m+150)`, `Warmth = min(1000, w+300)`
  carried in the event payload (gru-pattern).

## Goal vocabulary (extended)

```
existing: forage, chop, hunt, build_fire, build_shelter, eat, sleep, wander,
          goto_warmth, talk_to
new:      quarry, collect_water, craft_planks, craft_stone, craft_spear,
          build_oven, cook, bathe, refuel_fire
```

- All new goals planner/plan-step only, EXCEPT `refuel_fire`, which the reflex may
  also choose (the one reflex addition).
- `resolveGoal` targets: quarry → nearestAdjacentTo(Rock, not quarried); collect_water
  → nearestAdjacentTo(Water); craft_* → current tile; build_oven → nearest buildable
  (build_shelter pattern); cook/bathe → nearest lit fire or oven / nearest oven;
  refuel_fire → nearest fire.

## world.Manifest

`FormatVersion`: 1 → 2. No shape change; existing rejection path
(`world format_version %d unsupported`) is the compatibility behavior.
