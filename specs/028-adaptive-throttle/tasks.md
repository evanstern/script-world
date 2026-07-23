# Tasks: Adaptive Time Throttling

**Input**: Design documents from `specs/028-adaptive-throttle/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: included — the spec's SCs demand replay byte-identity, race safety, and hysteresis proofs; tests land
alongside code per project practice (constitution V: tests alongside code).

**Organization**: grouped by user story; each phase is an independently testable increment. All implementation
executes via the `spec-implementer` agent per constitution Principle V — tier ruling in plan.md (governor/
concurrency/sim slices → Opus 4.8; TUI/status/doc slices → Sonnet).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Verify worktree `.worktrees/task-33` (branch `task-33-adaptive-throttle`) is rebased on current
      `origin/main` and baseline `go test ./...` is green before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the pure debt arithmetic and the pending-thought inventory — every story reads these.

- [x] T002 [P] Add `Debt()` helper, `PendingDebtInput`, and the five doctrine constants (`GovernorCadence`,
      `ShedThreshold`, `BreachWindow`, `RecoverHeadroom`, `RecoveryWindow`) in
      `internal/cognition/governor.go` per contracts/internal-api.md; table-driven unit tests (zero-pending,
      overdue-floored-at-zero, mixed classes, unknown-kind skip) in `internal/cognition/governor_test.go`
- [x] T003 [P] Add the job registry (add on Submit-accept, stamp at dequeue, remove on every terminal path) and
      `Orchestrator.PendingCognition()` in `internal/llm/llm.go`; lifecycle + drain-to-empty + snapshot-copy
      tests under `-race` in `internal/llm/pending_test.go`

**Checkpoint**: `go test ./internal/cognition/ ./internal/llm/` green including `-race`.

---

## Phase 3: User Story 1 — Debt is measured and visible (Priority: P1) 🎯 MVP

**Goal**: the world continuously derives and exposes debt + contributing-job count; zero behavior change.

**Independent Test**: quickstart §2 — debt rises/drains in `promptworld status` with a slow model; exactly zero
when quiescent; fields absent/zero with no `llm.json`.

- [x] T004 [US1] Add the daemon governor sampler: goroutine constructed only when the orchestrator exists,
      polling `PendingCognition()` + loop status every `GovernorCadence`, computing `cognition.Debt`, exposing
      `GovernorSnapshot{Debt, Jobs}` (no decisions yet) in the daemon wiring (`internal/daemon/`)
- [x] T005 [US1] Add `RequestedSpeed`, `GovernorDebt`, `GovernorJobs` to `Status` in `internal/ipc/protocol.go`
      and fold the daemon snapshot in `internal/ipc/server.go` per contracts/status-protocol.md
- [x] T006 [US1] Tests: debt-visible integration (fake orchestrator jobs → status fields), quiesce-to-zero, and
      no-LLM inertness (zero machinery, zero values) in `internal/daemon/` and `internal/ipc/` test files

**Checkpoint**: US1 shippable — observability only, simulation behavior untouched (SC-004 provable).

---

## Phase 4: User Story 2 — The world sheds speed under debt (Priority: P2)

**Goal**: sustained breach sheds one ladder notch at a time, recorded, with the router widening at the governed
speed.

**Independent Test**: quickstart §3 first half — scripted burst at 32x produces `clock.governor_shed` events with
full arithmetic payloads; router verdicts at 16x admit what 32x refused.

- [x] T007 [US2] Add `RequestedSpeed` state field (`omitempty`), `GovernorPayload`, reducer arms for BOTH
      `clock.governor_shed` and `clock.governor_recovered`, and the `clock.speed_set` governed-state-clearing
      amendment in `internal/sim/state.go` per contracts/events.md; reducer + snapshot-byte-compatibility tests
      in `internal/sim/state_test.go`
- [x] T008 [US2] Add `Loop.Govern(to, debt, jobs)` — new `govern` command with boundary validation (one-notch,
      capped ladder, stale-decision drop, paused drop, direction→event-type) in `internal/sim/loop.go`; command
      semantics tests in `internal/sim/loop_test.go`
- [x] T009 [US2] Implement the `Governor` state machine shed path (breach-window accrual, resets on decision/
      player-change/pause/start) in `internal/cognition/governor.go`; table-driven tests: shed at sustained
      breach, multi-notch descent, 1x-floor saturation-no-decision, blip-no-shed in
      `internal/cognition/governor_test.go`
- [x] T010 [US2] Wire sampler decisions to `Loop.Govern` in the daemon governor (`internal/daemon/`); integration
      test with scripted debt driving a real loop shed
- [x] T011 [US2] Replay + composition proofs: log containing governor_shed events replays byte-identical
      (SC-001); mind-replica applies governor events so `routeVerdict` at the governed speed admits a class the
      requested speed refused (FR-010) — in `internal/sim/` and `internal/mind/` test files

**Checkpoint**: crisis at 32x sheds to sane speeds automatically; every shed auditable from the log.

---

## Phase 5: User Story 3 — Speed recovers without oscillating (Priority: P3)

**Goal**: notch-by-notch recovery gated on projected debt at the candidate notch; asymmetric windows; no flap.

**Independent Test**: quickstart §3 second half — recovery events climb back after the burst; marginal steady
load parks at a stable notch.

- [x] T012 [US3] Implement the recovery path (projection `debt × candidateTPS/currentTPS` vs
      `ShedThreshold × RecoverHeadroom`, `RecoveryWindow` accrual, never-above-requested, clear-governed-at-
      requested) in `internal/cognition/governor.go`; tests: notch-by-notch climb, marginal-load parking
      (no oscillation, SC-003), window asymmetry in `internal/cognition/governor_test.go`
- [x] T013 [US3] End-to-end recovery test through loop + reducer: `clock.governor_recovered` sequence restores
      requested speed and clears `RequestedSpeed`; replay byte-identity for shed→recover→shed logs in
      `internal/sim/` test files

**Checkpoint**: governor is a closed loop — sheds under load, climbs back, never flaps.

---

## Phase 6: User Story 4 — The player sees it and stays in charge (Priority: P4)

**Goal**: governed state is legible in the TUI, player commands always win instantly, pause suspends governing.

**Independent Test**: quickstart §4 — governed header text; speed commands below/above governed notch; pause/
resume window resets; max-speed refusal regression.

- [x] T014 [P] [US4] Governed header segment (`asked 32x — 3 minds in flight, debt 140%`) in
      `internal/tui/views.go` and digest lines for both governor event types in `internal/tui/digest.go`;
      render tests in `internal/tui/` test files
- [x] T015 [US4] Player-interaction + pause proofs: `set_speed` below governed notch runs immediately and clears
      governed state; raise-ceiling re-sheds within one cadence; sampler no-ops and resets windows while paused
      (FR-013); `max` refusal with LLM unchanged (FR-012 regression) — across `internal/sim/`,
      `internal/daemon/`, `internal/ipc/` test files

**Checkpoint**: all four stories independently proven; feature complete pending polish.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T016 Run quickstart.md end-to-end in a scratch `PROMPTWORLD_HOME` with a real local model, including the
      SC-002 governor-on/off stale-discard comparison; record results in
      `specs/028-adaptive-throttle/quickstart-results.md` (live-observation precedent: 012/T045)
- [ ] T017 [P] Full-suite gate `go test ./...` green + `go vet ./...`; confirm no format bump needed
      (snapshot-byte test from T007 passes on pre-028 fixtures)
- [ ] T018 Post-merge re-grounding: `/grounding-wiki:wiki-update` for touched notes (sim-loop, cognition,
      llm-orchestrator, event-types, ipc-protocol, ipc-server, tui-client, game-clock connections) + player-docs
      refresh (`node .claude/skills/player-docs/scripts/check-freshness.mjs --check`); then `spec-bridge:sync`
      and worktree cleanup

---

## Dependencies & Execution Order

- **Setup (P1)** → **Foundational (P2)** blocks everything.
- **US1** needs T002+T003. **US2** needs T002 (+T004's sampler for T010). **US3** needs US2's controller and
  reducer. **US4** needs US2 (something to render/override); T014 can start once US2's events exist.
- Within US2: T007 → T008 → T010/T011; T009 parallel with T007/T008 (different packages).
- Polish last; T018 is post-merge.

### Parallel Opportunities

- T002 ∥ T003 (different packages). T004/T005 ∥ T009 once foundational lands. T014 ∥ T012/T013. T017 ∥ T016.

## Implementation Strategy

MVP = Phase 3 (US1): pure observability, zero behavior change — shippable and independently valuable (it
quantifies the problem on real hardware). Then US2 (the governor's point), US3 (closing the loop), US4
(legibility), polish. One branch, one PR (TASK-33); commit per task or logical group; `go test ./...` green at
every checkpoint.
