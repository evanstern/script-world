# Tasks: Grounded Memories — Situated Episodic Capture & Agent-Authored Journal

**Input**: Design documents from `/specs/019-grounded-memories/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/memory-context.md, contracts/journal-tools.md, quickstart.md

**Tests**: REQUIRED — replay determinism is a spec invariant (FR-007/011/014); unit tests ship alongside code per constitution tier doctrine.

**Organization**: grouped by user story. US4 (replay fidelity) is the integration proof over US1–US3 and runs last as its own phase.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch/worktree per constitution Principle II; no scaffolding needed (existing module).

- [ ] T001 Create worktree `.worktrees/task-16` on branch `task-16-grounded-memories` from fresh `origin/main`; all subsequent tasks execute inside it

---

## Phase 2: Foundational (blocking US1, US2, US4)

**Purpose**: the shared memory-context data shapes every story reads/writes.

- [x] T002 Add `MemoryPlace` type and extend `MemoryAddedPayload` with `Where *MemoryPlace`, `Why string`, `Conv int64` (all omitempty) in internal/sim/agents.go per data-model.md §1–2
- [x] T003 Extend `Memory` with the same three fields (omitempty) in internal/sim/agents.go per data-model.md §3
- [x] T004 Copy `Where`/`Why`/`Conv` from payload to `Memory` in the `agent.memory_added` Apply arm in internal/sim/state.go; unit test: pre-019 payload (fields absent) reduces to a pre-019-shaped Memory in internal/sim/state_test.go (or the existing reducer test file)

**Checkpoint**: `go build ./... && go test ./internal/sim/` green; no behavior change yet.

---

## Phase 3: User Story 1 — Situated deterministic memories (P1)

**Goal**: every executor memory carries where/why (structured + in the text); soul.md renders it.

**Independent test**: quickstart §1 grammar/unit tests + §3 live smoke — memories show `· at <desc> (x,y)` and `· why:` for planner-driven acts, place-only for reflex.

- [x] T005 [US1] Add `Reason string json:"reason,omitempty"` to `Intent` in internal/sim/agents.go and populate it from the `agent.intent_set` payload's existing `Reason` in that event's Apply arm in internal/sim/state.go (research R2); unit test: planner intent carries reason, reflex intent carries ""
- [x] T006 [P] [US1] Implement deterministic `describePlace(s *State, x, y int) string` (same-tile feature, then fixed-radius "near <feature>", else "") in internal/sim/memory.go or internal/sim/journal.go-adjacent location per research R3, with table-driven unit test in internal/sim/memory_test.go
- [x] T007 [US1] Add situated constructor variants (`situatedMemoryEvent`, situated `memoryAboutEvent`/`memoryEventToned` forms) and the shared where/why text-grammar helper in internal/sim/memory.go per contracts/memory-context.md grammar; unit test pins exact composed strings (with/without desc, with/without why)
- [x] T008 [US1] Migrate executor memory call sites (hunt, spear-broke, fire, shelter, starving-forage, oven, chest, talk at internal/sim/executor.go:671–740 and :374, plus witness memories) to the situated variants, baking `Where` (agent tile + describePlace) and `Why` (`in.Reason`) at emission
- [x] T009 [US1] Render situated suffixes (`· at <desc> (x,y)` / `· at (x,y)` / `· why: <reason>`) from reduced `Memory` in the soul.md memory line in internal/scribe/scribe.go per contracts/memory-context.md; scribe unit test pins new-format line AND pre-019 memory rendering byte-identical to today's format in internal/scribe/scribe_test.go

**Checkpoint**: US1 independently demonstrable via quickstart §3.

---

## Phase 4: User Story 2 — Conversation memories reference their transcript (P1)

**Goal**: gist memories carry the conversation id; transcript recoverable from the log alone.

**Independent test**: quickstart §4 — sqlite query by `conv` returns the full ordered dialogue.

- [x] T010 [US2] Set `Conv: cc.conv` (and `Where` from the remembering agent's position) on the gist `MemoryAddedPayload` at internal/mind/convo.go:337 per research R5
- [x] T011 [US2] Render the `[conv <id>]` marker on conversation memory lines in internal/scribe/scribe.go (extends T009's line format); update scribe unit test
- [x] T012 [US2] Integration test: run a scripted conversation through the mind test harness, then recover the full ordered transcript from the event log using only `Memory.Conv` (event type `social.conversation_turn`, payload `conv`), asserting speaker + verbatim text per contracts/memory-context.md, in internal/mind/convo_test.go (or existing convo test file)

**Checkpoint**: US1+US2 together close Layer 1.

---

## Phase 5: User Story 3 — Agent-authored journal (P2)

**Goal**: per-agent journal, four mind tools, 4,000-rune reducer-enforced budget, journal.md view.

**Independent test**: quickstart §1 gates + §5 — scripted-driver test drives write→search→read→delete; over-budget write rejected at the door with journal unchanged.

- [x] T013 [P] [US3] Create internal/sim/journal.go: `Journal`/`JournalEntry` types, `journalBudgetRunes=4000`, `journalWriteCapRunes=1000`, `journalSearchResultCap=8`, `JournalWrittenPayload`/`JournalDeletedPayload`, and Apply helpers (append-with-NextID, budget-error, delete-by-id-with-absent-error) per data-model.md §5–7; add `Journal Journal json:"journal,omitempty"` to `Agent` in internal/sim/agents.go
- [x] T014 [US3] Wire `journal.entry_written`/`journal.entry_deleted` Apply arms into the reducer switch in internal/sim/state.go and add both types to `injectSocialWhitelist` in internal/sim/loop.go; reducer unit tests in internal/sim/journal_test.go: id stability, budget rejection leaves state untouched, delete-unknown-id errors, freed-budget reuse
- [x] T015 [P] [US3] Register the four tools (`write_journal_entry`, `delete_from_journal` Expressive with `Events`; `search_journal`, `read_journal` Read) in internal/tool/registry.go with guidance-free `PromptGloss` (states capability + budget number only) per contracts/journal-tools.md, and add all four to `LoopRosterVillager()` in internal/tool/roster.go; registry/roster unit tests; confirm `tool.Validate()` + `sim.ValidateToolCoverage()` pass (boot-gate tests)
- [x] T016 [P] [US3] Add `JournalPath` to internal/persona/files.go and seed an empty journal.md at genesis alongside soul.md
- [x] T017 [US3] Implement mind handlers in internal/mind/handlers.go per contracts/journal-tools.md: write (InjectSocial batch + cog.outcome, `rejected_gate` with budget reason on dry-run failure), delete (`rejected_gate` "no journal entry #id"), search (case-insensitive substring, newest-first, cap 8, explicit empty read_ok), read (entry or whole journal, `read_error` on unknown id); register in `villagerHandlers` in internal/mind/mind.go; unit tests via the scripted loop driver (`runLoopOverride` seam) covering the full write→search→read→delete cycle and the over-budget rejection feedback in internal/mind/handlers_test.go
- [x] T018 [US3] Render `agents/<name>/journal.md` in internal/scribe/scribe.go (`renderJournal`, dirty-marked on `journal.*` events in the run-loop switch) per contracts/journal-tools.md view contract; scribe unit test pins the render (header with used/budget runes, `## <clock> (#id)` sections, verbatim entry text)

**Checkpoint**: US3 independently demonstrable via quickstart §5.

---

## Phase 6: User Story 4 — Faithful replay of memories and journals (P1, integration proof)

**Goal**: byte-identical souls + journals live vs replay; zero model calls; pre-019 logs unaffected.

**Independent test**: quickstart §2.

- [ ] T019 [US4] Extend the replay/determinism suite with a fixture exercising situated memories (planner reason + reflex), a conversation, and journal write/delete/over-budget-rejection; assert live-vs-replay `State` equality and byte-identical soul.md + journal.md renders with zero orchestrator calls, in the existing determinism test location (internal/sim and/or the daemon-level replay test)
- [ ] T020 [P] [US4] Add/verify a pre-019 fixture log replays unchanged and renders byte-identically to its pre-019 output (FR-014/SC-007) in the same suite

**Checkpoint**: all four stories proven; SC-003/SC-007 closed.

---

## Phase 7: Polish & Cross-Cutting

- [ ] T021 [P] Run quickstart §3–§6 live smoke on a throwaway world (local tier up); record observed situated lines, a conv transcript recovery, and any journal tool calls on the board task as evidence
- [ ] T022 [P] `go vet ./...` + full `go test ./...` green; fix fallout
- [ ] T023 Reconcile spec-014 tool catalog contract (specs/014-tool-registry/contracts/tool-catalog.md) with the four new registry entries if that contract enumerates tools exhaustively (check first; skip with a note if additive entries don't belong there)

*Post-merge (not part of this PR)*: `/grounding-wiki:wiki-update` re-pins notes whose sources changed (agent-mind, executor, sim-state-reducer, event-types, tool-registry, tool-loop, social-fabric); spec-bridge:sync moves the board.

---

## Dependencies

```text
T001 → everything
Phase 2 (T002→T003→T004) → US1, US2, US4
US1: T005, T006 [P] → T007 → T008 → T009
US2: T010 → T011, T012   (needs Phase 2 only — can start parallel to US1 after T004; T011 merges into T009's line format, so T009 first or coordinate)
US3: T013 → T014 → T017; T015 [P with T013/T014]; T016 [P]; T018 after T013
     (US3 needs NOTHING from US1/US2 — only Phase 1; may run parallel to Phases 3–4)
US4: T019 after US1+US2+US3; T020 [P with T019]
Polish: T021–T023 after US4
```

## Parallel opportunities

- After T004: US1 (T005/T006 in parallel) and US2 prep and all of US3 (T013, T015, T016 in parallel) can proceed concurrently — US3 shares no files with US1/US2 except scribe.go (coordinate T009/T011/T018) and agents.go (small, sequential edits)
- T019/T020 parallel within US4; T021/T022 parallel in Polish

## Implementation strategy

MVP = Phase 2 + US1 (situated memories visible in souls) — already demonstrable value. Then US2 (small), then US3 (the experiment), then US4 seals the invariant. Single branch/PR throughout (TASK-16); phases are internal breakdown, committed incrementally in dependency order.

**Tier note (constitution V)**: T002–T014, T017 touch reducer/doors/whitelist/mind orchestration — doctrine-adjacent + cross-package ⇒ Opus 4.8 implementer. T015–T016, T018, T009/T011 rendering and T019–T023 are within reach of Sonnet but ride the same slices in practice; tier recorded on TASK-16 at delegation.
