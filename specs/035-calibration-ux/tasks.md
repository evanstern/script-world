# Tasks: Calibration UX — uncalibrated worlds warn instead of silently over-suppressing

**Input**: Design documents from `specs/035-calibration-ux/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/warnings.md, quickstart.md

**Tests**: included — the contract's Test obligations section requests them explicitly; project
convention is tests alongside code in the same package.

**Organization**: grouped by user story; US1 (set_speed warning) is the MVP slice. The
foundational phase carries the shared horizon lift and llm seed-state retention that US1/US2/US3
all read.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: baseline verification only — no scaffolding needed (no new packages, no deps).

- [x] T001 Verify clean baseline in the task worktree: `go test ./...` green and `gofmt -l` clean
      on the packages this feature touches (internal/cognition, internal/llm, internal/ipc,
      internal/daemon, cmd/promptworld); note TASK-83's pre-existing gofmt drift is out of scope —
      gate on touched files only

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the two shared mechanisms every story reads — one arithmetic home (research R1),
one seed-state fact (research R3).

**⚠️ CRITICAL**: no user story work until this phase completes.

- [x] T002 Create internal/cognition/horizon.go: move `horizonSummary` from
      cmd/promptworld/calibrate.go verbatim as exported `HorizonSummary(secPerPt float64) string`
      (same ladder, same class list, same wording), plus the minimal exported helper(s) the
      set_speed warning needs to ask "which classes are suppressed at speed S given a per-class
      sec/pt lookup" — built strictly on the existing `Route`/`ClassFor`; stdlib-only (leaf
      purity, doc.go doctrine); constants in estimate.go UNCHANGED (spec FR-007, Doctrine Review)
- [x] T003 Create internal/cognition/horizon_test.go: table tests for `HorizonSummary` at
      bootstrap (20.0) and calibrated (e.g. 0.94) values, plus the agreement property from
      contracts/warnings.md — summary/helper says suppressed ⇔ `Route(dc, tps, secPerPt).Allow`
      is false, across the full ladder × class matrix
- [x] T004 In cmd/promptworld/calibrate.go replace the local `horizonSummary` with delegation to
      `cognition.HorizonSummary`; run cmd/promptworld tests to prove calibrate output is
      byte-identical (spec 024 legacy guarantee untouched by the move itself)
- [x] T005 In internal/llm/llm.go record seed provenance: `provider` gains a `calibratedAt string`
      field set in `SeedCalibration` when (and only when) the profile has a usable entry for that
      provider name (same presence test `cognition.SeedFor` applies; carry the profile's
      `CalibratedAt` verbatim, empty = bootstrap); add the read the ipc gate needs (e.g.
      `Orchestrator.CalibrationFor(name string) string` or equivalent); unit tests in
      internal/llm covering: full profile, partial profile (some providers missing → those stay
      empty), nil profile / never-seeded (all empty)

**Checkpoint**: shared arithmetic + seed-state fact exist and are tested; stories can proceed
(sequential solo default: US1 → US2 → US3 → US4).

---

## Phase 3: User Story 1 - Raising speed on an uncalibrated world warns loudly (Priority: P1) 🎯 MVP

**Goal**: a set_speed that lands an uncalibrated world in suppressing territory returns a
warning naming the suppressed classes + the calibrate command; the speed change always applies.

**Independent Test**: spec US1 acceptance scenarios 1–4 — warning present on uncalibrated+32x,
absent on uncalibrated+4x, absent on calibrated any-speed, absent on no-LLM; speed applied in
all four.

### Implementation for User Story 1

- [x] T006 [US1] Add `Warning string `json:"warning,omitempty"`` to `StatusData` in
      internal/ipc/protocol.go with a doc comment scoping it to the set_speed path
      (contracts/warnings.md §2; additive-omitempty per spec FR-008)
- [x] T007 [US1] In internal/ipc/server.go set_speed handler (after validation + max-gate, which
      stay untouched and take precedence): compose the warning — for each cognition class in
      registry order, resolve the serving provider + current estimate via
      `srv.llm.EstimateForKind`, gate on that provider's empty calibration state (T005 read),
      evaluate suppression at the requested speed via the T002 helper; if any class suppressed,
      set StatusData.Warning naming the classes and suggesting `promptworld calibrate <world>`;
      the speed change applies unconditionally (warning-augmented success, never an error)
- [x] T008 [US1] Extend internal/ipc/ipc_test.go: warning present (uncalibrated + suppressing
      speed), absent (uncalibrated + non-suppressing speed; calibrated world; no-LLM world),
      speed actually applied alongside the warning, max-gate error reply unchanged and
      warning-free, pause/resume/status replies never carry the field
- [x] T009 [US1] Render the warning in the CLI set-speed command output in
      cmd/promptworld/commands.go (print after the normal confirmation line, visually distinct);
      extend cmd/promptworld/commands_test.go for with/without-warning rendering

**Checkpoint**: MVP — US1 fully functional; spec SC-001's speed-change half satisfiable live.

---

## Phase 4: User Story 2 - Boot warning states the concrete consequence (Priority: P2)

**Goal**: daemon boot on a profile-less (or unreadable-profile) LLM world prints the
uncalibrated warning block: statement + per-class horizon at bootstrap seeds + exact calibrate
command (contracts/warnings.md §1).

**Independent Test**: spec US2 acceptance scenarios 1–3 — warning block on absent profile;
byte-identical seeded line on calibrated world; warning alongside the existing note on
unreadable profile.

### Implementation for User Story 2

- [x] T010 [US2] In internal/daemon/daemon.go replace the single no-profile line (and augment the
      unreadable-profile branch) with the warning block: uncalibrated statement with bootstrap
      values, `cognition.HorizonSummary(cognition.BootstrapLocalSecPerPt)` line, and
      `run \`promptworld calibrate <world>\`` with the real world name; the profile-seeded branch
      stays byte-identical; extract the block composition into a small testable function if
      daemon.go has no direct test seam
- [x] T011 [US2] Test the boot warning: unit-test the composing function (all three bracketed
      contract elements present; both trigger branches produce it; seeded branch does not) in the
      appropriate existing test file for internal/daemon, or add one alongside daemon.go

**Checkpoint**: US1 + US2 — both moments of action covered (SC-001 fully satisfiable).

---

## Phase 5: User Story 3 - Calibration state visible in status (Priority: P3)

**Goal**: per-provider `calibrated_at` on the status wire (absent = bootstrap) and an explicit
`uncalibrated (bootstrap)` marker in the human rendering.

**Independent Test**: spec US3 acceptance scenarios 1–4 — bootstrap providers marked; calibrated
providers show timestamp; partial profiles truthful per provider; no-LLM status shape unchanged.

### Implementation for User Story 3

- [x] T012 [US3] In internal/llm/llm.go add `CalibratedAt string `json:"calibrated_at,omitempty"``
      to `ProviderStatus` and copy it from the provider in `StatusSnapshot`; extend internal/llm
      tests: snapshot carries it, omitempty verified by marshaling a bootstrap provider (field
      absent) and a calibrated one (present), partial-profile case per provider
- [x] T013 [US3] Render calibration state in the CLI status output in
      cmd/promptworld/commands.go: timestamp when present, `uncalibrated (bootstrap)` when
      absent, per provider row; extend cmd/promptworld/commands_test.go for both renderings and
      the no-LLM shape (unchanged)

**Checkpoint**: uncalibrated state survives the boot scroll (SC-004).

---

## Phase 6: User Story 4 - Calibrate discloses its sequential-measurement bias (Priority: P3)

**Goal**: every calibrate run prints the sequential-floor disclosure adjacent to the horizon
summary, both legacy and v2 paths (contracts/warnings.md §4; research R6 records the deliberate
spec-024 byte-identity supersession).

**Independent Test**: spec US4 acceptance scenarios 1–2 — disclosure present once per run, next
to the horizon summary, in both config generations' output.

### Implementation for User Story 4

- [x] T014 [US4] In cmd/promptworld/calibrate.go print the disclosure (all three contract
      elements: sequential measurement, floor under concurrent load, live estimator adapts)
      once per run adjacent to the horizon summary, in both `calibrateLegacy` and
      `calibrateDeclaredProviders`; extend cmd/promptworld/calibrate_test.go to assert the
      disclosure in both paths and that it appears exactly once per run

**Checkpoint**: all four stories independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [x] T015 Run quickstart.md end-to-end: `go test ./...` green, gofmt clean on touched files,
      and the live walk-through (steps 1–3, 5) against a scratch world where feasible; record
      outcomes in specs/035-calibration-ux/quickstart-results.md
- [ ] T016 Re-ground the wiki (constitution Principle IV): run `/grounding-wiki:wiki-update` —
      docs/wiki/cognition.md pins estimate.go/daemon boot behavior among sources; also re-verify
      any note sourcing internal/ipc protocol/server, internal/llm status, or
      cmd/promptworld/calibrate.go
- [ ] T017 Player docs freshness (project rule): run
      `node .claude/skills/player-docs/scripts/check-freshness.mjs --check` after the wiki
      re-pin and regenerate docs/player/ via the player-docs skill if stale

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (P1)**: none.
- **Foundational (P2)**: after Setup; **blocks all stories**. Internally: T002 → T003/T004
  (both read horizon.go); T005 independent of T002–T004 ([P] with them).
- **US1 (P3)**: after Foundational (needs T002 helper + T005 read). Internally T006 → T007 →
  T008; T009 after T006 (field shape), parallel-safe with T007/T008.
- **US2 (P4)**: after Foundational (needs T002/HorizonSummary only — not T005). Independent of
  US1.
- **US3 (P5)**: after T005 (reads provider.calibratedAt). Independent of US1/US2. T012 → T013.
- **US4 (P6)**: after T004 (touches the same calibrate output region). Independent of
  US1/US2/US3.
- **Polish (P7)**: after all stories. T015 → T016 → T017 (wiki re-pin wants final code; player
  docs read the wiki).

### Parallel Opportunities

- T003, T004, T005 once T002 lands.
- After Foundational: US1, US2, US3, US4 are pairwise independent story slices (different files
  except cmd/promptworld/commands.go shared by T009/T013 — serialize those two if parallelizing).

## Implementation Strategy

Sequential solo default (one implementer subagent): Setup → Foundational → US1 (**stop:
independent-test the MVP**) → US2 → US3 → US4 → Polish. Commit per task or logical group on the
single task branch (`task-40-*` worktree; one TASK, one PR). Spec-doc ticks in this file commit
to main at root per project convention.
