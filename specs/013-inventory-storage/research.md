# Research: Inventory & Storage v1

Phase 0 decisions. Grounding: docs/wiki (event-types, social-fabric, executor,
sim-state-reducer, world-migration — all pinned ≥ 1d1cc6f, post-TASK-50),
specs/012 contracts/data-model, and direct code reads (agents.go, plan.go,
recipes.go, social.go, policy.go). All unknowns from Technical Context resolved
below; no NEEDS CLARIFICATION remain.

## R1 — Pile state shape: batch-tracked food, flat non-food

**Decision**: `State.Piles []Pile` with

```go
type Pile struct {
    X, Y                                 int
    Wood, Stone, Water, Planks, RefinedStone int   // omitempty
    Spears []int                              // remaining uses, sorted ascending
    Food   []FoodBatch                        // drop-ordered
}
type FoodBatch struct {
    Kind    string // "food_raw" | "food_cooked" | "meals"
    N       int
    SpoilAt int64  // drop tick + rotWindowTicks
}
```

One pile per tile (invariant enforced in the reducer: drop onto an occupied tile
merges). Piles append in creation order; an emptied pile is removed in the same
reducer application. Food is batch-tracked because rot is per-drop (US5); batches
with identical `(Kind, SpoilAt)` merge on drop. Non-food is flat counts — it never
decays, so batches would be dead weight.

**Rationale**: structs-never-maps determinism; slice order is the canonical
iteration order for rot sweeps and pickup; mirrors the `Quarried []Point` overlay
precedent (event-sourced state over the static map, never a tile mutation).

**Alternatives**: reusing `sim.Inventory` inside Pile (rejected: its food fields
would have to stay zero beside a parallel batch list — two homes for one fact);
per-tile map keyed by coordinates (rejected: maps are banned in serialized state).

## R2 — Bulk is derived, never stored; cap enforced at every acquisition edge

**Decision**: `func bulk(inv Inventory) int` = sum of all count fields +
`len(Spears)`; const `bulkCap = 24`. No stored bulk field. Enforcement audit
(every inventory-increasing site):

| Site | Rule |
|---|---|
| gather completions (forage/chop/hunt/quarry/collect_water) | executor re-validates free bulk at completion: zero free ⇒ `agent.intent_done` only (no harvest event, **no depletion** — US1-AS1); partial ⇒ harvest event emitted, reducer clamps yield to free bulk, overlay/depletion applies, remainder forfeit (US1-AS2) |
| craft completions (planks/stone/spear) | completion re-validation extends to net bulk delta: if outputs−inputs won't fit, no event, intent cleared (crafts don't truncate — they don't happen). Only `craft_planks` has positive net (+3 with plankYield 4; the implementation derives the net from the recipe table, so the number here is informational) |
| cook / bathe / refuel / build | net delta ≤ 0 always — no check needed (assert in tests) |
| `social.gave` (executor give rule) | executor skips the give when receiver has zero free bulk; reducer clamps defensively |
| pick_up / withdraw | truncate to taker's free bulk (partial moves; payload records actual counts) |
| deposit | truncate to chest free space (`chestCap − bulk(chest contents)`) |
| migration transform (R3) | carried bulk over cap spills into a ground pile at the agent's tile — never destroyed |

**Rationale**: a derived value cannot drift from its parts (same doctrine as fire
lit-ness derived from `FuelUntil`). Cap 24 > largest single yield (spear hunt 12),
so no single completion is unsatisfiable from empty.

**Alternatives**: stored `Bulk int` maintained by the reducer (rejected: redundant
state, hash-visible drift risk); clamping only in the reducer with executor unaware
(rejected: executor must know free bulk to honor "no depletion at zero space").

## R3 — Format bump 2→3, people-preserving migration, no land reset

**Decision**: `world.FormatVersion` 2→3. The bulk cap, yield truncation, death
spill, and give-guard change reducer/executor behavior for *existing* event
shapes, so a v2 log replayed under v3 code would diverge — the format gate is the
shield (same doctrine as 012's v2 gate). `scriptworld migrate` gains a 2→3 step:
archive `world.db` → `world.v2.db`, fresh log `world.created` + `world.migrated
{from_format: 2, state}` (existing machinery, world-migration wiki note). The
transform is pure and people-preserving; **the land is NOT reset** — 013 changes
no terrain generation, so the map derives identically from the seed and agents,
structures, and overlays carry verbatim. Inventory over the cap spills to a pile
at the agent's tile (R2). Migration of a v1 world chains 1→2→3.

**Rationale**: reuses the entire R10/012 mechanism; the snapshot-cut's one
expensive property (land reset) is simply not triggered because no map inputs
changed.

**Decisions taken at implementation (v3-invariant principle)**: (a) over-cap
spill removes goods in canonical kind order — within food, least-nutritious
first (food_raw → food_cooked → meals) so a capped villager keeps its best
food; spears spill most-worn-first, mirroring the transfer idiom; (b) dead
agents' entire frozen inventory spills to a pile at their tile — v3's death
invariant carried forward, so a migrated world matches what v3 would have
produced; (c) mid-flight intents carry verbatim (unlike 1→2's wipe) — no map
inputs changed, so targets stay valid and the cap simply applies at completion;
(d) no separate v2 decoder — v3's additions are all additive omitempty, so
`sim.State` decodes v2 JSON exactly.

**Alternatives**: no bump, cap applies only to new acquisitions (rejected:
FR-001 says carried bulk MUST never exceed the cap, and old-log replay under new
reducer code would still diverge on death spills); land reset for symmetry with
1→2 (rejected: needless loss — reset was *caused* by map-input changes, absent
here).

## R4 — Goal argument surface: Kind + Qty on intents and plan steps

**Decision**: add `Kind string` + `Qty int` (both omitempty) to `Intent`,
`IntentSetPayload`, and `PlanStep`. Five new goals, all planner/plan-only
(added to `planGoals` and the inject_intent validation; **never** the reflex
ladder — FR-014):

| Goal | Target (resolveGoal) | Semantics |
|---|---|---|
| `drop` | current tile | instant; moves `Qty` of `Kind` (Qty 0 = all of kind) from inventory to the tile's pile |
| `pick_up` | nearest pile on/adjacent | instant; takes `Qty` of `Kind` truncated to free bulk (Kind empty = every kind, canonical field order, oldest food batches first; Qty 0 = as much as fits) |
| `deposit` | nearest chest | instant on arrival; moves `Qty` of `Kind` truncated to chest space |
| `withdraw` | nearest chest containing `Kind` (nearest chest if Kind empty) | instant on arrival; truncated to free bulk; non-owner ⇒ taking record (R5) |
| `build_chest` | nearest buildable tile (build_shelter pattern; pile tiles excluded) | timed build (`buildFireTicks`-comparable), recipe 6 planks, owner = builder |

Spears transfer most-worn-first (`Spears[0]`), preserving the sorted-ascending
invariant on both sides; durability rides the item (edge case pinned in spec).

**Rationale**: the spec's "chosen subset" (FR-004) requires kind/quantity
expressibility; two omitempty fields are the smallest additive surface and ride
the existing plan-step guard machinery unchanged.

**Alternatives**: fixed drop-everything semantics (rejected: violates "chosen
subset"); a per-goal args blob (rejected: schema-less state, validation burden).

## R5 — Theft: record event + existing relation/memory machinery as companions

**Decision**: when `withdraw` completes with taker ≠ owner, the executor emits, in
one batch: `agent.withdrew` (the mechanical move), `social.chest_taken
{owner, taker, x, y}` (the distinct record, FR-011 — reducer no-op beyond the
record itself, chronicle/TUI material), a `social.relation_changed`
(owner→taker, trust −120, affection −40, reason `"theft"`) through the existing
edge machinery, an owner `agent.memory_added` (subject = taker, tone −60,
salience high — a `TellableFor` gossip seed, regardless of distance), and witness
`agent.memory_added` companions for living, awake villagers within
`witnessRadius` (8) of the chest. Owner-from-own-chest emits `agent.withdrew`
only. A dead owner keeps the record and witness memories but gets no owner
memory (the dead don't remember; the village does — edge case pinned in spec).

**Rationale**: exactly the metatron/governance companion-batch pattern (memories
ride `agent.memory_added` in the same atomic batch; witness deltas are already an
established idiom); rumor birth then happens for free from the subject-tagged
memory (deterministic rumor machinery, social-fabric wiki).

**Alternatives**: reducer-internal trust delta on `agent.withdrew` (rejected:
FR-012 says "via the existing relation-change machinery" — the reason-tagged
`social.relation_changed` IS that machinery, and it keeps the delta visible in
the log); a permission check (rejected by spec: recorded, never prevented).

## R6 — Rot: per-minute sweep, batch deadlines, outcome events

**Decision**: `rotWindowTicks = 2 game days (172800)`. The executor's existing
per-game-minute heartbeat gains a rot sweep: for each pile (slice order), each
food batch with `SpoilAt ≤ tick` produces `sim.food_rotted{x, y, kind, n}`
(same-kind batches merged per pile per sweep). Reducer removes matching spoiled
batches (total: absent pile/batch no-ops). SC-004's +1 game-minute tolerance is
exactly the sweep cadence. Chests never rot (no deadlines on chest contents —
`Store` is a plain Inventory). Death spill batches get `SpoilAt = death tick +
rotWindowTicks`.

**Rationale**: per-minute matches the needs heartbeat and the spec's tolerance;
per-tick sweeping buys nothing but cost. Outcome-only payloads carry the removed
counts, replay applies them.

**Alternatives**: rot at pickup-time (lazy) (rejected: SC-004 requires visible,
event-sourced spoilage on the clock, and lazy rot makes pile contents lie to the
TUI).

## R7 — Death spill is reducer-internal on `agent.died`

**Decision**: no new event. The reducer's `agent.died` case moves the agent's
entire inventory into the pile at the death tile (creating/merging per R1),
emptying `Inv`. Under v3 this is a pure, total extension of an existing case —
shielded from v2 logs by the format bump (R3).

**Rationale**: precedent is debt-opening inside `social.gave` (reducer-internal
consequence); a separate spill event would record what the reducer can derive,
and could be forged/injected.

**Alternatives**: companion `agent.inventory_spilled` event (rejected: derivable;
one more injectable surface).

## R8 — Chest = Structure extension, not a new entity

**Decision**: `Structure` gains `Owner int` (omitempty; agent index — chests
always have owners, and non-chests never read it, so the zero-value round-trip
is unambiguous) and `Store *Inventory` (omitempty; nil for non-chests).
`agent.built{kind: "chest"}` sets both. `chestCap = 48` bulk (derived via
`bulk(*Store)`); recipe `build_chest: 6 planks` joins recipes.go. Build-site
validation (all builds, not just chests) rejects tiles holding a pile (FR-007).

**Rationale**: fires/shelters/ovens already prove the structure lifecycle
(placement, TUI, persistence); a parallel chest entity would duplicate all of it.

**Alternatives**: `State.Chests []Chest` (rejected: second structure-like
lifecycle to maintain); capacity stored per chest (rejected: constant in v1,
derive from kind).

## R9 — Implementation tiers (constitution V)

| Slice | Tier | Rubric |
|---|---|---|
| Substrate: state shapes, reducer cases, bulk audit, migration 2→3, format bump | **Opus 4.8** | cross-cutting determinism/replay surface; doctrine-adjacent (format gate, reducer totality) |
| Executor & minds wiring: goal resolution, completion semantics, theft companion batches, rot sweep, death spill, give-guard | **Opus 4.8** | executor + social machinery coupling; prior art shows completion re-validation is where live defects concentrate |
| Planner vocabulary & prompts: goalVocabulary, plan-step guard docs, kind/qty prompt guidance | Sonnet | single-package, pattern-following |
| TUI: pile/chest glyphs, zone grouping render, inspection panes, bulk display | Sonnet | view code, established pane patterns |

Escalation is one-way Sonnet→Opus per `.claude/agents/spec-implementer.md`; tier
+ justification recorded on TASK-51 at dispatch.
