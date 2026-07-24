# Feature Specification: Walls, Axes, and Paths

**Feature Branch**: `032-walls-axes-paths`

**Created**: 2026-07-23

**Status**: Draft

## Clarifications

### Session 2026-07-23

- Q: What damages/destroys walls in v1? → A: A deliberate villager demolish action (chips health per work cycle; Metatron removal still works; no environmental decay).
- Q: Do villagers actively prefer paved routes? → A: No — v1 is speed-only. Routing stays shortest-distance as today; time-weighted path-preferring routing is out of scope.

**Input**: User description: "Build-system additions for villagers: walls, axes, and paths. (1) Wall — a buildable structure costing either planks OR refined stone; occupies its tile and blocks pathing (first structure to do so); can be destroyed; the stone variant has more health than the plank variant; walls are repairable. (2) Axe — a craftable tool costing 1 plank + 1 stone; gates harvesting efficiency for BOTH trees (chop) and stone (quarry): without an axe a harvest yields 1 log/stone, with an axe it yields 3; follows the existing spear tool precedent (inventory-carried, durability). (3) Path — a buildable tile improvement costing stone; a villager walking on path tiles moves 2x as fast as off-path movement."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Walls shape the world (Priority: P1)

A villager (directed by its own plan or by the player through Metatron guidance) builds a wall on an open tile, choosing plank or stone as the material. Once standing, the wall's tile is impassable: villagers path around it, and connected walls can enclose areas. Walls take damage, can be repaired back to full health by a villager spending material, and collapse (tile becomes walkable again) when their health reaches zero. Stone walls endure substantially more damage than plank walls.

**Why this priority**: This is the headline capability — the first structure that changes the shape of the walkable world, enabling enclosures, defenses, and deliberate village layout. It introduces two reusable mechanics (structure blocking, structure health/repair) the other stories don't.

**Independent Test**: Can be fully tested by having a villager build a wall between two points, observing that routes now go around it, damaging it, repairing it, and destroying it — all without axes or paths existing.

**Acceptance Scenarios**:

1. **Given** a villager carrying enough planks and an open buildable tile, **When** it completes a plank-wall build there, **Then** a wall structure stands on that tile and the planks are deducted.
2. **Given** a standing wall on the only direct route between a villager and its target, **When** the villager travels, **Then** its route detours around the wall tile and it never occupies the wall's tile.
3. **Given** a plank wall and a stone wall at full health, **When** each takes the same damage, **Then** the stone wall survives strictly more total damage before collapsing.
4. **Given** a damaged wall and a villager carrying repair material, **When** the villager completes a repair, **Then** the wall's health is restored and the material is deducted.
5. **Given** a wall whose health reaches zero, **When** the collapse is applied, **Then** the wall is removed and its tile is walkable again.
6. **Given** a standing wall and a villager directed to demolish it, **When** the villager works the demolish action to completion, **Then** the wall's health is chipped down over successive work cycles until it collapses — and a full-health stone wall takes longer to demolish than a full-health plank wall.

---

### User Story 2 - Axes make harvesting worthwhile (Priority: P2)

A villager crafts an axe from 1 plank + 1 stone. Carrying the axe, each tree harvest yields 3 wood and each stone harvest yields 3 stone; bare-handed, either harvest yields only 1. The axe wears out with use and eventually breaks, like the existing spear.

**Why this priority**: Rebalances the whole resource economy (bare-handed yields drop from today's 2 to 1) and gives villagers their second tool, but it builds directly on the proven spear pattern and doesn't change the world's shape.

**Independent Test**: Can be fully tested by comparing wood/stone gained per harvest with and without an axe in inventory, and by exhausting an axe's durability.

**Acceptance Scenarios**:

1. **Given** a villager with 1 plank and 1 stone, **When** it completes an axe craft, **Then** an axe (with full durability) is in its inventory and the inputs are deducted.
2. **Given** a villager with no axe, **When** it harvests a tree or a stone source, **Then** it gains exactly 1 wood or 1 stone.
3. **Given** a villager carrying an axe, **When** it harvests a tree or a stone source, **Then** it gains 3 wood or 3 stone and the axe loses one use of durability.
4. **Given** an axe on its last use, **When** the harvest completes, **Then** the axe breaks and is removed from inventory, and subsequent harvests yield 1 until a new axe is carried.

---

### User Story 3 - Paths speed travel (Priority: P3)

A villager builds path segments on open tiles, spending stone. Any villager walking on a path tile moves twice as fast as one walking on ordinary ground, so well-placed paths between the village's frequent destinations cut travel time roughly in half along those corridors.

**Why this priority**: Pure quality-of-life speedup; valuable but nothing else depends on it, and the village functions without it.

**Independent Test**: Can be fully tested by timing a villager's trip along a fully-paved straight corridor versus the same trip on bare ground and observing a 2x speed difference on the paved run.

**Acceptance Scenarios**:

1. **Given** a villager carrying stone and an open buildable tile, **When** it completes a path build there, **Then** the tile carries a path and the stone is deducted, and the tile remains walkable.
2. **Given** a straight fully-paved corridor of N tiles, **When** a villager walks its length, **Then** the trip takes half the time of the identical trip on unpaved ground.
3. **Given** a route that is partly paved, **When** a villager walks it, **Then** only the steps taken from path tiles get the speed bonus; off-path steps move at normal speed.

---

### Edge Cases

- A wall build targets a tile currently occupied by a villager (or the builder would enclose itself): the build must not strand an agent inside a wall tile — the placement is rejected or deferred while the tile is occupied.
- Walls fully enclose an area containing villagers or resources: allowed (pens/defenses are the point); Metatron's existing entity-removal miracle and wall destruction remain the escape hatches.
- A villager's current route becomes invalid because a wall was just built on it: the villager must re-route rather than walk through or freeze permanently.
- A villager carries multiple axes: only one loses durability per harvest.
- Harvest with an axe when inventory has less free bulk than 3: yield still truncates to free capacity (existing rule), durability is still spent.
- Repair attempted on a wall already at full health, or with insufficient material: no-op or rejection, no material wasted.
- Building a wall on a path tile, or a path where a structure already stands: one improvement per tile — stacking is rejected (existing build-site rule).
- Axe breaks mid-plan while further harvest steps remain: later harvests proceed bare-handed at yield 1.

## Requirements *(mandatory)*

### Functional Requirements

**Walls**

- **FR-001**: Villagers MUST be able to build a wall on any tile that today qualifies as a build site, in either of two material variants: one paid in planks, one paid in refined stone.
- **FR-002**: A standing wall MUST block movement: no villager may enter its tile, and route-finding MUST treat the tile as impassable while still finding detours when one exists.
- **FR-003**: Walls MUST have health; the stone variant's maximum health MUST exceed the plank variant's.
- **FR-004**: A wall whose health reaches zero MUST collapse: the structure is removed and the tile becomes passable again.
- **FR-005**: Villagers MUST be able to repair a damaged wall by spending the wall's build material, restoring health up to (never past) its maximum.
- **FR-006**: Villagers MUST be able to deliberately demolish a wall: a demolish action chips the wall's health per work cycle until it collapses, so stone walls take proportionally longer to bring down than plank walls. Walls do NOT decay on their own in v1; repair undoes partial demolition damage.
- **FR-007**: A wall placement MUST NOT trap an agent on the wall's own tile (placement on an occupied tile is rejected or deferred).

**Axes**

- **FR-008**: Villagers MUST be able to craft an axe from exactly 1 plank + 1 stone, following the existing tool pattern (carried in inventory, finite durability, breaks when spent).
- **FR-009**: A tree harvest MUST yield 1 wood bare-handed and 3 wood when the harvester carries a working axe (baseline changes from today's 2).
- **FR-010**: A stone harvest MUST yield 1 stone bare-handed and 3 stone when the harvester carries a working axe (baseline changes from today's 2).
- **FR-011**: Each axe-assisted harvest MUST consume exactly one durability use from exactly one carried axe; an axe at zero uses breaks and leaves inventory.
- **FR-012**: Existing yield-truncation to the harvester's free carrying capacity MUST continue to apply on top of the axe rules.

**Paths**

- **FR-013**: Villagers MUST be able to build a path on any tile that today qualifies as a build site, paid in (raw) stone; the tile remains walkable.
- **FR-014**: A villager stepping from a path tile MUST move at twice the normal movement rate; steps from non-path tiles move at the normal rate.
- **FR-015**: Path speed is v1's only routing effect: route-finding continues to choose routes exactly as today (shortest-distance), and paths accelerate villagers whose chosen route crosses them. Path-preferring (time-weighted) routing is explicitly out of scope for this feature.

**Integration**

- **FR-016**: All three capabilities MUST be reachable through the villagers' existing planning/decision machinery (plannable goals), with the same startup coverage guarantees as existing verbs.
- **FR-017**: Walls and paths MUST be removable by Metatron's existing entity-removal miracle, like other structures.
- **FR-018**: All new state (walls with health, paths, axes) MUST persist and replay exactly like existing structures and tools (event-sourced, deterministic).

### Key Entities

- **Wall**: a placed structure occupying one tile; attributes: material variant (plank | stone), current health, maximum health (variant-dependent). Blocks movement while standing.
- **Axe**: a carried tool with remaining-uses durability; multiplies harvest yield for both wood and stone; sibling of the existing spear.
- **Path**: a tile improvement (not a blocking structure); grants a 2x movement rate to villagers stepping from it; coexists with the tile remaining walkable.
- **Repair action**: a villager work action targeting a damaged wall; consumes material, restores health.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A villager carrying an axe gains exactly 3 wood per tree harvest and 3 stone per stone harvest; bare-handed it gains exactly 1 of each — verified across repeated harvests.
- **SC-002**: With a wall line built across the direct route, 100% of villager trips crossing that line detour around it; no villager ever occupies a wall tile.
- **SC-003**: A villager traversing a fully-paved corridor completes it in 50% (±1 movement step of rounding) of the unpaved time.
- **SC-004**: A stone wall absorbs at least twice the total damage of a plank wall before collapsing; a repaired wall returns to full health; a collapsed wall's tile is immediately routable again.
- **SC-005**: Villagers can plan and execute all three builds/crafts end-to-end (gather → craft/build) without operator intervention, and a simulation save/replay reproduces identical wall/path/axe state.

## Assumptions

- "Processed stone" in the request maps to the existing refined stone material (produced today by refining raw stone); walls in stone cost refined stone, while paths and the axe cost raw stone (the request says "stone" for those, and distinguishes "processed stone" only for walls).
- Bare-handed yields for wood and stone drop from today's 2 to 1 — an intentional economy rebalance making the axe load-bearing, per the request's "1 ... without an axe".
- Axe durability follows the spear precedent (finite uses, breaks on the last use). Exact use-count is a tuning constant chosen at planning time (working default: 10 uses, reflecting that harvesting is far more frequent than hunting).
- Default costs where the request gave none: plank wall 2 planks; stone wall 2 refined stone; path 1 raw stone per tile; repair consumes 1 unit of the wall's build material per repair action.
- Wall health defaults: stone maximum health at least 2x plank maximum health; exact values are tuning constants chosen at planning time.
- The axe changes only harvest yield, not harvest duration (the spear changes both for hunting; the request specifies only yield).
- Paths do not decay and cannot be "damaged"; they are removed only by Metatron's entity-removal miracle.
- One improvement per tile: walls and paths follow the existing no-stacking build-site rule.
- Movement remains 4-directional on a uniform grid; the 2x path rate applies per-step based on the tile being stepped from.
