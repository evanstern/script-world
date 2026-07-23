---
id: TASK-35
title: >-
  Multi-provider LLM division of labor: routing criteria across providers and
  sources — design session
status: In Progress
assignee: []
created_date: '2026-07-21 02:17'
updated_date: '2026-07-23 17:31'
labels:
  - engine
  - llm
  - design-session
dependencies: []
references:
  - backlog/tasks/task-6 - LLM-orchestrator-tiers-budget-degraded-mode.md
  - >-
    backlog/tasks/task-24 -
    Local-tier-contention-concurrent-worlds-share-one-Ollama-with-no-coordination.md
priority: high
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Architect how model traffic divides across multiple LLM providers/sources (local Ollama models, 9router cloud endpoint, Anthropic direct, future providers) based on explicit routing criteria, evolving TASK-6's fixed kind→tier table into a real routing layer.

Questions to settle in the session:
- Routing criteria: what dimensions drive placement — call kind (planner/conversation/consolidation/narrator/drama), latency tolerance, cost per token, context size, quality floor, provider health/availability?
- Provider registry: how are providers/sources declared and capability-tagged in llm.json (models, pricing, concurrency limits, endpoints), and how does routing choose among multiple candidates for a tier?
- Fallback chains: when the preferred provider is down (circuit open), degraded, or budget-throttled, what is the ordered fallback — and which call kinds may NOT fall back (e.g. persona-sensitive calls)?
- Interaction with existing machinery: spend meter/ceiling (per-provider or global?), per-tier circuit breakers, bounded queues, and the TASK-24 contention problem (a routing layer that knows about per-endpoint concurrency could subsume the advisory-lock option).
- Operational surface: how status/TUI names where a call went and why (routing decision legibility).

Related: TASK-6 (two-tier orchestrator, Done), TASK-15 (9router cloud tier, Done), TASK-24 (local endpoint contention — its concurrency-guard option may become a routing criterion here), TASK-32 (cognition horizon — latency budgets are a routing input).

Session output (2026-07-23): decision-5 (provider division of labor) + the spec below, authored on branch task-35-provider-routing.

Spec: specs/024-provider-routing
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A design session produces a durable design doc (decision record or spec) defining the routing criteria, provider registry shape, and fallback-chain semantics
- [x] #2 The design states how routing interacts with the spend meter, circuit breakers, and the TASK-24 contention scenario
- [x] #3 Follow-on implementation tasks (or a Spec Kit spec) are cut from the design and placed on the board
- [x] #4 Spec phase: Setup
- [x] #5 Spec phase: Foundational (Blocking Prerequisites)
- [x] #6 Spec phase: User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP
- [x] #7 Spec phase: User Story 2 — Division of labor: per-provider speed truth (Priority: P2)
- [x] #8 Spec phase: User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3)
- [x] #9 Spec phase: User Story 4 — One wallet, per-provider attribution (Priority: P4)
- [x] #10 Spec phase: User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5)
- [x] #11 Spec phase: Polish for the engine slices (Opus tier wrap-up)
- [x] #12 Spec phase: User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]
- [ ] #13 Spec phase: Composition with spec 025 (post-merge reconciliation)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Follow the TASK-32 design-session pattern: 1) Cut worktree .worktrees/task-35 (branch task-35-provider-routing) from fresh origin/main. 2) Write decision-5 (provider-routing doctrine: registry + deterministic ordered fallback chains, per-provider breakers/slots/estimators, one global wallet) via backlog CLI in the worktree so it rides the PR. 3) speckit-specify the provider-routing spec (registry shape in llm.json, routing criteria, fallback semantics incl. no-fallback kinds, meter/breaker/TASK-24 interactions, status legibility). 4) spec-bridge:link the spec to TASK-35. 5) speckit-plan + speckit-tasks. 6) Delegate implementation to spec-implementer on Opus 4.8 (concurrency/scheduling logic in internal/llm — escalation rubric match). 7) Check ACs as artifacts land, sync, PR, wiki-update, Done.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live evidence for this design session (2026-07-21): local server parallelizes natively (4 concurrent cogito:3b calls in 0.98s wall vs 3.8s single cold call; no multi-instance setup needed — one loaded model, N slots). Cost/quality sketch from today's measurements: cogito:3b ~1s/call warm vs gemma4:12b-mlx ~20s under load; 48-128-token structured outputs (musings, conversation turns) are 3B-viable, planner/narrator prose is not — division of labor should route cheap chatty classes to the small parallel model and keep quality classes on gemma (both loaded simultaneously fits memory). Caution from TASK-42: small models raise empty-utterance rates — routing design must pair with the retry/tolerance work. Mechanical prerequisite now split out as the parallel-tier task (N workers per tier); this session owns the routing criteria (per-class? per-provider incl. cloud/9router? cost/latency/quality axes).

Re-grounding 2026-07-22: no drift — kind-to-tier table (llm.go:61) and breaker/queue machinery hold. Mechanical prereq TASK-45 (parallel local tier workers) is Done. TASK-24's endpoint-contention findings feed this session; its advisory-lock option may be subsumed by the per-endpoint concurrency guard designed here.

Design session complete (2026-07-23): doctrine recorded as decision-5 (registry + deterministic ordered chains; chain order IS the quality statement; one wallet with per-provider attribution; per-provider breaker/queue/lane/workers/estimator; endpoint-capacity advisory leases subsuming TASK-24; persona scene pinning; legacy llm.json equivalence). Spec specs/024-provider-routing authored on branch task-35-provider-routing (6 prioritized stories: registry+legacy equivalence P1 MVP, division of labor P2, chain-walking fallback + scene pinning P3, one wallet P4, endpoint leases P5, status/TUI legibility P6; 18 FRs, 8 SCs; quality checklist all-pass). AC1+AC2 proven by decision-5 + spec. Next: speckit-plan / speckit-tasks on the branch, then delegated implementation (Opus 4.8 rubric tier — concurrency/scheduling in internal/llm).

spec-bridge sync: Setup: 0/1 · Foundational (Blocking Prerequisites): 0/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 0/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 0/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 0/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 0/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 0/2 · Polish for the engine slices (Opus tier wrap-up): 0/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 0/3

AC-3 proven: follow-on implementation cut from the design as spec 024's tasks.md (19 tasks, 9 phases) and mirrored onto this task as Spec-phase ACs via spec-bridge. Tier decisions per constitution V recorded in plan.md slice map: Phases 1-8 (US1-US5 + engine polish) → spec-implementer on Opus 4.8 (concurrency/scheduling/governor logic in internal/llm + cross-package seams: cognition, mind, toolloop, ipc); Phase 9 (US6 status/TUI/CLI surfacing) → Sonnet (view/rendering code).

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 3/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 0/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 0/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 0/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 0/2 · Polish for the engine slices (Opus tier wrap-up): 0/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 0/3

Slice 1 (Opus 4.8, T001-T007) landed on the branch: commits 636df4e (registry generalization T002-T004) + a80b0b8 (equivalence/validation/chain-head tests T005-T007). T001 baseline: pre-change go test -race ./... fully green, 20 packages, no flakes. Post-slice: full -race suite green (uncached), go vet clean; orchestrator-session spot-check re-ran internal/llm green. Two spec-directed deviations accepted by gate review: budget gating keys on pricing class not tier identity (FR-009/decision-5), reasoning-effort nil-default keys on pricing class (data-model). Chain-walking, estimator seeding, attribution, leases, surface polish deliberately deferred to their slices.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 3/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 2/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 2/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 1/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 0/2 · Polish for the engine slices (Opus tier wrap-up): 0/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 0/4

Slice 2 (Opus 4.8, T008-T012) landed: 21477a3 (per-provider estimators + calibration seam), a4e1f47 (chain-walk fallback + scene pinning), dffc8e3 (spend attribution). Full -race suite green incl. unmodified legacy-equivalence suite; orchestrator spot-check re-ran llm/toolloop/cognition green. Gate ruling on the implementer's open question: v2-registry calibration in calibrate.go is real scope, added explicitly as T020 [US6] on the Sonnet slice (pin mechanism from slice 1 makes it view-layer work); legacy worlds already calibrate correctly via derived provider names. Remaining: US5 leases + engine polish (slice 3, Opus), US6 surfaces incl. T020 (slice 4, Sonnet).

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 3/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 2/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 2/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 1/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 2/2 · Polish for the engine slices (Opus tier wrap-up): 2/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 0/4

Slice 3 (Opus 4.8, T013-T016) landed: 91ed103 (flock lease pool + worker integration), 94e6a98 (package docs reconciled to providers/chains). Full -race suite green; quickstart automated sections all pass (T016 outputs recorded by implementer; orchestrator re-ran lease + validation tests green). Gate rulings: (1) quickstart §2 regex was an authoring bug — fixed in quickstart.md ('Validation|ValidV2|LoadConfig'); (2) contended is pool-scoped by design ruling — endpoint congestion is one truth shared by providers on that endpoint; data-model.md updated to say so; (3) lease boot warnings via overridable leaseWarnf to stderr accepted (warn-not-error; UserHomeDir failure only discoverable inside New). Remaining: slice 4 (Sonnet) — T017 TUI table, T018 CLI surfaces, T019 live e2e proof, T020 v2 calibrate.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 3/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 2/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 2/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 1/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 2/2 · Polish for the engine slices (Opus tier wrap-up): 2/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 0/4 · Composition with spec 025 (post-merge reconciliation): 0/2

Composition review vs TASK-72/spec 025 (user-requested, 2026-07-23): (1) max_tokens composes by construction — 025 made budgets top-level and KIND-scoped, exactly decision-5's kind/provider split; no per-tier token limits existed in 024's assumptions and no per-provider token fields are added; v2 marshal must round-trip max_tokens+loop_max_rounds byte-for-byte (cut as T021 with the conflict-bearing rebase of config.go). (2) 025's in-loop retry exposed a mid-cognition provider-switch hazard under per-call chain-walking (retry could land on a different provider than the transcript's earlier rounds — ID-convention mixing + estimator mis-attribution); ruled: tool-use loop pins its provider at run start incl. the retry, the run-level analog of scene pinning — FR-008 extended, research R9 added, cut as T022 (Opus). Rebase waits for the in-flight Sonnet US6 slice to land.

spec-bridge sync: Setup: 1/1 · Foundational (Blocking Prerequisites): 3/3 · User Story 1 — Providers are declared, routes are chains, yesterday's worlds still boot (Priority: P1) 🎯 MVP: 3/3 · User Story 2 — Division of labor: per-provider speed truth (Priority: P2): 2/2 · User Story 3 — Fallback is chain-walking; personas never switch voices (Priority: P3): 2/2 · User Story 4 — One wallet, per-provider attribution (Priority: P4): 1/1 · User Story 5 — Worlds sharing an endpoint coordinate instead of thrashing (Priority: P5): 2/2 · Polish for the engine slices (Opus tier wrap-up): 2/2 · User Story 6 — The operator can see where every call went and why (Priority: P6) [Sonnet slice]: 4/4 · Composition with spec 025 (post-merge reconciliation): 0/2

Slice 4 (Sonnet, T017-T020) landed: 1bf4ccd (TUI provider table), dd4279f (one-shot names provider + skip reasons), 0bc2fbd (v2 calibrate by declared provider, --tier kept as deprecated alias). T019 LIVE PROOF against real Ollama: conversation→cogito:3b (10.3s), planner→gemma4:12b-mlx (1.8s), forced fallback on meeting chain [bogus,gemma] — 3 hard failures opened bogus's breaker (post-dispatch-failure-is-final doctrine held), 4th call skipped with 'skipped: bogus (circuit-open)' and served by gemma; status --json provider table matched the contract; throwaway world cleaned up. Gate review: accepted the two additive read-only seams (Orchestrator.ProviderConfig accessor, toolloop.Job.Provider passthrough — the latter is exactly the seam T022 run-pinning needs) and the --tier deprecated-alias choice; TUI evidence rendered from live payload accepted (no interactive terminal available). Orchestrator spot-check: vet clean, tui+cmd suites green. Remaining: slice 5 (Opus) T021 rebase across the 025 merge + T022 run-level pinning.
<!-- SECTION:NOTES:END -->
