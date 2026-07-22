---
name: event-types
description: The event taxonomy — namespaced types, their payload structs, who emits them, and their reducer effects
kind: concept
sources:
  - internal/sim/state.go
  - internal/sim/agents.go
  - internal/sim/executor.go
  - internal/sim/recipes.go
  - internal/sim/gru.go
  - internal/sim/loop.go
  - internal/sim/miracles.go
  - internal/daemon/daemon.go
verified_against: c8fe41323c1155e8fda1619e4e0ed70ff3f37645
---

# Event types

Every event has a namespaced `type` and a canonical-JSON payload defined as a Go
struct in `internal/sim` (structs, never maps, so bytes are deterministic; core
payloads live in `state.go`, families note their own file below).
This catalog is the contract downstream consumers (chronicle, Metatron digests, the
TUI) will read. Spec 012 (resources/food/crafting) bumped the world save format to
v2 ([[world-save-directory]]): a widened `Inventory`, a new `agent.ate` payload
shape, and nine new event types. Spec 013 (inventory & storage — bulk cap, ground
piles, builder-owned chests, theft, food rot) bumped it again to **v3**: six more
event types below (drop/pick_up/deposit/withdraw completions, a theft record, and
the food-rot sweep), plus changed semantics on several existing event types (no new
types for these — yield clamping on gathers, net-bulk re-validation on crafts, a
zero-bulk give guard, inventory death-spill) that a v2 replay under this build would
get wrong; the format gate shields old logs from the new semantics. A v1 world
cannot boot under this build — it must run `promptworld migrate` first
([[world-migration]]), chaining 1→2→3 in one run; its sole output event,
`world.migrated`, is also cataloged here.

## How it works

| Type | Payload struct | Emitted by | Reducer effect |
|---|---|---|---|
| `world.created` | `WorldCreatedPayload{name, seed}` | CLI `new`, tick 0 | none (genesis marker) |
| `world.migrated` | `WorldMigratedPayload{from_format, source_events, source_tick, state}` (`state` embeds the full canonical `sim.State`) | `promptworld migrate` (client-side, offline — [[world-migration]]), once, right after a fresh `world.created` | replaces `State` wholesale (after checking `state.Seed` matches — a foreign payload is a no-op); the log alone (`world.created` → `world.migrated`) reproduces the migrated world with zero snapshots |
| `clock.paused` / `clock.resumed` | `{}` | loop command | pause flag (+ snapshot on pause) |
| `clock.speed_set` | `SpeedSetPayload{speed}` | loop command | `Speed` updated |
| `clock.degraded` / `clock.recovered` | `DegradedPayload{effective_rate}` / `{}` | loop auto-slow | degradation flags |
| `sim.day_started` / `sim.night_started` | `DayPayload{day}` | executor, 06:00/22:00 | `Night` flag only — waking is explicit |
| `sim.forage_regrown` | `RegrownPayload{x, y}` | executor, regrow tick | harvest overlay removed |
| `agent.intent_set` | `IntentSetPayload{agent, goal, target, res, source}` | reflex (grace-gated), planner injection, or a plan step firing | intent installed; `source` (`reflex`/`planner`/`plan`/`meeting`) says which mind chose it; also stamps `Agent.LastGoal`/`LastGoalTick` (spec 015 — never cleared by any event, the villagers tab's past-objective line, [[tui-client]]) |
| `agent.work_started` | `WorkStartedPayload{agent, tick}` | executor at target | `WorkStart` stamped |
| `agent.intent_done` | `AgentPayload{agent}` | executor (done/invalid/unreachable) | intent cleared |
| `agent.moved` | `AgentMovedPayload{agent, x, y}` | executor pathing | position updated |
| `agent.foraged` / `agent.chopped` / `agent.hunted` | `HarvestPayload{agent, x, y}` | work completion (spec 013: skipped entirely — no event — when the taker's free bulk is zero, US1-AS1, so depletion never happens with no room to carry the take) | +FoodRaw (forage `forageYieldV2`; hunt `huntYieldBare`, or `huntYieldSpear` + spends `Spears[0]`'s last use if carrying one) / +wood, each clamped to the taker's pre-event free bulk (`bulkCap − bulk(Inv)`, spec 013 US1-AS2 — the forfeited remainder is lost, not refunded); overlay (harvest/cleared/den cooldown) applies regardless of the clamp, intent cleared |
| `agent.quarried` | `HarvestPayload{agent, x, y}` | work completion (rock outcrop; same zero-free-bulk skip as above) | +`quarryYield` Stone clamped to free bulk; `(x,y)` appended to `State.Quarried` regardless (permanent — [[worldmap-generation]], [[executor]] — the outcrop depletes even when the yield is forfeit), intent cleared |
| `agent.collected_water` | `HarvestPayload{agent, x, y}` | work completion (any water tile; same zero-free-bulk skip) | +`collectWaterYield` Water clamped to free bulk, intent cleared (no overlay — water never depletes) |
| `agent.crafted` | `CraftedPayload{agent, kind}` (kind ∈ planks\|refined_stone\|spear) | work completion (hand-craft; completion re-validation extends to the net output−input bulk delta, spec 013 US1 — a craft whose net wouldn't fit is not emitted, `agent.intent_done` only; only `craft_planks` has a positive net) | recipe delta from `recipes.go` by goal (re-derived from `kind`); a fresh spear appends `spearDurability` (3) to `Spears`, kept sorted ascending, intent cleared |
| `agent.built` | `BuiltPayload{agent, kind, x, y}` (kind ∈ fire\|shelter\|oven\|chest) | work completion (site pre-validated as buildable; since spec 013, `buildSite` additionally rejects a tile holding a pile — FR-007) | structure added, recipe's inputs spent (via `recipes.go`'s `build_<kind>`); a fresh fire also gets `FuelUntil = tick + 2×fireBurnPerWood`; a fresh chest (spec 013 US3) gets `Owner = agent` (permanent, no transfer in v1) and an empty `Store`, intent cleared |
| `agent.dropped` | `DroppedPayload{agent, x, y, kind, n}` | executor, `drop` completion (instant, agent's current tile — spec 013 US2, planner/plan-only) | `Inv[kind] −= n`; the tile's pile created-or-merged `+= n` (food becomes/merges a batch stamped `spoil_at = tick + rotWindowTicks`; spears move most-worn-first with their durabilities), intent cleared |
| `agent.picked_up` | `PickedUpPayload{agent, x, y, kind, n}` | executor, `pick_up` completion (instant on arrival at a pile on/adjacent tile; one event per kind moved in the batch) | pile `−= n` (food oldest-batch-first), `Inv[kind] += n`; an emptied pile is removed; intent cleared on the last event of the batch |
| `agent.deposited` | `DepositedPayload{agent, x, y, kind, n}` | executor, `deposit` completion at a chest (instant on arrival — spec 013 US3) | `Inv[kind] −= n`, chest `Store[kind] += n`, both clamped to the chest's free space (`chestCap − bulk(*Store)`); intent cleared |
| `agent.withdrew` | `WithdrewPayload{agent, x, y, kind, n, owner}` | executor, `withdraw` completion at a chest (instant on arrival) | chest `Store[kind] −= n`, `Inv[kind] += n`, clamped to the taker's free bulk; intent cleared; a non-owner taker co-emits the theft companion batch (`social.chest_taken` + a reason-`"theft"` `social.relation_changed` + owner/witness `agent.memory_added`, all in the same batch — [[social-fabric]]) |
| `sim.food_rotted` | `FoodRottedPayload{x, y, kind, n}` | executor, per-game-minute rot sweep (spec 013 US5; same-kind spoiled batches merged per pile per sweep) | pile's food batches with `spoil_at ≤ tick` matching `kind` removed (up to `n`, oldest first); an emptied pile is removed; chest food never batches, so chests are never reached (FR-010) |
| `agent.cooked` | `CookedPayload{agent, station, consumed, produced, kind}` (station ∈ fire\|oven; kind ∈ food_cooked\|meals) | work completion (cook) | −FoodRaw(consumed), +kind(produced); an oven cook also −1 Wood, intent cleared |
| `agent.bathed` | `BathedPayload{agent, morale_after, warmth_after}` | work completion (bathe, oven only) | −1 Water, −1 Wood, Morale/Warmth set to the absolute post-cap values, intent cleared |
| `agent.refueled` | `RefueledPayload{agent, x, y, fuel_until}` | reflex/planner (instant on arrival) | −1 Wood, the fire at `(x,y)`'s `FuelUntil` set to the absolute (already-capped) deadline, intent cleared |
| `agent.spear_broke` | `SpearBrokePayload{agent}` | work completion (hunt, companion to `agent.hunted` in the same batch) | removes the now-zero `Spears[0]` entry |
| `sim.fire_burned_out` | `FireBurnedOutPayload{x, y}` | `stepEvents`, once per fuel-window transition (`tick-1 < FuelUntil <= tick`) | none — lit-ness stays derived from `FuelUntil`; chronicle/TUI material, plus a low-salience witness memory for nearby living agents |
| `agent.ate` | `AtePayload{agent, meals, cooked, raw, food_after}` | reflex/planner (instant), most-nutritious-first (Meals→FoodCooked→FoodRaw) to satiety (`eatOutcome`) | −Meals/−FoodCooked/−FoodRaw by the consumed counts, Food need set to the absolute `food_after` |
| `agent.slept` / `agent.woke` | `AgentPayload{agent}` | executor | sleep flag (slept clears intent) |
| `agent.needs_changed` | `NeedsPayload{agent, …}` | per-game-minute heartbeat | needs set to absolute values |
| `agent.died` | `DiedPayload{agent, cause}` | heartbeat at 0 health | `Dead`, intent cleared; spec 013 (US2, FR-006, research R7): the agent's entire carried inventory spills into a pile at the death tile (created/merged, food batches stamped `tick + rotWindowTicks`), emptying `Inv` — reducer-internal, no new event |
| `agent.talked` | `TalkedPayload{a, b}` | executor, adjacent pair (chat-while-working) | +morale both, talk cooldown; both remember |
| `agent.memory_added` | `MemoryAddedPayload{agent, text, salience, subject, tone}` | executor heuristics; convo gists (injected) | append to `Memories`; subject/tone mark gossip seeds ([[agent-mind]], [[social-fabric]]) |
| `agent.thought` | `ThoughtPayload{agent, text, source}` | `inject_intent` command (planner); `inject_social` (musing) | none (chronicle material) |
| `daemon.started` / `daemon.stopped` | `DaemonStartedPayload` / `DaemonStoppedPayload` | daemon lifecycle | none |
| `social.*` family | see `specs/003-social-fabric/contracts/social-events.md` | executor rules, genesis, convo driver (injected) | edges, ledger, rumors, secrets; `social.conversation` appends the bounded record ring (TASK-22, [[social-fabric]]); `social.gave` (spec 013 US1) is additionally skipped by the executor when the receiver has zero free bulk and the reducer clamps defensively (never over `bulkCap`) |
| `social.chest_taken` | `ChestTakenPayload{owner, taker, x, y}` (`social.go`) | executor, same batch as a non-owner `agent.withdrew` (spec 013 US4, FR-011) | none beyond the record itself — the distinct taking happening; chronicle/TUI material ([[social-fabric]]) |
| consolidation family: `agent.memory_promoted` / `agent.memory_faded` / `agent.belief_revised` / `agent.narrative_set` / `agent.consolidated` | payload structs in `internal/sim/consolidate.go`; contract in `specs/004-nightly-consolidation/contracts/` | consolidation driver (injected) | salience boost / memory removal / belief create-or-revise / narrative replace / once-per-night ledger ([[nightly-consolidation]]); all reducer-total (vanished targets no-op) |
| `gru.emerged` / `gru.moved` / `gru.sighted` / `gru.attacked` / `gru.withdrew` | payload structs in `internal/sim/gru.go` | `gruStep` (executor tick) | `State.Gru` lifecycle/position; sighting latch; attack sets absolute post-wound health, wakes victim, clears intent ([[gru]]); reducer-total (vanished gru no-ops) |
| `chronicle.entry` | `ChronicleEntryPayload{day, from_tick, to_tick, text, thread, agents}` in `internal/sim/chronicle.go` | narrator driver (injected, TASK-11) | appends the bounded `State.Chronicle` ring ([[chronicle]]) |
| `metatron.charge_regenerated` | `ChargeRegeneratedPayload{}` in `internal/sim/metatron.go` | executor, absolute 6-game-hour boundaries below cap | `MetatronCharges` +1, cap 3 ([[metatron]]) |
| `metatron.nudged` | `MetatronNudgedPayload{form, targets, text}` | Metatron console turn (injected, TASK-12) | validates (charges > 0, form, living targets, text cap) then `MetatronCharges` −1; villager memories ride companion `agent.memory_added` events in the same atomic batch |
| `metatron.time_snapped` | `TimeSnappedPayload{to_tick, gratis}` in `internal/sim/miracles.go` | angel's turn reply or the `promptworld miracle` CLI/IPC door (spec 016, [[metatron-miracles]]), injected via `InjectSocial` | rejects a target at or before the current tick (forward-only); spends 2 charges (the dearest miracle) unless `gratis`; `rebaseTicks` shifts every relative-duration field forward by the jump so remaining durations are preserved, then `State.Tick = to_tick`; the skipped regeneration boundaries mint no charges |
| `metatron.item_granted` | `ItemGrantedPayload{agent, kind, qty, gratis}` | angel's turn reply or the CLI/IPC door, injected | validates a living villager, a known item kind, positive qty, and the bulk cap (reject-whole, never clamp); spends 1 charge unless `gratis`; adds `qty` units (a spear grant appends `qty` fresh `spearDurability` entries, kept sorted) |
| `metatron.entity_moved` | `EntityMovedPayload{class, x, y, to_x, to_y, gratis}` (`class` ∈ villager\|structure\|pile) | angel's turn reply or the CLI/IPC door, injected | validates presence at the source and the destination's placement rule (villager/pile → passable, structure → buildSite); spends 1 charge unless `gratis`; relocates the entity (a moved villager drops its intent and goes idle at the landing tick; a moved structure carries its `FuelUntil`/`Owner`/`Store`; a moved pile merges onto any pile already at the destination) |
| `metatron.entity_removed` | `EntityRemovedPayload{class, x, y, gratis}` (`class` ∈ structure\|pile\|terrain; villager is rejected — never removable) | angel's turn reply or the CLI/IPC door, injected | validates presence; spends 1 charge unless `gratis`; deletes the structure (a chest first spills its `Store` to a ground pile — goods are never silently destroyed) or the pile (with contents), or overlays the terrain through the executor's own vocabulary (tree→`Cleared`, forage→`Harvested` with regrow, rock→`Quarried`; an already-overlaid tile is rejected as a no-op target) |
| `meeting.*` / `norm.*` families (TASK-13) | payload structs in `internal/sim/governance.go`; contract in `specs/006-norms-and-votes/contracts/governance-events.md` | all executor beats (`governanceEvents`) EXCEPT `meeting.proposal_rephrased`, the one injected governance type (mind phrasing driver), and a config-declared `meeting.convention_established`, seeded by the daemon on boot | meeting lifecycle on `State.Meeting`, norms enact/amend/repeal on `State.Norms`, reducer-internal voter/witness edge deltas; rephrase validates (norm exists, text ≤ 280) then swaps text only ([[governance]]) |
| `meeting.convention_established` (TASK-36) | `MeetingConventionPayload{convene_second, open_second, x, y, source}` in `internal/sim/governance.go` | executor emergent-gathering detector (`source: emergent`) or daemon boot seed from `world.json`'s `meeting` block (`source: config`) | one-shot: sets `State.MeetingConvention` (first source wins) and seeds `MeetingPlace`; clears the gathering watch ([[governance]]) |
| `sim.gathering_observed` (TASK-36) | `GatheringObservedPayload{x, y, start}` in `internal/sim/governance.go` | executor per-minute watch while no convention exists (start/break of a sustained gathering; all-zero = reset) | `Meeting.GatherStart/GatherX/GatherY` set, so replay reconstructs the emergent watch |
| `cog.thought` | `CogThoughtPayload{job, class, agent, snapshot_tick, generation, trigger_seq, points, predicted_wall_ms, predicted_land_tick}` in `internal/sim/cognition.go` | mind driver (injected) when a call passes the router; `trigger_seq` is the log seq of the arming stimulus (0 = pure cadence) | none (telemetry, TASK-32, [[cognition]]) |
| `cog.outcome` | `CogOutcomePayload{job, class, agent, outcome, snapshot_tick, landing_tick, staleness_ticks, predicted_wall_ms, actual_wall_ms, kind?, reason?}` | loop landing ladder (landed/adapted/rejected-* /superseded) or mind driver (suppressed/expired/unusable — router suppressions have no matching `cog.thought`) | none — the single terminal record of every thought; rejections carry `kind` `prediction-miss` or `world-change` |
| `agent.intent_rejected` | `IntentRejectedPayload{agent, goal, reason, staleness_ticks}` in `internal/sim/cognition.go` | loop, when the landing ladder refuses a metered intent (alongside its `cog.outcome`) | none — its own type so the villagers tab/chronicle can notice refused intentions without parsing `cog.*` |
| `cog.recalibration_recommended` | `RecalibrationPayload{tier, estimate_s_per_pt, spike_rate, window}` in `internal/sim/cognition.go` | mind driver (injected) when a tier's live estimator breaches the spike-rate drift threshold (once per breach episode) | none (telemetry) |
| `agent.plan_set` | `PlanSetPayload{agent, job, steps}` in `internal/sim/plan.go` | loop, on a guarded plan landing (TASK-32 US4) | `Agent.Plan` replaced with the steps |
| `agent.plan_step_started` / `agent.plan_expired` | `PlanStepPayload{agent, job, step, reason?}` in `internal/sim/plan.go` | executor (`planStepEvents`) on an idle agent's head step firing / window closing or resolve failing | head step popped / whole remaining plan cleared (a broken sequence is not resumed) |
| hail family (TASK-47): `social.hailed` / `social.hail_met` / `social.hail_expired` | `HailedPayload{from, to, until}` / `HailMetPayload{from, to}` / `HailExpiredPayload{from, to}` in `internal/sim/agents.go`; contract in `specs/010-hail-protocol/contracts/events.md` | loop (`inject_intent` talk_to landing) and executor (`planStepEvents` talk_to firing) emit `hailed`; the executor's per-tick `hailStep` sweep emits `met` (hailer adjacent, accompanied by the `agent.talked` talk shape bypassing the ambient cooldown) or `expired` (window closed) | `hailed` sets `Agent.Hail{By, Until}` (the movement-only pause); `met`/`expired` clear it — `agent.died` and `agent.slept` also clear it. World-emitted only, never model-injectable |

Conventions: `clock.*` are applied player/scheduler commands; `sim.*` and `agent.*`
are world happenings (pure functions of state + seed + tick); `daemon.*` are process
bookkeeping, wall-time dependent, and excluded from determinism comparisons (as are
`clock.*` in the binary-level test, since their ticks depend on command timing).
Payloads record **outcomes** (positions reached, absolute need values), never dice
rolls, so replay needs no RNG. Unknown types are no-ops in the reducer, so adding
types is backward-compatible with old replay code. The `cog.*` family and
`agent.intent_rejected` (TASK-32, [[cognition]]) are recorded observability —
explicit reducer no-ops whose wall-time fields are recorded input, so no failure
is silent and thought chains are walkable from the log alone; their payload
field order is canonical per `specs/007-cognition-horizon/contracts/events.md`.
`world.migrated` (spec 012 US6) is the one exception to "payloads are small
outcomes" — its payload embeds the entire canonical `sim.State`, by design: it is
the single record standing in for the whole pre-break history, and the reducer's
`state.Seed` check keeps it total (a mismatched payload no-ops rather than erroring).

## Connections

[[sim-state-reducer]] applies these; the [[executor]], [[reflex-policy]], and
[[sim-loop]] emit the sim/agent/clock families; `promptworld migrate`
([[cli-promptworld]], [[world-migration]]) emits `world.migrated`; the mind driver and the loop's
landing ladder emit the `cog.*` telemetry ([[cognition]]); [[daemon-lifecycle]]
emits `daemon.*`; [[event-log]] stores them;
[[ipc-protocol]] pushes them to subscribers verbatim. The `metatron.*` miracle
family is emitted through [[metatron]]'s two doors and reduced in
`internal/sim/miracles.go` — see [[metatron-miracles]] for the cost table,
gratis doctrine, and the shift-semantics re-base taxonomy.

## Operational notes

The outcome-payload
convention ([[deterministic-rng]]) is load-bearing — keep it; `gru.attacked`
carrying absolute post-wound health (never the wound roll) is the pattern.
