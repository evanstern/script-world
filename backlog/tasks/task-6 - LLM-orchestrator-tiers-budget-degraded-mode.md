---
id: TASK-6
title: 'LLM orchestrator: tiers, budget, degraded mode'
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 04:16'
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
- [x] #1 Planner/conversation calls route local; consolidation/narrator calls route cloud
- [x] #2 Live spend meter; hitting the budget ceiling throttles rather than silently overspending
- [x] #3 Killing the local model does not crash the world; a designed degraded state engages
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-6-llm-orchestrator. internal/llm: two-tier call layer fully quarantined from the deterministic loop (LLM results only ever enter the world as recorded inputs). Routing per grounding (planner/conversation->local Ollama-compatible HTTP; consolidation/narrator/drama->cloud via official anthropic-sdk-go, claude-opus-4-8, prompt caching on system blocks). Spend meter persisted monthly in store meta; $100/mo ceiling refuses cloud calls at admission (zero HTTP) while local continues. Circuit breaker per tier (3 fails -> open, backoff 15s..5m, half-open probe); bounded queues (32) with fast ErrQueueFull. Config llm.json per save dir (keys by env-var name only). AC#1 proven by TestRouting + protocol-level test (mock providers count hits per tier); AC#2 by TestBudgetCeiling (refusal before HTTP, meter in status) + TestMeterPersistsAcrossRestart; AC#3 by TestDegradedAndRecovery + TestLLMCallAndDegradedWorld (dead endpoints, world ticks on) — plus live smoke: real Ollama call routed end-to-end through the daemon (cogito:3b answered a planner prompt; status showed tiers/spend). -race suite green (8 packages). Wiki: llm-orchestrator note added, 12 notes re-verified, gate green (21 notes).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
LLM orchestrator shipped: all model traffic flows through one two-tier gateway — local (Ollama-compatible) for planner/conversation volume, cloud (Anthropic SDK, claude-opus-4-8) for consolidation/narrator/drama — with a persisted monthly spend meter that throttles at the $100 ceiling instead of overspending, per-tier circuit breakers that degrade gracefully when inference dies (the world never stops), bounded-queue backpressure for TASK-7, llm.json config per world, protocol/CLI/TUI surfaces, and live verification against real local inference.
<!-- SECTION:FINAL_SUMMARY:END -->
