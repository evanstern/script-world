---
id: TASK-27
title: 'Metatron v2: tool-gated long-running agency'
status: In Progress
assignee: []
created_date: '2026-07-20 19:06'
updated_date: '2026-07-24 05:05'
labels: []
dependencies:
  - TASK-53
  - TASK-52
priority: medium
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Evolve Metatron from a single-turn console responder into a long-running agent whose ONLY paths to world action are registered tools; conversation with the player stays free-form and tool-free.

Decisions from the 2026-07-20 design discussion (this task is the durable record):

1. Tool wire = strict-JSON envelope (1a). Extend the turn contract from {say, nudge} to {say, tool_calls: [{tool, args}]|null} with a bounded execute-and-feed-back loop in Go (cap ~4 iterations). Provider-agnostic (works through the 9router openai_compat cloud tier); the orchestrator's text-in/text-out Submit surface is unchanged. A Go tool registry (name -> schema, validator, executor, charge cost) is the single source of truth: it renders tool docs into the prompt now, and can emit native Anthropic tool schemas later if KindMetatron is ever pinned to the anthropic provider. The registry is the structural firewall generalized: no code path from model output to the world except through it; sentinel test in metatron_test.go extends to assert this.

2. Watch tier = compiled predicates + cheap LLM confirm (2b). monitor_and_act(condition, action_prompt) places a standing order; one model call at placement compiles the NL condition to structural predicates (event_types, agent, keywords) evaluated FREE in Go inside Observe (push-based — no polling; Metatron already receives every event). Fuzzy conditions compile to a coarse filter plus a rate-capped confirmation call per filter hit on a new KindMetatronWatch routed cheap (haiku or local gemma). Never per-event model evaluation without a structural filter. Standing orders are event-sourced state (metatron.order_placed/triggered/cancelled/expired), ride State through snapshots/replay, small concurrent cap (~3), TTL in game-days. Trigger execution = system-authored turn through the same single-flight turn path -> normal tool loop -> lands as recorded injection, appends to transcript, queues as a moment so the player sees what the angel did while away.

3. Nudge taxonomy = omen + vision, day omens defer (3a). dream folds away. send_omen = night-only, one villager or a group; send_vision = one villager, any time. A daytime send_omen auto-defers to the next sim.night_started as a system-generated standing order (unifies the machinery). Charges: 1 charge per landed omen/vision including triggered ones; an order firing on an empty bank queues a moment ('strength was spent') rather than silently dropping.

Meta tools: pause / start / adjust_speed wrap the existing loop controls (same functions the IPC server uses; Metatron needs a small loop-control interface). Free (no charge), but the fixed frame pins: use only when the player asks or a standing order says so — the v1 'acts only when told' contract is relaxed ONLY to what the player pre-authorized via monitor_and_act.

Budget honesty: triggered turns respect ErrBudgetExhausted/ErrTierDown like console turns — degrade to a queued moment, never retry-loop. Cost shape: tool-loop turn = 1+k cloud calls (~3-6k prompt tokens); pennies/day on haiku, safe against the $100 ceiling. The only dangerous shape is unfiltered per-event model evaluation — structurally prevented by the compile-at-placement design.

Grounding: docs/wiki/metatron.md, docs/wiki/llm-orchestrator.md, docs/wiki/chronicle.md, docs/wiki/ipc-protocol.md. Event vocabulary confirmed: agent.slept/agent.woke exist, so 'when Rowan next falls asleep' compiles to a structural predicate. Next step when work begins: speckit-specify -> specs/006-metatron-agency + spec-bridge:link back to this task.

Spec: specs/029-metatron-agency
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Tool registry is the sole world-action path: a sentinel/audit test proves no code path from Metatron model output to InjectSocial or loop controls exists outside registered tools; conversation (say) requires no tool
- [x] #2 Turn contract extended to {say, tool_calls} with a bounded Go tool loop; unusable output still downgrades to safe apology with nothing landed and nothing spent
- [x] #3 Game/meta tools pause, start, adjust_speed work end-to-end from a console instruction, are charge-free, and the fixed frame restricts them to player-requested or pre-authorized use
- [x] #4 send_omen (night-only, individual or group) and send_vision (one villager, anytime) land as atomic InjectSocial batches spending 1 charge; daytime send_omen auto-defers to next sim.night_started; dream form is retired
- [x] #5 monitor_and_act places an event-sourced standing order (placed/triggered/cancelled/expired events on State, survives restart+replay, cap ~3, game-day TTL); compiled predicates evaluate in Go with zero per-event model calls
- [x] #6 Fuzzy conditions use KindMetatronWatch (routed cheap) as a rate-capped confirm on filter hits only; a condition that cannot compile is refused with counsel
- [x] #7 A triggered order executes through the single-flight turn path, lands its nudge, appends to transcript, and surfaces as a queued moment in the next console reply
- [x] #8 Budget/degraded honesty: an order firing with empty charge bank or exhausted budget queues an honest moment instead of acting or retry-looping
- [x] #9 docs/wiki re-pinned for touched notes (metatron, llm-orchestrator, event-types) via grounding-wiki:wiki-update before merge
- [x] #10 Spec phase: Foundational (Blocking Prerequisites)
- [x] #11 Spec phase: User Story 1 — Omens and visions replace dreams (P1)
- [x] #12 Spec phase: User Story 2 — Standing orders via monitor_and_act (P1)
- [x] #13 Spec phase: User Story 3 — Triggered orders act while away (P1)
- [x] #14 Spec phase: User Story 4 — Daytime omens defer to nightfall (P2)
- [x] #15 Spec phase: User Story 5 — Meta tools: pause, start, adjust speed (P2)
- [x] #16 Spec phase: User Story 6 — Fuzzy conditions confirmed cheaply (P3)
- [x] #17 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Re-ground against shipped TASK-52/53 substrate (done: metatron runs toolloop.Run with LoopRosterMetatron; registry owns schemas/costs; capabilities.json gates roster)
2. Worktree .worktrees/task-27 (branch task-27-metatron-agency) — root stays on main
3. speckit-specify -> specs/029-metatron-agency (next free number; 006 taken)
4. Clarify ambiguities from the task's recorded design decisions (artifact-first), then speckit-plan + speckit-tasks
5. spec-bridge:link back to TASK-27
6. Implement via spec-implementer on Opus 4.8 (doctrine-adjacent, cross-package: internal/llm routing kind, internal/metatron, internal/sim event-sourced orders)
7. wiki-update re-pin (metatron, llm-orchestrator, event-types, tool-registry, tool-loop) + player-docs freshness
8. One PR from the worktree
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: Decision #1's infrastructure (Go tool registry as single source of truth + bounded execute-and-feed-back loop) is now owned by TASK-53 (registry, Layer 1) and TASK-52 (agent tool-use loop), written 2026-07-21 after this task. Re-scope: TASK-27 consumes that substrate and contributes the Metatron-specific pieces — roster (send_omen, send_vision, monitor_and_act, pause/start/adjust_speed), KindMetatronWatch routing, standing-order event sourcing, charge economy expressed as tool costs. Decision 1a's strict-JSON envelope may be superseded by TASK-52's provider-native tool calling — resolve in the 52 spec, not here. Other grounding verified current: sentinel test metatron_test.go:272, agent.slept/woke executor.go:385/128, KindMetatron llm.go:37. Stale next-step: 'specs/006-metatron-agency' — 006 is taken (norms); use the next free spec number. Deps added: TASK-53, TASK-52.

Batch A (T001-T004) gated PASS: 4 commits on task-27-metatron-agency (a972b62 llm kind+backfill, 79a9042 toolloop schema walker, c4eb9c5 registry migration, 9302af7 sim order substrate). Orchestrator re-verified: go build/vet + fresh go test on tool/sim/metatron green. Implementer findings folded: event_types enum pinned to norm.violated (meeting.norm_enacted emitted by nowhere — contract updated); metatron_watch mapped to cognition class metatron (Batch C T021 to confirm estimator fit or split a cheap class); route-backfill names default providers verbatim (bites only custom-renamed configs missing the route — review note); BATCH-A BRIDGE comments in internal/metatron mark landNudge mapping + handler-less declared tools for Batch B.

Model-tier record: Batch B (T005-T015, Phases 3-5 P1 core) → spec-implementer on Opus 4.8 — rubric: doctrine-adjacent (single-flight turn path refactor, firewall sentinel, charge economy honesty) + concurrency (trigger queue/worker, turnBusy bounded-wait, absorb-path matching).

Batch B (T005-T015) gated PASS: 3 commits (8ddb4cc US1, 47d72bf US2, 1681ab7 US3). Orchestrator re-verified: fresh go test + go test -race on internal/metatron green. Gated decisions accepted: known-act precheck keyed on Origin==system (dormant until T016 deferral orders — matches R11/R12); multi-target vision structurally refused via single target param resolution. Batch C hand-offs on record: meta-tool handlers (T018 + clockSpeeds drift guard), daytime deferral (T016 — placeOrder system plumbing ready), fuzzy confirm (T021 — fuzzy orders matched but deliberately skipped in matchOrders until then).
Model-tier record: Batch C (T016-T022) → spec-implementer on Opus 4.8 — rubric: cross-package (daemon LoopControl wiring, llm KindMetatronWatch consumer) + concurrency (confirm rate-cap in absorb path) + doctrine-adjacent (charge economy at trigger time, fixed-frame edit).

Batch C (T016-T022) gated PASS: 3 commits (f05c36a US4 deferral, 14c7980 US5 meta tools + LoopControl, 6ca10f8 US6 fuzzy confirm). Orchestrator re-verified: fresh tests on metatron/tool/daemon + race on metatron, all green. Findings adjudicated: (1) start's speed arg inert at the loop's resume command — planning ruling: honor supplied speed as set_speed THEN resume; contract amended; fix lands in Batch D. (2) deferred-omen 400-rune action cap edge accepted + documented in spec assumptions. (3) metatron_watch estimator normalization accepted per the digest bare-Submit precedent (mildly pessimistic = sheds first under pressure, which is the honest degradation).
Model-tier record: Batch D (T023-T025 + start-speed fix) → spec-implementer on Sonnet (default tier) — rubric: routine slices (view/rendering, doc reconciliation, live validation run, single-package two-line handler fix with exact instruction).

Batch D (start-fix + T023-T025) gated PASS: fc2b3ad start-with-speed = set_speed then resume (per ruling; failing set_speed never reaches resume — pinned), 6a27a94 CLI/TUI order+clock surfaces (calibrate/Kinds enumeration confirmed automatic), 1442a11 docs reconciliation. Orchestrator re-verified metatron/tui/cmd fresh — green. T025 validation matrix on record: full suite + race green; live model-free scenarios ran (boot gates, new roster via metatron_status, pause/resume IPC, log stream); conversational Scenarios 1-5 BLOCKED in this environment — no Anthropic credentials and the default local model absent (TASK-84's dead-default reproduced live; failed fast+clean, no hang). Live conversational validation needs credentials — flagged for reign-test after merge. T026 (wiki re-pin, AC #9) next: running wiki-update in the worktree so the re-pin rides the PR (AC says before merge).

Wiki re-pin + player docs complete IN-BRANCH (AC #9 'before merge' honored): 18 notes re-verified + new metatron-orders.md (67a9f9a), 6 player pages refreshed (126f810), plan/freshness gates green (37 notes), player-docs check 7/7 fresh. The sweep flushed two real defects, both fixed + tested before pinning: TUI digest catalog missing the four metatron.order_* types (7699e72, Sonnet tier — view code) and a doubled/mislabeled 'The player says:' directive in the turn prompt where a system turn's order text masqueraded as player speech (bd02ecc, Opus tier — doctrine-adjacent; adversarially confirmed; never reached durable records). ACs 1-8 proven by the test suite (sentinel firewall audit, reducer matrices, race-clean concurrency, replay identity); live conversational reign-test remains env-blocked (no credentials — TASK-84) and is the one open follow-up. 26/26 tasks done; spec state Done-eligible; PR next.

PR #59 open: https://github.com/evanstern/promptworld/pull/59 — 17 commits, one branch, one PR. Remaining after merge: worktree/branch cleanup, ff-pull root, live conversational reign-test when credentials are available (TASK-84 env gap).
<!-- SECTION:NOTES:END -->
