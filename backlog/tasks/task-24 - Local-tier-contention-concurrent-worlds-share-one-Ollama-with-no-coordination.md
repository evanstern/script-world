---
id: TASK-24
title: 'Local tier contention: concurrent worlds share one Ollama with no coordination'
status: Done
assignee: []
created_date: '2026-07-20 00:40'
updated_date: '2026-07-23 18:15'
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
- [x] #1 Two worlds pointed at one local endpoint either coordinate (no mutual circuit-thrash) or the status surface names the contention plainly
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: no 90s callTimeout exists — per-call cap is workerCallCap = 2 min (llm.go:130), HTTP client timeout 120s (providers.go:41). Breaker/fail-fast machinery unchanged (health.go; ErrTierDown/ErrQueueFull llm.go:81-82). Treat this task's findings as INPUT to the TASK-35 design session — its per-endpoint concurrency guard may subsume the advisory-lock option here; ordered adjacent on the board.

Closed by spec 024 US5 (TASK-35, PR #52, merge d56b272): providers may declare endpoint_capacity; participating worlds coordinate via advisory flock lease pools (~/.promptworld/endpoint-leases/, crash-reclaimable), bounding combined in-flight calls at the declared capacity so the observed mutual circuit-thrash cannot recur — and a world waiting on a saturated endpoint reports 'contended' in status/TUI instead of striking its breaker (both halves of AC#1: coordinate AND name the contention). Proven by internal/llm/lease_test.go under -race (two orchestrators, one endpoint: combined in-flight ≤ C, no contention-induced breaker opens, contended set/clear, slot reclaim after process death); the undeclared-capacity default keeps pre-024 behavior (advisory, opt-in). Operator doc: docs/llm-providers.md 'Sharing an endpoint'.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Subsumed and shipped by TASK-35/spec 024's endpoint-lease story (decision-5): registry-declared endpoint_capacity + advisory flock leases coordinate worlds sharing one endpoint; contended surfaces in status. Evidence in internal/llm/lease.go + lease_test.go, merged in PR #52.
<!-- SECTION:FINAL_SUMMARY:END -->
