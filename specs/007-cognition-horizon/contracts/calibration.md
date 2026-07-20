# Contract: Calibration Profile & Estimator

## `calibration.json` (save dir root, `world.CalibrationPath()`)

Written **only** by `scriptworld calibrate`. Read once at daemon start. Absent file
is legal (bootstrap defaults apply).

```json
{
  "calibrated_at": "2026-07-20T21:40:00Z",
  "tiers": {
    "local": {
      "model": "gemma4:12b-mlx",
      "endpoint": "http://localhost:11434/v1",
      "seconds_per_point": 17.2,
      "samples": [
        { "shape": "musing-1pt", "points": 1, "wall_ms": [16100, 17800, 17200, 16900, 18000] },
        { "shape": "planner-3pt", "points": 3, "wall_ms": [50300, 52100, 51000, 49800, 53400] }
      ]
    },
    "cloud": {
      "model": "claude-haiku-4-5-20251001",
      "seconds_per_point": 2.1,
      "samples": [ ]
    }
  }
}
```

- `seconds_per_point` = median of per-point-normalized samples across shapes.
- `samples` are the audit trail; the daemon uses only `seconds_per_point`.
- Unknown fields are ignored on read (forward compatibility); a malformed file is a
  startup warning + bootstrap defaults, never a crash.

## Bootstrap defaults (no profile)

`local: 20.0 s/pt`, `cloud: 10.0 s/pt` — deliberately pessimistic: under bootstrap,
high speeds suppress more classes (fail toward reflex, never toward stale action).

## Live estimator (in-memory, per tier)

- Sample source: the orchestrator worker's measured call duration ÷ the job's
  points, fed on every completed call (successes and provider-side failures that
  returned; caller-abandoned jobs don't sample).
- Update: EWMA, α = 0.2, seeded from the profile (or bootstrap).
- Spike rule: sample > 3× current estimate → excluded from EWMA, `spikeCount++`,
  enters the 20-sample rolling window as a spike.
- Drift signal: spike rate > 30% over the window → emit
  `cog.recalibration_recommended` once per breach episode.
- Never persisted: restarts re-seed from the profile. The recorded baseline moves
  only when a human re-runs `calibrate` (auditability; decision-4's no-self-tuning).

## Prediction consumers

- Router: `points × estimate × Speed.TicksPerSecond()` vs `BudgetTicks`.
- Future-dated prompts: same arithmetic → `predicted_land_tick` in the situation
  block and in `cog.thought`.
- Learn-rung classification: `actual_wall_ms > 3× predicted_wall_ms` ⇒
  `prediction-miss`, else `world-change`.
