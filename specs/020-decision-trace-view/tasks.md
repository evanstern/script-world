# Tasks: Decision-Trace View

**Input**: Design documents from `specs/020-decision-trace-view/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/decision-trace-ui.md, quickstart.md

**Tests**: included — this project ships tests alongside code (constitution Principle V
routine-tier definition; every existing TUI feature is unit-tested in-package).

**Organization**: tasks grouped by user story; US1 (decisions sub-view) is the MVP.
All work lands in `internal/tui/` on the single TASK-63 branch.

## Phase 1: Setup

*(No scaffolding needed — existing package, existing toolchain. Worktree/branch
creation is orchestration, handled outside these tasks.)*

- [ ] T001 Create `internal/tui/decisions.go` with package doc comment naming the
      feature (spec 020, TASK-63) and `internal/tui/decisions_test.go` with the test
      file skeleton, both compiling empty (`go build ./internal/tui/`)

## Phase 2: Foundational (blocking all user stories)

**Purpose**: the projection and the glossary — every surface renders from these.

- [ ] T002 Define `decisionChain`, `decisionCall`, `decisionTraces` types with bounds
      constant `decisionChainCap = 20` and the Metatron sentinel key, per
      data-model.md, in `internal/tui/decisions.go`
- [ ] T003 Implement projection ingest `(*decisionTraces).ingest(e store.Event, names []string, ring []store.Event)`
      handling cog.thought / cog.tool_call / cog.outcome: job join, ordinal-ordered
      call insertion, attribution precedence (thought agent → outcome agent → villager
      job-ID regexp parse; `turn-metatron-` → sentinel; `conversation-` → skip),
      suppression detection (outcome with no thought/calls), per-agent cap eviction
      removing both indexes, in `internal/tui/decisions.go` (contract §1 R1–R4)
- [ ] T004 Implement stimulus resolution at thought-ingest: trigger_seq 0 → cadence
      phrase; ring hit → `formatChronicleLine` summary flattened to plain text; miss →
      neutral "stimulus #N (before this view connected)" reference, stored on the
      chain, in `internal/tui/decisions.go` (research D3, contract R6)
- [ ] T005 [P] Implement the verdict glossary `verdictPhrase(string) string` covering
      all 8 toolloop verdicts + all 10 sim outcome strings with plain-language
      phrases and a safe generic fallback for unknown strings (never the raw enum),
      in `internal/tui/decisions.go` (contract §4 R15, R17)
- [ ] T006 Wire the projection into the Model: add `traces decisionTraces` field,
      initialize at `New`/`connectedMsg` (reset on reconnect, contract R5), call
      `ingest` from `applyEvent` after the seq-skip guard and replica fold, in
      `internal/tui/tui.go`
- [ ] T007 Foundational tests in `internal/tui/decisions_test.go`: ingest join across
      the three event types; out-of-order ordinal insertion; fragment chains
      (tool_call-first, outcome-first) with job-ID-parse attribution; conversation-job
      skip; metatron-sentinel attribution; suppression detection; cap eviction (21st
      chain evicts oldest from both indexes); stimulus resolution (cadence / ring hit
      / ring miss); reconnect reset via the Model
- [ ] T008 [P] Glossary sweep test importing `internal/toolloop` verdict constants and
      `internal/sim` outcome constants: every constant has a non-empty phrase that
      does not equal the raw enum string; unknown-verdict fallback covered, in
      `internal/tui/decisions_test.go` (contract R16 — mechanical proof of SC-002)

**Checkpoint**: projection + glossary proven by unit tests; no UI yet.

## Phase 3: User Story 1 — "Why did my villager do that?" (P1) 🎯 MVP

**Goal**: decisions sub-view inside the villager detail pane rendering per-cognition
chains most-recent-first.

**Independent Test**: quickstart.md §2 — open villager detail → `d` → read chains
(stimulus, class, calls with plain-language verdicts + reasons, outcome); `j`/`k`
scroll; `esc` unwinds.

- [ ] T009 [US1] Add `villDecisions bool` + `villDecisionsScroll int` Model state with
      resets (villager change, detail close, reconnect) and extend
      `handleVillagersKey`: `d` toggles decisions while detail is open, `j`/`k`
      scroll while decisions is open, `esc` unwinds decisions → detail → roster ahead
      of the existing chain, in `internal/tui/tui.go` (contract R7)
- [ ] T010 [US1] Implement `villagerDecisionsBody(width, height int) string` rendering
      the selected villager's chains most-recent-first — when/class header, stimulus
      line, per-call `tool — phrase (reason)` rows, terminal outcome line or
      in-progress marker, suppression entries, explicit empty state — clipped to the
      row budget with render-time scroll clamp, in `internal/tui/views.go`
      (contract R9–R11)
- [ ] T011 [US1] Dispatch the decisions body from the villagers pane render path and
      add the `d decisions` hint to the detail view (and the decisions view's own
      footer/hint line), in `internal/tui/views.go` (contract R8)
- [ ] T012 [US1] US1 tests: key routing (`d` toggle gated on detail; scroll; esc
      unwind order; no silent no-ops), rendering (chain order, in-progress marker,
      suppression row, empty state, dead villager retains chains, exact-height
      clipping at small budgets, scroll reveal), in `internal/tui/decisions_test.go`
      and `internal/tui/villagers_test.go`

**Checkpoint**: US1 fully functional and independently testable — MVP.

## Phase 4: User Story 2 — Metatron's own verdict trail (P2)

**Goal**: inline verdict rows in the metatron transcript for `turn-metatron-*` calls.

**Independent Test**: quickstart.md §4 — a Metatron turn with tool calls shows one
verdict row per call, in order, before the reply row; prose-only turns add none.

- [ ] T013 [US2] Append a styled transcript row per ingested `turn-metatron-*`
      cog.tool_call (tool + glossary phrase + reason) from the applyEvent path,
      preserving the 200-row cap, in `internal/tui/tui.go` (contract R12–R14)
- [ ] T014 [US2] Add the verdict-row prefix to `classifyTranscriptLine` /
      `transcriptRowLines` so verdict rows style as telemetry (distinct from
      you/angel) and wrap correctly, in `internal/tui/views.go`
- [ ] T015 [US2] US2 tests: one row per metatron call in emission order; villager
      calls add no transcript rows; prose-only turn adds none; row cap holds;
      wrapping/styling classification, in `internal/tui/decisions_test.go`

**Checkpoint**: US2 independently testable on top of Foundational (does not need US1).

## Phase 5: User Story 3 — Legible to a non-engineer (P3)

**Goal**: prove and polish plain-language rendering across both surfaces.

**Independent Test**: quickstart.md §2 step 4 + §4 — force each rejection family;
no raw enum appears; suppressions read as "didn't think because…".

*(The glossary itself landed in Phase 2 — this phase is the proof + phrasing pass.)*

- [ ] T016 [US3] Surface sweep test: render a chain containing every verdict and an
      outcome of every kind through `villagerDecisionsBody` and the metatron
      transcript path, assert no raw verdict/outcome enum string appears in either
      surface's output, in `internal/tui/decisions_test.go` (SC-002 end-to-end)
- [ ] T017 [US3] Phrasing review pass against spec US3 acceptance scenarios: verdict
      phrases read as cause-first plain language ("the gate refused it because…",
      "its one action for this thought was already spent"), suppressions read
      "didn't think because…" with router reason; adjust glossary wording in
      `internal/tui/decisions.go` as needed

**Checkpoint**: all three stories complete.

## Phase 6: Polish & Cross-Cutting

- [ ] T018 Regression sweep: full `go test ./...`, `gofmt -l .` clean, `go vet
      ./internal/tui/` clean; confirm digest catalog sweep in
      `internal/tui/digest_test.go` passes unchanged (contract R18)
- [ ] T019 Run quickstart.md live validation (§2–§5) against a real world and record
      outcomes (including the ring-eviction check §3) in the implementation report

## Dependencies & Execution Order

- **Phase 2 blocks everything**: T002 → T003 → T004/T006; T005 [P] parallel with
  T003/T004; T007/T008 after their subjects.
- **US1 (Phase 3)**: depends only on Phase 2. T009 → T010 → T011 → T012.
- **US2 (Phase 4)**: depends only on Phase 2 — parallelizable with US1 (different
  render surfaces; both touch tui.go/views.go, so sequence commits or coordinate).
- **US3 (Phase 5)**: T016 depends on US1 + US2 renderers; T017 only on Phase 2.
- **Polish (Phase 6)**: last.

## Implementation Strategy

MVP = Phase 1 + Phase 2 + Phase 3 (US1): the decisions sub-view alone delivers the
teaching-goal payoff. US2 and US3 are additive increments on the same projection and
glossary. Single implementer, sequential phases; [P]-marked tasks (T005, T008) may
interleave. All commits on the one TASK-63 branch; one PR.
