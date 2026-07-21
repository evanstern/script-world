# Tasks: Parallel Local Tier

**Input**: Design documents from `/specs/009-parallel-local-tier/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md,
contracts/llm-config.md, quickstart.md

**Tests**: included — the spec's success criteria (SC-003/005) and FR-005/006 are
explicitly discharged by tests (plan R5), and the package has an established test
suite that must stay green.

**Organization**: grouped by user story; US1 (concurrent workers) is the MVP and
US2/US3 are thin, independently testable increments on top of it.

## Format: `[ID] [P?] [Story] Description`

## Path Conventions

Single Go module at repo root; all implementation inside `internal/llm/` plus one
boot-surface touch in `internal/daemon/daemon.go`. Work happens in worktree
`.worktrees/task-45` on branch `task-45-parallel-local-tier`.

---

## Phase 1: Setup

**Purpose**: confirm a green baseline so every later diff is attributable.

- [ ] T001 Baseline: `go build ./... && go test ./internal/llm/ ./internal/cognition/ -race -count=1` green in the worktree before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the config knob + normalization helper every story reads.

- [ ] T002 Add `Parallel int` (`json:"parallel,omitempty"`) to `LocalConfig` and implement `Workers() (n int, warn string)` normalization (absent/0→1 silent; 1–16 verbatim; <0→1 warn; >16→16 warn) in internal/llm/config.go per data-model.md; `DefaultConfig`/`WriteDefault` omit the field
- [ ] T003 Unit tests for `Workers()` normalization table (0, absent, 1, 4, 16, -2, 64) and llm.json round-trip with `parallel` present/absent in internal/llm/llm_test.go — confirm `LoadConfig` never errors on any integer value (FR-007)

**Checkpoint**: config surface complete; `go test ./internal/llm/` green.

---

## Phase 3: User Story 1 — Timely village cognition at speed (Priority: P1) 🎯 MVP

**Goal**: N concurrent local-tier workers so thoughts stop serializing behind one
slot; queue order and cloud tier untouched.

**Independent Test**: with `parallel: 4`, four submitted calls are in flight
simultaneously (overlapping wall clock against a parking test server); with the
field absent, behavior is byte-identical to today (existing suite green).

### Implementation for User Story 1

- [ ] T004 [US1] Extend `tier` with `slots int` and `inflight atomic.Int32`; `New` sets local `slots` from `cfg.Local.Workers()` and cloud `slots` to 1 (FR-008), then spawns `slots` copies of `worker(t)` per tier; worker increments `inflight` at dequeue and decrements on every reply path (stale-skip, error, success) in internal/llm/llm.go per data-model.md state transitions
- [ ] T005 [US1] Concurrency tests in internal/llm/llm_test.go: (a) `parallel: 4` + parking httptest server → 4 calls verifiably in flight at once, all complete when released (SC-004 shape); (b) >N submissions → overflow queues, none lost, conversation still jumps via prio lane (FR-002); (c) `parallel` absent → exactly 1 in flight even with a full queue (SC-003)
- [ ] T006 [US1] Boot surface in internal/daemon/daemon.go: compute `cfg.Local.Workers()`, print the clamp warning when non-empty, and include effective `parallel N` in the existing `daemon: llm orchestrator on (...)` line when N > 1, per contracts/llm-config.md

**Checkpoint**: US1 fully functional — concurrent local calls, unchanged defaults.

---

## Phase 4: User Story 2 — Best-effort thoughts stop losing every race (Priority: P2)

**Goal**: musings refused only when no slot is free, not whenever anything is
happening.

**Independent Test**: at `parallel: 4` with one call parked in flight and queues
empty, a best-effort musing is admitted; with all 4 slots parked, it gets
`ErrTierBusy`.

### Implementation for User Story 2

- [ ] T007 [US2] Rewrite `Submit`'s best-effort refusal to slot-aware form — refuse iff `len(t.queue) > 0 || len(t.prio) > 0 || t.inflight.Load() >= int32(t.slots)` — in internal/llm/llm.go (FR-003, research R3)
- [ ] T008 [US2] Best-effort slot tests in internal/llm/llm_test.go: (a) `parallel: 4`, one parked call, empty queues → musing admitted and served; (b) all 4 slots parked → `ErrTierBusy` immediately; (c) existing `TestMusingBestEffort` (queued-work-refuses, quiet-serves) still green unchanged (SC-002, SC-003)

**Checkpoint**: US1 + US2 independently green.

---

## Phase 5: User Story 3 — Honest speed governance under concurrency (Priority: P3)

**Goal**: prove estimator, breaker, and meter stay exact under parallel completions
— by test, not new code (plan R4/R5).

### Implementation for User Story 3

- [ ] T009 [US3] Race-proof accounting tests in internal/llm/llm_test.go, run under `-race`: (a) N concurrent successful local calls → estimator `Stats()` sample count equals call count and estimate moved (FR-004); (b) concurrent failures/successes → breaker opens only on `failuresToOpen` consecutive failures, one failing call never corrupts others' replies (FR-005); (c) concurrent completions on a 1-slot cloud tier plus direct concurrent `Meter.Add` exercise → final `Snapshot()` spend equals the exact sum of costs (FR-006, SC-005)
- [ ] T010 [US3] Full verification sweep: `go test ./... -race -count=1` green from the worktree root; `go vet ./...` clean (SC-003 whole-suite gate)

**Checkpoint**: all three stories proven; package race-clean.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T011 [P] Reconcile package doc comment in internal/llm/llm.go (header still says "bounded queues" — mention N-slot local concurrency) and `TierStatus`/status semantics if any comment now lies; no protocol shape changes
- [ ] T012 Live smoke per specs/009-parallel-local-tier/quickstart.md §2–3: create a scratch world, copy `~/.scratch/calibration.json` and `~/worlds/village03/llm.json` in, set `"parallel": 4`, boot the daemon, capture the boot line (effective parallel + calibration seed); then set `"parallel": 64` and `-2`, confirm clamp warnings and clean boots (FR-007); record transcript evidence in the implementer report
- [ ] T013 Mark spec.md Status: Implemented and note the delegated cap decision (16) in specs/009-parallel-local-tier/spec.md Assumptions once T001–T012 pass

> Post-merge (orchestrator, not implementer): `/grounding-wiki:wiki-update` — the
> change touches sources pinned by docs/wiki/llm-orchestrator.md and
> docs/wiki/cognition.md (constitution Principle IV).

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2**: T001 before everything; T002 before T003.
- **Phase 2 blocks all stories** (every story reads `Workers()`/slots).
- **US1 (T004–T006)**: T004 before T005; T006 only needs T002+T004.
- **US2 (T007–T008)**: needs T004's `inflight`/`slots`; independent of T005/T006.
- **US3 (T009–T010)**: needs T004; T009 before T010. Independent of US2.
- **Polish**: T011 anytime after T004; T012 after T010; T013 last.

### Parallel Opportunities

Most of this feature is sequential edits to two files (llm.go, llm_test.go) — the
real parallelism is small: T006 (daemon.go) alongside T005; T011 alongside T009.
A single implementer executing in order T001→T013 is the expected mode.

---

## Implementation Strategy

MVP is Phase 1–3 (US1): the concurrency unlock itself. US2 is a one-condition
change plus tests; US3 is tests-only. All land as commits on the single
`task-45-parallel-local-tier` branch and merge in TASK-45's one PR — phases are
internal breakdown, never PR boundaries.
