# Implementation Plan: Parallel Local Tier

**Branch**: `task-45-parallel-local-tier` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/009-parallel-local-tier/spec.md`

## Summary

The LLM orchestrator (`internal/llm/llm.go`) runs exactly one worker goroutine per tier,
serializing every local-model call; the local server demonstrably services concurrent
requests (4 × cogito:3b in 0.98s wall vs 3.8s one cold call). This feature adds a
`parallel` knob to `llm.json`'s local tier (default 1, byte-for-byte compatible) that
spawns N worker goroutines against the local tier, converts best-effort admission from
"any work waiting" to "no free slot", and lets the existing per-completion estimator
sampling observe true concurrent-rate latency. Cloud tier stays fixed at one worker.

Technical approach: keep the existing worker loop untouched and spawn it N times per
tier (`tier.slots`), add an atomic in-flight counter for slot-aware best-effort
admission, and clamp invalid `parallel` values to a safe range with a boot-time warning
printed by the daemon. Health breaker, spend meter, and estimator are already
mutex-protected; the plan adds `-race`-verified concurrent tests to prove exactly-once
accounting rather than new synchronization.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: stdlib only for this slice (`sync/atomic`, `context`,
`net/http/httptest` in tests); `internal/cognition` (estimator), `internal/store`
(meter persistence) — both already wired.

**Storage**: `llm.json` in the world save directory (new optional field
`local.parallel`); spend meter persists via the existing world meta table. No schema
migration — absent field means 1.

**Testing**: `go test ./internal/llm/ ./internal/cognition/ -race`; live validation
against the operator's Ollama-compatible server per [quickstart.md](quickstart.md).

**Target Platform**: the `scriptworld` daemon (macOS dev machine; any platform Go
targets).

**Project Type**: single Go module; this slice is almost entirely
`internal/llm` + a boot-warning line in `internal/daemon`.

**Performance Goals**: at `parallel: 4`, four short local calls complete in ≤2× the
wall time of one warm call (SC-004); post-restart planner herds see ≥80% fewer
rejected-stale landings vs serial (SC-001).

**Constraints**: default MUST remain 1 with behavior indistinguishable from today
(SC-003); cloud tier untouched (FR-008); no per-class/per-provider routing (FR-009,
reserved for TASK-35); a world must never fail to boot over this setting (FR-007).

**Scale/Scope**: one config field, one package's concurrency model, ~6 files touched
(`config.go`, `llm.go`, `llm_test.go`, `daemon.go`, wiki re-pin, docs). Concurrency cap
decision delegated to this plan: **16** (see research.md R2).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Check | Verdict |
|---|---|---|
| I. Artifact-Grounded Action | Plan/research/data-model/quickstart live in `specs/009-parallel-local-tier/`; evidence (measured concurrency, queue-wait pathology) is recorded in spec + TASK-45. Tier choice will be recorded on the board task. | PASS |
| II. One Task, One PR | TASK-45 ↔ branch `task-45-parallel-local-tier` in worktree `.worktrees/task-45` ↔ one PR. Root stays on `main`. | PASS |
| III. Gates Over Assertions | `spec-bridge:link` runs before implementation; board status advances only via `spec-bridge:sync` against artifacts. | PASS |
| IV. Grounding Freshness | `docs/wiki/llm-orchestrator.md` and `docs/wiki/cognition.md` list touched files as sources → `/grounding-wiki:wiki-update` is part of the Definition of Done, after merge. | PASS (tracked) |
| V. Model-Tiered Workflow | Planning (this document) on Fable 5. Implementation delegated to `spec-implementer` at **Opus 4.8** — rubric: concurrency/scheduling logic in `internal/llm` is explicitly senior-tier. No inline implementation. | PASS |

**Post-Phase-1 re-check** (design complete): no new violations. The design adds one
config field and reuses the existing worker loop — no new packages, no new abstraction
layers, nothing for Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/009-parallel-local-tier/
├── spec.md              # Feature specification (complete)
├── plan.md              # This file
├── research.md          # Phase 0: decisions R1–R7
├── data-model.md        # Phase 1: config field, tier slots, admission verdict
├── quickstart.md        # Phase 1: build/test/live-validation guide
├── contracts/
│   └── llm-config.md    # llm.json contract: local.parallel semantics
├── checklists/          # spec quality checklist (pre-existing, passing)
└── tasks.md             # Phase 2 (/speckit-tasks — not created by plan)
```

### Source Code (repository root)

```text
internal/llm/
├── config.go        # LocalConfig gains Parallel; Workers() normalization helper
├── llm.go           # tier gains slots + inflight; New spawns N local workers;
│                    #   Submit's best-effort check becomes slot-aware
├── llm_test.go      # concurrency tests: N-in-flight, slot admission, -race
│                    #   exactly-once health/meter/estimator accounting
├── health.go        # unchanged (already mutex-protected; verified under -race)
├── meter.go         # unchanged (already mutex-protected; verified under -race)
└── providers.go     # unchanged

internal/daemon/
└── daemon.go        # boot line prints effective parallel; warning when clamped

internal/cognition/
└── estimate.go      # unchanged — Sample() is mutex-protected; concurrent
                     #   completions feed it true concurrent-rate latency (FR-004)
```

**Structure Decision**: single-package change inside the existing `internal/llm`
orchestrator plus a one-line boot surface in `internal/daemon`. No new packages.

## Design Highlights

The load-bearing decisions, argued in [research.md](research.md):

- **R1 — N workers, not a semaphore**: spawn the existing `worker(t)` loop
  `t.slots` times. The two-level priority select, stale-skip, breaker discipline, and
  estimator sampling are all already per-job-correct; duplication of the loop is the
  minimal-diff, behavior-preserving mechanism.
- **R2 — cap = 16, clamp-with-warning**: `parallel` ∈ [1,16]; 0/absent → 1 silently
  (compat); <0 or >16 → clamped with a daemon-boot warning (FR-007). Rationale:
  `queueCap` is 32; 16 slots already exceeds any measured local server benefit.
- **R3 — slot-aware best-effort admission**: `tier.inflight` (atomic) counts jobs a
  worker has dequeued and not yet answered. Best-effort refuses iff
  `len(queue)>0 || len(prio)>0 || inflight ≥ slots`. At `parallel: 1` this is the
  documented "admitted only when the tier is otherwise quiet" contract; the existing
  test suite pins only the queued-work-refuses and fully-quiet-serves cases, both
  preserved (SC-003).
- **R4 — estimator needs no change**: each worker samples per-call wall time at
  completion into the mutex-protected EWMA; under concurrency those samples ARE the
  concurrent-rate latency (FR-004). Contention shows up honestly as longer per-call
  wall time.
- **R5 — health/meter correctness is proven, not rebuilt**: `tierHealth` and `Meter`
  are already lock-protected; FR-005/FR-006 are discharged by new `-race` tests that
  drive N concurrent successes/failures and assert exact counts, not by new code.
- **R6 — cloud tier pinned at 1 worker** (FR-008): `slots` is 1 for cloud regardless
  of config; the field lives only under `local`.
- **R7 — shutdown discipline unchanged**: `Close()` still closes `o.done`; all N
  workers exit between jobs; in-flight calls remain bounded by `workerCallCap`.

## Complexity Tracking

> No Constitution Check violations — table intentionally empty.
