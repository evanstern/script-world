# Data Model: Adaptive Time Throttling

Entities from the spec, mapped to their concrete homes. All shapes additive; no format bump (research R3).

## World state (`internal/sim/state.go`)

| Field | Type | Semantics |
|---|---|---|
| `Speed` | `clock.Speed` | UNCHANGED meaning: the speed the loop paces at — now the **effective** speed. Read by the router and auto-slow (they need no changes). |
| `RequestedSpeed` | `clock.Speed`, `json:"requested_speed,omitempty"` | The player's ceiling, present **only while governed** (`!= Speed`). Empty = ungoverned. Pre-028 snapshots (field absent) are therefore valid ungoverned states. |

Invariants: `RequestedSpeed` is never `max` and never equal to `Speed` when set; `Speed ≤ RequestedSpeed` on the
ladder whenever `RequestedSpeed` is set; both always parse via `clock.ParseSpeed`.

## Governor events (`internal/sim/state.go` payloads, reducer-applied)

`GovernorPayload` (shared by both types):

```json
{ "requested": "32x", "from": "32x", "to": "16x", "debt": 1.4, "jobs": 3 }
```

| Event | Reducer effect |
|---|---|
| `clock.governor_shed` | `Speed = to`; `RequestedSpeed = requested` (to ≠ requested by construction); `EffectiveRate = to.TicksPerSecond()` unless `Degraded` |
| `clock.governor_recovered` | `Speed = to`; `RequestedSpeed = requested` if `to != requested`, else cleared; `EffectiveRate` same rule |
| `clock.speed_set` (existing) | gains one line: clears `RequestedSpeed` — a player command always collapses governed state (FR-009) |

`debt` is the measured budget-fraction sum at decision time; `jobs` the contributing-thought count (FR-008,
SC-005).

## Pending thought (`internal/llm`)

```go
type PendingThought struct {
    Kind         string  // llm call kind, maps to a cognition class
    Provider     string  // serving (in-flight) or queued-at provider
    PredictedSec float64 // class points × provider's current sec/pt estimate
    ElapsedSec   float64 // 0 while queued; wall since dispatch while in flight
}
```

Produced by `Orchestrator.PendingCognition()` from a mutex-guarded job registry: entry added when `Submit` accepts
(after admission), dispatch time stamped at worker dequeue, entry removed on every reply path (success, error,
caller-abandoned skip). Registry size is bounded by queue caps × providers (32 per lane + slots), so the snapshot
is O(tens).

## Debt (derived, never stored — `internal/cognition`)

```
fraction(job) = max(0, PredictedSec − ElapsedSec) × ticksPerSecond / BudgetTicks(class)
debt          = Σ fraction(job)   // dimensionless budget-fractions
```

Pure function of the pending set, the registry budgets, and the effective tick rate (FR-002). Zero when nothing is
pending (US1-AC2).

## Governor controller (`internal/cognition/governor.go`)

Pure state machine; the daemon owns the goroutine and the wall clock, the controller owns the decision logic.

State: current breach-window accumulation, recovery-window accumulation (both in sampler ticks), last decision.
Inputs per sample: `debt`, `paused`, `effectiveTPS`, `requestedTPS`, candidate-notch TPS values (the capped
ladder). Output: `Decision ∈ {none, shed(to), recover(to)}` plus the measured debt and jobs for the event payload.

Constants (doctrine, R6): `GovernorCadence` 1 s · `ShedThreshold` 1.0 · `BreachWindow` 5 s · `RecoverHeadroom`
0.5 · `RecoveryWindow` 20 s.

Transitions:

- breach accrues while `debt > ShedThreshold` ∧ effective above 1x; continuous `BreachWindow` → shed one notch.
- recovery accrues while governed ∧ `debt × (nextTPS/currentTPS) < ShedThreshold × RecoverHeadroom`; continuous
  `RecoveryWindow` → recover one notch.
- any decision, player speed change, pause, or governor start resets both windows; paused samples are no-ops.
- at the 1x floor with debt over threshold: no decision; saturation is visible via status (debt > threshold while
  floor-governed).

## Protocol status (`internal/ipc/protocol.go`)

Additive fields on `Status`: `RequestedSpeed string` (from state; empty = ungoverned), `GovernorDebt float64`,
`GovernorJobs int` (from the daemon governor's snapshot, folded in by `ipc-server` like the LLM status). No-LLM
worlds report zero values with `RequestedSpeed` always empty.
