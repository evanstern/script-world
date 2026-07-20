# Tasks: Norms and Votes

**Input**: Design documents from `/specs/006-norms-and-votes/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: included — the success criteria demand auditable determinism (SC-005),
degraded-mode proof (SC-007), and witnessed/unwitnessed asymmetry (SC-006); the house
rule is a race-clean suite per phase.

**Organization**: grouped by user story; each phase is an independently testable
increment. One branch, one PR (task-13-norms-and-votes).

## Phase 1: Setup

**Purpose**: name the new surfaces so every later task compiles against them

- [x] T001 Add `KindMeeting` routed to `TierLocal` in internal/llm/llm.go (+ routing table test touch in internal/llm/llm_test.go)
- [x] T002 [P] Add `VillageCharterPath()` helper in internal/world/world.go (distinct from Metatron's `CharterPath()`; test in internal/world/world_test.go)

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: the event-sourced governance substrate every story rides

**⚠️ CRITICAL**: no user story work until this phase is complete

- [x] T003 Create internal/sim/governance.go: `Norm`, `NormViolation`, `MeetingState` structs, all event payload structs per contracts/governance-events.md, tuning const block per contracts/meeting-lifecycle.md, `DayIndex(tick)` helper, computed helpers `ActiveNorms`, `IsExiled`, `ViolationCount`
- [x] T004 Add `MeetingPlace *Point`, `Meeting MeetingState`, `Norms []Norm`, `NextNormID`, `NextProposalID` to `State` in internal/sim/state.go (fixed JSON field order) and dispatch `meeting.*`/`norm.*` to `applyGovernance` in `Apply`
- [x] T005 Reducer arms `applyGovernance` in internal/sim/governance.go: place_designated (set-once), convened, opened (snapshot + `LastMeetingDay`), turn_taken, proposal_tabled (`NextProposalID++`), proposal_resolved (enact/amend/repeal from denormalized payload + reducer-internal pairwise voter edge deltas), proposal_rephrased (text-only, validated), closed (phase reset), violated (bounded ring append + witness→violator edge deltas) — all arms total (vanished subjects → no-op)
- [x] T006 [P] Add meeting/violation memory salience constants (`salMeetingOutcome`, `salNormViolation`, speaker-turn salience) to the table in internal/sim/memory.go
- [x] T007 Reducer tests in internal/sim/governance_test.go: table-driven `Apply` coverage for every governance event, invariants (one active norm per kind / per exile target, text cap, set-once place), pre-TASK-13 snapshot unmarshal to zero values, `canonicalLog` replay hash equality

**Checkpoint**: governance state compiles, replays, snapshots round-trip

---

## Phase 3: User Story 1 — The village convenes at noon (P1) 🎯 MVP

**Goal**: villagers break routines, gather at an event-sourced meeting place, get
speaking turns inside the timebox (+grace), disperse after; once per game day

**Independent test**: quickstart §4 first bullet — drive a noon boundary and observe
convene → open (attendance) → turns → close, then normal behavior resumes

- [x] T008 [P] [US1] Meeting-place derivation in internal/sim/governance.go: pure fn of (state, map) — first fire's tile, else first shelter's, else map-center-nearest passable
- [x] T009 [US1] Executor beats in internal/sim/executor.go: convene at `meetingConveneSecond` (once-per-day guard vs `LastMeetingDay`, ≥2 living villagers; emit place_designated first time + convened), open at `meetingOpenSecond` with attendance snapshot (living ∧ awake ∧ within `meetingRadius` ∧ not exiled), turn beat every `meetingTurnTicks` (turn_taken with grievance `raised` note + low-salience speaker memory), close on agenda-done / timebox / grace cap per contracts/meeting-lifecycle.md
- [x] T010 [US1] Attendee pinning in internal/sim/executor.go: during convening/open, pin awake living non-exiled villagers to `Intent{Goal:"attend_meeting"}` toward the place (source "meeting") on the staggered beat; arrived agents idle; pinning stops at close; no `resolveGoal` case (never planner-choosable)
- [ ] T011 [P] [US1] Planner suppression in internal/mind/mind.go: absorb sees `meeting.convened` → skip planner scheduling for attendees until `meeting.closed` (asleep-agent precedent); test in internal/mind/mind_test.go
- [x] T012 [US1] Sim tests in internal/sim/governance_test.go: full-day `driveTicks` lifecycle (convene→open→turns in seating order→close before 3600+900), exactly one meeting per day, place persists across days and structure churn, asleep villagers miss attendance, empty attendance opens-then-closes, agents converge (positions at open within radius)
- [ ] T013 [US1] Chronicle cases in internal/mind/narrate.go: `meeting.opened` (attendance named), `meeting.turn_taken` (grievances), `meeting.closed`; `ChronicleEntryPayload.Agents` populated (TASK-17 convention); test in internal/mind/narrate_test.go

**Checkpoint**: MVP — the village visibly gathers at noon, speaks, and disperses, model-free

---

## Phase 4: User Story 2 — Propose, vote, pass (P1)

**Goal**: fodder-driven proposals, relationship-deterministic votes, strict-majority
outcomes with visible positions and edge consequences; model phrasing as flavor only

**Independent test**: seed a grievance, watch a proposal table and resolve at the next
meeting; recorded votes match the vote function; replay reproduces everything model-free

- [x] T014 [US2] Fodder rules 1–2 in internal/sim/governance.go: add_curfew (gru memory within 3 game days ∧ no active curfew) and add_repay_debts (creditor of a broken debt ∧ no active repay norm), with deterministic template texts; first-match-wins tabling on the turn beat in internal/sim/executor.go
- [x] T015 [US2] Vote function in internal/sim/governance.go per contracts/meeting-lifecycle.md (base / amend-repeal self-interest / exile inversion; proposer always yea; yea iff score ≥ 0; strict majority of eligible; ties fail) and same-beat `meeting.proposal_resolved` emission with denormalized payload + yeas/nays
- [x] T016 [US2] Outcome companions in internal/sim/executor.go: one toned, subject-tagged `agent.memory_added` per attendee about the proposer (gossip-seed shape) in the resolution beat
- [x] T017 [US2] Whitelist `meeting.proposal_rephrased` in `injectSocialWhitelist` in internal/sim/loop.go; dry-run rejections (unknown norm, empty/oversized text) via the reducer arm; test in internal/sim/governance_test.go
- [ ] T018 [US2] Meeting phrasing driver in internal/mind/meeting.go: observe `meeting.proposal_tabled` (absorb-side bounded queue, single-flight), one best-effort `KindMeeting` call phrasing the template in the proposer's voice, inject `meeting.proposal_rephrased`; any failure → skip (template stands); tests with mocked Submitter in internal/mind/meeting_test.go (skip-on-ErrTierDown, cap enforcement, no call when queue empty)
- [x] T019 [US2] Vote-table tests in internal/sim/governance_test.go: authored Relation fixtures → exact yea/nay sets, tally strings, tie-fails, duplicate-of-active never tables, pairwise aligned/opposed edge deltas land, degraded-mode day (no driver) passes a rule with template text (SC-007)
- [ ] T020 [P] [US2] Chronicle cases in internal/mind/narrate.go: `meeting.proposal_tabled` (proposer + text), `meeting.proposal_resolved` (outcome + tally, voters named); test in internal/mind/narrate_test.go

**Checkpoint**: the village legislates itself — proposal → vote → law, replayable

---

## Phase 5: User Story 3 — The charter remembers (P2)

**Goal**: `village_charter.md` renders the law with provenance; amend/repeal via vote
changes it; log-only reconstruction and restart survival

**Independent test**: quickstart §4 third bullet — pass, amend, repeal across meetings;
the file tracks each change; replay yields identical charter

- [x] T021 [US3] Amend/repeal fodder rule in internal/sim/governance.go: proposer with ≥ `repealViolationCount` entries in an active norm's ring → amend (curfew only, `Param += 7200`, `Amended` guard, affection ≥ 0 toward norm's proposer) else repeal; wire into the turn-beat tabling order (rule 3)
- [ ] T022 [US3] Charter render in internal/scribe/scribe.go: dirty-mark on `meeting.*`/`norm.*` event types, `renderVillageCharter()` (rules in force with proposer/day/tally/amendment note, standing judgments, repealed struck-through) to `world.VillageCharterPath()`, render-on-start; golden-file test in internal/scribe/scribe_test.go
- [ ] T023 [US3] Persistence tests: replay a governed log → identical `State.Norms` and identical charter bytes; snapshot round-trip mid-meeting; amend/repeal reducer semantics (in-place param/text update, `DayRepealed`, no-op on missing/inactive) in internal/sim/governance_test.go + internal/scribe/scribe_test.go

**Checkpoint**: the law is durable, amendable, and reconstructible from the log alone

---

## Phase 6: User Story 4 — Norms bind (and get broken) (P2)

**Goal**: norms enter planner context; witnessed violations land memories, edge
penalties, and rumor fodder; unwitnessed breaches cost nothing

**Independent test**: pass a curfew, drive a night wanderer past a witness → violation
event, witness memory, edges move, rumor spreads; same breach unwitnessed → nothing

- [ ] T024 [US4] "Village law" prompt section in internal/mind/prompt.go: active norms one line each with provenance, standing exile judgments, convening-time meeting line; empty when lawless; test in internal/mind/prompt_test.go (reflex policy stays norm-blind — no `decideIntent` change)
- [x] T025 [US4] Curfew detector in internal/sim/executor.go on the per-game-minute beat: night ∧ awake ∧ uncovered ∧ past `Param` ∧ ≥1 witness in `witnessRadius` → `norm.violated` + per-witness toned subject-tagged memories (companion events); once-per-agent-per-night latch (near-death-latch pattern)
- [x] T026 [US4] Repay-debts piggyback in internal/sim/executor.go: `promise_broken` emitted while a repay norm is active → same-beat `norm.violated` for the debtor with witnesses-within-radius (may be empty → no violation event)
- [x] T027 [US4] Violation tests in internal/sim/governance_test.go: witnessed → ring append + witness memories + edge penalties; unwitnessed → zero events (SC-006 asymmetry); latches hold across the night; violation memories are `TellableFor`-eligible (rumor-seed shape asserted)
- [ ] T028 [P] [US4] Chronicle case `norm.violated` (violator + witnesses named) in internal/mind/narrate.go; test in internal/mind/narrate_test.go

**Checkpoint**: law has teeth the village can feel — and gossip about

---

## Phase 7: User Story 5 — Exile is on the table (P3)

**Goal**: exile proposals resolve like any vote (target excluded), land as standing
charter judgments, and are socially enforced through the same violation machinery

**Independent test**: author village-wide hostility toward one villager → exile tables,
passes, enters charter; exile stops being convened; proximity violation fires

- [x] T029 [US5] Exile fodder rule (rule 4) in internal/sim/governance.go: mean (trust+affection) from living others < `exileHostilityGate` ∧ proposer hostile ∧ no active exile of target; target excluded from eligible voters (never in yeas/nays); reducer no-op on dead target
- [x] T030 [US5] Exile effects in internal/sim/executor.go: exiled villagers never pinned/convened, excluded from attendance snapshots; proximity detector (within `exileShunRadius` of meeting place or any structure ∧ witness ∧ once-per-game-hour latch) → `norm.violated`
- [x] T031 [US5] Exile tests in internal/sim/governance_test.go: vote exclusion arithmetic (eligible = attendees − 1), passed exile in charter render + prompt judgment line, shun detector + latch, exile repealable (the village forgives → pinning resumes), exile of dead villager moot
- [ ] T032 [P] [US5] Chronicle case for exile resolution (gravity-appropriate line, target named) in internal/mind/narrate.go; test in internal/mind/narrate_test.go

**Checkpoint**: the miscast valve of last resort exists — socially enforced, reversible

---

## Phase 8: Polish & cross-cutting

- [ ] T033 Determinism e2e in e2e/determinism_e2e_test.go: extend the scenario to a governed run (meetings, votes, violations, rephrased text) → byte-identical replay hash (SC-005)
- [ ] T034 Quickstart §4 live acceptance on a real world (32x across ≥2 noons, then model-off crossing): record evidence per AC in specs/006-norms-and-votes/quickstart-results.md
- [ ] T035 [P] README.md: add norms/votes to the feature list; docs touch for `village_charter.md` in the save-dir layout
- [ ] T036 Wiki re-ground after merge-ready: new docs/wiki note for governance + re-verify notes whose sources changed (executor, social-fabric, agent-mind, chronicle, event-types) via /grounding-wiki:wiki-update

---

## Dependencies

```
Setup (T001–T002)
  └─▶ Foundational (T003–T007)   ← blocks everything below
        └─▶ US1 (T008–T013)      ← MVP; independently shippable
              └─▶ US2 (T014–T020)  ← needs turn beats (T009)
                    ├─▶ US3 (T021–T023)  ← needs enactment (T015); T021 also reads the
                    │                       violation ring (tests seed events directly)
                    ├─▶ US4 (T024–T028)  ← needs active norms (T015)
                    └─▶ US5 (T029–T032)  ← needs vote fn (T015) + detectors pattern (T025)
                          └─▶ Polish (T033–T036)
```

US3, US4, US5 are mutually independent once US2 lands (parallelizable if staffed).

## Parallel opportunities

- Setup: T002 beside T001.
- Foundational: T006 beside T004–T005.
- US1: T008 and T011 in parallel with T009 prep; T013 after events exist.
- US2: T018 (mind driver) and T020 (narration) parallel to T016–T017.
- Across stories: after US2, US3/US4/US5 phases can proceed in parallel.
- Polish: T035 anytime; T033–T034 after all stories; T036 last.

## Implementation strategy

MVP = Phases 1–3 (a village that visibly assembles daily is already a story beat).
Then US2 makes it a legislature; US3 makes law durable; US4 gives it teeth; US5 arms
the valve. Each checkpoint leaves a green `go test ./... -race` and a demonstrable
increment; commits land per task-cluster on the single task-13 branch.
