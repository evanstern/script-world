---
id: TASK-69
title: >-
  Flaky test: TestQueueBackpressure hangs 10m when saturation poll misses its 2s
  deadline
status: To Do
assignee: []
created_date: '2026-07-23 04:58'
updated_date: '2026-07-24 02:42'
labels: []
dependencies: []
priority: medium
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found while gating TASK-48 (commit 67c648b worktree, identical to main for this file region). Under CPU contention the test's saturation poll (llm_test.go:291-297, 2s deadline waiting for Queue >= queueCap) can expire before the 33 goroutine submits saturate the tier. The overflow Submit (llm_test.go:302) then ENQUEUES instead of getting ErrQueueFull and blocks forever in the reply select (llm.go:421, background ctx). The single worker grinds release-blocked jobs at workerCallCap=2min each (llm.go:225) — goroutine dump showed handler arrivals at ~2min cadence (9/7/4/2 min waits) until the go test 10m timeout panics. Fix direction: make the overflow submit carry a short context timeout, or t.Fatal when the saturation poll times out instead of proceeding, so a missed race fails in seconds not 10 minutes. Evidence: goroutine dump in TASK-48 session, 2026-07-23.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Root cause confirmed against a reproduced hang or reasoned trace
- [ ] #2 Test fails fast (seconds) when saturation is not reached; go test ./internal/llm/ -count=10 green under load
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: facts hold; line pins moved. Saturation poll 2s deadline now llm_test.go:307-309 (test at :288); overflow Submit with background ctx llm_test.go:303-304; blocking reply select is llm.go:721-722 (NOT :421); workerCallCap=2min at llm.go:284 (used :799).
<!-- SECTION:NOTES:END -->
