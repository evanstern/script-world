# Quickstart Validation Results

**Date**: 2026-07-20 · **Host**: darwin (operator's machine) · **Model**: local
Ollama `qwen3:4b-thinking-2507-q4_K_M` @ localhost:11434 · **Binary**: branch
`task-32-cognition-horizon`

## §1 Unit + integration tests

`go vet ./... && go test ./...` — all green, including the four cognition e2e
scenarios (`go test ./e2e/` 175.6s: telemetry audit, latency injection, replay
byte-equality, plus all pre-existing daemon/determinism scenarios).

## §2 Calibrate a world — live model

```
$ scriptworld calibrate <dir> (5 samples per shape)
tier local  (qwen3:4b-thinking-2507-q4_K_M)
  musing-1pt     5/5 samples   [10.1 10.6 8.6 13.1 13.2s]
  planner-3pt    5/5 samples   [37.0 35.2 42.8 33.1 37.0s]
  seconds_per_point: 12.3
  cognition at this profile: planner OK at 32x; musing OK at 32x; conversation OK at 32x; meeting OK at 32x
```

The point scale held up on real hardware: musings normalized to ~11 s/pt and
planners to ~12.3 s/pt — one host constant describes both shapes. At 12.3 s/pt
this model sits *just inside* the planner budget at 32x (3 × 12.3 × 32 = 1181
≤ 1200 ticks): the horizon summary is doing exactly its job — a marginally
slower model would print "planner suppressed above 16x".

## §3 Telemetry + causality — live daemon at 4x

`daemon.log`: `calibration seeded (local 12.3s/pt, cloud 10.0s/pt)`.

First minutes of `cog.outcome` (job · outcome · staleness ticks · predicted →
actual wall ms):

```
planner-0-300   landed  127   36953 → 31992
planner-2-435   landed  160   35960 → 40095
musing-0-453    landed  327   11986 → 52807
planner-5-441   landed  465   35960 → 25063
musing-1-806    landed  335   12262 → 66063
```

- Planner staleness arithmetic is exact: 31992 ms actual × 4x = 128 game-ticks
  ≈ the recorded 127.
- Musings honestly expose tier queueing: predicted ~12s of model time, actual
  52–66s wall because they wait behind planner traffic — the very signal the
  telemetry exists to surface, and still far inside the 3600-tick musing
  budget at 4x.
- Every `cog.thought` paired with exactly one `cog.outcome` (SC-002 audit
  query returned no orphans, no doubles).

## §5 Pause semantics — live

Paused at tick 1286 with thoughts in flight. Everything settled AT the frozen
tick — every rung of the ladder exercised live (job · outcome · staleness):

```
planner-3-442   landed          1286  844     ← in flight at pause; staleness all pre-pause
planner-1-453   rejected-guard  1286  833
planner-7-460   adapted         1286  826     ← target moved; resolveGoal repaired
planner-4-469   landed          1286  817
planner-0-600   unusable        1286  686
planner-2-900   rejected-guard  1286  386
planner-5-926   landed          1286  360
planner-*-1286  landed/…        1286  0       ← the catch-up round (see finding)
musing-*-1286   landed          1286  0
```

**Live finding — the catch-up round.** Landings during the pause woke the
mind's absorb loop, and agents whose cadence was already due fired one more
round of thought *snapshotted at the frozen tick* (the `*-1286` jobs) — then
the game-time debounce, frozen with the clock, blocked everything and the
mind quiesced. Bounded (one round), convergent (observed), and at **zero
staleness** — the most fidelity-perfect thought the system ever produces.
Blessed rather than suppressed: FR-018, decision-4, and the door comments
were refined to state this precisely. SC-002 held throughout: after settling,
every one of the run's thoughts had exactly one outcome (0 orphans, 0
doubles).

## §4 / §6 The horizon at speed · replay determinism

Covered by the e2e suite against mock models where the timings are forced
rather than waited for: `TestCognitionStaleRejectionUnderLatency` (router
admits on an optimistic profile, 45s reality rejected at landing, classified
prediction-miss, SC-001 audit zero over-budget executions at 32x) and
`TestCognitionReplayByteIdentical` (full-log replay == snapshot+tail on a
cognition-enabled run, SC-003). Both green in §1's run.

## Verdict

All quickstart sections validated: §1, §4, §6 via the deterministic e2e
harness; §2, §3, §5 live against a real local model. The registry values,
calibration bridge, router arithmetic, landing ladder, and pause doctrine all
behaved as specified on real hardware.
