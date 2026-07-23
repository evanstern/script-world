# Tasks: Extract the Intent-Landing Ladder into Named Rungs

**Input**: Design documents from `/specs/022-landing-ladder-rungs/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: test tasks INCLUDED — rung-isolation unit tests are an explicit requirement
(FR-006, board AC #3), and the existing determinism suite is the behavioral gate (FR-005).

**Organization**: grouped by the spec's user stories. This is a single refactor slice —
US1 (extraction) is the MVP; US2 (proof) and US3 (rung tests) gate its acceptance.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: worktree + baseline evidence

- [x] T001 Create the task worktree per constitution II: `git worktree add .worktrees/task-70 -b task-70-landing-ladder-rungs origin/main` (root stays on main)
- [x] T002 Record the baseline: run `go test -race ./internal/sim/` at the worktree tip and confirm green BEFORE any change (baseline evidence for the bit-identical claim)

---

## Phase 2: Foundational

*(none — the refactor has no blocking infrastructure; the ladder exists only at internal/sim/loop.go:437-632)*

---

## Phase 3: User Story 1 — named-rung extraction (P1) 🎯 MVP

**Goal**: the landing decision lives in `landIntent` + doctrine-named rungs; the
`adapted`/`failed`/`hailTarget` flag interplay is gone.

**Independent test**: read `internal/sim/landing.go` top-to-bottom against research.md D4's
rung table; `grep` per quickstart.md §4 shows no ladder flags left in loop.go.

- [x] T003 [US1] Create `internal/sim/landing.go`: `landingDecision` type (outcome/reason/hailTarget per data-model.md) and `(*Loop).landIntent(in, emit)` carrying the full former case body — bounds check, staleness clamp, reject helper, plan/goal paths, final cog.outcome — with behavior frozen (same events, order, error strings; see research.md D2/D3)
- [x] T004 [US1] Decompose the guard walk in `internal/sim/landing.go` into doctrine-named rung functions per research.md D4: `rungUnavailable`, `rungSuperseded`, `rungStale`, guard walk with `rungHailRelaxed` (mutual-hailer then hailable), `rungGuardFailed`, `rungAdapted` (moved-target), in-radius hail marking; preserve walk order, short-circuit-on-reject, and last-write-wins hailTarget
- [x] T005 [US1] Shrink the `inject_intent` case in `internal/sim/loop.go` to `err = l.landIntent(cmd.inject, emit)`; delete the inline ladder; keep the case comment pointing at landing.go
- [x] T006 [US1] Build + vet: `go build ./... && go vet ./internal/sim/` in the worktree

**Checkpoint**: extraction complete, compiles clean — but NOT acceptable until US2 proves identity.

---

## Phase 4: User Story 2 — behavior-identity proof (P1)

**Goal**: bit-identical behavior proven by the existing determinism harness, unedited.

**Independent test**: quickstart.md §1–§2 commands green; diff shows no existing test/fixture touched.

- [x] T007 [US2] Run the determinism gate per quickstart.md §1: `go test -race ./internal/sim/ -run 'TestDeterminism|TestReplay' -v` — all pass with zero edits to existing test files
- [x] T008 [US2] Run the full suite: `go test -race ./...` — all packages pass
- [x] T009 [US2] Verify diff surface: `git diff --stat origin/main` touches only `internal/sim/loop.go`, `internal/sim/landing.go`, `internal/sim/landing_test.go` (+ spec artifacts); no event-schema/doctrine/guard files changed (FR-008)

---

## Phase 5: User Story 3 — rung isolation tests (P2)

**Goal**: every rung and every hail special-case exercised directly, no command round-trip.

**Independent test**: quickstart.md §3 — `go test ./internal/sim/ -run 'TestLanding' -v`.

- [x] T010 [P] [US3] In `internal/sim/landing_test.go`: per-rung tests — unavailable (dead; asleep; dead-before-asleep ordering), superseded (generation mismatch), stale (staleness > class budget), guard-failed (plain Eval failure, incl. short-circuit after first failing guard), fresh fall-through (OutcomeLanded)
- [x] T011 [P] [US3] In `internal/sim/landing_test.go`: hail special-case tests — mutual-hailer (target is actor's own hailer → adapted, NO new hail), in-radius hailable target (fresh + hailTarget set), moved-target adapt (guard holds at repaired coords → adapted), dead/out-of-range target falls through to guard-failed
- [x] T012 [P] [US3] In `internal/sim/landing_test.go`: decision-consumption tests through `landIntent` with a capturing emit — rejection emits `agent.intent_rejected` + `cog.outcome` AND returns the error (pairing preserved); uncognized (Class=="") rejection emits nothing; plan path never emits `social.hailed`; goal path emits it once when hailTarget ≥ 0; adapted vs landed reflected in final `cog.outcome`

---

## Phase 6: Polish & Cross-Cutting

- [x] T013 Re-run quickstart.md §1–§4 end-to-end in the worktree; record results in the PR body; commit, push, open the TASK-70 PR (one task, one PR)
- [ ] T014 After merge: `/grounding-wiki:wiki-update` to re-verify and re-pin `docs/wiki/sim-loop.md` (add landing.go to its sources) — FR-009 / board AC #4; then remove the worktree and ff-pull root

---

## Dependencies

- T001 → T002 → US1 (T003 → T004 → T005 → T006) → US2 (T007, T008, T009) → T013 → T014
- US3 (T010–T012, parallel-eligible) requires US1 complete; independent of US2 and may
  interleave with it; must be green before T013.

## Parallel Example

After T006: run T007/T008/T009 (US2) while writing T010/T011/T012 (US3) — different
files, no write conflicts (`landing_test.go` is new; US2 only runs existing tests).

## Implementation Strategy

MVP = US1, but this slice merges as one PR only when US1+US2+US3 are all green: a
"pure refactor" without its identity proof (US2) and isolation tests (US3) does not
satisfy the board ACs. No incremental delivery below the single PR.
