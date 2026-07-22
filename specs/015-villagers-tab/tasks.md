# Tasks: Villagers Tab — per-villager inspection

**Input**: Design documents from `/specs/015-villagers-tab/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/state-and-keys.md

**Tests**: included — the project's testing strategy is tests-alongside-code, and quickstart.md names the expected suites.

**Organization**: grouped by user story; US1 is the MVP increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependencies)
- **[Story]**: US1 (select & inspect), US2 (last objective), US3 (soul content)

## Phase 1: Setup

**Purpose**: branch + baseline per the worktree discipline

- [x] T001 Create the task worktree (`git worktree add .worktrees/task-<N> -b task-<N>-villagers-tab origin/main` after ff-pulling root) and verify baseline: `go build ./... && go test ./internal/sim/ ./internal/tui/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the reducer-maintained last-goal fields (research.md R1) and the tab rename (R3) — every story renders on top of these

- [x] T002 Add `LastGoal string` (`json:"last_goal,omitempty"`) and `LastGoalTick int64` (`json:"last_goal_tick,omitempty"`) to `Agent` in internal/sim/agents.go with a doc comment stating the write rule (set on agent.intent_set, never cleared; omitempty for pre-feature byte-stability, precedent Generation/Plan/Hail)
- [x] T003 In `State.Apply` case `"agent.intent_set"` (internal/sim/state.go:313), also set `a.LastGoal = p.Goal; a.LastGoalTick = e.Tick`
- [x] T004 [P] Sim tests in internal/sim (new or existing suite file): intent_set sets both fields; intent_done clears Intent but preserves LastGoal/LastGoalTick; gru.attacked preserves them; a replay-determinism pass over a timeline with set→done→set; decoding an agent JSON without the fields yields zero values ("never")
- [x] T005 Rename the tab across internal/tui: `paneSouls`→`paneVillagers`, `soulsView`/`soulsBody`→`villagersView`/`villagersBody`, `paneNames[3]` "souls"→"villagers", footer hint (internal/tui/views.go:119) and header "SOUL READER"→"VILLAGERS", tab-row label (views.go:236); update every existing test in internal/tui that references souls names/strings so the suite is green before any new behavior lands

**Checkpoint**: `go test ./internal/sim/ ./internal/tui/` green; tab renders as before under its new name; SC-003 satisfiable (`grep -ri soul internal/tui` shows no user-visible strings)

---

## Phase 3: User Story 1 — Select a villager and inspect them (P1) 🎯 MVP

**Goal**: keyboard selection over the roster + a live per-villager detail view (identity/vitals, itemized inventory, objective)

**Independent Test**: quickstart.md manual steps 1–3, 6–7; grammar/render suites below

- [x] T006 [US1] Add `villSelected int` and `villDetail bool` to `Model` in internal/tui/tui.go (zero values: cursor 0, detail closed); clamp `villSelected` against `len(m.replica.Agents)` wherever read, and keep state across tab switches and reconnect (`connectedMsg` handler ~tui.go:280 clamps, mirroring chronSelected clamping at tui.go:827)
- [x] T007 [US1] Key handling in internal/tui/tui.go, bound only while the villagers tab is the visible dock tab or solo (contract table): `j`/`k` move selection, `g`/`G` first/last, `⏎` opens detail (strict no-op when replica nil/empty), `esc` closes detail before the existing solo-release chain (focus-contract rule 3: minibuffer → detail → solo → home); no collisions with chronicle-inspect j/k (different visible tab) or arrow-key map pan
- [x] T008 [US1] Roster cursor rendering in `villagersBody` (internal/tui/views.go): selection glyph on the selected row in both wide (≥40 col) and narrow layouts, preserving today's columns and the drop-trailing-agents height rule
- [x] T009 [US1] Detail view renderer in internal/tui/views.go: when `villDetail`, render the selected villager within (width,height) budget — identity/vitals (name, awake/asleep/dead incl. dead villagers, position, needs bars), objective line (active `Intent.Goal`+target marked current; else `LastGoal`+tick marked past; else "no objective yet"), itemized inventory (all kinds incl. spear wear; empty pack stated plainly); sections truncate from the bottom, narrow width condenses per contract
- [x] T010 [US1] Footer hints (internal/tui/views.go): global hint says `4 villagers`; while the villagers tab is visible advertise `j/k select · ⏎ inspect`, and in detail `esc back`
- [x] T011 [P] [US1] Grammar/focus tests in internal/tui/focus_test.go (or new file): j/k/g/G move+clamp, ⏎ opens, esc chain detail→solo→home, nil-replica no-ops, selection survives tab switch and reconnect clamp
- [x] T012 [P] [US1] Render tests in internal/tui/render_test.go: roster cursor present, detail sections in priority order, live-update (render reflects mutated replica without re-selection), narrow/short budgets never overflow, no "soul" in any rendered string

**Checkpoint**: US1 fully functional — MVP deliverable

---

## Phase 4: User Story 2 — Most recent objective when idle (P2)

**Goal**: idle villagers still show their last objective, correctly marked; fresh attaches see it via snapshot

**Independent Test**: quickstart.md steps 4 and 9; tests below

- [x] T013 [US2] Verify/finish the objective display states in the detail renderer (internal/tui/views.go): active (marked current) / past (`LastGoal` + tick, visually distinct e.g. "last:") / "no objective yet" — exactly the derived-state table in data-model.md
- [x] T014 [P] [US2] Render tests in internal/tui/render_test.go covering all three objective states, including an agent with `Intent == nil` and non-empty `LastGoal` (idle after work) and a zero-value agent (never worked)
- [x] T015 [US2] Snapshot round-trip test in internal/sim: marshal a state where an agent finished an intent, unmarshal (fresh-attach simulation), confirm LastGoal/LastGoalTick survive — proving FR-006's "freshly attached observer" path

**Checkpoint**: idle inspection answers "what did they last do?"

---

## Phase 5: User Story 3 — Memories, beliefs, narrative (P3)

**Goal**: the soul-depth layer in the detail view, budget-bounded

**Independent Test**: quickstart.md step 5; tests below

- [x] T016 [US3] Memories section in the detail renderer (internal/tui/views.go): most-recent-first (iterate `Memories` from the end), formatted via the existing memory line formatting (`FormatMemory`, internal/sim/memory.go:172) or a width-aware equivalent, bounded to remaining height; "no memories yet" when empty
- [x] T017 [US3] Beliefs + narrative section (internal/tui/views.go): render `Beliefs` and `Narrative` when present, between inventory and memories per the contract's section order; omitted silently when absent
- [x] T018 [P] [US3] Render tests in internal/tui/render_test.go: most-recent-first ordering, height-bounded truncation (memories shed first, identity/objective never pushed off), empty-memories message, beliefs/narrative shown when present and absent when not

**Checkpoint**: all three stories complete

---

## Phase 6: Polish & Cross-Cutting

**Purpose**: design-doc truth, full validation, grounding freshness

- [x] T019 [P] Update docs/design/tui/panels/dock.md: "Tab: souls" → "Tab: villagers" with the new roster+selection+detail spec (keys, section order, budgets) replacing "content unchanged"
- [x] T020 [P] Update docs/design/tui/patterns/keymap.md: `4` label, villagers-tab key additions (j/k/g/G/⏎/esc scoped to the visible tab), footer hints block
- [x] T021 [P] Update docs/design/tui/pages/solo-views.md and docs/design/tui/pages/home.md wherever "souls" is named
- [x] T022 Full validation: `go build ./... && go vet ./... && go test ./...`; run quickstart.md manual steps against a live world; `grep -ri soul internal/tui docs/design/tui` shows no user-visible/tab references remaining
- [ ] T023 Open the single PR from `.worktrees/task-<N>` (one task, one PR); after merge: `/grounding-wiki:wiki-update` for notes sourcing internal/tui/* and internal/sim/agents.go|state.go (tui-client.md, sim-state-reducer.md at minimum), worktree cleanup, root ff-pull

---

## Dependencies & Execution Order

- Phase 1 → Phase 2 → Phase 3 (US1) → Phase 4 (US2) → Phase 5 (US3) → Phase 6
- US2/US3 are additive layers on US1's detail renderer (same function) — priority order is also the natural build order; US2 and US3 touch disjoint sections and could swap if needed.
- Within phases, [P] tasks touch different files (or test-only files) and can run in parallel with their peers once their prerequisites exist.

## Parallel Example (Phase 3)

After T006–T010 land: T011 (focus_test.go) ∥ T012 (render_test.go).
Phase 6: T019 ∥ T020 ∥ T021.

## Implementation Strategy

MVP = Phases 1–3 (US1): a selectable roster with a live detail view showing
inventory and the objective. US2 costs almost nothing extra (the fields land
in Phase 2 foundational); US3 completes the soul depth. Stop-and-ship is
viable after any checkpoint.
