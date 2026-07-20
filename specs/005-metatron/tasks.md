# Tasks: Metatron v1 — the editable angel

**Input**: Design documents from `/specs/005-metatron/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: included — the spec's success criteria demand auditable evidence (firewall
sentinel audit, charge invariants, determinism), and the house rule is a race-clean
suite per phase.

**Organization**: grouped by user story; each phase is an independently testable
increment. One branch, one PR (task-12-metatron).

## Phase 1: Setup

**Purpose**: name the new surfaces so every later task compiles against them

- [x] T001 Add `KindMetatron` routed to `TierCloud` in internal/llm/llm.go (+ routing table test touch in internal/llm/llm_test.go)
- [x] T002 [P] Add `CharterPath()` and `MetatronDir()` helpers in internal/world/world.go
- [x] T003 [P] Author the default charter (faithful, competent, professional-almost-robotic; 4,000-char cap documented in a header comment) as a constant in internal/persona/charter.go

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: the event-sourced substrate and the component skeleton every story rides

**⚠️ CRITICAL**: no user story work until this phase is complete

- [x] T004 Create internal/sim/metatron.go: `MetatronNudgedPayload{form, targets, text}`, empty `ChargeRegeneratedPayload`, reducer arms `applyMetatron` (nudged: charges −1 floor 0; regenerated: +1 cap 3) per contracts/metatron-events.md
- [x] T005 Add `MetatronCharges int` (json `metatron_charges,omitempty`, genesis = 1) to `State` in internal/sim/state.go and dispatch `metatron.*` to `applyMetatron` in `Apply`
- [x] T006 Add `salDream = 8` to the salience table in internal/sim/memory.go
- [x] T007 Executor regeneration in internal/sim/executor.go: emit `metatron.charge_regenerated` when a tick crosses an absolute 6-game-hour boundary (multiples of 21600) and charges < 3 — pure function of (state, tick)
- [x] T008 Whitelist `metatron.nudged` in `injectSocialWhitelist` in internal/sim/loop.go with dry-run rejections (charges = 0, bad form, dream targets ≠ 1, dead/unknown target, empty/over-cap text) enforced by the reducer dry-run + a pre-validation in the door
- [x] T009 Sim tests in internal/sim/metatron_test.go: charge invariants 0..3 under regen/spend storms, genesis = 1, regen boundary exactness, nudged+memory batch atomicity via `InjectSocial`, old-snapshot (no field) unmarshal default, replay reproduces charges
- [x] T010 Charter loading in internal/metatron/charter.go: read per call; missing → recreate default + notice; empty → default + notice; > 4,000 chars → truncate + notice; unit tests in internal/metatron/charter_test.go
- [x] T011 Component skeleton in internal/metatron/metatron.go: `New(orch, injector, worldDir, seed, map, stateJSON)` with own replica, `Observe` (non-blocking), `Close`, absorb goroutine, single-flight turn guard, `metatron/` dir + empty soul.md creation; wire into internal/daemon/daemon.go behind the LLM-config gate
- [x] T012 `scriptworld new` seeds charter.md (root) in cmd/scriptworld (via internal/world creation path), never overwriting an existing charter; test in internal/world/world_test.go

**Checkpoint**: substrate compiled, charges replayable, component observes a running world

---

## Phase 3: User Story 1 — Converse with the angel that watches (P1) 🎯 MVP

**Goal**: free console conversation, charter-voiced, grounded in real observation; honest degradation; restart survival

**Independent test**: quickstart §1 — fresh world converses honestly from an empty soul; a day-old world answers with real names/events; cloud down → honest error, world unaffected

- [x] T013 [US1] Turn pipeline (say-only) in internal/metatron/turn.go: prompt = fixed system frame (role, roster, rubric placeholder) + charter + soul.md tail + transcript tail + live status (clock, charges, alive/dead); one `KindMetatron` call; strict-JSON parse of `{"say": …}`; unusable output → safe apology reply per research R10
- [x] T014 [US1] Soul/transcript persistence in internal/metatron/metatron.go: append console turns to metatron/transcript.md; bounded tails feed T013's prompt; files survive restart
- [x] T015 [US1] IPC `metatron_chat` request/response per contracts/console-protocol.md in internal/ipc (server dispatch + client call + long-call read deadline); errors: no-metatron, tier-down reason, turn-in-flight
- [x] T016 [US1] CLI `scriptworld metatron <dir> [message…]` in cmd/scriptworld: message → one turn (print reply, charges); no message → status peek (charges + last soul entries), no model call
- [x] T017 [US1] TUI console in internal/tui: metatron pane → transcript viewport + input line + ⚡-charges header (tier health retained); key contract per contracts/console-protocol.md (printable → input, Enter send w/ in-flight spinner, Esc → map, globals only on empty input)
- [x] T018 [US1] Tests: turn round-trip with mock cloud (charter voice in system prompt, real status in user prompt), degraded honesty (ErrTierDown/ErrBudgetExhausted → clean error, no charge), single-flight rejection, restart keeps transcript/soul, in internal/metatron/turn_test.go + internal/ipc tests + TUI pane tests in internal/tui/tui_test.go

**Checkpoint**: MVP — a player can talk to their angel about the real world

---

## Phase 4: User Story 2 — Nudge the world through a gatekeeper (P2)

**Goal**: dream/omen mediation with judgment, refusal-with-counsel, charge economy, structural firewall

**Independent test**: quickstart §2 — dream lands (atomic batch, salience 8, prefix), omen hits all living, refusal free, exhaustion honest, sentinel audit clean, replay identical

- [x] T019 [US2] Extend turn contract in internal/metatron/turn.go: judgment rubric (persuadability, impact, method) in the system frame; parse `{"say", "nudge": {form, target, text}|null}`; validation (form ∈ dream|omen, target resolves to living villager, text ≤ 400 chars, charges ≥ 1 re-check) → downgrade to refusal reply on any failure
- [x] T020 [US2] Nudge landing in internal/metatron/turn.go: build atomic batch per contracts/metatron-events.md (`metatron.nudged` + prefixed `agent.memory_added` × targets; omen targets = all living at landing) → `InjectSocial`; injection rejection → refusal reply, no charge lost; landed → confirmation appended to reply + soul.md nudge record
- [x] T021 [US2] Surface charges in the IPC status payload (internal/ipc + internal/sim loop status) and in the TUI metatron pane header in internal/tui/views.go
- [x] T022 [US2] Tests in internal/metatron/turn_test.go + internal/sim: dream memory lands with salience `salDream` and `"You dreamed: "` prefix on exactly the target; omen memory on every living villager (dead excluded); refusal spends nothing; exhaustion (⚡0) always refuses; dead-target abort refunds (nothing spent); charge never leaves 0..3
- [x] T023 [US2] Firewall sentinel audit test (SC-002) in internal/metatron/firewall_test.go: run a console turn containing `XYZZY-INJECTION-TEST` through a mock that lands a nudge; assert the sentinel appears in NO villager memory, NO villager-facing prompt builder output (planner/musing/convo/consolidation prompts for every agent), and NO injected event payload — only Metatron's own prompt may contain it
- [x] T024 [US2] Determinism: extend internal/sim replay tests + e2e determinism scenario to a log containing nudge batches and regen events; byte-identical replay (SC-005)

**Checkpoint**: the player's verb works, gated and auditable

---

## Phase 5: User Story 3 — Edit the charter, change the angel (P3)

**Goal**: live charter editability with safe fallbacks; the only editable prompt

**Independent test**: quickstart §3 — distinctive edit changes the very next reply, no restart; delete → default restored with notice

- [x] T025 [US3] Tests in internal/metatron/charter_test.go + turn_test.go: edited charter text appears in the next turn's system prompt (mock captures prompts); missing file recreated + reply notice; empty → default + notice; oversized → truncated + notice; charter content never appears in any villager-facing prompt (extends T023 audit)
- [x] T026 [US3] TUI/CLI affordance: `scriptworld metatron <dir>` status peek and the TUI pane header show the charter path and whether the default or a custom charter is active (internal/metatron reports it in status; cmd/scriptworld + internal/tui display)

**Checkpoint**: the meta-game is live — prompt-engineer your angel mid-reign

---

## Phase 6: User Story 4 — The angel keeps watch (P4)

**Goal**: 6-game-hour digests into soul.md; moments flagged and surfaced; never autonomous

**Independent test**: quickstart §4 — digest entry after a boundary with activity; empty window costs nothing; staged drama leads the next reply; log audit shows zero unprompted nudges

- [x] T027 [US4] Digest collector in internal/metatron/digest.go: notable-line vocabulary (reuse TASK-11 line phrasing where it fits), absolute 6-game-hour windows, skip-empty at zero cost, single-flight `KindMetatron` summarization call, dated soul.md entries, carry-on-failure (TASK-11 pattern)
- [x] T028 [US4] Moments in internal/metatron/digest.go: `agent.died` / `gru.attacked` / `social.promise_broken` → immediate model-free soul.md line + queue; turn.go surfaces queued moments at the start of the next reply (oldest first) and clears them
- [x] T029 [US4] Tests in internal/metatron/digest_test.go: boundary windowing, empty-window skip, digest failure carry, moment queue ordering + surfacing + clearing, and the acts-only-when-told invariant (digests/moments can never construct a nudge batch — assert no injection occurs without a console turn)

**Checkpoint**: the angel briefs you; it never acts alone

---

## Phase 7: Polish & cross-cutting

- [x] T030 Full suite `go test ./... -race` green; fix any interaction fallout (mind/scribe/TUI consumers unaffected by the new consumer)
- [x] T031 Quickstart validation run: execute quickstart.md §1–§5 against a fresh world with mock-free cloud (9router or Anthropic); record outcomes in specs/005-metatron/quickstart-results.md
- [x] T032 Live acceptance (quickstart §6) on ~/worlds/chronicle-proof with the upgraded binary: converse about real history, land one dream against a live storyline, verify villager interpretation surfaces (soul.md/chronicle), verify charge ledger in the live log; record in quickstart-results.md
- [x] T033 Wiki re-ground per PDLC: new docs/wiki/metatron.md note; re-verify touched notes (event-types, sim-state-reducer, sim-loop, executor, llm-orchestrator, tui-client, ipc-protocol, cli-scriptworld, agent-mind); freshness gate green
- [x] T034 README/help touch: `scriptworld metatron` in the usage block (cmd/scriptworld) and README.md feature list

---

## Dependencies

```
Phase 1 (T001–T003) ──▶ Phase 2 (T004–T012) ──▶ US1 (T013–T018) ──▶ US2 (T019–T024) ──▶ US3 (T025–T026)
                                                                  └▶ US4 (T027–T029, needs only US1's turn.go surface for T028's surfacing)
US2/US3/US4 complete ──▶ Phase 7 (T030–T034)
```

- US1 is the MVP and blocks US2 (nudges extend the turn contract) and US4's surfacing.
- US3 is mostly proof-of-behavior atop foundational charter loading (T010); only T025–T026 remain by design.
- US4 can start after US1's T013 (turn.go exists) in parallel with US2.

## Parallel execution examples

- Phase 1: T002 and T003 in parallel after T001 lands (different packages).
- Phase 2: T004+T005+T006 serial (same package, shared file edits), then T007/T008 serial in sim; T010 [P] with T011 (different files); T012 [P] with T009.
- After T013: US2's T019 and US4's T027 can proceed in parallel (turn.go vs digest.go), converging at T028.
- Phase 7: T031 and T033 in parallel; T032 after T031.

## Implementation strategy

MVP first: Phases 1–3 deliver a conversing, observing angel (independently shippable).
US2 adds the verb; US3 and US4 are thin, high-value layers on proven machinery. Live
acceptance runs on the existing 14-day chronicle-proof world — the richest possible
test bed for "what did I miss?" and a real nudge against a live storyline.
