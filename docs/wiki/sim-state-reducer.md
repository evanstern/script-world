---
name: sim-state-reducer
description: sim.State and Apply — the single event-driven mutation path used identically live and in replay; canonical JSON for hashing
kind: component
sources:
  - internal/sim/state.go
  - internal/sim/agents.go
  - internal/sim/recipes.go
verified_against: 367d689446f502d9351ee48959c5397d4db037a0
---

# Sim state & reducer

`sim.State` is the whole world in one struct: clock state (tick, paused, speed,
degraded, effective rate) plus the living world — agents with needs/intents/
inventories (the v2 resource set, spec 012 — [[executor]])/memories (with
`IdleSince` for the reflex grace, a `NearDeath`
latch, a `Generation` interrupt counter and pending `Plan` steps for the
[[cognition]] horizon — both `omitempty` so pre-TASK-32 snapshots stay
byte-stable), structures (`fire`/`shelter`/`oven`/`chest`, fires carrying a
`FuelUntil`; chests (spec 013 US3) carrying a permanent `Owner` — the builder's
agent index, zero-value round-tripping unambiguously since every chest has one —
and a `Store *Inventory` capped at `chestCap` via the same derived `bulk()` used
for agents), cleared trees, harvested forage, den cooldowns, `Quarried` depleted
rock outcrops (spec 012, permanent, `omitempty`), `Piles []Pile` — the per-tile
ground commons of dropped/spilled goods (spec 013 US2): event-sourced overlay
state like `Quarried`, one pile per tile (a reducer invariant), non-food a flat
count, food batch-tracked (`FoodBatch{Kind, N, SpoilAt}`, same `(Kind,SpoilAt)`
merges), spears sorted ascending, `omitempty` — the social
fabric — relation edges, the debt ledger, the rumor registry with per-holder
variants and the bounded conversation-record ring ([[social-fabric]]) — the
consolidated inner life: per-agent beliefs, self-narrative, and the
once-per-night consolidation ledger ([[nightly-consolidation]]) — the
[[gru]] (`Gru *Gru`, nil while not abroad; `omitempty` keeps pre-TASK-10
snapshots valid) — and the narrated story: the bounded `State.Chronicle`
ring ([[chronicle]], TASK-11), which rides snapshots so attaching clients
get catch-up history for free — Metatron's charge bank
(`MetatronCharges`, genesis 1, deliberately not `omitempty` so a
spent-to-zero bank round-trips as 0; [[metatron]], TASK-12) — and the village's
law ([[governance]], TASK-13): `MeetingPlace` (set once), the `Meeting`
lifecycle (including the TASK-36 emergent-gathering watch fields
`GatherStart/GatherX/GatherY`), the `MeetingConvention` (TASK-36, nil until a
source establishes it — pre-TASK-36 snapshots load nil, a village with no
standing agreement to meet), and the `Norms` list with monotonic
`NextNormID`/`NextProposalID`, all zero-valued in pre-TASK-13 snapshots (a
lawless village) (executor types in `agents.go`; memories belong to
[[agent-mind]]). Its
`Apply(event)` method is the **only** event-driven mutation path — the live loop and
crash recovery run the exact same code, which is what makes replay provably equal to
live execution. Spec 012 bumped the save format to v2, and spec 013 (inventory &
storage — bulk cap, piles, chests, theft, rot) bumped it again to **v3**
([[world-save-directory]]); a v1 world's `Inventory` (just `wood`/`food`) cannot
decode under this build at all — [[world-migration]] is the bridge, chaining 1→2→3
in one run and landing as a single wholesale-replace event rather than incremental
`Apply` calls (below).

## How it works

`NewState(seed, m)` is genesis: tick 0 (day 1 06:00), `DefaultSpeed` (4x), eight
named agents on distinct passable tiles via `genesisPlacement` ([[deterministic-rng]]),
with deliberately imperfect needs — day 1 must demand foraging, wood, and a fire
before dark. `genesisPlacement` (spec 012 US6) is factored out so [[world-migration]]
can re-place carried souls on a regenerated v2 map byte-identically to a fresh
genesis of the same seed.

`Apply` switches on event type: `clock.*` maintain pause/speed/degradation;
`sim.night_started`/`sim.day_started` flip `Night` (waking is an explicit
`agent.woke`, never implicit); `sim.forage_regrown` clears a harvest overlay; the
`agent.*` family ([[event-types]]) drives intents (`agent.intent_set` carries a
storage goal's `Kind`/`Qty` onto the `Intent`, spec 013 R4, and also stamps
`Agent.LastGoal`/`LastGoalTick` — spec 015 R1, `omitempty`, written here and
never cleared by any event, so the [[tui-client]] villagers tab can show an
idle villager's most recent objective from any snapshot), movement, work
products (inventory + overlays + structures), eating (`agent.ate`'s `AtePayload`
sets the absolute post-eat food need and decrements each carried food form by its
consumed count — no reducer-side arithmetic), sleep, talk, needs (absolute
values), and death; the v2 resource/crafting events (`agent.quarried`/
`collected_water`/`crafted`/`cooked`/`bathed`/`refueled`/`spear_broke`,
`sim.fire_burned_out`) apply inventory deltas and structure/overlay changes,
several by re-deriving the recipe from `recipes.go` (the single source for
craft/build magnitudes — a duplicated number here would drift from the contract
table), and — since spec 013 — clamp their gather yields to the taker's free
bulk (`bulkCap − bulk(Inv)`); the v3 storage events (spec 013 US2/US3/US5)
move goods between an agent's `Inv` and a `Pile`/chest `Store`:
`agent.dropped`/`agent.picked_up` create-or-merge or drain a tile's pile
(food oldest-batch-first, spears most-worn-first), `agent.deposited`/
`agent.withdrew` do the same against a chest's `Store`, and `sim.food_rotted`
drains a pile's spoiled food batches (`SpoilAt ≤ tick`) — every one of these
defensively re-clamps to what's actually carried/held/available, so the reducer
stays total even against a contested or forged event, and an emptied pile is
removed in the same application; `social.chest_taken` is an effect-free record
(its consequences — the reason-`"theft"` `social.relation_changed` and the
owner/witness `agent.memory_added` events — ride the same companion batch,
[[social-fabric]]); the `gru.*` family dispatches to
`applyGru` in `gru.go` ([[gru]]);
the `meeting.*`/`norm.*` families — plus `meeting.convention_established` and
the `sim.gathering_observed` watch event (TASK-36) — dispatch to
`applyGovernance` in `governance.go` ([[governance]]).
`world.migrated` (spec 012 US6) is the one case that does not incrementally mutate
fields: after checking the payload's `State.Seed` matches (a mismatched payload
no-ops, keeping `Apply` total), it replaces `*s` wholesale with the embedded state —
[[world-migration]] is the only producer.
`agent.memory_added` additionally bumps `Agent.Generation` when the memory's
salience is at or above `GenerationBumpSalience` (9) — in-flight thoughts
snapshotted under the old generation are superseded at landing ([[cognition]],
[[sim-loop]]). The plan family maintains `Agent.Plan`: `agent.plan_set`
replaces the steps, `agent.plan_step_started` pops the head, and
`agent.plan_expired` clears the whole remaining plan (a broken sequence is
not resumed). The hail family (TASK-47) maintains `Agent.Hail *AgentHail`
(`{By, Until}`, `omitempty` so pre-TASK-47 snapshots and un-hailed agents stay
byte-stable): `social.hailed` sets it, `social.hail_met`/`social.hail_expired`
clear it, and `agent.died`/`agent.slept` also clear it (the dead and the
sleeping shed hails). `agent.died` also spills the dying agent's entire carried
`Inv` onto a pile at its own tile (create-or-merge, food batches stamped
`tick + rotWindowTicks`), emptying `Inv` — reducer-internal, no new event (spec
013 US2, FR-006, research R7's debt-opening precedent). The cognition telemetry types — `cog.thought`, `cog.outcome`,
`cog.recalibration_recommended`, `agent.intent_rejected` — are explicit
reducer no-ops: recorded observability with zero state effect.
Unknown types — including `daemon.*` and `world.created` — are recorded
history but state no-ops, so new event types never break old replay.

**Tick is deliberately not event-sourced**: quiet ticks (no events) advance the clock
without a log row. The live loop mutates `state.Tick` directly; recovery sets it to
`max(snapshot tick, last event tick)` and re-lives any quiet tail deterministically.

Canonical bytes: `Marshal()` uses `encoding/json` over structs only (fixed field
order — payload shapes like `AgentMovedPayload` are structs, never maps), so equal
states produce identical bytes. `Hash()` is SHA-256 of that, used by [[snapshots]]
verification and the determinism tests. Wall-clock time never appears in state.

## Connections

[[sim-loop]] generates events via the [[executor]] and applies them here;
[[daemon-lifecycle]] replays the [[event-log]] through `Apply` at startup;
[[event-types]] lists every payload struct (the cognition-horizon payloads
live in sibling files `cognition.go`, `guard.go`, and `plan.go`); [[world-migration]]
is the sole producer of `world.migrated`.

## Operational notes

`EffectiveRate`/`Degraded` are part of state (hence snapshots) but only change via
explicitly emitted transition events, so unloaded same-machine runs stay
byte-deterministic. Adding a state field means adding events that set it — direct
mutation outside `Apply` (except `Tick`) breaks the replay contract.
