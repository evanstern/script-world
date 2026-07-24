# Contract: governor debt arithmetic (spec 033 revision of spec 028 FR-001/FR-002)

The debt a governor sample reads is a pure, deterministic function of the pending
snapshot and the sampled tick rate. No wall-clock reads, no randomness, no inputs
beyond `{Kind, PredictedSec, ElapsedSec}` per thought and `ticksPerSecond`.

## Per-thought contribution

```
seconds(job) = PredictedSec − ElapsedSec    if ElapsedSec < PredictedSec   // remaining work — drains, as spec 028
             = ElapsedSec                    if ElapsedSec ≥ PredictedSec   // accrued drift — grows (spec 033)

fraction(job) = seconds(job) × ticksPerSecond / BudgetTicks(class)
debt          = Σ fraction(job)
jobs          = count of jobs with fraction > 0
```

## Invariants

- **Within prediction, bit-identical to spec 028**: for any pending set where every
  `ElapsedSec < PredictedSec`, debt and jobs equal the previous implementation's
  output exactly (SC-003).
- **Overdue is monotonic**: for a fixed thought sampled repeatedly while pending,
  once `ElapsedSec ≥ PredictedSec` its fraction is non-decreasing in `ElapsedSec`
  and can never return to zero (SC-002).
- **Boundary jump is doctrine**: as `ElapsedSec` crosses `PredictedSec` the
  contribution steps from ~0 (drained remaining) to the full accrued drift — the
  overrun proves the prediction wrong, so the honest floor becomes "already this
  stale".
- **Dead-on-arrival dominance**: a thought whose accrued drift alone exceeds its
  budget contributes fraction > 1.0 by itself and may legitimately hold debt over
  ShedThreshold.
- **Unchanged guards**: unknown kinds skipped; `ticksPerSecond ≤ 0` yields
  debt 0, jobs 0; unit stays budget-fractions (1.0 = one whole staleness budget).
- **Untouched surfaces**: `Governor.Sample` hysteresis, all constants
  (GovernorCadence, ShedThreshold, BreachWindow, RecoverHeadroom, RecoveryWindow),
  `Decision`, `clock.governor_shed`/`clock.governor_recovered` event shapes, the
  daemon sampler, and the pending registry.

## Worked example (world-01 regression shape)

8 planner thoughts (budget 1200 ticks), each `PredictedSec 1.573`,
`ElapsedSec 30`, at 8 ticks/sec:

- spec 028: `max(0, 1.573 − 30) = 0` per thought → debt 0.0, jobs 0 → no shed ever.
- spec 033: `30 × 8 / 1200 = 0.2` per thought → debt 1.6, jobs 8 → over
  ShedThreshold 1.0; a shed fires on the 5th consecutive over-threshold sample.
