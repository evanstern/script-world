# Tasks: Hail Protocol

**Input**: Design documents from `/specs/010-hail-protocol/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/events.md, quickstart.md

**Tests**: included — this project's constitution ships tests alongside code, and the
spec's Independent Test criteria are executable only as `internal/sim` unit tests.

**Organization**: grouped by user story; US1 is the MVP increment.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: branch/worktree per constitution Principle II

- [X] T001 Create worktree `.worktrees/task-47` on branch `task-47-hail-protocol` from fresh `origin/main` (`git worktree add .worktrees/task-47 -b task-47-hail-protocol origin/main`); all subsequent tasks execute inside it

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: state shape, event vocabulary, and reducer transitions every story sits on

**⚠️ CRITICAL**: no user story work until this phase completes

- [X] T002 Add `AgentHail` type (`By int`, `Until int64`) and `Hail *AgentHail json:"hail,omitempty"` field on `Agent`, plus `HailedPayload{From,To,Until}`, `HailMetPayload{From,To}`, `HailExpiredPayload{From,To}` payload structs (fields serialize as `from`/`to`/`until`) in internal/sim/agents.go
- [X] T003 Create internal/sim/hail.go with tunables `hailRadius = 64`, `hailWindowTicks = 480` and pure predicates `hailable(s, hailer, target)` and `hailPaused(a, tick)` per data-model.md (alive, awake, not already hailed, not an active hailer, not meeting-pinned, within radius)
- [X] T004 Add reducer cases in internal/sim/state.go: `social.hailed` sets target `Hail`, `social.hail_met` / `social.hail_expired` clear it; extend existing `agent.died` and `agent.slept` cases to clear `Hail` (contracts/events.md)
- [X] T005 [P] Foundational tests in internal/sim/hail_test.go: reducer lifecycle transitions (set → met/expired/died/slept clears) and snapshot round-trip — canonical bytes unchanged for un-hailed agents, `Hail` survives marshal/unmarshal mid-pause (FR-010)

**Checkpoint**: state machine exists and round-trips; no behavior yet

---

## Phase 3: User Story 1 - A talk_to decision survives target movement at speed (Priority: P1) 🎯 MVP

**Goal**: out-of-radius talk_to landings become adapted landings that hail + pause the
target; the pair meets and a talk is founded deterministically.

**Independent Test**: inject talk_to with target at distance 35 (beyond presentRadius,
inside hailRadius): lands adapted, `social.hailed` emitted, target frozen; walk hailer
adjacent → `social.hail_met` + `agent.talked` despite fresh cooldown.

### Implementation for User Story 1

- [X] T006 [US1] Add the hail rung to the landing ladder in internal/sim/loop.go `handleCommand` (`inject_intent`): on `target_present` guard failure for a `talk_to` landing, (a) target is actor's own hailer → proceed as adapted with no new hail; (b) `hailable` → proceed as adapted; otherwise reject with the existing reason (contracts/events.md rung order; guard.go stays untouched)
- [X] T007 [US1] Emit `social.hailed` (until = tick + `hailWindowTicks`) after every successful/adapted `talk_to` landing whose target is hailable, in internal/sim/loop.go — including in-radius landings (FR-001, research D2)
- [X] T008 [US1] Emit `social.hailed` on plan-step `talk_to` firing when target is hailable in internal/sim/plan.go `planStepEvents`
- [X] T009 [US1] Enforce the pause in internal/sim/executor.go per-agent step: an agent with active hail (`hailPaused`) skips the reflex branch, plan-step evaluation, and en-route movement; `executeAtTarget` still runs when already standing on its intent target; needs decay and social participation unaffected (FR-004, research D3)
- [X] T010 [US1] Implement the per-tick hail sweep met-branch in internal/sim/hail.go (`hailStep`), wired into `stepEvents` in internal/sim/executor.go before the per-agent loop: hailer within Manhattan ≤ 1 of paused target → emit `social.hail_met` + `agent.talked` + the ambient beat's relation/memory event shape, bypassing `canTalk` (FR-006, research D4)
- [X] T011 [P] [US1] US1 tests in internal/sim/hail_test.go: distance-35 landing → adapted outcome + hail + pause; in-radius landing also hails; arrival founds talk despite fresh `LastTalk`; paused target emits no `agent.moved` across the window; hailer's seek completes normally

**Checkpoint**: MVP — conversations survive movement at speed in unit-verifiable form

---

## Phase 4: User Story 2 - A stood-up target resumes its life safely (Priority: P2)

**Goal**: bounded, non-destructive pause — expiry restores the target exactly.

**Independent Test**: hail a target whose hailer never approaches; advance past the
window; `social.hail_expired` recorded, intent/plan byte-identical, movement resumes.

### Implementation for User Story 2

- [X] T012 [US2] Add the expiry branch to `hailStep` in internal/sim/hail.go: `tick >= Until` and hailer not adjacent → emit `social.hail_expired`; adjacency (met) checked first so met wins the same-tick race (FR-005, data-model.md)
- [X] T013 [P] [US2] US2 tests in internal/sim/hail_test.go: expiry event lands at the right tick; target's `Intent` and `Plan` byte-identical pre-pause vs post-expiry (SC-003); needs kept decaying during pause; movement resumes on the next move-cadence tick after expiry

**Checkpoint**: US1 and US2 independently green

---

## Phase 5: User Story 3 - Un-interruptible villagers are left alone (Priority: P2)

**Goal**: exemptions hold and the protocol can never deadlock or chain pauses.

**Independent Test**: hail attempts against asleep/dead/meeting-pinned/already-hailed/
active-hailer targets emit no hail; out-of-radius landings against them reject as today.

### Implementation for User Story 3

- [X] T014 [US3] Verify/complete exemption coverage end-to-end: `hailable` exemptions from T003 exercised through the landing path in internal/sim/loop.go (no hail event, fallback to today's present-radius verdict) and through plan-step firing in internal/sim/plan.go (FR-009)
- [X] T015 [P] [US3] US3 tests in internal/sim/hail_test.go: full hailable matrix (asleep, dead, meeting-pinned, already-hailed, active-hailer, out-of-hail-range); mutual-hail scenario ends in a meeting, never two frozen agents (research D6); second hail against a paused target neither re-targets nor extends `Until` (spec US3-3)

**Checkpoint**: all behavioral stories functional

---

## Phase 6: User Story 4 - Hails are visible to the observer (Priority: P3)

**Goal**: hail lifecycle is legible in tail/TUI with agent attribution.

**Independent Test**: grammar renders `social.hailed`/`social.hail_met`/
`social.hail_expired` with `from`/`to` resolved to agent names.

### Implementation for User Story 4

- [X] T016 [US4] Add table-driven cases to internal/tui/grammar_test.go asserting the three hail event types render with resolved agent names via the existing `from`/`to` resolution (classDefault path — no grammar.go change expected; if resolution gaps surface, fix in internal/tui/grammar.go)

**Checkpoint**: all four stories done

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T017 Extend the replay/determinism coverage in internal/sim/sim_test.go (or hail_test.go using the same harness): a scripted run containing hailed → met and hailed → expired sequences replays from the event log to an identical state hash (SC-004)
- [X] T018 Full gate inside the worktree: `go build ./... && go vet ./... && go test ./...` all green (quickstart.md)
- [ ] T019 Live before/after measurement per quickstart.md on the baseline world shape (local tier, 8x): record "is gone" rejection count, conversation count, and hail met/expired mix on TASK-47 via `backlog task edit TASK-47 --append-notes` (SC-001, SC-002)
- [X] T020 Post-merge re-grounding: run `/grounding-wiki:wiki-update` for wiki notes whose sources changed (sim-loop, executor, sim-state-reducer, event-types, cognition) — constitution Principle IV

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2**: worktree first; T002 → T003 (types before predicates), T004
  after T002; T005 after T002–T004
- **Phase 3 (US1)**: T006/T007 after T003+T004 (same file, sequential); T008 after
  T003; T009 after T003; T010 after T004; T011 after T006–T010
- **Phase 4 (US2)**: T012 after T010 (same function); T013 after T012
- **Phase 5 (US3)**: T014 after T006–T008; T015 after T014
- **Phase 6 (US4)**: T016 independent after Phase 2 (only needs event shapes)
- **Phase 7**: T017 after all behavior tasks; T018 after everything code; T019 after
  T018 (and after the PR's build is runnable); T020 after merge

### Parallel Opportunities

- T005 alongside early Phase 3 reading; T008 ∥ T009 (different files); T011 ∥ T013 ∥
  T015 (test authoring, same file — coordinate or serialize commits); T016 ∥ any
  Phase 3–5 work (different package)

### Within-story order

Types → predicates → reducer → emitters → executor wiring → tests.

## Implementation Strategy

Single implementer, sequential phases (the feature is one surgical package). MVP
checkpoint after Phase 3; Phases 4–6 are small increments on the same mechanism.
T019 (live measurement) is the only wall-clock-expensive step and runs once at the
end. One branch, one PR (TASK-47).
