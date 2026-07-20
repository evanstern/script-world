# Contract: New Event Types

All payloads are canonical JSON (struct-marshaled, fixed field order) per the
event-log contract. Telemetry types are reducer no-ops whitelisted on the
`inject_social` door unless noted. `seq`/`tick`/`wall_time` envelope semantics are
unchanged.

## `cog.thought` — a model call was admitted (telemetry, no-op)

Emitted by the mind when a job passes the router and is enqueued.

```json
{
  "job": "planner-3-123456",
  "class": "planner",
  "agent": 3,
  "snapshot_tick": 123456,
  "generation": 7,
  "trigger_seq": 88231,
  "points": 3,
  "predicted_wall_ms": 51000,
  "predicted_land_tick": 125088
}
```

`trigger_seq` = event-log seq of the stimulus that armed the trigger; `0` for pure
cadence. This is the causality edge stimulus → thought (FR-020).

## `cog.outcome` — a thought terminated (telemetry, no-op)

Exactly one per thought (FR-015), including router suppressions (which have no
`cog.thought`).

```json
{
  "job": "planner-3-123456",
  "class": "planner",
  "agent": 3,
  "outcome": "landed",
  "snapshot_tick": 123456,
  "landing_tick": 125102,
  "staleness_ticks": 1646,
  "predicted_wall_ms": 51000,
  "actual_wall_ms": 51400,
  "kind": "world-change",
  "reason": ""
}
```

- `outcome` ∈ `landed | adapted | rejected-stale | rejected-guard | superseded |
  expired | rejected-unavailable | unusable | suppressed`.
- `kind` (rejections only) ∈ `prediction-miss | world-change` — the learn-rung
  classification (FR-013). Prediction-miss: `actual_wall_ms > 3× predicted`.
- `suppressed` outcomes carry the routing arithmetic in `reason`
  (e.g. `"3pt × 17.2s/pt × 32x = 1651 ticks > budget 1200"`).
- Emitted by the mind for suppressed/unusable/abandoned jobs; emitted **by the loop**
  (same atomic append as the rejection) for landing-time verdicts.

## `agent.intent_rejected` — the loop refused a landing intent (no-op)

Companion to a `cog.outcome` rejection, kept as its own type so souls/chronicle can
later choose to notice refused intentions without parsing telemetry.

```json
{ "agent": 3, "goal": "talk_to", "reason": "stale", "staleness_ticks": 1646 }
```

## `agent.plan_set` — a guarded conditional plan was accepted (reducer: stores plan)

```json
{
  "agent": 3,
  "job": "planner-3-123456",
  "steps": [
    { "goal": "goto_square", "until": 130656 },
    { "goal": "talk_to", "target": 5, "when": { "type": "target_present", "target": 5 }, "until": 130656 }
  ]
}
```

Reducer replaces `Agent.Plan`. ≤3 steps enforced at parse and at the dry-run.

## `agent.plan_step_started` / `agent.plan_expired` (reducer: advances/clears plan)

Emitted by the executor at the tick a head step's `when` guard passes / its `until`
deadline lapses. Payloads: `{agent, job, step}` / `{agent, job, step, reason}`.

## `cog.recalibration_recommended` — estimator drift signal (telemetry, no-op)

```json
{ "tier": "local", "estimate_s_per_pt": 17.2, "spike_rate": 0.35, "window": 20 }
```

Emitted at most once per breach episode (rate returns below threshold → re-armed).

## `inject_intent` door changes (command args, not an event type)

Args gain `snapshot_tick`, `generation`, `class`, `job`, `predicted_wall_ms`,
`guards[]`, and optional `plan[]` (mutually exclusive with `goal`). The handler:

1. dead/asleep → reject (`rejected-unavailable`) — recorded, no longer silent
2. `generation` mismatch → reject (`superseded`)
3. `landing − snapshot > budget(class)` → reject (`rejected-stale`)
4. guards: try adapt rung (re-resolve via `resolveGoal`); irreparable → reject
   (`rejected-guard`)
5. accept → existing `agent.intent_set`/`agent.thought` (or `agent.plan_set`) +
   `cog.outcome{landed|adapted}` — all in one atomic batch

Replay reads these recorded verdicts; it never re-runs the checks against a model.
