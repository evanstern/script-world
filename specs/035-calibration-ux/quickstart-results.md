# Quickstart results: Calibration UX (spec 035)

Run: 2026-07-24, branch `task-40-calibration-ux` (worktree `.worktrees/task-40`, base fe39d92),
executed by the spec-implementer agent (Sonnet tier) headlessly against a scratch world
(`/tmp/promptworld-035`, binary built from the branch); test-suite evidence re-verified by the
orchestrator in the worktree.

## 1. Uncalibrated boot warns (US2) — PASS

Boot on a world with llm.json and no calibration.json printed the full warning block:

```
daemon: WARNING — world is UNCALIBRATED: latency estimates are pessimistic bootstrap defaults ...
daemon: at these estimates: planner suppressed above 16x; conversation suppressed above 16x; meeting OK at 32x
daemon: run `promptworld calibrate demo-ux` ...
```

All three contract elements (statement / horizon summary / remedy with real world name) present
(contracts/warnings.md §1). A world with a hand-written calibration.json booted with the
pre-existing `daemon: calibration seeded (...)` line only — byte-identical, no warning (SC-002).
Unreadable-profile branch covered by `TestUncalibratedBootWarningContainsContractElements`
(internal/daemon/daemon_test.go).

## 2. Speed into suppression warns, still applies (US1) — PASS

- `promptworld speed demo-ux 32x` → `speed 32x` applied AND
  `WARNING: uncalibrated world at 32x: planner, conversation suppressed at current estimates — run \`promptworld calibrate demo-ux\``
- `promptworld speed demo-ux 4x` → no warning.
- Calibrated world at any speed → no warning; no-LLM world → no warning; `max` on an LLM world →
  existing error, unchanged and warning-free.

Wire-level scenarios locked in internal/ipc/ipc_test.go: `TestSetSpeedWarnsUncalibratedSuppressing`,
`TestSetSpeedWarningAbsentWhenCalibrated`, `TestSetSpeedWarningAbsentNoLLM`,
`TestSetSpeedMaxGateStillPrecedesWarning`, `TestStatusPauseResumeNeverCarryWarning`,
`TestStatusDataWarningOmitempty`.

## 3. Status shows calibration state persistently (US3) — PASS

`promptworld status demo-ux` (uncalibrated world):

```
llm cloud (claude-opus-4-8): uncalibrated (bootstrap)
llm local (gemma4:12b-mlx): uncalibrated (bootstrap)
```

Calibrated/partial-profile truth per provider locked in internal/llm
(`TestStatusSnapshotCarriesCalibratedAt`, `TestProviderStatusCalibratedAtOmitempty`,
internal/llm/calibration_test.go full/partial/nil-profile cases). Note: the human status
rendering had no LLM section before this feature — the per-provider calibration line is the
minimal new rendering scoped to FR-004; wire field is `calibrated_at,omitempty`.

## 4. No behavior change (FR-007 / SC-003) — PASS

Full `go test ./...` green across all 19 packages (uncached re-run by the orchestrator in the
worktree). No routing/estimator/governor test modified; `go vet ./...` clean; `gofmt -l` empty on
all touched files (only the 5 pre-existing TASK-83 drift files repo-wide, none touched).
Agreement property locked in internal/cognition/horizon_test.go:
`TestHorizonSummaryAgreesWithRoute`, `TestSuppressedAtAgreesWithRoute` (summary/helper says
suppressed ⇔ `Route` disallows, full ladder × class matrix).

## 5. Calibrate discloses the sequential floor (US4) — PASS (output shape)

Live calibrate ran against dead endpoints in the sandbox (no local Ollama / cloud creds), which
still exercised the disclosure path: printed exactly once per run. Exactly-once + both-paths
(legacy and v2, including a 3-zero-priced-provider run and a cloud-only run with no horizon
line) locked in cmd/promptworld/calibrate_test.go. A successful end-to-end calibrate against a
live endpoint remains an operator step (any future `promptworld calibrate` run on a real rig).

## Sign-off checklist

- [x] Boot warning: absent-profile AND unreadable-profile worlds warn; calibrated world byte-identical
- [x] set_speed warning: fires only on (uncalibrated ∧ suppressing speed); speed always applies
- [x] Status: `calibrated_at` per provider truthful, incl. partial profiles
- [x] Calibrate: disclosure present in legacy and v2 output paths
- [x] `go test ./...` green; gofmt clean on touched files (TASK-83 pre-existing drift excluded)
