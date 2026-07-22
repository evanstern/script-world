---
id: TASK-35
title: >-
  Multi-provider LLM division of labor: routing criteria across providers and
  sources — design session
status: To Do
assignee: []
created_date: '2026-07-21 02:17'
updated_date: '2026-07-22 04:34'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A design session produces a durable design doc (decision record or spec) defining the routing criteria, provider registry shape, and fallback-chain semantics
- [ ] #2 The design states how routing interacts with the spend meter, circuit breakers, and the TASK-24 contention scenario
- [ ] #3 Follow-on implementation tasks (or a Spec Kit spec) are cut from the design and placed on the board
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live evidence for this design session (2026-07-21): local server parallelizes natively (4 concurrent cogito:3b calls in 0.98s wall vs 3.8s single cold call; no multi-instance setup needed — one loaded model, N slots). Cost/quality sketch from today's measurements: cogito:3b ~1s/call warm vs gemma4:12b-mlx ~20s under load; 48-128-token structured outputs (musings, conversation turns) are 3B-viable, planner/narrator prose is not — division of labor should route cheap chatty classes to the small parallel model and keep quality classes on gemma (both loaded simultaneously fits memory). Caution from TASK-42: small models raise empty-utterance rates — routing design must pair with the retry/tolerance work. Mechanical prerequisite now split out as the parallel-tier task (N workers per tier); this session owns the routing criteria (per-class? per-provider incl. cloud/9router? cost/latency/quality axes).

Re-grounding 2026-07-22: no drift — kind-to-tier table (llm.go:61) and breaker/queue machinery hold. Mechanical prereq TASK-45 (parallel local tier workers) is Done. TASK-24's endpoint-contention findings feed this session; its advisory-lock option may be subsumed by the per-endpoint concurrency guard designed here.
<!-- SECTION:NOTES:END -->
