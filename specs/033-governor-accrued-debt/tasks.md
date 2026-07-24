# Tasks: Governor Accrued-Drift Debt

**Input**: Design documents from `/specs/033-governor-accrued-debt/`

**Prerequisites**: plan.md, spec.md, research.md (R1 piecewise arm is normative), data-model.md, contracts/debt-formula.md, quickstart.md

**Tests**: INCLUDED — FR-005 mandates the world-01 regression red-first; SC-002/SC-003 demand property tests.

**Organization**: grouped by user story; US1 is the MVP increment.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Create worktree: from repo root run `git fetch origin && git worktree add .worktrees/task-87 -b task-87-governor-accrued-debt origin/main`; all subsequent work happens inside `.worktrees/task-87/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: pin the defect red before touching the arithmetic (FR-005)

- [x] T002 RED-FIRST regression in `internal/cognition/governor_test.go`: encode the world-01 shape from contracts/debt-formula.md — 8 planner-kind inputs, PredictedSec 1.573, ElapsedSec 30, ticksPerSecond 8 → assert debt ≈ 1.6 and jobs 8, then feed 5 consecutive such samples to a Governor at 8x (requested 8x) and assert an ActionShed decision on the 5th. Run it and CONFIRM IT FAILS against current main (debt 0, jobs 0, no shed) before proceeding; commit the failing test with a message noting red-first

**Checkpoint**: the defect is executable

---

## Phase 3: User Story 1 — Overdue thoughts contribute their true, growing drift (Priority: P1) 🎯 MVP

**Goal**: piecewise debt arm per research.md R1 / contracts/debt-formula.md; within prediction drains as today, overdue counts full accrued elapsed.

**Independent Test**: T002 turns green; monotonic and bit-identical properties hold.

- [x] T003 [US1] Implement the piecewise arm in `internal/cognition/governor.go` `Debt`: `seconds = PredictedSec − ElapsedSec` when `ElapsedSec < PredictedSec`, else `seconds = ElapsedSec` (NOT plain max — see research.md R1 for why plain max(Predicted, Elapsed) breaks SC-003). Jobs rule unchanged (positive fraction counts). Rewrite the Debt doc comment to the accrued-drift doctrine ("an overdue thought's elapsed time IS its grounded debt"; the boundary jump is doctrine — contracts/debt-formula.md), citing spec 033. Constants, Sample, Decision untouched. T002 now green
- [x] T004 [P] [US1] Property tests in `internal/cognition/governor_test.go`: (a) monotonic stuck thought — one input sampled at ElapsedSec 2→30→45→120 past PredictedSec 2 yields strictly non-decreasing, never-zero fractions (SC-002, US1-AC1/AC2); (b) within-prediction bit-identical — table of healthy sets (elapsed < predicted, mixed kinds/speeds incl. elapsed 0 queued) asserts Debt output equals the spec 028 arithmetic (inline reference implementation) to full float64 equality (SC-003, US1-AC4); (c) boundary — elapsed == predicted contributes elapsed (the doctrine jump); (d) unchanged guards — unknown kind skipped, tps ≤ 0 → 0/0
- [x] T005 [US1] Sampler-level scenario in `internal/daemon/governor_test.go`: using the existing sampler test harness, a pending set of overdue thoughts (predicted 1.573 / elapsed 30, planner kind) at effective 8x drives sample() to a shed decision within breachSamples consecutive samples and issues the Govern call; assert the governor snapshot (Debt/Jobs) now reflects the overdue set (visible-jobs fix rides along)

**Checkpoint**: US1 provable — `go test ./internal/cognition/ ./internal/daemon/`

---

## Phase 4: User Story 2 — The fix is provably live in a real run (Priority: P2)

**Goal**: positive evidence in a real daemon, not just green units (FR-006).

**Independent Test**: a saturated run's event log contains `clock.governor_shed`.

- [x] T006 [US2] Doctrine text update in `specs/028-adaptive-throttle`: revise the debt definition (FR-001/FR-002 there) and the "an overdue thought invents no debt it cannot ground" rationale to the accrued-drift doctrine, cross-referencing specs/033-governor-accrued-debt/contracts/debt-formula.md; keep hysteresis doctrine untouched
- [ ] T007 [US2] Operational verification, recorded on TASK-87 (coordinate with the operator — restarts their live world): rebuild `go build -o promptworld ./cmd/promptworld` from the merged (or branch) build; stop/restart world-01; confirm the governor is sampling (status carries governor debt/jobs); drive deliberate saturation per quickstart.md and capture at least one `clock.governor_shed` event (seq + tick + payload) plus the governed status; append the evidence to TASK-87 notes. If impractical pre-merge, an e2e-harness equivalent satisfies SC-004 and the live probe follows the merge

**Checkpoint**: full story set

---

## Phase 5: Polish & Cross-Cutting Concerns

- [x] T008 Full gates inside the worktree: `go test ./...`, `gofmt -l .` (only the 5 pre-existing TASK-83 files may appear), `go vet ./...`; record results on TASK-87
- [ ] T009 Open the PR from `.worktrees/task-87` (one TASK, one PR; branch `task-87-governor-accrued-debt`), body linking specs/033 + TASK-87 evidence + red-first proof; after merge: `/grounding-wiki:wiki-update` for notes sourcing governor.go, then player-docs freshness check (FR-007 — post-merge gate, tracked on TASK-87)

---

## Dependencies

- T001 → T002 (red) → T003 (green) → {T004, T005 in parallel}
- T006 [P] anytime after spec approval; T007 after T003 (needs the fixed build)
- T008 after all code tasks; T009 last

## Parallel Execution Examples

- After T003: T004 and T005 in parallel (different packages); T006 alongside
- T007's e2e/live probe can overlap T008 gate runs

## Implementation Strategy

MVP = Phases 1–3: the inversion is fixed and provable red→green. US2 is evidence
discipline (FR-006) — world-01's zero-shed had two candidate causes, so the live
probe is not optional even with green units. One branch, one PR (TASK-87);
implementation delegated to spec-implementer on **Opus 4.8** (governor logic,
constitution Principle V). Out of scope: rejection-grounded breach (option B,
future hardening on TASK-87).
