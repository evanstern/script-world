---
id: TASK-72
title: 'llm.json robustness knobs: in-loop cognition retry + configurable max_tokens'
status: To Do
assignee: []
created_date: '2026-07-23 06:34'
labels:
  - review-2026-07-22
  - code-quality
dependencies: []
priority: medium
ordinal: 65000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (improvements 3 and 4, the non-attribution half), re-verified 2026-07-23 (no retry exists in internal/toolloop/loop.go). Two related llm.json robustness knobs, one PR:

(a) In-loop transport retry: a single provider_error currently terminates the whole cognition (toolloop/loop.go — TermProviderError paths at ~269/287/332); only conversations get a one-shot retry (mind/convo.go:216-256). A flaky local call thus wastes an entire planner/metatron thought and waits out the 120-tick rearm. Add ONE in-loop retry on TermProviderError before terminating. Doctrine constraints: the retry must not double-feed the latency estimator (successes-only / SkipObserve discipline stays intact — only the whole-Run wall time samples), must not strike the circuit breaker differently than today (busy-is-not-down preserved), and the retried round must be observable in the CallRecord/event trail, never silent.

(b) Configurable max_tokens: per-call-site hardcodes today (loopMaxTokens=512 in mind.go ~372, turnMaxTokens=1024, consolidate 1024). Add per-kind llm.json overrides following the established warn-not-error clamp convention (same as loop_max_rounds: absent/0 = current default, out-of-range clamps with an operator warning at boot, never a boot failure).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 One retry on TermProviderError for planner/metatron cognitions; second failure terminates exactly as today
- [ ] #2 Estimator feeding unchanged: retries produce no extra latency observations; breaker semantics unchanged
- [ ] #3 Retry visible in the recorded trail (CallRecord or event), never silent
- [ ] #4 Per-kind max_tokens knobs in llm.json with warn-not-error clamping; defaults match current hardcodes
- [ ] #5 go test -race ./... passes; wiki notes re-pinned (llm-orchestrator, tool-loop)
<!-- AC:END -->
