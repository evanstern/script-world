# Tasks: Behavioral Test Coverage for Metatron and Persona Packages

**Input**: Design documents from `/specs/023-metatron-persona-tests/`

**Prerequisites**: plan.md, spec.md, research.md (R1 gap list is binding), data-model.md, quickstart.md

**Tests**: This feature IS tests — every "implementation" task writes a behavioral test. The anti-duplication rule from research.md R1 is binding: do NOT re-test the instruction surface (fixed frame, skills order, manifest gating), charter fallbacks, digest windows, nudge/miracle landing, or charge decrement/zero-refusal — all already covered.

**Organization**: grouped by the spec's user stories. US1 = metatron economy/serialization gaps, US2 = context-window (tail) gaps, US3 = persona lifecycle gaps. Wiki note is polish.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different test functions, no ordering dependency)
- Test names below are canonical — use them verbatim so quickstart.md §3 spot-checks work.

## Path Conventions

Single Go module. New tests extend the existing white-box suites:
`internal/metatron/metatron_test.go` (or a sibling `metatron_gaps_test.go` if the implementer prefers by size) and `internal/persona/persona_test.go`. All conventions per research.md R5.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: the one shared fixture the new metatron tests need

- [x] T001 Add a live-angel test constructor (e.g. `newLiveTestAngel(t, reply)`) in internal/metatron/metatron_test.go: identical to `newTestAngel` but does NOT `Close()` after `New` — registers `t.Cleanup(mt.Close)` instead — so the absorb (`run()`) and digest goroutines stay alive for Observe-driven tests. Reuse `mockOrch`/`stateInjector` unchanged.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: none — the packages' existing stubs and fixtures (research.md R5) suffice; no new infrastructure beyond T001.

**Checkpoint**: after T001, all three user stories can proceed in parallel.

---

## Phase 3: User Story 1 - Metatron economy and turn dispatch are provably correct (Priority: P1) 🎯 MVP

**Goal**: close the economy/serialization gaps — charge cap + regeneration through the replica seam, true concurrent turn serialization, notify backpressure, absorb-mirror refresh. (Charge decrement, zero-bank refusal, and per-kind dispatch are already covered — R1; do not duplicate.)

**Independent Test**: `go test -race ./internal/metatron/ -run 'ChargeMirror|TurnBusyConcurrent|ObserveNeverBlocks|AbsorbRefreshesMirrors' -v`

### Implementation for User Story 1

- [x] T002 [P] [US1] `TestChargeMirrorAccrualAndCap` in internal/metatron/metatron_test.go: using the live angel (T001), deliver `metatron.charge_regenerated` events via `Observe` and assert `Status().Charges` accrues +1 per event and NEVER exceeds `sim.MetatronChargeCap` (deliver cap+2 events); then a `metatron.nudged` event decrements the mirror. Poll `Status()` with a bounded deadline loop (channel-paced, no sleeps as the only gate) so the test is race-clean and fails in seconds, never hangs (TASK-69 lesson; spec AC US1-2/US1-3, FR-002).
- [x] T003 [P] [US1] `TestTurnBusyConcurrent` in internal/metatron/metatron_test.go: install a scripted `runLoop` that parks on a release channel; start `Turn` A in a goroutine, wait until it is provably inside the loop (a "loop entered" signal channel), call `Turn` B and assert it returns `ErrTurnBusy` immediately; release A and assert it completes normally with its reply intact. Two real goroutines contending on the CAS — meaningful under `-race` (spec AC US1-4, FR-006; upgrades the manual-flag `TestTurnSingleFlight`, which stays).
- [x] T004 [P] [US1] `TestObserveNeverBlocks` in internal/metatron/metatron_test.go: with the standard closed-goroutine angel (absorb not draining), send >256 batches through `Observe` and assert it returns promptly every time (bounded total wall time, e.g. done-channel + timeout guard) — the notify path drops rather than wedges the loop (data-model.md §1 notify backpressure).
- [x] T005 [US1] `TestAbsorbRefreshesMirrors` in internal/metatron/metatron_test.go: using the live angel (T001), Observe a batch containing an `agent.died` event and assert (bounded poll) the next `Turn`'s user prompt (captured via the bridged mock) lists the villager under "Departed:" and the alive map excludes them; also assert the chronicle story mirror carries at most the last 8 entries after applying >8 chronicle-bearing events. Covers the `run()`/`mirrorState` pipeline end-to-end (data-model.md §1 absorb mirrors). Runs after T002 if sharing the live-angel helper file region, otherwise parallel.

**Checkpoint**: US1 independently green under `-race`.

---

## Phase 4: User Story 2 - Instruction-surface composition is pinned by tests (Priority: P2)

**Goal**: close the ONLY remaining instruction-context gap — the soul/transcript tail windows (spec AC US2-5). Everything else in US2 (fixed frame, provenance, skills order, manifest gating: AC US2-1..4) is already thoroughly covered by `TestFixedFrameHolds`, `TestStatusProvenance`, `TestLoadSkills`, `TestPromptDeterminism`, `TestLoadManifest`, `TestGatingLayers`, `TestNoManifestByteCompat` — R1 forbids duplicating them.

**Independent Test**: `go test -race ./internal/metatron/ -run 'TailOfFile|SoulTailWindow|TranscriptTailTurns' -v`

### Implementation for User Story 2

- [x] T006 [P] [US2] `TestTailOfFile` in internal/metatron/metatron_test.go: table test over the low-level reader — file longer than n → exactly the trailing n bytes; file shorter than n → whole file; missing file → `""`; empty file → `""` (data-model.md §1 tail windows).
- [x] T007 [P] [US2] `TestSoulTailWindow` in internal/metatron/metatron_test.go: write a soul.md well over 4000 bytes with a distinctive head marker and tail marker; assert `soulTail()` is exactly 4000 bytes (`soulTailBytes`), contains the tail marker, excludes the head marker; and that a `Turn`'s user prompt carries the windowed tail, not the full soul (spec AC US2-5, FR-006).
- [x] T008 [P] [US2] `TestTranscriptTailTurns` in internal/metatron/metatron_test.go: append >6 well-formed `[<clock>]`-delimited turns (within the 3000-byte read so the turn-trim rule is what's under test); assert `transcriptTail()` returns exactly the last 6 whole turns (`transcriptTailTurns`), oldest first / newest last, with no partial leading turn (spec AC US2-5, FR-006).

**Checkpoint**: US1 + US2 independently green.

---

## Phase 5: User Story 3 - Persona lifecycle guarantees are enforced by tests (Priority: P3)

**Goal**: close the persona gaps — index-aligned map sweep, anchor≡temperament invariant, unreadable-file degrade, genesis charter/journal seeding, `SecretEvents`. (Genesis-once, 0444 mode, missing-file load already covered — keep those tests untouched.)

**Independent Test**: `go test -race ./internal/persona/ -v`

### Implementation for User Story 3

- [x] T009 [P] [US3] `TestPersonaMapsSweepAligned` in internal/persona/persona_test.go: the sweep — for every `sim.AgentNames` entry, `Texts`, `Anchors`, `DriftMarkers`, and `Secrets` each contain a non-empty entry (DriftMarkers additionally a non-empty list of non-empty words); and none of the four maps carries a key outside `sim.AgentNames`. Gaining or losing an entry in any one map fails the sweep (spec AC US3-5, FR-007, SC-004).
- [x] T010 [P] [US3] `TestAnchorsMatchTemperamentLine` in internal/persona/persona_test.go: each `Anchors[name]` string appears verbatim inside its persona's `**Temperament:**` line in `Texts[name]` — the documented "deliberately identical" invariant (research.md R1; FR-007).
- [x] T011 [P] [US3] `TestLoadUnreadableDegrades` in internal/persona/persona_test.go: genesis a temp world, `os.Chmod` one persona.md to 0o000 (restore via `t.Cleanup`; skip if running as root since root ignores modes), assert `Load` yields `""` for that agent and intact text for all others — read-error degrade mirrors the missing-file contract (research.md R2; spec AC US3-3/US3-4).
- [x] T012 [P] [US3] `TestGenesisSeedsCharterAndJournal` in internal/persona/persona_test.go: fresh genesis seeds `charter.md` equal to `DefaultCharter` and a `journal.md` per agent bearing the rune-budget header; a world with a pre-existing custom `charter.md` keeps it byte-identical through... (note: `Genesis` errors on existing personas, so assert never-overwrite by seeding charter.md alone in an empty world dir before genesis) — the charter belongs to the player after first write (data-model.md §2 genesis seeding).
- [x] T013 [P] [US3] `TestSecretEvents` in internal/persona/persona_test.go: exactly one event per `sim.AgentNames` entry, each `Tick` 0 and type `social.secret_seeded`, payload `Agent` index-aligned with the name order, `Tone` −70, `Text` equal to `Secrets[name]` (data-model.md §2 secret genesis).

**Checkpoint**: all three stories independently green.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: whole-suite proof and the grounding re-pin

- [x] T014 Run `go test -race ./...` from the worktree root and confirm the full suite passes with no material duration regression (~25 s baseline); run quickstart.md scenarios 3 and 5 (named-test spot-check; `git diff` shows no non-test/non-docs changes) (spec SC-003, FR-011).
- [x] T015 Update docs/wiki/testing-strategy.md: add a section narrating the metatron/persona behavioral suites (what the packages' own tests now prove: economy mirror, turn serialization, tail windows, persona sweep/lifecycle), and add `internal/metatron/metatron_test.go` + `internal/persona/persona_test.go` to `sources:` (FR-010; final `verified_against` re-pin lands post-merge via `/grounding-wiki:wiki-update` per the board plan).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: first — US1's live-angel tests depend on it.
- **Foundational**: none.
- **US1 (T002–T005)**: T002/T005 need T001; T003/T004 do not (can start immediately).
- **US2 (T006–T008)**: independent of T001 and of US1 — can start immediately.
- **US3 (T009–T013)**: fully independent (different package) — can start immediately.
- **Polish (T014–T015)**: after all stories.

### Parallel Opportunities

- T003, T004, T006–T013 have no shared prerequisites and touch independent test functions; US3 lives in a different package entirely.
- Practical batches: {T001 → T002, T005} ∥ {T003, T004} ∥ {T006–T008} ∥ {T009–T013}, then T014–T015.

---

## Implementation Strategy

MVP is US1 (the economy/serialization gaps are the highest-value contracts). Each story is a checkpointed, independently green increment; commit per phase or logical group on the single task branch `task-74-metatron-persona-tests` (one TASK, one PR). The R1 anti-duplication rule is a review gate: any new test that re-asserts an inventoried behavior gets dropped, not merged.
