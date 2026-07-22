---
id: TASK-24
title: 'Local tier contention: concurrent worlds share one Ollama with no coordination'
status: To Do
assignee: []
created_date: '2026-07-20 00:40'
updated_date: '2026-07-22 04:34'
labels:
  - engine
  - llm
dependencies: []
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Observed live (2026-07-19): two daemons (a proving world + the operator's own world from a second checkout) each ran their serialized local-tier traffic against the same gemma endpoint; combined load pushed single calls past the 90s callTimeout, tripping both circuit breakers into a deadline-exceeded/circuit-open thrash. Each orchestrator assumes exclusive ownership of the local model. Options to design: an advisory lock/lease on the endpoint, a per-endpoint concurrency guard, adaptive timeouts from measured latency, or document one-world-per-model as an operational rule and surface a 'model contended' hint in status when latency collapses.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Two worlds pointed at one local endpoint either coordinate (no mutual circuit-thrash) or the status surface names the contention plainly
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: no 90s callTimeout exists — per-call cap is workerCallCap = 2 min (llm.go:130), HTTP client timeout 120s (providers.go:41). Breaker/fail-fast machinery unchanged (health.go; ErrTierDown/ErrQueueFull llm.go:81-82). Treat this task's findings as INPUT to the TASK-35 design session — its per-endpoint concurrency guard may subsume the advisory-lock option here; ordered adjacent on the board.
<!-- SECTION:NOTES:END -->
