# Tasks: Tool Registry — single source of truth for agent capabilities (Layer 1)

**Input**: Design documents from `/specs/014-tool-registry/`

**Prerequisites**: plan.md, spec.md (clarified), research.md (R1–R9), data-model.md,
contracts/tool-catalog.md, contracts/registry-api.md, quickstart.md

**Tests**: REQUIRED — the spec's guarantees are test-shaped (byte-identical prompts,
replay identity, ladder coverage per AC #4); the golden-prompt fixture MUST be captured
against pre-refactor code (quickstart step 0), which fixes the phase order below.

**Organization**: grouped by user story; US1 = derivation (MVP), US2 = migration
identity, US3 = rosters.

**Sequencing gate (clarified 2026-07-22)**: implementation starts only after TASK-51's
PR merges. T001 enforces this.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: worktree, vocabulary re-enumeration, and the pre-refactor anchors.

- [X] T001 Verify TASK-51 (spec 013) PR is merged; `git fetch origin && git pull --ff-only` in root; create worktree `git worktree add .worktrees/task-53 -b task-53-tool-registry origin/main`; all subsequent tasks run inside `.worktrees/task-53`
- [X] T002 Re-enumerate the verb vocabulary on post-TASK-51 main (`internal/mind/prompt.go` goalVocabulary, `internal/mind/parse.go` validGoals, `internal/sim/plan.go` planGoals) and update `specs/014-tool-registry/contracts/tool-catalog.md` world-tool table if spec 013 added verbs (catalog shape is fixed; rows extend)
- [X] T003 Write the golden-prompt anchor test AGAINST CURRENT CODE in `internal/mind/prompt_golden_test.go` with fixture `internal/mind/testdata/planner_prompt.golden`: pins full `systemPrompt` output (persona + vocabulary + gloss) byte-for-byte; commit it green BEFORE any registry code (quickstart step 0, R3)
- [X] T004 Record baseline: `go test ./...` green on the branch tip; note run in task notes (`backlog task edit TASK-53 --append-notes`), and record the implementation tier choice + Principle V rubric justification on TASK-53

**Checkpoint**: fixture and baseline committed; refactor may begin.

## Phase 2: Foundational — the registry package (blocks all stories)

**Purpose**: `internal/tool` leaf package per contracts/registry-api.md; no consumer
changes yet, so the world is bit-for-bit unchanged at this checkpoint.

- [X] T005 [P] Create `internal/tool/tool.go`: `Tool`, `EffectClass` (World/Expressive/Read), `Param`/`ParamKind`, `GateClass` (Resolvable/Charge/Scene/None), `Cost` types per data-model.md
- [X] T006 Create `internal/tool/registry.go`: all catalog entries in `goalVocabulary` registration order (world verbs with durations + PromptGloss strings carried byte-exact from `internal/mind/prompt.go:27–33`; say/gist/muse with caps 300B/200B/200 runes and Events; converse/nudge_dream/nudge_omen with charge cost + 400 cap + Events) + `All()`, `Lookup()` per contracts/registry-api.md
- [X] T007 [P] Create `internal/tool/roster.go`: `RosterVillager` (world verbs in registration order + say, muse, gist), `RosterMetatron` (converse, nudge_dream, nudge_omen), `OnRoster()`
- [X] T008 Create `internal/tool/derive.go`: `VocabularyLine()`, `PromptGlossBlock()`, `WorldGoals()`, `PlanStepGoals()` — each one walk of the registry (data-model.md derived-surfaces table)
- [X] T009 Create `internal/tool/validate.go`: `Validate()` per R9 (unique non-empty names; known effect class; Events non-empty iff Expressive; PlanStep/ReflexEligible only on World; roster names resolve; no Read tools on rosters) returning ALL violations
- [X] T010 Registry unit tests in `internal/tool/registry_test.go` + `internal/tool/validate_test.go`: catalog completeness vs contracts/tool-catalog.md (every row present, nothing extra), single-walk invariant (VocabularyLine names ≡ WorldGoals ≡ PlanStepGoals — TASK-55 AC #2), Validate() rejects each malformed-fixture case (FR-003)

**Checkpoint**: `go test ./internal/tool` green; `go test ./...` still green (nothing consumes the registry yet).

## Phase 3: User Story 1 — One place to define a capability (P1) 🎯 MVP

**Goal**: prompt vocabulary, mind parse validation, and sim-door validation all derive
from the registry; duplicate maps die; adding a tool touches ≤2 sites.

**Independent Test**: register a test-only tool in a test build → appears in all three
derived surfaces with zero other edits; grep finds no duplicate maps; golden prompt
unchanged; drift-cure test passes.

- [X] T011 [US1] Derive the prompt: replace `goalVocabulary` const and gloss prose in `internal/mind/prompt.go` with `tool.VocabularyLine()` / `tool.PromptGlossBlock()`; `prompt_golden_test.go` fixture must pass UNCHANGED (SC-003)
- [X] T012 [US1] Derive mind parse validation: replace `validGoals` map in `internal/mind/parse.go` with `tool.WorldGoals()` (accept set identical — FR-005)
- [X] T013 [US1] Derive the sim door: in `internal/sim/loop.go` validate plan steps via `tool.PlanStepGoals()` and single goals via registry membership; delete `planGoals` from `internal/sim/plan.go`; generation/staleness/guard rungs byte-untouched (FR-006, FR-014)
- [X] T014 [US1] Table-ize durations: replace `intentDuration` switch in `internal/sim/agents.go` with a table built from `tool.All()` Cost.DurationTicks at init; `workDuration` context overrides in `internal/sim/executor.go` untouched (R2, R7)
- [X] T015 [US1] Table-ize goal resolution: restructure `resolveGoal` switch in `internal/sim/policy.go` into a name-keyed resolver table with identical per-verb semantics; `decideIntent` (reflex ladder) untouched (R2)
- [X] T016 [US1] Create `internal/sim/toolcheck.go`: startup coverage check — every World tool on a roster has a resolver-table entry and a duration; wire `tool.Validate()` + this check into daemon boot in `internal/daemon` (error aborts boot, FR-003/R9)
- [X] T017 [P] [US1] Test-only-tool exercise in `internal/sim/toolreg_test.go` (or `internal/tool` export_test pattern): a tool registered only in the test appears in VocabularyLine/WorldGoals/PlanStepGoals and is accepted at the door; removing it removes it everywhere (US1 acceptance scenario 4, SC-001)
- [X] T018 [P] [US1] Drift-cure test `TestPlanStepVocabulary` in `internal/sim/plan_test.go`: plan steps naming each of the 9 spec-012 verbs are ACCEPTED at the door and execute; documents FR-012 as the sole delta (TASK-55 AC #1)
- [X] T019 [US1] Dead-map sweep: `grep -rn "goalVocabulary\|validGoals\|planGoals" internal/` returns nothing in non-test code (SC-004, quickstart step 2); fix any stragglers

**Checkpoint**: `go test ./...` green; golden prompt unchanged; US1 fully demonstrable.

## Phase 4: User Story 2 — Existing capabilities migrate unchanged (P2)

**Goal**: expressive + metatron capabilities read the registry; behavior- and
replay-identical (the FR-012 delta excepted).

**Independent Test**: replay suite passes unmodified; caps enforced at same values;
whitelist diff empty; live smoke world behaves as before.

- [X] T020 [P] [US2] Expressive caps from registry: replace utterance/gist/musing cap literals in `internal/mind/parse.go` (300 bytes `parseSay`, 200 bytes `parseOutcome`, 200 runes `parseMusing`) with `tool.Lookup` Cost reads; parse behavior byte-identical (FR-010, R7)
- [X] T021 [P] [US2] Metatron from registry: in `internal/metatron/turn.go` validate nudge form against `tool.RosterMetatron` and text cap via `tool.Lookup("nudge_…").Cost`; keep `internal/sim/metatron.go` charge economy + reducer dry-run as enforcers, reading the 400 cap from the registry constant (FR-010, R7)
- [X] T022 [US2] Whitelist-subset startup check in `internal/sim/toolcheck.go`: every Expressive tool's declared Events ⊆ `injectSocialWhitelist`; assert in test that the whitelist map itself is diff-identical to pre-refactor (zero entries added/removed — FR-013)
- [X] T023 [US2] Identity verification pass: full replay suite (`internal/sim/whole_feature_test.go`, `sim_test.go` determinism/replay, per-capability replay tests incl. `metatron_test.go` charges, `internal/mind/convo_test.go` golden path) passes with ZERO test-file edits; `go test ./...` green (SC-002, FR-011)
- [X] T024 [US2] Live smoke per quickstart step 7: new world runs one planner cadence; villagers plan/speak/muse, metatron nudge lands, a multi-step plan with `cook`/`quarry` lands (the delta, visible live); record observation in TASK-53 notes

**Checkpoint**: migration proven behavior-identical; both doors and ladder untouched.

## Phase 5: User Story 3 — Capability is roster membership (P3)

**Goal**: door-enforced roster membership as data; out-of-roster = rejected like unknown.

**Independent Test**: villager naming a metatron tool rejected at `InjectIntent`;
metatron naming a world verb rejected; roster/registry inconsistency fails startup.

- [ ] T025 [US3] Roster enforcement at the intent door: `internal/sim/loop.go` goal validation becomes `tool.OnRoster(tool.RosterVillager, goal)` ∧ effect-class check (accept set unchanged for real traffic); rejection recorded via the existing `reject` path, non-fatal (FR-009)
- [ ] T026 [US3] Roster enforcement for metatron: nudge form validation in `internal/metatron/turn.go` + reducer dry-run keyed off `RosterMetatron` membership (converse/nudge_dream/nudge_omen only)
- [ ] T027 [P] [US3] Rejection tests in `internal/sim/roster_test.go`: villager action naming `nudge_dream` → rejected, no event lands; unknown name → rejected identically; metatron form outside roster → refused with counsel as today (US3 scenarios 1–2, SC-005)
- [ ] T028 [P] [US3] Startup inconsistency tests in `internal/tool/validate_test.go` + `internal/sim/toolcheck_test.go`: roster naming a missing tool, Read tool on a roster, World tool without resolver — each aborts boot with a config error, never a tick-time failure (US3 scenario 3)

**Checkpoint**: all three stories independently demonstrated.

## Phase 6: Polish & Cross-Cutting

- [ ] T029 [P] Ladder regression audit: confirm generation/staleness/guard tests (`internal/sim/memory_test.go`, `hail_test.go`, `cognition_test.go`) pass unmodified and cover the registry-fed door path; add a rung-coverage case only if a gap is found (AC #4)
- [ ] T030 [P] Doc reconciliation: update `specs/014-tool-registry/contracts/tool-catalog.md` "= today" cells with the actual shipped values; note any spec-013 verbs added in T002
- [ ] T031 Run full quickstart.md (steps 1–6) top to bottom in the worktree; `go vet ./...` clean; record results in TASK-53 notes
- [ ] T032 Wiki re-pin: run `/grounding-wiki:wiki-update` for touched sources (agent-mind, sim-loop, reflex-policy, executor, metatron, cognition, event-types notes) — constitution Principle IV; lands in the same PR
- [ ] T033 Board close-out: `spec-bridge:sync` TASK-53; tick TASK-55 ACs (cure + never-diverge test shipped here) and close TASK-55 referencing the PR; open the single PR from `.worktrees/task-53`

## Dependencies & Execution Order

- **Setup (Phase 1)** → gates everything; T003's fixture MUST predate all refactor commits.
- **Foundational (Phase 2)** → blocks all stories. T005 ∥ T007 after T002; T006 needs T005; T008–T010 need T006+T007.
- **US1 (Phase 3)**: T011/T012 parallel-capable (different packages) after Phase 2; T013 after T008; T014/T015 independent of T011–T013; T016 after T014+T015; T017–T019 last.
- **US2 (Phase 4)**: T020/T021 [P] after Phase 2 (independent of US1 door work); T022 after T021; T023 after ALL US1+US2 code tasks; T024 after T023.
- **US3 (Phase 5)**: T025 after T013; T026 after T021; T027/T028 [P] after T025/T026.
- **Polish (Phase 6)**: after all stories; T032 before T033 (wiki gate precedes Done claim).

## Implementation Strategy

MVP = Phase 1–3 (US1): the registry exists, all three surfaces derive, duplicate maps
dead — the core deliverable, mergeable alone if needed. US2 then proves identity, US3
formalizes rosters; both are small on top of the foundation. One worktree, one branch,
one PR (constitution II); commits at every checkpoint. Implementation executes via the
`spec-implementer` agent — **Opus 4.8** for T005–T016, T020–T026 (cross-package,
door/replay-sensitive); Sonnet eligible for T017–T019, T027–T031 (tests alongside
landed code, docs, sweeps) — tier + rubric recorded on the board (T004).
