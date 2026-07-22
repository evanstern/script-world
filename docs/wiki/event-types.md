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
  - internal/daemon/daemon.go
verified_against: 1d1cc6ff8cad2414108f7e768f61eb0faaea3088
---

# Event types

Every event has a namespaced `type` and a canonical-JSON payload defined as a Go
struct in `internal/sim` (structs, never maps, so bytes are deterministic; core
payloads live in `state.go`, families note their own file below).
This catalog is the contract downstream consumers (chronicle, Metatron digests, the
TUI) will read. Spec 012 (resources/food/crafting) bumped the world save format to
**v2** ([[world-save-directory]]): a widened `Inventory`, a new `agent.ate` payload
shape, and nine new event types below. A v1 world cannot boot under a v2 build — it
must run `scriptworld migrate` first ([[world-migration]]), whose sole output event,
`world.migrated`, is also cataloged here.

## How it works

| Type | Payload struct | Emitted by | Reducer effect |
|---|---|---|---|
| `world.created` | `WorldCreatedPayload{name, seed}` | CLI `new`, tick 0 | none (genesis marker) |
| `world.migrated` | `WorldMigratedPayload{from_format, source_events, source_tick, state}` (`state` embeds the full canonical `sim.State`) | `scriptworld migrate` (client-side, offline — [[world-migration]]), once, right after a fresh `world.created` | replaces `State` wholesale (after checking `state.Seed` matches — a foreign payload is a no-op); the log alone (`world.created` → `world.migrated`) reproduces the migrated world with zero snapshots |
| `clock.paused` / `clock.resumed` | `{}` | loop command | pause flag (+ snapshot on pause) |
| `clock.speed_set` | `SpeedSetPayload{speed}` | loop command | `Speed` updated |
| `clock.degraded` / `clock.recovered` | `DegradedPayload{effective_rate}` / `{}` | loop auto-slow | degradation flags |
| `sim.day_started` / `sim.night_started` | `DayPayload{day}` | executor, 06:00/22:00 | `Night` flag only — waking is explicit |
| `sim.forage_regrown` | `RegrownPayload{x, y}` | executor, regrow tick | harvest overlay removed |
| `agent.intent_set` | `IntentSetPayload{agent, goal, target, res, source}` | reflex (grace-gated), planner injection, or a plan step firing | intent installed; `source` (`reflex`/`planner`/`plan`/`meeting`) says which mind chose it |
| `agent.work_started` | `WorkStartedPayload{agent, tick}` | executor at target | `WorkStart` stamped |
| `agent.intent_done` | `AgentPayload{agent}` | executor (done/invalid/unreachable) | intent cleared |
| `agent.moved` | `AgentMovedPayload{agent, x, y}` | executor pathing | position updated |
| `agent.foraged` / `agent.chopped` / `agent.hunted` | `HarvestPayload{agent, x, y}` | work completion | +FoodRaw (forage `forageYieldV2`; hunt `huntYieldBare`, or `huntYieldSpear` + spends `Spears[0]`'s last use if carrying one) / +wood, overlay (harvest/cleared/den cooldown), intent cleared |
| `agent.quarried` | `HarvestPayload{agent, x, y}` | work completion (rock outcrop) | +`quarryYield` Stone; `(x,y)` appended to `State.Quarried` (permanent — [[worldmap-generation]], [[executor]]), intent cleared |
| `agent.collected_water` | `HarvestPayload{agent, x, y}` | work completion (any water tile) | +`collectWaterYield` Water, intent cleared (no overlay — water never depletes) |
| `agent.crafted` | `CraftedPayload{agent, kind}` (kind ∈ planks\|refined_stone\|spear) | work completion (hand-craft) | recipe delta from `recipes.go` by goal (re-derived from `kind`); a fresh spear appends `spearDurability` (3) to `Spears`, kept sorted ascending, intent cleared |
| `agent.built` | `BuiltPayload{agent, kind, x, y}` (kind ∈ fire\|shelter\|oven) | work completion | structure added, recipe's inputs spent (via `recipes.go`'s `build_<kind>`); a fresh fire also gets `FuelUntil = tick + 2×fireBurnPerWood`, intent cleared |
| `agent.cooked` | `CookedPayload{agent, station, consumed, produced, kind}` (station ∈ fire\|oven; kind ∈ food_cooked\|meals) | work completion (cook) | −FoodRaw(consumed), +kind(produced); an oven cook also −1 Wood, intent cleared |
| `agent.bathed` | `BathedPayload{agent, morale_after, warmth_after}` | work completion (bathe, oven only) | −1 Water, −1 Wood, Morale/Warmth set to the absolute post-cap values, intent cleared |
| `agent.refueled` | `RefueledPayload{agent, x, y, fuel_until}` | reflex/planner (instant on arrival) | −1 Wood, the fire at `(x,y)`'s `FuelUntil` set to the absolute (already-capped) deadline, intent cleared |
| `agent.spear_broke` | `SpearBrokePayload{agent}` | work completion (hunt, companion to `agent.hunted` in the same batch) | removes the now-zero `Spears[0]` entry |
| `sim.fire_burned_out` | `FireBurnedOutPayload{x, y}` | `stepEvents`, once per fuel-window transition (`tick-1 < FuelUntil <= tick`) | none — lit-ness stays derived from `FuelUntil`; chronicle/TUI material, plus a low-salience witness memory for nearby living agents |
| `agent.ate` | `AtePayload{agent, meals, cooked, raw, food_after}` | reflex/planner (instant), most-nutritious-first (Meals→FoodCooked→FoodRaw) to satiety (`eatOutcome`) | −Meals/−FoodCooked/−FoodRaw by the consumed counts, Food need set to the absolute `food_after` |
| `agent.slept` / `agent.woke` | `AgentPayload{agent}` | executor | sleep flag (slept clears intent) |
| `agent.needs_changed` | `NeedsPayload{agent, …}` | per-game-minute heartbeat | needs set to absolute values |
| `agent.died` | `DiedPayload{agent, cause}` | heartbeat at 0 health | `Dead`, intent cleared |
| `agent.talked` | `TalkedPayload{a, b}` | executor, adjacent pair (chat-while-working) | +morale both, talk cooldown; both remember |
| `agent.memory_added` | `MemoryAddedPayload{agent, text, salience, subject, tone}` | executor heuristics; convo gists (injected) | append to `Memories`; subject/tone mark gossip seeds ([[agent-mind]], [[social-fabric]]) |
| `agent.thought` | `ThoughtPayload{agent, text, source}` | `inject_intent` command (planner); `inject_social` (musing) | none (chronicle material) |
| `daemon.started` / `daemon.stopped` | `DaemonStartedPayload` / `DaemonStoppedPayload` | daemon lifecycle | none |
| `social.*` family | see `specs/003-social-fabric/contracts/social-events.md` | executor rules, genesis, convo driver (injected) | edges, ledger, rumors, secrets; `social.conversation` appends the bounded record ring (TASK-22, [[social-fabric]]) |
| consolidation family: `agent.memory_promoted` / `agent.memory_faded` / `agent.belief_revised` / `agent.narrative_set` / `agent.consolidated` | payload structs in `internal/sim/consolidate.go`; contract in `specs/004-nightly-consolidation/contracts/` | consolidation driver (injected) | salience boost / memory removal / belief create-or-revise / narrative replace / once-per-night ledger ([[nightly-consolidation]]); all reducer-total (vanished targets no-op) |
| `gru.emerged` / `gru.moved` / `gru.sighted` / `gru.attacked` / `gru.withdrew` | payload structs in `internal/sim/gru.go` | `gruStep` (executor tick) | `State.Gru` lifecycle/position; sighting latch; attack sets absolute post-wound health, wakes victim, clears intent ([[gru]]); reducer-total (vanished gru no-ops) |
| `chronicle.entry` | `ChronicleEntryPayload{day, from_tick, to_tick, text, thread, agents}` in `internal/sim/chronicle.go` | narrator driver (injected, TASK-11) | appends the bounded `State.Chronicle` ring ([[chronicle]]) |
| `metatron.charge_regenerated` | `ChargeRegeneratedPayload{}` in `internal/sim/metatron.go` | executor, absolute 6-game-hour boundaries below cap | `MetatronCharges` +1, cap 3 ([[metatron]]) |
| `metatron.nudged` | `MetatronNudgedPayload{form, targets, text}` | Metatron console turn (injected, TASK-12) | validates (charges > 0, form, living targets, text cap) then `MetatronCharges` −1; villager memories ride companion `agent.memory_added` events in the same atomic batch |
| `meeting.*` / `norm.*` families (TASK-13) | payload structs in `internal/sim/governance.go`; contract in `specs/006-norms-and-votes/contracts/governance-events.md` | all executor beats (`governanceEvents`) EXCEPT `meeting.proposal_rephrased`, the one injected governance type (mind phrasing driver), and a config-declared `meeting.convention_established`, seeded by the daemon on boot | meeting lifecycle on `State.Meeting`, norms enact/amend/repeal on `State.Norms`, reducer-internal voter/witness edge deltas; rephrase validates (norm exists, text ≤ 280) then swaps text only ([[governance]]) |
| `meeting.convention_established` (TASK-36) | `MeetingConventionPayload{convene_second, open_second, x, y, source}` in `internal/sim/governance.go` | executor emergent-gathering detector (`source: emergent`) or daemon boot seed from `world.json`'s `meeting` block (`source: config`) | one-shot: sets `State.MeetingConvention` (first source wins) and seeds `MeetingPlace`; clears the gathering watch ([[governance]]) |
| `sim.gathering_observed` (TASK-36) | `GatheringObservedPayload{x, y, start}` in `internal/sim/governance.go` | executor per-minute watch while no convention exists (start/break of a sustained gathering; all-zero = reset) | `Meeting.GatherStart/GatherX/GatherY` set, so replay reconstructs the emergent watch |
| `cog.thought` | `CogThoughtPayload{job, class, agent, snapshot_tick, generation, trigger_seq, points, predicted_wall_ms, predicted_land_tick}` in `internal/sim/cognition.go` | mind driver (injected) when a call passes the router; `trigger_seq` is the log seq of the arming stimulus (0 = pure cadence) | none (telemetry, TASK-32, [[cognition]]) |
| `cog.outcome` | `CogOutcomePayload{job, class, agent, outcome, snapshot_tick, landing_tick, staleness_ticks, predicted_wall_ms, actual_wall_ms, kind?, reason?}` | loop landing ladder (landed/adapted/rejected-* /superseded) or mind driver (suppressed/expired/unusable — router suppressions have no matching `cog.thought`) | none — the single terminal record of every thought; rejections carry `kind` `prediction-miss` or `world-change` |
| `agent.intent_rejected` | `IntentRejectedPayload{agent, goal, reason, staleness_ticks}` in `internal/sim/cognition.go` | loop, when the landing ladder refuses a metered intent (alongside its `cog.outcome`) | none — its own type so souls/chronicle can notice refused intentions without parsing `cog.*` |
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
[[sim-loop]] emit the sim/agent/clock families; `scriptworld migrate`
([[cli-scriptworld]], [[world-migration]]) emits `world.migrated`; the mind driver and the loop's
landing ladder emit the `cog.*` telemetry ([[cognition]]); [[daemon-lifecycle]]
emits `daemon.*`; [[event-log]] stores them;
[[ipc-protocol]] pushes them to subscribers verbatim.

## Operational notes

The outcome-payload
convention ([[deterministic-rng]]) is load-bearing — keep it; `gru.attacked`
carrying absolute post-wound health (never the wound roll) is the pattern.
