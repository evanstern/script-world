# Research: Walls, Axes, and Paths (spec 032)

All decisions below were made against the live code at the current main head; file:line
references are to that tree. No NEEDS CLARIFICATION markers remain (the two spec-level
questions were resolved in the spec's Clarifications session: demolish verb, speed-only paths).

## R1 — Walls are two Structure kinds, not one kind + variant field

**Decision**: `wall_plank` and `wall_stone` join the `Structure.Kind` vocabulary
(`internal/sim/agents.go:187-194`), with one new field `HP int` (json `hp,omitempty`).
Max HP is derived from the kind via constants (`wallPlankHP`, `wallStoneHP`), never stored —
same doctrine as fire lit-ness derived from `FuelUntil`. A helper `isWall(kind string) bool`
names the family.

**Rationale**: the reducer's `agent.built` arm already derives build cost via
`recipeFor("build_" + p.Kind)` (`internal/sim/state.go:626-628`). Two kinds ⇒ two recipes
(`build_wall_plank`: 2 planks, `build_wall_stone`: 2 refined_stone) and the existing arm
works unchanged except for stamping `HP`. A single "wall" kind with a material field would
break that derivation and need a payload extension.

**Alternatives considered**: `Kind: "wall"` + `Material` field — rejected (breaks the
`build_<kind>` recipe convention, touches `BuiltPayload`); a separate `Walls []Wall` state
list — rejected (walls would fall outside `buildSite`'s structure scan, the miracle
`entity_removed` path, and the TUI structure renderer for no benefit).

## R2 — Blocking: `passable` gains a wall scan; builds go adjacent-stand

**Decision**: `passable` (`internal/sim/terrain.go:38-44`) additionally returns false when a
standing wall occupies (x,y) — a linear scan of `s.Structures` for `isWall`, matching the
overlay scans already in `effectiveKind`. `buildSite` is unchanged (already rejects
structure-occupied tiles).

Wall builds use the **chop-style adjacent-stand pattern** (`nearestAdjacentTo`,
`internal/sim/path.go:83-101`): the builder stands on a passable tile (Intent.Target) and
builds on the adjacent tile (Intent.Res) — unlike fire/shelter/oven/chest, which build on the
tile the agent stands on (`internal/sim/executor.go:628-629`). Building where you stand would
entomb the builder the moment the wall lands. Completion re-validates
`buildSite(ResX,ResY) && no agent occupies (ResX,ResY)` (spec FR-007); a new `agentAt(s,x,y)`
helper backs the occupancy check.

**Rationale**: re-routing around fresh walls needs no new code — `nextStep` re-runs BFS over
current state every movement step (`executor.go:258-266`), so a route invalidated by a new
wall re-plans automatically, and an unreachable target already resolves via
`agent.intent_done` (`path.go:42-72` returning the current tile). Enclosure (walling in an
area, even with villagers inside) is permitted by the spec; BFS simply finds no route and
intents resolve.

**Alternatives considered**: teaching `effectiveKind` a Wall pseudo-kind — rejected
(structures are overlay entities, not terrain; `effectiveKind` is documented as
map+depletion only); edge-based walls (walls on tile borders) — rejected (whole new geometry
concept; tile-occupying walls match the grid model).

## R3 — Paths are `Structure{Kind:"path"}`; 2x speed via a second cadence phase slot

**Decision**: a path is a Structure of kind `"path"` (no HP, no extra fields). This gives
for free: the `build_path` recipe/reducer path, `buildSite`'s one-improvement-per-tile rule
(can't stack a wall on a path or vice versa), Metatron `entity_removed`, and TUI rendering.
`passable` ignores it (only wall kinds block).

Speed: movement today steps one tile when `(nextTick+int64(i)*3)%moveEveryTicks == 0`
(`executor.go:258-266`, `moveEveryTicks = 5`). The 2x path rate adds a **second phase slot**
gated on standing on a path:

```
phase := (nextTick + int64(i)*3) % moveEveryTicks
canStep := phase == 0 || (phase == 2 && pathAt(s, a.X, a.Y))
```

Two steps per 5-tick window is exactly 2x the average rate, stateless, integer-only, and
deterministic; the tile being stepped FROM (the agent's current tile) decides, matching the
spec ("steps taken from path tiles"). `pathAt` is a structure scan sibling of `chestAt`
(`terrain.go:107-115`).

**Rationale**: `moveEveryTicks/2` is not an integer (5/2), so a naive "half cadence" either
changes global baseline speed (redefining the constant to 6/3 alters every existing
behavior and test) or gives 2.5x/1.67x. A per-agent movement-points accumulator would give
smooth 2x but adds new persisted Agent state for zero observable benefit at these speeds.
SC-003's ±1-step rounding tolerance absorbs the uneven intra-window spacing (steps at
phases 0 and 2).

**Alternatives considered**: weighted pathfinding (Dijkstra) so villagers prefer paths —
explicitly out of scope per the spec clarification (v1 is speed-only; BFS untouched);
`Paths []Point` overlay state — rejected for the same reasons as R1's separate wall list.

## R4 — Axe clones the spear pattern end-to-end

**Decision**: `Inventory.Axes []int` (remaining uses per axe, sorted ascending), exactly
mirroring `Spears` (`agents.go:33-43`). New recipe `craft_axe`: 1 plank + 1 raw stone → 1
axe (`axeDurability` uses; default 10 — chopping/quarrying is far more frequent than
hunting, so 3 would evaporate). Yield rebalance in the reducer arms, derived from
pre-mutation state exactly like the spear hunt (`state.go:596-603`):

- `agent.chopped` (`state.go:561-574`): `chopWood = 2` is replaced by
  `chopYieldBare = 1` / `chopYieldAxe = 3`; carrying an axe spends one use from `Axes[0]`.
- `agent.quarried` (`state.go:665-678`): `quarryYield = 2` replaced by
  `quarryYieldBare = 1` / `quarryYieldAxe = 3`; same spend.
- Companion `agent.axe_broke` co-emitted by the executor when `Axes[0] == 1` pre-completion,
  riding the same batch immediately after the harvest event (clone of `agent.spear_broke`,
  `executor.go:696-703`, reducer `state.go:785-800`).

Free-bulk truncation (`minInt(yield, freeBulk(a.Inv))`) is untouched and applies on top.
The axe does NOT change work duration (spec assumption; `workDuration` untouched for chop/
quarry).

Storage/commons integration for symmetry with spears (nothing may be carryable but
un-droppable): `Pile.Axes []int` (`agents.go:213-223`), `canonicalKinds` + `foodKinds`-
adjacent transfer plumbing gains `"axes"` (`agents.go:253-256`), the storage-param enum
`itemKinds` in `internal/tool/registry.go` gains `"axes"`, and the `give_item` miracle
(`internal/sim/miracles.go:254-299`) can grant fresh axes.

**Behavior consequence (accepted)**: the reflex ladder chops for fire wood
(`policy.go:55-59, 85-87`); at bare yield 1 it takes two chops to afford a 2-wood fire.
This is the intended economy pressure making the axe load-bearing.

## R5 — Demolish and repair are multi-cycle work under a single intent

**Decision**: two new planner-only world verbs, both adjacent-stand (wall tiles are
impassable):

- **`demolish`**: resolver targets the nearest wall (`nearestAdjacentTo` over `isWall`
  structures). Each work cycle (`demolishTicks`) the executor emits `agent.wall_chipped`
  {Agent, X, Y}; the reducer subtracts `demolishChipHP` and **resets the intent's
  `WorkStart` to 0**, so the executor's existing work gate re-arms and the next cycle runs
  under the same intent — no new scheduling machinery, just the `work_started` →
  duration-gate loop (`executor.go:651-657`) re-entered. When remaining HP ≤ chip, the
  executor instead emits `agent.wall_destroyed` {Agent, X, Y}: reducer removes the
  structure (tile passable again) and clears the intent.
- **`repair`**: resolver targets the nearest **damaged** wall (HP < derived max) and
  requires 1 unit of the matching material carried (planks for `wall_plank`, refined stone
  for `wall_stone`). Each cycle (`repairTicks`) emits `agent.wall_repaired` {Agent, X, Y}:
  reducer consumes 1 unit, adds `repairHPPerUnit` clamped to max, and either resets
  `WorkStart` (still damaged AND material remains) or clears the intent (full or out of
  material). Repair at full health never resolves (resolver error), and a repair completing
  on an already-full wall resolves via `intent_done` (contested-wall pattern).

Both completions re-validate the wall still stands at (ResX, ResY) — someone else may have
destroyed it (contested-resource pattern, `executor.go:618-649`).

**Rationale**: single-shot demolition (duration ∝ HP) was rejected because it never
produces a *damaged* wall, making repair dead code; the chip loop is what makes
partial damage — and therefore repair — a real state. WorkStart-reset reuses the existing
work gate rather than inventing repeat-scheduling.

**Damage sources in v1**: demolish is the only in-sim damage source (spec clarification).
The gru does not attack walls in v1.

## R6 — Registry, planner, and coverage-gate integration

**Decision**: six new `worldToolsBase` entries (`internal/tool/registry.go:270-295`), all
`Effect: World, Gate: Resolvable, PlanStep: true`, none reflex-eligible:
`build_wall_plank` (600), `build_wall_stone` (600), `build_path` (240), `craft_axe` (240),
`demolish` (300/cycle), `repair` (240/cycle). Each gets a PromptGloss teaching villagers the
capability (walls block movement and can pen or protect; axes triple harvests; paths double
walking speed). Durations flow into `intentDurations` automatically (derived table,
`agents.go:647`), and each verb gets a `goalResolvers` arm (`policy.go:170-414`) — the boot
coverage gate (`internal/sim/toolcheck.go:38-60`) enforces exactly this pairing.
`set_plan`'s schema picks the new verbs up automatically (`legacyWorldNamesFrom(worldTools)`,
`registry.go:307`).

The reflex ladder (`decideIntent`) is deliberately untouched: all six verbs are
planner/plan-only, like every post-012 verb (quarry precedent, `policy.go:238-247`).

## R7 — Rendering and snapshots

**Decision**: TUI map view (`internal/tui/views.go:385-403`) gains glyphs: `wall_plank` "▤",
`wall_stone` "▩" (dim style when HP < max, like the cold-fire precedent), `path` "·"
(rendered under agents; structures map already wins over terrain). New Inventory/Pile/
Structure fields are all `omitempty` additive — pre-032 snapshots unmarshal unchanged (the
Generation/Plan/Hail precedent), so **no format-version bump** (unlike spec 012, which
removed fields). Determinism: all new yields/HP arithmetic is integer, reducer-only, derived
from pre-event state; replay hashes stay byte-identical by construction.

## R8 — Constants (single tuning surface, mirrored in contracts/recipes.md)

```
wallPlankCost   = 2    // planks → wall_plank
wallStoneCost   = 2    // refined_stone → wall_stone
wallPlankHP     = 200
wallStoneHP     = 600  // 3x plank (spec: ≥2x)
buildWallTicks  = 600
demolishChipHP  = 100  // plank wall: 2 cycles; stone: 6
demolishTicks   = 300  // per chip cycle
repairHPPerUnit = 100  // 1 material unit per cycle
repairTicks     = 240  // per repair cycle
pathStoneCost   = 1    // raw stone per tile
buildPathTicks  = 240
axeDurability   = 10   // uses per axe
chopYieldBare   = 1    // replaces chopWood = 2
chopYieldAxe    = 3
quarryYieldBare = 1    // replaces quarryYield = 2
quarryYieldAxe  = 3
```
