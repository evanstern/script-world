# Contract: Internal APIs

## `internal/llm` — pending-thought inventory

```go
type PendingThought struct {
    Kind         string
    Provider     string
    PredictedSec float64 // class points × provider's CURRENT sec/pt estimate, computed at read time
    ElapsedSec   float64 // 0 while queued; wall-clock since dispatch while in flight
}

func (o *Orchestrator) PendingCognition() []PendingThought
```

- Entry lifecycle: added when `Submit` accepts a job (post-admission), dispatch time stamped at worker dequeue,
  removed on EVERY terminal path (reply, provider error, worker-cap kill, caller-abandoned stale skip). A leaked
  entry is a bug: the registry must drain to empty when the world quiesces.
- Thread-safe under `-race` with concurrent Submit/complete; snapshot is a copy, never a live view.
- Every model-bound kind participates; no kind filtering.

## `internal/cognition` — debt arithmetic + governor controller (pure)

```go
// Debt: Σ over pending of max(0, predictedSec−elapsedSec) × ticksPerSecond / budgetTicks(class).
// Pure; no wall-clock reads; unknown kinds are skipped (they cannot reach a model per FR-002/spec 007).
func Debt(pending []PendingDebtInput, ticksPerSecond float64) (debt float64, jobs int)

// Governor: pure hysteresis state machine over the capped ladder (research R6 constants).
// Sample is called once per GovernorCadence by the owner; paused samples reset windows and return no decision.
func (g *Governor) Sample(debt float64, jobs int, paused bool, effective, requested clock.Speed) Decision
// Decision: {Action: none|shed|recover, To: clock.Speed, Debt: float64, Jobs: int}
```

- Constants `GovernorCadence`, `ShedThreshold`, `BreachWindow`, `RecoverHeadroom`, `RecoveryWindow` are exported
  doctrine constants — changing one is a reviewed code change (FR-007).
- Recovery projection: `debt × (candidateTPS/currentTPS) < ShedThreshold × RecoverHeadroom` (FR-006).
- `internal/cognition` stays a leaf: it may import `internal/clock` (pure math peer) but nothing from
  `sim`/`llm`/`mind`.

## `internal/sim` — the `govern` command

```go
// Loop.Govern applies a governor decision at the next tick boundary.
// Emits clock.governor_shed or clock.governor_recovered; a decision that no longer applies
// (speed changed since sampling, world paused, to == current) emits nothing and returns cleanly.
func (l *Loop) Govern(to clock.Speed, debt float64, jobs int) (Status, error)
```

- Validation at the boundary: `to` must be on the capped ladder and exactly one notch from the CURRENT effective
  speed in the implied direction; stale decisions (player changed speed in between, or paused) are dropped
  silently — the controller re-samples next cadence. Direction infers the event type.
- Requested speed for the payload comes from state (`RequestedSpeed`, defaulting to current `Speed` when
  ungoverned and shedding for the first time).

## `internal/daemon` — wiring

- Governor goroutine constructed ONLY when the orchestrator exists (no-LLM worlds: zero machinery, FR-003).
- Loop each `GovernorCadence`: read loop status → `PendingCognition()` → `cognition.Debt` → `Governor.Sample` →
  `Loop.Govern` when a decision fires. Exposes `GovernorSnapshot{Debt, Jobs}` for `ipc-server` status folding.
- Shutdown: goroutine exits with the daemon ctx; no persistence (window accumulators are wall-side observer
  state, never saved — data-model).
