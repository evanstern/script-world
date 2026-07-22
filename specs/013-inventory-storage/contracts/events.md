# Event Contract: Inventory & Storage v1

Extends the catalog in `docs/wiki/event-types.md`. Conventions inherited:
namespaced types, canonical-JSON payload structs (field order below is
canonical), outcome-only payloads (actual moved counts, absolute values, no
dice rolls), unknown types no-op in old reducers, `sim.*`/`agent.*` are pure
world happenings. New payload structs live in `internal/sim/agents.go` unless
noted. Format v3 (research R3) shields v2 logs from the changed semantics.

## New event types

| Type | Payload struct (canonical field order) | Emitted by | Reducer effect |
|---|---|---|---|
| `agent.dropped` | `DroppedPayload{agent, x, y, kind, n}` | executor, drop completion (instant, current tile) | `Inv[kind] −= n`; pile at `(x,y)` created-or-merged `+= n` (food becomes/merges a batch with `spoil_at = tick + rotWindowTicks`); spears move most-worn-first with durabilities; intent cleared |
| `agent.picked_up` | `PickedUpPayload{agent, x, y, kind, n}` | executor, pick_up completion (instant; one event per kind moved, same batch) | pile `−= n` (food oldest-batch-first), `Inv[kind] += n`; emptied pile removed; intent cleared on last event of the batch |
| `agent.deposited` | `DepositedPayload{agent, x, y, kind, n}` | executor, deposit completion at a chest (instant on arrival) | `Inv[kind] −= n`, chest `Store[kind] += n`; intent cleared |
| `agent.withdrew` | `WithdrewPayload{agent, x, y, kind, n, owner}` | executor, withdraw completion at a chest (instant on arrival) | chest `Store[kind] −= n`, `Inv[kind] += n`; intent cleared |
| `social.chest_taken` | `ChestTakenPayload{owner, taker, x, y}` (social.go) | executor, same batch as a non-owner `agent.withdrew` | none beyond the record itself — the distinct taking happening (FR-011); chronicle/TUI material |
| `sim.food_rotted` | `FoodRottedPayload{x, y, kind, n}` | executor per-game-minute rot sweep (same-kind batches merged per pile per sweep) | pile's food batches with `spoil_at ≤ tick` and matching kind removed (up to `n`); emptied pile removed |
| `agent.built{kind: "chest"}` | `BuiltPayload` (existing; kind gains `chest`) | executor, build_chest completion | `Planks −= 6`; structure added with `Owner = agent`, `Store = &Inventory{}` |

## Companion batch on a non-owner withdrawal (theft, FR-011/012)

Emitted by the executor in ONE batch with `agent.withdrew`, all existing types:

- `social.chest_taken{owner, taker, x, y}` — the record (above).
- `social.relation_changed` owner→taker: trust `theftTrustDelta` (−120),
  affection `theftAffectionDelta` (−40), reason `"theft"` — the existing edge
  machinery, log-visible.
- `agent.memory_added` for the owner (any distance, if living): subject = taker,
  tone `theftMemoryTone` (−60), high salience — a `TellableFor` gossip seed; the
  deterministic rumor machinery takes it from there.
- `agent.memory_added` for each living, awake villager within `witnessRadius`
  (8) of the chest (excluding the taker): witness memory, subject = taker,
  negative tone.

Owner withdrawing from their own chest ⇒ `agent.withdrew` only (FR-011). A dead
owner: record + relation delta + witness memories still emitted; the owner
memory is skipped (the dead don't remember; the village does).

## Changed semantics under format v3 (no new types)

- `agent.foraged` / `agent.chopped` / `agent.hunted` / `agent.quarried` /
  `agent.collected_water`: reducer clamps the yield to the taker's free bulk
  (`bulkCap − bulk(Inv)`); the remainder is forfeit (the overlay/depletion still
  applies — US1-AS2). The executor does not emit the event at all when free bulk
  is zero (below), so depletion-at-zero-space never occurs.
- `agent.crafted`: completion re-validation extends to net bulk delta — outputs
  that would not fit ⇒ no event, `agent.intent_done` only (crafts don't
  truncate; they don't happen). Only `craft_planks` has a positive net (+1).
- `social.gave`: executor skips the give when the receiver has zero free bulk;
  reducer clamps defensively.
- `agent.died`: reducer additionally spills the agent's entire inventory into
  the pile at the death tile (created/merged; food batches stamped
  `tick + rotWindowTicks`), emptying `Inv` — reducer-internal, no new event
  (debt-opening precedent, research R7).
- `build_*` site validation: a tile holding a pile is not buildable (FR-007).

## Emission rules

- All storage completions re-validate at completion tick (contested-resource
  pattern): vanished pile, missing/full chest, zero free bulk, missing carried
  goods ⇒ `agent.intent_done` only, no effect event. Two takers same tick:
  deterministic agent-order arbitration; the second finds what remains. A rot
  tick and a pickup on the same tick: the sweep runs in the executor's
  established phase order; whichever lands first in the batch wins (contested
  re-validation, spec edge case).
- Payloads carry **actual** moved counts (post-clamp outcomes), never requests;
  the reducer applies recorded counts and stays total (absent pile/chest/batch
  ⇒ no-op).
- `drop`/`pick_up`/`deposit`/`withdraw` are planner/plan-only goals — the five
  new goals are in `planGoals` + inject_intent validation, and none appear in
  the reflex ladder (FR-014). Reflex code is untouched.
- None of the new types are model-injectable; all are world-emitted (executor).
- New memorable moments (salience table, memory.go): chest built (high,
  village-visible — oven precedent), taking witnessed/suffered (high, negative,
  subject-tagged), own death-adjacent pile looted — none beyond these in v1.

## Determinism notes

- Structs, never maps; field order above is canonical serialization order.
- Pile iteration = `State.Piles` slice order; batch iteration = drop order;
  "all kinds" transfers use canonical inventory field order (data-model.md).
- Rot deadlines (`spoil_at`) are absolute ticks recorded at drop time; the sweep
  is a pure function of (state, tick) — the fuel-sweep pattern.
- Replay reproduces byte-identical state including piles, batches, chest
  contents/owners (SC-005); all new types no-op under pre-013 replay code.
