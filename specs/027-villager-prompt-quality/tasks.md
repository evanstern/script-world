# Tasks: Villager Prompt Quality

**Input**: Design documents from `/specs/027-villager-prompt-quality/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/system-prompt.md, quickstart.md

**Tests**: REQUESTED — the spec pins SC-002/SC-005 to automated tests and FR-008 to the passing stub suite, so contract tests are first-class tasks (written first, name-once failing red against the old prompt).

**Organization**: build order intentionally differs from story priority: US2 (the rewrite) and US3 (the exemplar variant) must exist before US1 (the eval gate) can compare them. US1 remains the P1 value — nothing ships without it.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: eval scaffolding so every later artifact has a tracked home

- [X] T001 Create `scripts/eval-prompt-73.sh` skeleton (usage stub, arg parsing: `<variant> <git-ref>`) and `specs/027-villager-prompt-quality/eval/` with a one-line README naming the record shape (data-model.md §2)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the frame contract, executable — written BEFORE the rewrite so C2 (name-once) is red against the old prompt and doctrine tests can't silently weaken

- [X] T002 Write contract tests C1–C5 from `specs/027-villager-prompt-quality/contracts/system-prompt.md` in `internal/mind/prompt_test.go`: purity/byte-identical renders (C1), name-exactly-once with sentinel name (C2 — MUST FAIL against the old prompt), doctrine meaning assertions incl. no free-text path (C3), persona block verbatim + clean empty-persona render (C4); C5 exemplar assertions added later in T006
- [X] T003 Add `TestPromptFrameReport` to `internal/mind/prompt_test.go`: renders the frame for a fixed sample agent (name + representative persona) and logs bytes / words / approx tokens (`len/4`, research D4)

**Checkpoint**: `go test ./internal/mind/` shows exactly one red test (C2 name-once); everything else green

---

## Phase 3: User Story 2 — The prompt reads as exemplary craft (Priority: P2, built first)

**Goal**: the three-part frame — one identity statement, persona block, tight task framing — doctrine preserved, cacheable purity intact (data-model.md §1 parts 1–3)

**Independent Test**: `go test ./internal/mind/` fully green (C1–C4); rendered frame inspectable via T003's report

- [X] T004 [US2] Rewrite `systemPrompt` in `internal/mind/prompt.go` per data-model.md §1: identity statement naming the agent once, persona as its own block (clean when empty), task framing in second person with doctrine meaning intact (FR-001/002/003/005); update the function's doc comment to match
- [X] T005 [US2] Run `go test ./...`; update any scripted-stub tests that pinned old wording in `internal/mind/` or `internal/toolloop/` without weakening what they test (FR-008); commit as the **`new`** variant ref (research D2)

**Checkpoint**: US2 complete — branch commit = variant `new`, all tests green

---

## Phase 4: User Story 3 — The exemplar question gets evidence (Priority: P3, variant build)

**Goal**: the `new+exemplar` variant exists as a commit so US1 can measure it (decision itself lands in US1)

**Independent Test**: `go test ./internal/mind/` green including C5 assertions

- [X] T006 [US3] Author the worked exemplar per research D5 (situation-generic, no real name, no literal JSON args, not muse-featuring), append as frame part 4 in `internal/mind/prompt.go`, add contract C5 assertions to `internal/mind/prompt_test.go`, run `go test ./...`; commit as the **`new+exemplar`** variant ref

**Checkpoint**: three refs exist — `origin/main` (old), `new`, `new+exemplar`

---

## Phase 5: User Story 1 — Villager decisions keep landing (Priority: P1) 🎯 the ship gate

**Goal**: before/after numbers on the scripted-stub suite + live soak; ship only on no-regression (FR-006/007)

**Independent Test**: `specs/027-villager-prompt-quality/eval/` holds one record per variant + a decision note whose numbers satisfy SC-001…SC-004

- [X] T007 [US1] Implement `scripts/eval-prompt-73.sh` per research D1–D3: build `promptworld` from `<git-ref>` into a temp dir; `new eval73-<variant> --seed 4242`; `start`; set speed; wait until world clock passes the 6-game-hour window; `stop`; `tail --since 0`; tally villager-planner `cog.tool_call` verdicts (join `cog.thought` class to exclude Metatron/conversation jobs) + acting-tool distribution; write `specs/027-villager-prompt-quality/eval/<variant>.md`
- [X] T008 [US1] Soak variant `old` (ref `origin/main`) → `specs/027-villager-prompt-quality/eval/old.md`; confirm ≥200 villager acting decisions else extend window for ALL variants (research D3)
- [X] T009 [US1] Soak variant `new` → `specs/027-villager-prompt-quality/eval/new.md` (serial, same machine, same window)
- [X] T010 [US1] Soak variant `new+exemplar` → `specs/027-villager-prompt-quality/eval/new-exemplar.md` (serial, same machine, same window)
- [X] T011 [P] [US1] Record per-variant token counts (bytes/words/approx from T003's report, run at each ref) into the matching `eval/<variant>.md` records (SC-004)
- [X] T012 [US1] Gate decision in `specs/027-villager-prompt-quality/eval/decision.md`: compare rejection rates (SC-001) and run the distribution screen (SC-003); pick the shipped variant per FR-004 (better, or equal-and-cheaper); if the exemplar loses, revert its commit so the branch tip is the shipped variant and record the measured rejection reason

**Checkpoint**: eval dir complete; branch tip = shipped variant; gate verdict written

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T013 Record the before/after numbers table, exemplar decision, and implementation-tier justification on TASK-73 via the `backlog` CLI (run from the repo ROOT, not the worktree) and tick ACs #1–#4 as proven
- [X] T014 Re-pin the wiki: `/grounding-wiki:wiki-update` for `docs/wiki/agent-mind.md` (sources `internal/mind/prompt.go`), committed on this branch
- [X] T015 Run `specs/027-villager-prompt-quality/quickstart.md` top to bottom as validation; prepare the single PR from `.worktrees/task-73`

---

## Dependencies & Execution Order

- **Phase 1 → 2**: T001 independent; T002/T003 need nothing from T001 but precede all code change
- **US2 (Phase 3)** depends on Phase 2 (tests must exist red-first)
- **US3 (Phase 4)** depends on US2 (exemplar stacks on the rewrite)
- **US1 (Phase 5)** depends on US2 + US3 (needs all three refs); T007 can be built in parallel with Phases 3–4; T008–T010 are strictly SERIAL (same machine, quiet box); T011 parallel to soaks; T012 last
- **Phase 6** depends on T012

### Parallel Opportunities

- T001 ∥ T002/T003
- T007 (eval driver) ∥ T004–T006 (different files)
- T011 ∥ T008–T010
- Soaks themselves are deliberately NOT parallel (research D3)

## Implementation Strategy

Single increment — the feature is one gated rewrite, not independently shippable story slices: land tests red (Phase 2), make them green with the rewrite (US2), add the exemplar variant (US3), then run the gate (US1) and let the numbers pick the tip. MVP = US2 + US1 without T006/T010 would still satisfy FR-001–003/005–008 but violate FR-004 (exemplar must be *evaluated*), so all three stories ship together in TASK-73's one PR.
