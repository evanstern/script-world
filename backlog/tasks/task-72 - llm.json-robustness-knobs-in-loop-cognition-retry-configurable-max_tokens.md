---
id: TASK-72
title: 'llm.json robustness knobs: in-loop cognition retry + configurable max_tokens'
status: Done
assignee: []
created_date: '2026-07-23 06:34'
updated_date: '2026-07-23 17:17'
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

Spec: specs/025-llm-robustness-knobs
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 One retry on TermProviderError for planner/metatron cognitions; second failure terminates exactly as today
- [x] #2 Estimator feeding unchanged: retries produce no extra latency observations; breaker semantics unchanged
- [x] #3 Retry visible in the recorded trail (CallRecord or event), never silent
- [x] #4 Per-kind max_tokens knobs in llm.json with warn-not-error clamping; defaults match current hardcodes
- [x] #5 go test -race ./... passes; wiki notes re-pinned (llm-orchestrator, tool-loop)
- [x] #6 Spec phase: Setup
- [x] #7 Spec phase: User Story 1 — A flaky provider call no longer wastes a whole thought (Priority: P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Operator tunes cognition token budgets in llm.json (Priority: P2)
- [x] #9 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Spec Kit flow complete (2026-07-23): specs/025-llm-robustness-knobs — spec.md, plan.md, research.md (R1–R8), data-model.md, contracts/{llm-json,loop-retry}.md, quickstart.md, tasks.md (17 tasks). Implementation tier: Opus 4.8 via spec-implementer (constitution Principle V rubric: cross-package change touching internal/llm + internal/toolloop orchestration; estimator/breaker doctrine-adjacent — senior tier, not Sonnet). Branch: task-72-llm-robustness-knobs in .worktrees/task-72.

Implementation complete on branch task-72-llm-robustness-knobs (PR #51, commit 9e62208 after rebase onto origin/main). Implemented by spec-implementer @ Opus 4.8; reviewed + independently re-verified on Fable 5 (retry tests, token-budget normalization, TestCatalogSweep, full go test -race ./... green on rebased branch). ACs 1-4 proven by tests in the PR: toolloop/retry_test.go (fail-once/fail-twice/never-retry matrix, estimator invariance = AC1+AC2), mind/metatron retry-visibility via non-terminal cog.outcome OutcomeRetried (AC3), llm config normalization table + boot smoke with clamp warning (AC4). AC5 (wiki re-pin) after merge. Notable accepted deviations: Config.MaxTokens is *TokenBudgets (value struct defeats omitempty), tui/decisions.go guard so the non-terminal retried marker never overwrites the earned terminal outcome, TestProviderErrorFromSubmit updated to fail-twice premise. tasks.md 16/17 (T017 post-merge).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Shipped in PR #51 (merge 8ada105). (a) One in-loop transport retry per cognition run in toolloop.run() — provider_error via terminationForSubmitErr only; admission/ctx/handler failures never retry; identical transcript re-submitted, no round consumed; Result.Retried/RetryReason surfaced by mind.runPlan and metatron.Turn as a non-terminal cog.outcome OutcomeRetried (TASK-42 vocabulary, no new event type); estimator/breaker invariance structural and test-locked (recovered run = 1 ObserveCognition, twice-failed = 0). (b) llm.json max_tokens {planner 512, metatron_turn 1024, consolidation 1024} as *TokenBudgets with warn-not-error clamping (1-4096), threaded daemon → mind.New/metatron.New; WriteDefault stays minimal (omitempty via pointer). Implemented by spec-implementer @ Opus 4.8, reviewed + independently verified on Fable 5; full go test -race ./... green. Wiki re-pinned to 8ada105: 11 notes (7b0f6b2). Spec 025 tasks 17/17. Follow-up observations (non-blocking, in notes): metatron retry markers carry no class/agent for future rendering; decisions.go 'retried' gloss is conversation-specific; 5 pre-existing gofmt-nonconforming files out of scope.
<!-- SECTION:FINAL_SUMMARY:END -->
