# Tasks: Agent Tool-Use Loop

**Input**: Design documents from `/specs/017-agent-tool-loop/`

**Prerequisites**: plan.md, spec.md, research.md (R1–R15), data-model.md,
contracts/loop-api.md, contracts/events.md, contracts/provider-wire.md, quickstart.md

**Tests**: INCLUDED — the contracts define an explicit test contract (loop-api.md §Test
contract, events.md §Byte-stability obligations) and replay byte-identity is a
constitution-level gate for this codebase.

**Organization**: grouped by spec user story. Every task carries a model-tier tag per
constitution Principle V — `(Opus 4.8)` for cross-package/concurrency/doctrine slices,
`(Sonnet)` for routine single-package slices; escalation is one-way Sonnet → Opus.

**Board mapping**: this whole file is internal breakdown of TASK-52 — one branch
(`task-52-agent-tool-loop` in `.worktrees/task-52`), one PR.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

- [x] T001 Create worktree `.worktrees/task-52` on branch `task-52-agent-tool-loop` from
      fresh `origin/main`; verify `go test ./...` green at baseline

---

## Phase 2: Foundational (blocking all stories)

**Purpose**: registry vocabulary + llm transport that every story consumes.

- [x] T002 [P] (Sonnet) Add `Number` ParamKind with `Param.Min/Max`, declare `qty`
      Number param on drop/pick_up/deposit/withdraw, and extend `Validate` (Min≤Max,
      Read-roster entries now legal, InputSchemaJSON must be a JSON object) in
      internal/tool/tool.go + internal/tool/registry.go, tests in internal/tool
- [x] T003 [P] (Sonnet) Implement `tool.InputSchema(t)` derivation (data-model §1 rules,
      deterministic key order) honoring new `Tool.InputSchemaJSON` override, in
      internal/tool/derive.go with table-driven tests
- [x] T004 (Sonnet) Add `set_plan` catalog entry (World, Resolvable, authored steps
      schema per data-model §2) and `LoopRosterVillager()`/`LoopRosterMetatron()`
      derived surfaces; pin byte-stability of the legacy surfaces
      (VocabularyLine/WorldGoals/PlanStepGoals exclude set_plan) in internal/tool
      (depends on T002, T003)
- [x] T004b (Sonnet) Fix sim-side coverage validator for non-goal-door World tools:
      `ValidateToolCoverage` (internal/sim/toolcheck.go) must require resolver/duration
      arms only for the goal-door vocabulary (`tool.WorldGoals()` / legacy world tools),
      not every World-effect tool — `set_plan` grounds via injectPlan and never rides
      `resolveGoal` (discovered by T004: daemon boot + e2e red without this); update
      TestToolCoverageClean/TestWorldToolDurationsMatchSimConstants accordingly and
      restore full-suite green including e2e
- [x] T005 (Opus 4.8) Extend llm transport types: `Request.Tools/Turns/SkipObserve`,
      `Response.ToolCalls/Stop`, Role/Turn/Block/ToolCall/StopReason per
      contracts/loop-api.md; config `loop_max_rounds` (+`Rounds()` clamp 1–16 default 8)
      and `local.tool_mode`/`cloud.tool_mode` in internal/llm/llm.go +
      internal/llm/config.go; nil-Tools/nil-Turns behavior byte-identical (regression
      fixture test)
- [x] T006 [P] (Opus 4.8) anthropicCaller native tool exchange (tools param,
      tool_use/tool_result blocks, stop_reason mapping, cache-control preserved) per
      contracts/provider-wire.md §1, fixture round-trip tests, in
      internal/llm/providers.go (depends on T005)
- [x] T007 [P] (Opus 4.8) openaiCompat native function calling (tools/tool_calls,
      arguments-as-string decode, finish_reason mapping) AND `tool_mode:"json"`
      fallback envelope (system-prompt tool rendering from tool.InputSchema,
      response_format envelope, synthesized call IDs, user-turn result feedback) per
      provider-wire.md §2–3, fixture tests for both modes, in internal/llm/providers.go
      (depends on T005)
- [x] T008 (Opus 4.8) Governor observation seam: `Orchestrator.ObserveCognition(kind,
      totalMillis)`; worker skips estimator feeding when `SkipObserve` (metering,
      admission, breaker untouched — test-pinned) in internal/llm/llm.go (depends on
      T005)

**Checkpoint**: transport + vocabulary complete; `go test ./internal/llm/...
./internal/tool/...` green; zero behavior change for existing callers.

---

## Phase 3: User Story 1 — a mind acts by calling a tool (P1) 🎯 MVP

**Goal**: bounded loop driver + villager cognition migrated; muse is a roster choice.

**Independent Test**: stub-caller driver tests (cap, cardinality, read-then-act) +
local-tier smoke where planner cognitions land intents/plans/musings via tool calls
(quickstart §2).

- [x] T009 (Opus 4.8) [US1] Implement `internal/toolloop` package: `Run(ctx, orch, Job)`
      per contracts/loop-api.md — round loop, roster/schema validation
      (rejected_unknown/rejected_malformed driver-side), one-landed-acting-call
      cardinality with trailing-call rejection, MaxRounds cap, Termination taxonomy,
      one CallRecord per model call via `Record`, SkipObserve on every Submit, exactly
      one ObserveCognition report (depends on T005–T008)
- [x] T010 (Opus 4.8) [US1] toolloop unit tests with stub caller in
      internal/toolloop/loop_test.go: cap exhaustion, batched calls, rejected-acting
      retry within cap, read-then-act, admission_refused mid-loop, provider_error,
      ctx_done — every path's records + observation counts (with T009)
- [x] T011 (Opus 4.8) [US1] Villager handlers in internal/mind: world verbs + set_plan
      wrap InjectIntent/injectPlan translating door accept/reject → verdicts; muse
      handler lands agent.thought through the social door; Record sink buffers
      CallRecords for the cognition's telemetry batch (depends on T009)
- [x] T012 (Opus 4.8) [US1] Migrate runPlan to toolloop.Run: tool-era systemPrompt in
      internal/mind/prompt.go (declared tools replace vocabulary/gloss prose),
      planner-path parseReply/plannerReplySchema retired, cog.thought/cog.outcome
      emission preserved around the loop, rearm semantics kept in
      internal/mind/mind.go + parse.go (depends on T011; retire the 014 golden-prompt
      test in the same commit, noted per contracts/loop-api.md)
- [x] T013 (Opus 4.8) [US1] Delete scheduled musing: muse queue/worker/cadence fields in
      internal/mind/mind.go, `KindMusing` routing in internal/llm/llm.go, `musing`
      DecisionClass + kindToClass entry in internal/cognition/registry.go; update
      ValidateKinds and its daemon-start gate tests (depends on T012)

**Checkpoint**: US1 MVP — villager cognitions act via the loop on a stub/local model;
`go test ./...` green.

---

## Phase 4: User Story 2 — replay reproduces state without re-running loops (P1)

**Goal**: determinism proven, old logs byte-stable.

**Independent Test**: quickstart §1 + §5 — pre-feature fixtures pass unmodified;
live-vs-replay byte-identity on a loop-era run.

- [x] T014 (Opus 4.8) [US2] Run the full pre-existing replay/byte-identity suite
      UNMODIFIED (whole_feature_test.go, sim_test.go, per-capability replay tests);
      fix any regression in the migration (never the fixtures) — evidence recorded in
      the PR (depends on T013)
- [x] T015 (Opus 4.8) [US2] Add live-vs-replay determinism test for a loop-era run:
      scripted stub-model run in internal/sim or internal/mind integration test —
      genesis replay and snapshot replay byte-identical, zero handler/model invocations
      during replay (depends on T013)

**Checkpoint**: both P1 stories done — deliverable core proven.

---

## Phase 5: User Story 3 — every tool call is a first-class correlatable artifact (P2)

**Goal**: `cog.tool_call` events + `IntentSetPayload.Job` close the AC#5 chain.

**Independent Test**: quickstart §2 log inspection + SC-003 correlation test.

- [x] T016 [P] (Sonnet) [US3] `CogToolCallPayload` per contracts/events.md (canonical
      order, args 2 KiB cap+truncation marker, reason required on rejections),
      whitelist entry + reducer no-op arm in internal/sim/loop.go +
      internal/sim/state.go + internal/sim/cognition.go, marshal-order + no-op tests
- [x] T017 [P] (Sonnet) [US3] Add `Job string json:"job,omitempty"` to
      `IntentSetPayload` (LAST field) in internal/sim/agents.go, populate from
      `InjectArgs.JobID` at the inject-landing arm only in internal/sim/loop.go;
      byte-stability pin: reflex/executor emissions and pre-feature marshaling
      unchanged (TASK-32 pattern test)
- [x] T018 (Opus 4.8) [US3] Land CallRecords as cog.tool_call events through each
      consumer's telemetry door (mind emitCog batch in internal/mind/telemetry.go;
      metatron's landing path) with job+ordinal threading (depends on T009, T016)
- [x] T019 (Sonnet) [US3] SC-003 correlation test: from a stub-run fixture resolve
      intent_set→job→cog.tool_call chains and rejected-call artifacts purely by
      identifier, in internal/sim or internal/mind integration test (depends on
      T017, T018)

**Checkpoint**: AC#5 queryable end-to-end.

---

## Phase 6: User Story 4 — both tiers + documented fallback (P2)

**Goal**: metatron proves the cloud-native leg; fallback mode proven equivalent.

**Independent Test**: quickstart §3 + §4.

- [x] T019b (Sonnet) [US4] Register `work_miracle` metatron tool (charge-gated,
      authored InputSchemaJSON over spec 016's miracle parameter surface — kind/day/
      time/villager/item/qty/class/x/y/to_x/to_y, gratis structurally absent) and add
      it to LoopRosterMetatron, in internal/tool/registry.go + roster.go + tests
      (post-PR-#38 amendment, research R13; depends on T002–T004)
- [x] T020 (Opus 4.8) [US4] Migrate `Metatron.Turn` to toolloop.Run with
      LoopRosterMetatron: converse = Final text, nudge handlers wrap `landNudge`,
      work_miracle handler wraps `landMiracle` (charge economy stays
      reducer-enforced; spec 016's one-mediated-act-per-turn rule = loop cardinality),
      parseTurn/turnReply retired, cog.tool_call telemetry landed, in
      internal/metatron/turn.go + tests (depends on T009, T018, T019b)
- [x] T021 [P] (Sonnet) [US4] Fallback-mode equivalence test: same scripted cognition
      through `tool_mode:"native"` and `tool_mode:"json"` stub servers produces
      identical event-log shape (verdicts, correlation, outcomes) in
      internal/llm or internal/mind integration tests (depends on T012)
- [x] T022 [P] (Sonnet) [US4] Operator docs for tier strategy + fallback + knobs
      (`tool_mode`, `loop_max_rounds`) in README/config docs where llm.json is
      documented (depends on T007)

**Checkpoint**: AC#3 satisfied — native cloud, native-first local, documented fallback.

---

## Phase 7: User Story 5 — governor stays sane on multi-call cognitions (P3)

**Goal**: whole-loop observation unit; per-call spend intact.

**Independent Test**: quickstart §6.

- [x] T023 (Opus 4.8) [US5] Estimator/budget tests: whole-loop observations converge
      (no per-call samples when SkipObserve), budget-exhausted mid-loop refuses
      pre-spend → `admission_refused` termination + failure cog.outcome, route verdict
      arithmetic unchanged, in internal/llm + internal/cognition tests (depends on
      T008, T009)
- [x] T024 (Opus 4.8) [US5] Update `promptworld calibrate` to probe a representative
      loop cognition (whole-loop sec/pt seeding) and reconcile the calibration profile
      docs, in the calibrate command + internal/cognition/calibration.go (depends on
      T023)

**Checkpoint**: AC#4 satisfied.

---

## Phase 8: Polish & Cross-Cutting

- [x] T025 (Opus 4.8) Adversarial verification pass over the loop driver + door
      handlers (cardinality races, batched-call edge cases, cap off-by-one, transcript
      growth vs MaxTokens) — findings fixed or filed
- [x] T025b (Opus 4.8) Cure the two T025 filed findings: (1) FILED-1 governor defect —
      loop feeds estimator only on completed terminations (landed/model_done/
      cap_exhausted), nothing on admission_refused/provider_error/ctx_done, per the
      amended loop-api.md Run guarantee (successes-only doctrine); failing-test-first
      in internal/toolloop, adjust T023's whole-loop tests if they pinned the old rule.
      (2) FILED-2 pre-existing -race failure in the mind test harness
      (internal/mind/handlers_test.go newLoopMind aliases live sim state as replica) —
      freeze the replica snapshot so go test -race ./internal/mind/ passes.
- [x] T026 [P] (Sonnet) Reconcile spec-014 tool-catalog contract (the recorded qty/
      ParamKind debt note) and this spec's contracts to as-shipped values, in
      specs/014-tool-registry/contracts/tool-catalog.md +
      specs/017-agent-tool-loop/contracts/
- [ ] T027 Run quickstart.md end-to-end (both tiers, fallback, replay-verify, budget
      sanity) and record evidence on TASK-52
- [ ] T028 Re-pin grounding wiki notes touched by this feature
      (llm-orchestrator, cognition, agent-mind, tool-registry, event-types,
      sim-state-reducer, metatron) via /grounding-wiki:wiki-update (constitution
      Principle IV; after merge-ready diff is final)

---

## Dependencies & Execution Order

- **Phase 2** blocks everything; T002/T003 parallel, then T004; T005 blocks T006–T008
  (T006/T007 parallel).
- **US1 (Phase 3)** is strictly ordered T009→T013 (single Opus lane through
  toolloop→mind).
- **US2 (Phase 4)** needs US1 complete.
- **US3 (Phase 5)**: T016/T017 parallel Sonnet lanes any time after Phase 2; T018 needs
  T009+T016; T019 last.
- **US4 (Phase 6)**: T020 needs T018; T021/T022 parallel after their deps.
- **US5 (Phase 7)**: T023 after T008+T009; T024 last.
- **Polish** after all stories.

### Parallel opportunities

- T002 ∥ T003 (registry, Sonnet)
- T006 ∥ T007 (two provider callers, Opus)
- T016 ∥ T017 (sim payloads, Sonnet) — may run alongside Phase 3's Opus lane
- T021 ∥ T022; T026 ∥ T027 prep

## Implementation Strategy

MVP = Phases 1–4 (both P1 stories: the loop + determinism). Then US3 (observability)
→ US4 (metatron/fallback) → US5 (governor) → polish. Stop-and-validate at every
checkpoint; commit per task or logical group on the single task-52 branch. The
orchestrator (planning tier) delegates each task to `spec-implementer` with the tier
tag above via the Agent tool's `model` param, recording tier + rubric justification on
TASK-52.
