---
id: TASK-6
title: 'LLM orchestrator: tiers, budget, degraded mode'
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
labels:
  - engine
  - llm
dependencies:
  - TASK-2
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Call layer for all model traffic: local tier via Ollama/9router (OpenAI-compatible HTTP), cloud tier for consolidation/narrator/drama; per-call metering against the hard $100/month ceiling; queueing + backpressure (local throughput caps max sim speed); graceful degradation when inference is unreachable (executor keeps ticking, thoughts queue/reflex); prompt caching. Grounding: grounded-assumptions.md (Cost & inference).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Planner/conversation calls route local; consolidation/narrator calls route cloud
- [ ] #2 Live spend meter; hitting the budget ceiling throttles rather than silently overspending
- [ ] #3 Killing the local model does not crash the world; a designed degraded state engages
<!-- AC:END -->
