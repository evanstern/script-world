# Tasks: The Cognition Horizon

**Input**: Design documents from `specs/007-cognition-horizon/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: included — the spec's success criteria are log-audit and determinism properties; tests ride each story's phase per the project's established pattern.

**Organization**: phases mirror the spec's prioritized stories; Setup and Foundational (the `internal/cognition` package and event vocabulary) come first.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: package scaffolding and save-dir plumbing

- [x] T001 Scaffold `internal/cognition` package: doc.go stating the doctrine (decision-4) and the purity rule (stdlib-only, no mind/sim/llm imports)
- [x] T002 [P] Add `CalibrationPath()` helper (→ `calibration.json`) in internal/world/world.go with test in internal/world/world_test.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the registry, router, estimator, profile I/O, startup completeness gate, and the new event vocabulary — everything every story consumes

**⚠️ CRITICAL**: no user story work can begin until this phase is complete

- [x] T003 Implement DecisionClass registry in internal/cognition/registry.go per contracts/registry.md (classes, Fibonacci points, BudgetTicks, degrade, FutureDated; init-time validation: Fibonacci membership, positive budgets) with table tests in internal/cognition/registry_test.go
- [x] T004 [P] Implement pure `Route(class, ticksPerSecond, secondsPerPoint) Verdict` in internal/cognition/route.go (verdict carries predicted wall ms, predicted drift ticks, allow/suppress + arithmetic string) with the contract's sanity table as test cases in internal/cognition/route_test.go
- [x] T005 [P] Implement per-tier Estimator in internal/cognition/estimate.go (EWMA α=0.2, spike >3× excluded+counted, 20-sample window, >30% breach signal with re-arm) with spike-rejection/drift-following/bootstrap tests in internal/cognition/estimate_test.go
- [x] T006 [P] Implement CalibrationProfile load/save/bootstrap in internal/cognition/calibration.go per contracts/calibration.md (bootstrap 20/10 s/pt; malformed file → warning + bootstrap, never crash) with tests in internal/cognition/calibration_test.go
- [x] T007 Add startup completeness gate: every `llm.Kind` the orchestrator accepts must map to a registered class or daemon start fails naming the kind — wire in the daemon start path (internal/daemon) next to LLM config load, with test
- [x] T008 Add new event payload structs + whitelist entries in internal/sim (cog.thought, cog.outcome, cog.recalibration_recommended, agent.intent_rejected as reducer no-ops on the inject_social door; canonical JSON field order per contracts/events.md) with whitelist/no-op/dry-run tests in internal/sim/loop_test.go

**Checkpoint**: `go test ./internal/cognition/ ./internal/world/` green; registry/router/estimator usable by all stories

---

## Phase 3: User Story 1 — Staleness is measured, never guessed (Priority: P1) 🎯 MVP

**Goal**: calibration stage + full telemetry with causality ids; zero behavior change to routing or enforcement

**Independent Test**: quickstart §2–3 — calibrate a fresh world, run at 4x, audit the log: every model call has a complete `cog.*` record; every `cog.thought` has exactly one `cog.outcome`; chains walk back to stimuli

- [x] T009 [US1] Extend internal/llm requests with class/points metadata and feed measured per-call duration ÷ points into the Estimator from the worker (successes and returned failures sample; caller-abandoned don't) in internal/llm/llm.go, with mock-provider latency tests in internal/llm/llm_test.go
- [x] T010 [US1] Thread ThoughtJob identity through the mind driver (JobID, SnapshotTick, TriggerSeq from the arming event, prediction from Route verdict; Generation recorded as 0 until US3) and emit `cog.thought` on enqueue + `cog.outcome` for mind-side terminals (unusable, abandoned) via InjectSocial, in internal/mind/mind.go with tests in internal/mind/mind_test.go
- [x] T011 [P] [US1] Same for conversation scenes in internal/mind/convo.go (JobID from founding tick; scene-level cog.thought/cog.outcome) with tests in internal/mind/convo_test.go
- [x] T012 [US1] Implement `scriptworld calibrate` in cmd/scriptworld per contracts/cli.md (reference workload musing-1pt/planner-3pt, medians, writes calibration.json, human summary including the per-speed cognition-horizon verdict line, exit codes, cloud opt-in) with mock-provider test
- [x] T013 [US1] Seed the Estimator from calibration.json (or bootstrap) at daemon start and wire `cog.recalibration_recommended` emission from the breach signal, in internal/daemon + internal/mind, with test
- [x] T014 [US1] e2e telemetry audit in e2e/: run a short world with mock LLM, assert every cog.thought has exactly one cog.outcome (SC-002) and an intent's chain walks intent → job → trigger_seq → stimulus (SC-007)

**Checkpoint**: telemetry complete and audited; no routing/enforcement behavior changed yet

---

## Phase 4: User Story 2 — Doomed thoughts are never attempted (Priority: P2)

**Goal**: the deterministic router gates every enqueue site; suppressed decisions run their degrade action and are recorded

**Independent Test**: quickstart §4 — same world at 1x routes planners to the model; at 32x (slow local profile) planner jobs are `suppressed` with arithmetic in `reason`, musings still land

- [x] T015 [US2] Consult Route before planner and musing enqueue in internal/mind/mind.go (speed from replica state, estimate from orchestrator handle); suppressed → degrade (skip; reflex floor covers) + `cog.outcome{suppressed}` with arithmetic; tests: 32x suppresses planner, 1x routes, musing allowed at 32x, in internal/mind/mind_test.go
- [x] T016 [P] [US2] Consult Route at conversation founding (internal/mind/convo.go), meeting rephrase (internal/mind/meeting.go), and consolidation/chronicle handoffs (internal/mind/consolidate.go, internal/mind/narrate.go) with per-class degrade actions and suppressed telemetry, with tests
- [x] T017 [US2] Router determinism + regression tests: identical inputs → identical verdicts across repeated calls; SC-006 guard asserting the 1x verdict set matches today's routed kinds, in internal/cognition/route_test.go

**Checkpoint**: at high speed no doomed calls are attempted; event log shows suppressions with arithmetic

---

## Phase 5: User Story 3 — Stale intents never act (Priority: P3)

**Goal**: authoritative landing enforcement — staleness stamp, generation supersede, guard ladder (adapt → reject+record → learn)

**Independent Test**: quickstart §4 audit — zero executed intents with staleness over budget (SC-001); inject artificial latency and observe recorded rejections while reflex covers

- [ ] T018 [US3] Add `Agent.Generation` to sim State with reducer bump on the high-salience set (agent attacked, witnesses a death, emergency on own/adjacent tile) in internal/sim/state.go + internal/sim/memory.go, with reducer tests
- [ ] T019 [P] [US3] Implement Guard type + deterministic evaluation over State (target_alive, target_present, not_superseded, after_tick, before_tick) in internal/sim/guard.go with table tests in internal/sim/guard_test.go
- [ ] T020 [US3] Extend inject_intent args (SnapshotTick, Generation, Class, JobID, PredictedWallMs, Guards) and implement the landing ladder in internal/sim/loop.go per contracts/events.md order (unavailable → superseded → stale → guards-with-adapt-via-resolveGoal → accept), emitting agent.intent_rejected + cog.outcome in the same atomic batch, with a test per rung in internal/sim/loop_test.go
- [ ] T021 [US3] Learn rung: classify rejections prediction-miss (actual > 3× predicted) vs world-change in the loop's outcome payloads, and bump the rejected agent's re-plan (nextDue → now, debounce floor honored) in internal/mind/mind.go, with tests
- [ ] T022 [US3] e2e latency injection in e2e/: mock provider with configurable delay ⇒ stale rejections recorded with reasons, reflex floor covers, SC-001 audit query returns zero over-budget executions

**Checkpoint**: wrong predictions are harmless; every landing verdict is recorded

---

## Phase 6: User Story 4 — Thoughts aim at the world they will land in (Priority: P4)

**Goal**: future-dated prompts and guarded conditional plans; timed guards are act-at-time-T

**Independent Test**: prompt snapshots carry the landing estimate; a scripted stub returning a timed-guard plan executes at tick T with no model call at firing time

- [ ] T023 [P] [US4] Future-dating line in planner situation block ("It is now …; your decision will take effect around …") for FutureDated classes only, in internal/mind/prompt.go with test in internal/mind/mind_test.go
- [ ] T024 [P] [US4] Guarded-plan vocabulary in internal/mind/parse.go (optional `plan` array ≤3 steps of {goal, when?, until?} from the closed guard vocabulary; reject >3 steps or unknown guards as unusable) with parse tests
- [ ] T025 [US4] PlanStep state + `agent.plan_set` reducer + executor per-tick head-step guard evaluation with `agent.plan_step_started`/`agent.plan_expired` and the 2-game-hour default window, in internal/sim/state.go + internal/sim/agents.go, with executor tests
- [ ] T026 [US4] Wire plan-form replies through InjectIntent (Plan args, mutually exclusive with Goal; dry-run enforces step cap) in internal/mind/mind.go + internal/sim/loop.go, and add a scripted-stub e2e proving act-at-tick-T in e2e/

**Checkpoint**: conditional plans execute deterministically; expiry is recorded

---

## Phase 7: User Story 5 — Pause has defined cognition semantics (Priority: P5)

**Goal**: bless world-freezes-minds-catch-up with regression tests so the semantics are chosen, not accidental

**Independent Test**: quickstart §5 — pause with in-flight thought and conversation; both land at the frozen tick at staleness 0; no new jobs; resume without burst

- [ ] T027 [US5] Pause-semantics regression tests: in-flight planner result lands at frozen tick with staleness_ticks 0 and guards still checked; no new planner/musing/conversation jobs while paused; founded conversation lands atomically at the frozen tick, in internal/mind/mind_test.go + internal/sim/loop_test.go
- [ ] T028 [US5] Resume-no-burst test (cadence self-heals, no compensating flood after a long pause) and doctrine comments at the two injection doors in internal/sim/loop.go stating pause-open is deliberate (FR-018)

**Checkpoint**: FR-018 is executable doctrine

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T029 [P] README: cognition-horizon section (doctrine, calibrate usage, reading cog.* telemetry)
- [ ] T030 Replay byte-equality e2e on a cognition-enabled run (SC-003) in e2e/
- [ ] T031 Run quickstart.md end-to-end against a live local model and record results in the spec dir (quickstart-results.md)
- [ ] T032 Full-suite gate: `go vet ./... && go test ./...` green; SC-002 audit query over the quickstart run shows zero silent drops

---

## Dependencies & Execution Order

- **Setup (Phase 1)** → **Foundational (Phase 2)** → stories.
- **US1 (P3)** is the MVP: telemetry + calibration, no behavior change. **US2** consumes US1's estimator wiring (T009, T013). **US3** consumes US1's job identity (T010) and is strengthened by US2 but independently testable via direct injection. **US4** consumes US3's guard machinery (T019). **US5** is independent after Foundational (tests exercise US1 telemetry if present).
- Within stories: registry/state before services; loop-door changes (T020) before mind wiring that depends on them (T021, T026).

### Parallel Opportunities

- Phase 2: T004, T005, T006 in parallel after T003; T008 parallel with all of them.
- US1: T011 ∥ T010's review; T012 ∥ T013.
- US3: T019 ∥ T018.
- US4: T023 ∥ T024.
- Polish: T029 ∥ T030.

## Implementation Strategy

MVP first: Phases 1–3 (telemetry + calibration) land with zero behavior change and immediately quantify the problem on real hardware — validate quickstart §2–3, then layer US2 (router), US3 (enforcement), US4 (quality), US5 (pause codification), each independently checkpointed. Commit after each task or logical group on `task-32-cognition-horizon`; one task, one PR.
