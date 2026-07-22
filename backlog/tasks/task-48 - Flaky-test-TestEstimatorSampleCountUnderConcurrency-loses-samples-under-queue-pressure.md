---
id: TASK-48
title: >-
  Flaky test: TestEstimatorSampleCountUnderConcurrency loses samples under queue
  pressure
status: To Do
assignee: []
created_date: '2026-07-21 19:10'
updated_date: '2026-07-22 04:34'
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
- [ ] #1 Root cause pinned: test expectation vs estimator behavior under queue-full
- [ ] #2 go test ./internal/llm/ -count=10 green
<!-- AC:END -->
