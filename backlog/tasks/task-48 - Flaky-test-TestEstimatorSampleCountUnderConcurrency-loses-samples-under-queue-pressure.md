---
id: TASK-48
title: >-
  Flaky test: TestEstimatorSampleCountUnderConcurrency loses samples under queue
  pressure
status: Done
assignee: []
created_date: '2026-07-21 19:10'
updated_date: '2026-07-23 05:04'
labels: []
dependencies: []
priority: medium
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Pre-existing failure on clean main (found at a4b9c92 while baselining TASK-43): go test ./internal/llm/ -run TestEstimatorSampleCountUnderConcurrency fails consistently (37/40, 39/40 samples; 'tier queue full; back off and retry' at llm_test.go:739, counts checked at llm_test.go:743/749). Either the test must tolerate queue-full backoff or the estimator drops samples for real calls. Not TASK-43 scope.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Root cause pinned: test expectation vs estimator behavior under queue-full
- [x] #2 go test ./internal/llm/ -count=10 green
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Reproduce failure on fresh main. 2. Read test (llm_test.go:739 area) + estimator sample path to pin root cause (AC#1). 3. Decide trivial exemption vs spec per constitution. 4. Worktree task-48, delegate fix per model-tier rubric, verify -count=10 (AC#2), one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
ROOT CAUSE (AC#1): test expectation bug, not an estimator defect. llm_test.go fires a synchronized 40-wide burst of non-best-effort submits at Parallel=8; Submit (llm.go:417-419) fails fast with ErrQueueFull when the tier queue (queueCap=32, llm.go:221) is full — documented backpressure design ('full queue fails fast rather than piling work up', llm.go:384). When the burst outruns worker dequeues, up to 8 submits lose the race. Estimator is exact in every failure: samples == server hits == completed calls (37==37) — 'one sample per completed call' holds. FIX: test backs off and retries on ErrQueueFull (what the error instructs), preserving the exact-count assertion at n=40. SPEC RIGOR: trivial exemption applies — surgical test-only fix, file:line diagnosis above, ACs on task. TIER: Opus 4.8 per rubric line 'concurrency/scheduling logic (internal/llm)' — the test encodes concurrent admission semantics.

AC#2 evidence (orchestrator's independent run, commit 67c648b): go test ./internal/llm/ -count=10 → ok 8.435s; -race -count=5 on the fixed test → ok. Implementer (Opus 4.8) run also green: -count=10 ok 8.396s, vet clean, build clean. NOTE: one orchestrator gate run hung 10m in TestQueueBackpressure (NOT touched by this diff) under CPU contention from a concurrent session's go test — pre-existing flaw, filed as TASK-69.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Merged to main as PR #44 (merge 2f91252, fix commit 67c648b). Root cause: test expectation bug — the 40-wide synchronized burst races the cap-32 fail-fast tier queue (ErrQueueFull is designed backpressure); estimator was exact (samples==hits==completed calls in every failure). Fix: test-only — submit goroutine backs off 2ms and retries on ErrQueueFull, 5s deadline; assertions unchanged, llm.go untouched. Implemented by spec-implementer on Opus 4.8 (rubric: concurrency/scheduling in internal/llm); trivial spec exemption (surgical fix, file:line diagnosis on task). Gates: go test ./internal/llm/ -count=10 ok (implementer + orchestrator independently), -race -count=5 ok, vet/build clean, re-verified on merged main (0.374s). No wiki re-pin needed (no note sources llm_test.go). Side finding filed as TASK-69 (TestQueueBackpressure 10m hang under CPU contention).
<!-- SECTION:FINAL_SUMMARY:END -->
