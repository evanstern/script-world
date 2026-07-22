---
id: TASK-52
title: 'Agent tool-use loop: minds call tools instead of prompt stuffing'
status: In Progress
assignee: []
created_date: '2026-07-22 02:20'
updated_date: '2026-07-22 23:00'
labels:
  - agent-mind
  - llm
dependencies:
  - TASK-53
priority: high
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
<!-- SECTION:DESCRIPTION:BEGIN -->
Prerequisite for the agent-authored journal in TASK-16 and any future agent-callable capability.

Current state: the LLM layer is strictly single-shot — internal/llm Orchestrator.Submit(ctx, Request) returns one Response (llm.go:254), providers send one messages array with no tools parameter (providers.go), and internal/mind parses free-text replies (parse.go). There is no tool schema, no tool-call parsing, no multi-turn loop anywhere in the codebase.

Needed: an agentic loop for agent minds — a mind call can declare a set of tools; the model may respond with tool calls; the loop executes them and feeds results back until the model produces a final answer (with a hard iteration/budget cap).

Design considerations to resolve in the spec:
- Tool declaration + dispatch: a small registry mapping tool name -> handler; handlers are ordinary Go funcs. Read-only tools (search_journal, read_journal) just return data; mutating tools (write_journal_entry, delete_from_journal) must emit events and be reducer-applied so replay stays deterministic (the model transcript is not replayed — only the emitted events are).
- Provider support across tiers: Anthropic SDK has native tool use; local tier (gemma via OpenAI-compat / 9router) has varying function-calling quality — spec must decide between native tool-calling APIs per provider vs a provider-agnostic structured-output convention, and what the fallback is when a tier cannot tool-call reliably.
- Metering/governor: today one Submit = one metered call; a tool loop is N calls per cognition — cognition estimates, calibration, and the governor (internal/cognition, llm/meter.go) must account for multi-call cognitions.
- Determinism boundary: tool loops happen at decision time (like existing planner calls); everything durable they cause lands as events. Replay never re-runs the loop.

First consumer: TASK-16 journal tools (write_journal_entry, search_journal, optional read_journal / delete_from_journal). Spec via Spec Kit before implementation per constitution.
<!-- SECTION:DESCRIPTION:END -->

Spec: specs/017-agent-tool-loop
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Mind LLM calls can declare tools; a loop executes model tool calls via a registry and feeds results back until a final answer, with a hard iteration/budget cap
- [ ] #2 Mutating tool handlers emit events and are reducer-applied; replay never re-runs the tool loop and reproduces identical state
- [ ] #3 Works on at least one local-tier and the cloud-tier provider, with an explicit documented fallback for tiers that cannot tool-call reliably
- [ ] #4 Metering/governor accounts for multi-call cognitions (estimates + calibration remain sane)
- [ ] #5 Tool-call trace is first-class and correlatable end-to-end: every tool call is a recorded artifact (including rejected/never-grounded calls), and downstream grounding events link back to the causing call — e.g. JobID carried into IntentSetPayload — so 'tool call → verdict → grounding chain' is queryable from the event log without adjacency inference
- [x] #6 Spec phase: Setup
- [x] #7 Spec phase: Foundational (blocking all stories)
- [ ] #8 Spec phase: User Story 1 — a mind acts by calling a tool (P1) 🎯 MVP
- [ ] #9 Spec phase: User Story 2 — replay reproduces state without re-running loops (P1)
- [ ] #10 Spec phase: User Story 3 — every tool call is a first-class correlatable artifact (P2)
- [ ] #11 Spec phase: User Story 4 — both tiers + documented fallback (P2)
- [ ] #12 Spec phase: User Story 5 — governor stays sane on multi-call cognitions (P3)
- [ ] #13 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit at specs/017-agent-tool-loop: spec (3 clarifications resolved with Evan 2026-07-22), plan, research R1-R15, data-model, contracts (loop-api, events, provider-wire), quickstart, tasks (28 tasks, 8 phases). Architecture: internal/llm gains tool-call transport (Tools/Turns/ToolCalls, one Submit stays one metered call); NEW internal/toolloop drives the bounded loop (cap 8 rounds default, one landed acting call per cognition, read tools exempt); mind planner + metatron turn migrate; scheduled musing deleted (muse = roster choice); cloud native Anthropic tools, local native-first with per-model json-envelope fallback; cog.tool_call artifact events + IntentSetPayload.Job (omitempty) close the AC#5 correlation chain; governor observes whole-loop wall time, meter stays per-call. Implementation in .worktrees/task-52, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
2026-07-21 design exploration decisions (with Evan):
1. Layer split confirmed — TASK-53 (tool registry, behavior-identical formalization) stands alone and precedes this task; TASK-52 is now specifically the agentic loop (Layer 2): per-provider native tool calling + bounded execute-and-feed-back loop.
2. Cardinality: ONE acting tool per cognition (world or expressive) — read tools (search_journal/read_journal) are exempt, they are mid-loop lookups that inform the cognition, not actions. Journal writes therefore carry opportunity cost: a cognition spent journaling is not spent acting.
3. muse merges into the tool roster (no separate scheduled musing channel long-term); agents choosing to muse via tool call lands with this task's loop.
4. Core principle to preserve verbatim in the spec: a tool call is a REQUEST; an event is the FACT; the gate decides; the executor grounds work in time and space. Speaking/musing/thinking are tools too — game-state integrity applies to expression, not just world mutation.

2026-07-22 (with Evan): added the tool-call observability AC. Today (post-TASK-53) tool usage is visible only as the landing (agent.intent_set{goal, source} / agent.plan_set); the call itself has no independent record, and correlating a completion back to its causing thought requires agent+adjacency inference (cog.outcome carries the job id, IntentSetPayload does not). Fold the cure into this task's loop design: the request artifact plus JobID threading on IntentSetPayload (additive payload field — verify snapshot/replay byte-stability for old logs via omitempty, the TASK-32 pattern). Related registry note: a numeric ParamKind (for storage-verb qty) is also owed to this task — recorded in specs/014-tool-registry/contracts/tool-catalog.md.

2026-07-22 planning session (Fable 5): spec 017 authored+clarified+planned+tasked; linked via spec-bridge. Clarification decisions (Evan): local tier native-first w/ per-model json fallback; scheduled musing channel removed NOW; scope = villager planner + metatron turn. Tier decision per constitution v1.1.0 Principle V rubric: Opus 4.8 for T005-T015, T018, T020, T023-T025 (cross-package llm/cognition/mind/metatron orchestration, concurrency, doctrine-adjacent landing-door behavior); Sonnet for T002-T004, T016-T017, T019, T021-T022, T026 (single-package registry additions, additive sim payloads with pinned byte-stability contracts, docs). Justification: the loop driver, transport, and channel removal touch internal/llm + internal/cognition + internal/mind orchestration = senior tier per rubric; registry/payload slices are routine with explicit contracts.

2026-07-22 implementation underway in .worktrees/task-52 (branch task-52-agent-tool-loop, forked c0206a6). T001 baseline green (full suite incl e2e). Sonnet spec-implementer landed T002 (4c96954: Number ParamKind, qty params on 4 storage verbs, Validate extensions, Read-roster legal), T003 (64fceea: tool.InputSchema derivation), T004 (0915fb8: set_plan entry w/ authored schema, LoopRosterVillager/Metatron, legacy surfaces byte-stable), T004b (a607896: implementer-found plan gap cured — spec-014 boot gate ValidateToolCoverage re-keyed on goal-door vocabulary so non-goal-door World tools like set_plan are deliberately exempt; daemon boot verified, full suite incl e2e green). Known pre-existing flake: internal/metatron TestDigestFailureCarries under full-suite load. Phases Setup + Foundational-registry portion complete; next: T005-T008 llm transport (Opus 4.8).

2026-07-22 Opus 4.8 spec-implementer landed the llm transport slice: T005 (da95388: Turn/Block/ToolDecl/ToolCall/StopReason types, Request.Tools/Turns/SkipObserve, Response.ToolCalls/Stop, loop_max_rounds + tool_mode config w/ warn-not-error normalizers, caller interface returns callResult), T006 (373c16b: anthropic native tools; SDK v1.58.0 drops unknown schema keywords — routed via ExtraFields), T007 (561ccad: openaiCompat native function calling + tool_mode:json fallback envelope; ToolResult→user-turn mapping transport-side; env-<round> IDs derived from assistant-turn count — T009 driver must record one assistant Turn per round, pinned in driver contract), T008 (7d1a354: ObserveCognition + SkipObserve estimator seam; metering/admission/breaker untouched). Full suite incl e2e green. Foundational phase complete. Follow-up candidate noted: pre-existing flake TestEstimatorSampleCountUnderConcurrency (capacity-boundary race, reproduced on base commit).

spec-bridge sync: Setup: 1/1 · Foundational (blocking all stories): 8/8 · User Story 1 — a mind acts by calling a tool (P1) 🎯 MVP: 0/5 · User Story 2 — replay reproduces state without re-running loops (P1): 0/2 · User Story 3 — every tool call is a first-class correlatable artifact (P2): 0/4 · User Story 4 — both tiers + documented fallback (P2): 0/3 · User Story 5 — governor stays sane on multi-call cognitions (P3): 0/2 · Polish & Cross-Cutting: 0/4

2026-07-22 Opus 4.8 landed T009+T010 (branch commits 548a1a6, 49ed3fd post-rebase): internal/toolloop driver — contract surface exact, unexported submitter seam for deterministic tests, transcript invariant one-assistant-turn-per-round pinned (the env-<round> fallback dependency), final-round reads recorded unlanded by design, CallRecord.Reason = handler ResultForModel for rejections. Full suite green. Branch then rebased onto origin/main after PRs #37 (chronicle digest) + #38 (metatron miracles) merged — clean rebase, overlapping packages green. Post-#38 impact absorbed into artifacts: metatron turn now works miracles with an at-most-one-mediated-act rule that maps exactly onto loop cardinality; added T019b (work_miracle registry entry, Sonnet) and amended T020 + research R13. Next: T011-T013 mind migration (Opus).

2026-07-22 Opus 4.8 landed US1 (MVP core): T011 (fa62655: villager handlers — world verbs/set_plan wrap InjectIntent w/ talk_to guards preserved, muse lands via InjectSocial atomically, CallRecord sink buffered for T018), T012 (0ffda92: runPlan → toolloop.Run w/ runLoop seam, loopRounds threaded from llm.Config.Rounds(), MaxTokens 512 documented, tool-era system prompt, parseReply/plannerReplySchema/golden-prompt test retired, e2e fake LLM scripts native tool_calls), T013 (8f2067b: scheduled musing deleted — muse scheduling/musingSystemPrompt/KindMusing/musing DecisionClass removed; BestEffort machinery stays; parseMusing kept for meeting rephraser). Full suite incl e2e green; TestCognitionReplayByteIdentical passes on a loop-era run. As-built decisions gated and recorded: door owns the single cog.outcome on landings (mind emits OutcomeUnusable only when nothing reached a door); intra-cognition retries can yield multiple door cog.outcomes per job (events.md amended); rearm preserves today's semantics (rejection rearms, failure does not); BEHAVIOR CHANGE by design: world-verb landings carry no planner free-text reason — interiority flows via the muse tool (opportunity-cost doctrine), chronicle richness shifts accordingly.

2026-07-22 Sonnet landed T016 (0657530: CogToolCallPayload canonical order, whitelist entry, reducer no-op arm; updated the TestWhitelistDiffIdentical trip-wire deliberately per contract) and T017 (965dc4c: IntentSetPayload.Job last-field omitempty, populated only at the inject-landing arm; byte-stability pins incl absent-key assertion for reflex/executor paths). Full suite green, replay byte-identity unmodified. Next: T018+T019 (record landing + SC-003 correlation, Opus).

2026-07-22 Opus 4.8 landed T018 (7fe3b58: CallRecords → cog.tool_call via dedicated all-or-nothing emitCog batch on every termination path, ordinal-sorted, empty-buffer guarded; conversion constructor sim.NewCogToolCallPayload placed sim-side w/ plain arg types so metatron T020 reuses it without dependency inversion; reason invariant enforced-at-emission w/ backfill+log, never fatal) and T019 (84d578f: TestToolCallCorrelationChainSC003, 8/8 under -count=8; chain-granularity refinement recorded in events.md — grounding events carry job not ordinal, so rejected-grounds-nothing is job-resolvable only for rejected-only cognitions). T019 ran on Opus for seam continuity (recorded per rubric). US3 complete — board AC#5 provable from the event log. Full suite incl e2e green. Next: T019b+T020 metatron migration (Opus, T019b rides along w/ justification: work_miracle schema must mirror landMiracle expectations the same agent studies).

2026-07-22 Opus 4.8 landed T019b (2ef5477) + T020 (61d8db2): metatron on the loop. As-built decisions gated and recorded in data-model.md: work_miracle = Expressive (InjectSocial family, Events-declaration coherence) w/ flat Params not authored schema (driver validateArgs hard-routes authored schemas to the set_plan validator — latent generalization debt noted for a third authored-schema tool); converse excluded from declared roster (final text IS the reply; model_done = natural termination). Behavior upgrades vs today, intended: gate refusals feed back so the angel can correct a bad target within the cap; spec-016 nudge-over-miracle precedence dissolves into loop cardinality; no synthetic refusal suffix. turnMaxTokens 700→1024. Full suite incl e2e green. Pre-existing repo-wide gofmt discrepancy (8 untouched files, present on origin/main) noted, out of scope. US4 remaining: T021 fallback equivalence + T022 docs (Sonnet). Then US2 T014-T015, US5 T023-T024, polish.

2026-07-22 Sonnet landed T021 (ba1dd4f: TestToolModeEquivalenceNativeVsJSON in internal/toolloop — real Orchestrator vs two scripted httptest servers; full driver-level equivalence (records/termination/final/rounds identical) with confirmed wire-level divergence; zero bugs found) and T022 (836cd27: README operator docs for loop_max_rounds + tool_mode incl when-to-flip-to-json cues; docs/wiki untouched per lifecycle). US4 complete — board AC#3 satisfied. Remaining: T014-T015 (US2), T023-T024 (US5), polish T025-T028.
<!-- SECTION:NOTES:END -->
