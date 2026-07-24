# Tasks: Estimator Breach Adoption

**Input**: Design documents from `/specs/031-estimator-breach-adoption/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/adoption-event.md, quickstart.md

**Tests**: INCLUDED — the spec's success criteria explicitly demand regression tests
(SC-001 freeze regression, SC-002 one-shot preservation, SC-005 conscious retune),
and TASK-86 AC #1 requires the world-01 shape as a test.

**Organization**: grouped by user story; US1 is the MVP increment.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch/worktree per constitution (root stays on main)

- [x] T001 Create worktree: from repo root run `git fetch origin && git worktree add .worktrees/task-86 -b task-86-estimator-breach-adoption origin/main`; all subsequent work happens inside `.worktrees/task-86/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the window ring must retain sample VALUES before any story can act on them (FR-001)

**⚠️ CRITICAL**: complete before user-story phases

- [x] T002 Refactor the window ring in `internal/cognition/estimate.go`: replace `window []bool` with a ring retaining `{secPerPoint float64, spike bool}` per slot (same WindowSize, same cursor/fill semantics, `rateLocked` reads the flags). BEHAVIOR-NEUTRAL refactor: `Sample`'s decisions and returns unchanged; `go test ./internal/cognition/` green unmodified, including TestEstimatorSampleCountUnderConcurrency

**Checkpoint**: ring carries values; estimator behavior identical to main

---

## Phase 3: User Story 1 — The estimate follows a sustained slowdown (Priority: P1) 🎯 MVP

**Goal**: on first breach over a full window, adopt the window median, reset, re-arm — the freeze is dead.

**Independent Test**: seed 0.52 s/pt, feed 20× ~12 s/pt samples, assert adoption at the window-completing sample; router arithmetic flips from blind admission to honest prediction.

- [x] T003 [US1] Implement adoption in `internal/cognition/estimate.go`: add exported evidence type (`Adoption{Prior, Adopted, SpikeRate float64}` — plain values, package stays stdlib-only leaf); change `Sample` to return `*Adoption` (nil = no breach); on the sample that first drives rate > BreachRate over a full window, atomically (single mutex hold) set estimate = median of retained window values, zero the ring (`wi=0, wn=0`), clear `breached`, keep lifetime `samples`/`spikes` counters, and return the evidence (research.md R3/R4; data-model.md state transition 3). Doctrine constants untouched (FR-007). Update doc comments to the new doctrine ("systemic drift is followed — by adoption at breach")
- [x] T004 [US1] Freeze-regression test in `internal/cognition/estimate_test.go`: world-01 shape — seed 0.52, 20 consecutive ~12 s/pt samples → exactly one non-nil Adoption at sample 20 with Prior≈0.52, Adopted == window median (≈12), estimate == Adopted afterward (SC-001); plus downward-step case: samples below estimate follow via EWMA with no adoption (US1 scenario 3, research.md R5)
- [x] T005 [US1] Re-arm test in `internal/cognition/estimate_test.go`: after adoption, a full window of stable samples at the adopted level yields nil Adoptions and no breach; a second sustained >3× step (e.g. 12 → 40 s/pt) adopts again after its own full window (US3 scenario 2 semantics live here because re-arm is estimator state)
- [x] T006 [US1] Router-truthfulness test in `internal/cognition/route_test.go` (or estimate_test.go if Route coverage lives there): with the adopted estimate (12 s/pt), `Route` for the planner class (3pt, budget 1200) admits at 8x (288 ticks) and suppresses at 32x (1152 ticks — verify against actual budget arithmetic), demonstrating honest pre-dispatch verdicts (SC-003 unit-level)

**Checkpoint**: US1 independently testable — `go test ./internal/cognition/` proves the freeze is fixed

---

## Phase 4: User Story 2 — One-shot lag spikes are still rejected (Priority: P2)

**Goal**: prove zero regression on isolated-spike rejection.

**Independent Test**: ≤2 spikes per window leave the estimate bit-identical to pre-change arithmetic.

- [x] T007 [P] [US2] One-shot preservation tests in `internal/cognition/estimate_test.go`: windows with 1–2 spikes interleaved among normal samples → every Adoption return nil, spike samples contribute nothing, and the final estimate equals the EWMA over non-spike samples alone computed to full float64 equality (SC-002). Audit existing estimator tests for any assertion that encodes the freeze itself; retune consciously with a comment naming spec 031 (SC-005)

**Checkpoint**: US1+US2 — behavior change is provably confined to the breach episode

---

## Phase 5: User Story 3 — Adoption is auditable (Priority: P3)

**Goal**: the adoption's arithmetic rides the existing breach event, additively.

**Independent Test**: drive a breach; exactly one `cog.recalibration_recommended` event carries prior→adopted; historical payloads still decode.

- [x] T008 [US3] Plumb evidence through the hook in `internal/llm/llm.go`: `feedEstimate` consumes `Sample`'s `*Adoption`; extend the `recalibrate` hook field + `SetRecalibrateHook` signature to carry prior and adopted values (hook stays per-provider, fired in its own goroutine as today); update any llm tests touching the hook
- [x] T009 [US3] Additive payload fields in `internal/sim/cognition.go`: `RecalibrationPayload` gains `PriorSPerPt float64 \`json:"prior_s_per_pt,omitempty"\`` and `AdoptedSPerPt float64 \`json:"adopted_s_per_pt,omitempty"\`` (contracts/adoption-event.md); `Mind.RecalibrateSignal` in `internal/mind/telemetry.go` accepts and marshals them; `estimate_s_per_pt` keeps meaning "current estimate at emission" (post-adoption value)
- [x] T010 [US3] Extend the digest renderer for `cog.recalibration_recommended` in `internal/tui/digest.go`: when the new fields are present render `prior→adopted` (e.g. `est=0.52→11.80s/pt`); keep rendering legacy payloads without the fields; `go test ./internal/tui/ -run CatalogSweep` green
- [x] T011 [US3] Telemetry round-trip tests: payload decodes with and without the new fields (legacy replay compatibility per contracts/adoption-event.md); a driven breach emits exactly one event carrying tier, spike rate, window, prior, adopted (place beside existing RecalibrateSignal/telemetry tests in `internal/mind/` or `internal/sim/`)

**Checkpoint**: full story set — breach episodes are acted on AND fully auditable

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T012 [P] Append breach-adoption doctrine to `specs/007-cognition-horizon/contracts/calibration.md`: at breach the estimator adopts the window median (cross-reference specs/031-estimator-breach-adoption/contracts/adoption-event.md for the wire shape); constants remain doctrine
- [x] T013 Full gates inside the worktree: `go test ./...`, `gofmt -l .` (empty), `go vet ./...`; run the quickstart unit-level validation section and record results on TASK-86
- [x] T014 Verify `internal/daemon/daemon.go` hook install site (`orch.SetRecalibrateHook(md.RecalibrateSignal)`) compiles against the new signature with no daemon logic change; if a change proves necessary, keep it mechanical and note it on TASK-86
- [x] T015 Open the PR from `.worktrees/task-86` (one TASK, one PR; branch `task-86-estimator-breach-adoption`), body linking specs/031 and TASK-86 evidence; after merge: run `/grounding-wiki:wiki-update` (docs/wiki/cognition.md sources include estimate.go) then `node .claude/skills/player-docs/scripts/check-freshness.mjs --check` (FR-008 — post-merge gate, tracked on TASK-86)

---

## Dependencies

- T001 → T002 → {US1: T003 → T004, T005, T006}
- US2 (T007) depends only on T003 (parallel with T004–T006)
- US3 (T008 → T009 → T010, T011) depends on T003 (the evidence type); independent of T004–T007
- Polish: T012 anytime after spec approval [P]; T013–T014 after all stories; T015 last

## Parallel Execution Examples

- After T003 lands: T004, T005, T006 (US1 tests), T007 (US2), and T008 (US3 plumbing start) can proceed in parallel — different test families/files
- T012 (doctrine append) is independent of all code tasks

## Implementation Strategy

MVP = Phase 1–3 (US1): the freeze itself is fixed and provable at that point. US2 is
pure test assurance; US3 is the audit surface. All three ride ONE branch and ONE PR
(TASK-86) — phases are internal breakdown, not PR boundaries. Implementation is
delegated to the spec-implementer agent on **Opus 4.8** (constitution Principle V:
`internal/cognition` doctrine-adjacent estimator/governor-family logic).
