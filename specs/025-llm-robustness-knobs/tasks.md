# Tasks: llm.json robustness knobs — in-loop cognition retry + configurable max_tokens

**Input**: Design documents from `/specs/025-llm-robustness-knobs/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/llm-json.md, contracts/loop-retry.md, quickstart.md

**Tests**: INCLUDED — the spec's success criteria (SC-001…SC-007) and TASK-72's acceptance criteria are test-gated; each contract invariant in `contracts/loop-retry.md` names a locking test. Tests are written FIRST and must fail before implementation.

**Organization**: grouped by user story. US1 (retry) and US2 (token budgets) touch disjoint code regions and are independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependencies on incomplete tasks)
- **[US1/US2]**: maps to spec.md user stories
- Every task names exact file paths

## Path Conventions

Single Go module at repo root; packages under `internal/`, entrypoints under `cmd/`. All implementation happens on branch `task-72-llm-robustness-knobs` in `.worktrees/task-72` (one task, one PR).

---

## Phase 1: Setup

**Purpose**: branch/worktree ready, baseline green

- [x] T001 Cut worktree `.worktrees/task-72` on branch `task-72-llm-robustness-knobs` from fresh `origin/main` (`git fetch origin && git worktree add .worktrees/task-72 -b task-72-llm-robustness-knobs origin/main`) and confirm baseline `go test -race ./...` passes there

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: none — no shared prerequisite exists. US1 modifies `internal/toolloop` + consumer telemetry; US2 modifies `internal/llm` config + constructor plumbing. The two stories share no files and can proceed in either order or in parallel after T001.

**Checkpoint**: T001 done → both stories unblocked

---

## Phase 3: User Story 1 — A flaky provider call no longer wastes a whole thought (Priority: P1) 🎯 MVP

**Goal**: one in-loop retry on transport provider_error in the tool-loop driver; recovery observable in the trail; estimator/breaker/round-cap doctrine provably unchanged (spec FR-001…FR-006; contracts/loop-retry.md).

**Independent Test**: `go test ./internal/toolloop/ ./internal/mind/ ./internal/metatron/ -run 'Retry|Retr' -v` — fail-once stub recovers, fail-twice terminates as today, admission refusals never retry, retry visible as a `cog.outcome` retried event (quickstart.md §1–2).

### Tests for User Story 1 (write first, confirm they FAIL)

- [x] T002 [P] [US1] Scripted-stub retry tests in `internal/toolloop/retry_test.go` (new file, existing stub pattern from `loop_test.go`): (1) fail-once → run completes in success family, `Result.Retried == true`, `RetryReason` = first error, transcript re-submitted byte-identical; (2) fail-twice → `TermProviderError`, second error propagated, exactly 2 Submit attempts; (3) `ErrTierBusy`/`ErrQueueFull`/`ErrBudgetExhausted`/`ErrTierDown` → zero retries, `TermAdmissionRefused` (busy-is-not-down); (4) context cancelled → zero retries, `TermCtxDone`; (5) tool-handler `Err` (loop.go:269/287 paths) → zero retries, terminates as today; (6) estimator invariance: recovered run → exactly 1 `ObserveCognition` call, twice-failed run → 0 (contract invariant 2, SC-004); (7) round-cap invariance: fail on round 1, recover, run to cap → `MaxRounds` full rounds completed (contract invariant 1); (8) `RetryReason` empty ⇔ `Retried` false
- [x] T003 [P] [US1] Mind retry-visibility test in `internal/mind/mind_test.go` (stubbed `runLoop` returning `Result{Retried: true, ...}`): a recovered planner cognition emits a non-terminal `cog.outcome` event with outcome `sim.OutcomeRetried` carrying the retry reason, BEFORE the terminal outcome; a non-retried run emits none (SC-003)
- [x] T004 [P] [US1] Metatron retry-visibility test in `internal/metatron/metatron_test.go` (stubbed `runLoop`): a recovered console turn emits the same `cog.outcome` retried event through the InjectSocial door; a non-retried turn emits none (SC-003)

### Implementation for User Story 1

- [x] T005 [US1] In `internal/toolloop/loop.go`: add `Retried bool` / `RetryReason string` to `Result`; in `run()`'s Submit-error branch (loop.go:195-199), when `terminationForSubmitErr(serr) == TermProviderError` and the run's retry is unspent, set `res.Retried`/`res.RetryReason` and `continue` (re-submit identical transcript, no round consumed); otherwise terminate exactly as today. Update the `Run` doc comment (loop.go:99-107) with the retry guarantee per `contracts/loop-retry.md`. T002 tests go green
- [x] T006 [US1] In `internal/mind/mind.go` (`runPlan`, after the loop returns) + `internal/mind/telemetry.go`: when `res.Retried`, emit the non-terminal `cog.outcome` with `sim.OutcomeRetried` and `res.RetryReason` via the existing `cogOutcomeEvent`/`emitCog` family (TASK-42 shape). T003 goes green
- [x] T007 [US1] In `internal/metatron/turn.go` (after `mt.runLoop` returns) + `internal/metatron/toolcalls.go` as needed: when `res.Retried`, emit the same `cog.outcome` retried event through metatron's InjectSocial door (the `emitToolCalls` channel). T004 goes green
- [x] T008 [US1] Story gate: `go test -race ./internal/toolloop/ ./internal/mind/ ./internal/metatron/` green and `go test ./... -run TestCatalogSweep` green (no digest-catalog drift — no new event type was added)

**Checkpoint**: US1 fully functional and independently testable — the MVP increment

---

## Phase 4: User Story 2 — Operator tunes cognition token budgets in llm.json (Priority: P2)

**Goal**: `max_tokens.{planner,metatron_turn,consolidation}` knobs with warn-not-error clamping (defaults 512/1024/1024, range 1–4096), plumbed daemon → constructors → call sites (spec FR-007…FR-010; contracts/llm-json.md; data-model.md §1, §5).

**Independent Test**: `go test ./internal/llm/ -run 'TokenBudget' -v` normalization table green; scratch-world boot with an out-of-range value warns and clamps but boots (quickstart.md §3–4).

### Tests for User Story 2 (write first, confirm they FAIL)

- [x] T009 [P] [US2] Normalization table tests in `internal/llm/llm_test.go` (mirror `TestRoundsNormalization`, llm_test.go:1080): for each of the three keys — absent → default (512/1024/1024) no warning; 0 → default no warning; in-range (e.g. 2048) → verbatim no warning; negative → default + warning naming `max_tokens.<key>`, the value, and the effective value; > 4096 → 4096 + warning; fields normalize independently and warnings accumulate; JSON round-trip of a config WITHOUT `max_tokens` marshals without the key (`omitempty`, `WriteDefault` stays minimal)
- [x] T010 [P] [US2] Budget plumbing tests: in `internal/mind/mind_test.go`, a Mind constructed with explicit planner/consolidation budgets passes them as `Job.MaxTokens` on the planner loop (capture via stubbed `runLoop`) and as `Request.MaxTokens` on the consolidation call; in `internal/metatron/metatron_test.go`, a Metatron constructed with an explicit turn budget passes it on the console-turn `Job.MaxTokens`; defaults reproduce 512/1024/1024 (FR-010)

### Implementation for User Story 2

- [x] T011 [US2] In `internal/llm/config.go`: add `TokenBudgets` struct (`Planner`, `MetatronTurn`, `Consolidation int64` with json tags per `contracts/llm-json.md`), `MaxTokens TokenBudgets \`json:"max_tokens,omitempty"\`` on `Config`, shared `maxTokenBudget = 4096` const, and per-field normalizers returning `(effective int64, warn string)` mirroring `Rounds()` (config.go:43-54) with defaults 512/1024/1024. T009 goes green
- [x] T012 [P] [US2] In `internal/mind/mind.go` + `internal/mind/consolidate.go`: `mind.New` gains planner + consolidation budget params (following the `loopRounds` pattern, mind.go:122); store as Mind fields; `mind.go:405` reads the planner field (delete the `loopMaxTokens` const, moving its rationale comment to the config default); `consolidate.go:133` reads the consolidation field. Mind half of T010 goes green
- [x] T013 [P] [US2] In `internal/metatron/metatron.go` + `internal/metatron/turn.go`: `metatron.New` gains a turn budget param (metatron.go:100); store as field; `turn.go:157` reads it (delete the `turnMaxTokens` const, moving its rationale comment to the config default). Metatron half of T010 goes green
- [x] T014 [US2] In `internal/daemon/daemon.go` (knob block, daemon.go:158-175): resolve the three budgets, print each non-empty warning on the existing `"daemon: %s"` channel, pass resolved values into `mind.New` / `metatron.New`; update `cmd/promptworld/calibrate.go` mechanically for the new signatures (calibrate.go:269 area). `go build ./...` green
- [x] T015 [US2] Story gate: `go test -race ./internal/llm/ ./internal/mind/ ./internal/metatron/ ./internal/daemon/` green; scratch-world boot smoke per quickstart.md §4 (out-of-range value → warning line + successful boot; valid value → silent, on the wire)

**Checkpoint**: both stories independently functional

---

## Phase 5: Polish & Cross-Cutting

**Purpose**: whole-suite gates and grounding freshness (TASK-72 AC #5)

- [x] T016 Full-suite gate: `go test -race ./...` green in the worktree; run the quickstart.md scenarios end to end and record results in the PR description
- [x] T017 Post-merge (root checkout, after the PR lands): `/grounding-wiki:wiki-update` re-pins the touched notes (`docs/wiki/tool-loop.md`, `docs/wiki/llm-orchestrator.md`, `docs/wiki/agent-mind.md`, `docs/wiki/metatron.md`, `docs/wiki/nightly-consolidation.md`, `docs/wiki/daemon-lifecycle.md` — exact set as wiki-update computes against the diff)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: T001 first — blocks everything
- **Foundational (Phase 2)**: empty — no blocking prerequisites
- **US1 (Phase 3)** and **US2 (Phase 4)**: both depend only on T001; fully independent of each other (disjoint files except the shared test files `mind_test.go`/`metatron_test.go`, where the stories add separate test functions)
- **Polish (Phase 5)**: T016 after both stories; T017 strictly post-merge

### Within US1

- T002, T003, T004 [P] first (failing tests) → T005 (unblocks T002) → T006 (unblocks T003) and T007 (unblocks T004) may run [P] → T008 gate

### Within US2

- T009, T010 [P] first (failing tests) → T011 (the type — blocks T012/T013) → T012, T013 [P] (different packages) → T014 (needs both signatures) → T015 gate

### Parallel Opportunities

- All four test-authoring tasks T002/T003/T004 + T009/T010 are mutually parallel after T001
- T006 ∥ T007 (different packages); T012 ∥ T013 (different packages)
- US1 and US2 may be implemented in either order or interleaved — but they land as commits on the ONE task branch and merge in the ONE TASK-72 PR (constitution Principle II); no per-story PRs

---

## Parallel Example: after T001

```text
# Author all failing tests concurrently:
T002 internal/toolloop/retry_test.go
T003 internal/mind/mind_test.go        (retry visibility)
T004 internal/metatron/metatron_test.go (retry visibility)
T009 internal/llm/llm_test.go          (normalization table)
T010 internal/mind/mind_test.go + internal/metatron/metatron_test.go (budget plumbing)
```

---

## Implementation Strategy

**MVP first (US1 only)**: T001 → T002–T008 → validate via quickstart §1–2. US1 alone delivers the pain-relief the task was filed for, with zero configuration surface.

**Incremental**: add US2 (T009–T015) → validate via quickstart §3–4 → T016 full gate → PR → merge → T017 re-ground.

**Tier note (constitution Principle V)**: implementation executes on the `spec-implementer` agent at **Opus 4.8** (cross-package; toolloop/estimator/breaker doctrine — senior-tier rubric); the tier choice and justification are recorded on TASK-72.
