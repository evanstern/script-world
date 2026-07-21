---
id: TASK-35
title: >-
  Multi-provider LLM division of labor: routing criteria across providers and
  sources — design session
status: To Do
assignee: []
created_date: '2026-07-21 02:17'
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
ordinal: 29000
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
