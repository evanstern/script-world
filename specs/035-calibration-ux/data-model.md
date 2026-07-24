# Data Model: Calibration UX (spec 035)

No persistence changes — `calibration.json` is read exactly as today and never written by this
feature. Two in-memory entities and two additive wire fields.

## Calibration seed state (per provider, in-memory, process-lifetime)

| Field | Type | Meaning |
|-------|------|---------|
| `calibratedAt` | string (RFC3339, may be empty) | The loaded profile's `calibrated_at` when this provider had a usable profile entry at seeding; **empty = bootstrap-seeded (uncalibrated)** |

- Set exactly once, in `Orchestrator.SeedCalibration`, from profile-entry presence — the same
  presence test that picks the seed value. Providers never seeded (no profile load at all) keep
  the zero value: bootstrap.
- Never mutated afterward: spec 031 adoption moves the **live estimate**, not the seed state
  (spec Key Entities; research R2). Restart re-derives it from the profile on disk.
- Validation: none needed — the timestamp string is carried verbatim from the profile, which is
  already validated/tolerated at load (`cognition.LoadProfile`).

## Suppression summary (computed, never stored)

Inputs: cognition class registry (points, budget ticks, degrade), speed ladder, one sec/pt value
per evaluation.

| Evaluation site | Estimate input | Output |
|-----------------|----------------|--------|
| Daemon boot warning (FR-001) | bootstrap seed constants | ladder-wide per-class summary string (existing `horizonSummary` shape) |
| set_speed warning (FR-002) | serving provider's current estimate via `EstimateForKind`, gated on that provider's empty `calibratedAt` | list of class names suppressed at the requested speed |
| Calibrate horizon summary (existing) | freshly measured sec/pt | unchanged shape + adjacent disclosure (FR-005) |

State transitions: none — pure function of (registry, speed, estimate), same determinism contract
as `cognition.Route` (decision-4: no model in the loop, no self-tuning).

## Wire additions (additive, omitempty — FR-008)

| Surface | Field | Type | Present when |
|---------|-------|------|--------------|
| `llm.ProviderStatus` (status reply, TUI feed) | `calibrated_at` | string | provider was profile-seeded; **absent = bootstrap** |
| `ipc.StatusData` (set_speed reply only) | `warning` | string | set_speed landed on a speed where a bootstrap-seeded provider's class is suppressed |

Byte-identity consequences: no-LLM worlds — no providers array, no warning ever → identical
everywhere. Calibrated worlds — `warning` never set, boot line unchanged; status gains only
`calibrated_at`. Uncalibrated worlds — new boot block, `warning` on suppressing set_speed,
providers show no `calibrated_at`.
