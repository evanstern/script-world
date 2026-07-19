---
id: TASK-6
title: 'LLM orchestrator: tiers, budget, degraded mode'
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 04:05'
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

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-6-llm-orchestrator off main
2. internal/llm package: Orchestrator with two tiers — local (OpenAI-compatible HTTP, Ollama default) and cloud (Anthropic Messages API); kind->tier routing table (planner/conversation->local, consolidation/narrator/drama->cloud)
3. Bounded per-tier queues + worker pool; sync Submit with admission control (backpressure surface for TASK-7)
4. Spend meter: per-call token costs from config pricing, persisted monthly in store meta; ceiling (default $100/mo) refuses cloud calls with ErrBudget — throttle, never silent overspend
5. Health/degraded: consecutive failures mark a tier down with backoff probes; Submit fails fast while down; the sim loop is structurally isolated (orchestrator never touches it)
6. Config: llm.json in save dir (endpoints/models/pricing; API keys via env refs only, never stored); scriptworld new writes defaults
7. Integration: daemon starts orchestrator when config present; protocol status gains llm section; llm_call protocol cmd + scriptworld llm subcommand for end-to-end proof; metatron pane shows orchestrator status
8. Tests: httptest mock providers — routing (AC#1), metering + ceiling throttle (AC#2), tier-down/fast-fail/recovery + world-keeps-ticking (AC#3); -race suite
9. Wiki note llm-orchestrator + re-pins; PR; board close-out
<!-- SECTION:PLAN:END -->
