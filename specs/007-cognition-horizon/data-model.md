# Data Model: The Cognition Horizon

**Feature**: specs/007-cognition-horizon | **Date**: 2026-07-20

## DecisionClass (static registry entry — `internal/cognition`)

| Field | Type | Notes |
|---|---|---|
| `Class` | string | stable id, e.g. `planner`, `musing`, `conversation`, `meeting`, `consolidation`, `chronicle`, `metatron` |
| `Points` | int | Fibonacci only: 1, 2, 3, 5, 8, 13 — validated at init |
| `BudgetTicks` | int64 | staleness budget in game ticks (1 tick = 1 game second) |
| `Degrade` | enum | `skip`, `reflex`, `template`, `faster-tier` (registry-expressible; not wired v1) |
| `FutureDated` | bool | whether prompts of this class carry the landing estimate |

**Validation**: points ∈ Fibonacci set; budget > 0; every `llm.Kind` maps to exactly
one class (completeness check at daemon start — startup failure names the offender).
Initial values: see [contracts/registry.md](contracts/registry.md).

## CalibrationProfile (`calibration.json` in the save dir)

| Field | Type | Notes |
|---|---|---|
| `calibrated_at` | RFC3339 | metadata only, never enters sim state |
| `tiers.<local\|cloud>.seconds_per_point` | float | baseline from the reference workload |
| `tiers.<tier>.samples` | array | per-shape raw durations (audit trail) |
| `tiers.<tier>.model` | string | identity of what was measured |

**Lifecycle**: written only by `scriptworld calibrate`; read once at daemon start to
seed the live estimator; missing file → pessimistic bootstrap defaults (local 20
s/pt, cloud 10 s/pt).

## Estimator (in-memory, per tier — `internal/cognition`)

| Field | Type | Notes |
|---|---|---|
| `estimate` | float (atomic) | current seconds-per-point; EWMA α = 0.2 |
| `spikeCount`, `sampleCount` | counters | sample > 3× estimate → excluded, counted |
| `window` | ring[20] | spike-rate window; > 30% → recalibration-recommended signal |

**State transitions**: seeded (from profile or bootstrap) → converging → steady;
spike-rate breach emits `cog.recalibration_recommended` (telemetry, once per breach
episode). Process-lifetime only; never written back to disk.

## ThoughtJob (mind-side, per model interaction)

| Field | Type | Notes |
|---|---|---|
| `JobID` | string | `<class>-<agent>-<snapshotTick>` (conversations: `<class>-<convID>`) |
| `Class` | string | registry key |
| `Agent` | int | −1 for non-agent classes (chronicle) |
| `SnapshotTick` | int64 | tick the prompt snapshot was built at |
| `Generation` | int64 | agent's generation at snapshot |
| `TriggerSeq` | int64 | event log seq of the stimulus that armed the trigger (0 = cadence) |
| `PredictedWallMs` | int64 | points × seconds-per-point at enqueue |
| `PredictedLandTick` | int64 | snapshot + predicted wall × speed |

Terminal outcome (exactly one, FR-015): `landed`, `adapted`, `rejected-stale`,
`rejected-guard`, `superseded`, `expired`, `rejected-unavailable` (dead/asleep),
`unusable`, `suppressed`.

## Agent additions (sim `State` — deterministic)

| Field | Type | Notes |
|---|---|---|
| `Generation` | int64 | bumped by reducer on the high-salience set (attacked, witnessed death, emergency on own/adjacent tile) |
| `Plan` | []PlanStep | pending guarded steps (≤3), head evaluated per tick |

## PlanStep / Guard (deterministic, model-selected from vocabulary)

| Field | Type | Notes |
|---|---|---|
| `Goal` | existing goal vocabulary | same terms the planner uses today |
| `When` | Guard? | gate to start: `after_tick`, `at_location`, `target_present` |
| `Until` | int64 | validity deadline (tick); default snapshot + 2 game-hours |

Guard types (v1, closed set): `target_alive`, `target_present` (adapt-rung
re-resolvable), `not_superseded` (generation equality), `after_tick`, `before_tick`.
Guards are predicates over `State` only — no model, no wall clock.

## InjectIntent args (loop command — extended)

Existing: `Agent`, `Goal`, `TargetAgent`, `Reason`. Added: `SnapshotTick`,
`Generation`, `Guards []Guard`, `Class`, `JobID`, `PredictedWallMs` (telemetry
passthrough), `Plan []PlanStep` (mutually exclusive with `Goal`).

## Relationships

```
Registry ──(class)──▶ ThoughtJob ──(inject)──▶ loop enforcement ──▶ events
Estimator ◀─(durations)── llm worker            │
   │                                            ├─ agent.intent_set / agent.plan_set (executed)
   └──(estimate)──▶ Route() verdict             ├─ agent.intent_rejected + cog.outcome (refused)
                                                └─ cog.thought / cog.outcome (telemetry, no-op)
```

Event payload schemas: [contracts/events.md](contracts/events.md).
