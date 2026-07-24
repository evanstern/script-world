# Data Model: Walls, Axes, and Paths (spec 032)

All changes are additive `omitempty` fields on existing event-sourced state — pre-032
snapshots unmarshal unchanged; no format-version bump (research R7).

## Structure (extended) — `internal/sim/agents.go`

```
Kind vocabulary: "fire" | "shelter" | "oven" | "chest" | "wall_plank" | "wall_stone" | "path"

Structure {
    Kind      string     // + wall_plank, wall_stone, path
    X, Y      int
    FuelUntil int64      // fires only (unchanged)
    Owner     int        // chests only (unchanged)
    Store     *Inventory // chests only (unchanged)
    HP        int        // NEW — walls only: current health, 1..max; json "hp,omitempty"
}
```

- **Max HP is derived, never stored**: `wallMaxHP(kind)` → `wallPlankHP` (200) /
  `wallStoneHP` (600). Doctrine: derived values cannot drift (fire lit-ness precedent).
- **Invariants**: a standing wall always has `HP ≥ 1` (the reducer removes the structure in
  the same application that would take it to ≤ 0, so `hp` never serializes as 0);
  `isWall(kind)` ⇔ kind ∈ {wall_plank, wall_stone}; a wall tile is impassable
  (`passable` consults structures — first structure family to do so); paths carry no HP and
  never block.
- **State transitions (wall)**:
  - built (`agent.built`, kind wall_*) → HP = wallMaxHP(kind)
  - chipped (`agent.wall_chipped`) → HP -= demolishChipHP (reducer guarantees result ≥ 1;
    the executor emits `agent.wall_destroyed` instead when the chip would reach ≤ 0)
  - repaired (`agent.wall_repaired`) → HP = min(max, HP + repairHPPerUnit), consumes 1
    matching material from the repairer
  - destroyed (`agent.wall_destroyed` | `metatron.entity_removed`) → structure removed,
    tile passable again

## Inventory (extended) — `internal/sim/agents.go`

```
Inventory {
    ...existing fields...
    Spears []int   // unchanged
    Axes   []int   // NEW — remaining uses per carried axe, sorted ascending; json "axes,omitempty"
}
```

- Fresh axe = `axeDurability` (10) uses; harvests spend `Axes[0]` (most-worn first);
  `Axes[0]` reaching 0 is removed by the companion `agent.axe_broke` in the same batch —
  exact `Spears` clone.
- `bulk()` counts `len(Axes)` (one bulk per axe, like spears); `freeBulk` unchanged.
- `canonicalKinds` gains `"axes"` (after `"spears"` — appended, preserving existing
  transfer iteration order for all pre-032 kinds).

## Pile (extended) — `internal/sim/agents.go`

```
Pile {
    ...existing fields...
    Spears []int
    Axes   []int   // NEW — remaining uses, sorted ascending; json "axes,omitempty"
}
```

- `Pile.empty()` gains the `len(Axes) == 0` conjunct. Drop/pick_up/deposit/withdraw move
  axes exactly as spears (kind key `"axes"`); chests store them via the shared Inventory.

## Intent (unchanged shape)

Wall builds, demolish, and repair use the existing adjacent-stand fields: `TargetX/Y` = the
passable tile the agent stands on, `ResX/Y` = the wall / build tile (chop/quarry precedent).
Path and other builds keep the stand-on-target convention. Multi-cycle work (demolish,
repair) is expressed by the reducer resetting `Intent.WorkStart` to 0 on a chip/repair
application, re-arming the executor's existing work gate under the same intent.

## Tuning constants — `internal/sim/agents.go` (spec 032 block)

See research.md R8 for the full table (wall costs/HP, chip/repair magnitudes, path cost,
axe durability, rebalanced bare/axe yields). `chopWood` and `quarryYield` are **deleted**,
replaced by the bare/axe pairs — the compiler surfaces every stale use.

## New/changed pure helpers

| Helper | Home | Semantics |
|---|---|---|
| `isWall(kind)` | terrain.go | kind ∈ {wall_plank, wall_stone} |
| `wallMaxHP(kind)` | terrain.go | derived max HP by kind |
| `wallAt(s, x, y) *Structure` | terrain.go | standing wall on tile (chestAt sibling) |
| `pathAt(s, x, y) bool` | terrain.go | path structure on tile |
| `agentAt(s, x, y) bool` | terrain.go | any living agent occupies tile (wall-build guard) |
| `passable(m, s, x, y)` | terrain.go | **extended**: false on wall tiles |
| `wallRepairMaterial(kind)` | recipes.go or terrain.go | "planks" for wall_plank, "refined_stone" for wall_stone |
