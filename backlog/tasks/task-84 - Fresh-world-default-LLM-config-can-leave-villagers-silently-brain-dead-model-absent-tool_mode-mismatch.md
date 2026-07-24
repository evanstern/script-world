---
id: TASK-84
title: >-
  Fresh-world default LLM config can leave villagers silently brain-dead (model
  absent / tool_mode mismatch)
status: In Progress
assignee: []
created_date: '2026-07-23 23:17'
updated_date: '2026-07-24 13:22'
labels:
  - onboarding
  - llm
dependencies: []
priority: high
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found during TASK-73's eval bring-up (spec 027, implementer finding, 2026-07-23): DefaultConfig() (internal/llm/config.go:454) writes a fresh world's local tier as model gemma4:12b-mlx with tool_mode unset, which resolveToolMode (config.go:310-318) resolves to "native". On a machine without that model pulled, a brand-new world's villagers make ZERO successful planner tool calls — and even with cogito:3b (the model the operator guide and wiki document as the working local choice) native OpenAI-compat function-calling emits no tool calls; it needs tool_mode: json. Nothing fails loudly: the world runs, minds plan, every call comes back empty — reflex floor forever.

Two threads to pull (scope for spec/clarify):
1. Silent failure: consider a startup/daemon preflight that checks the declared local model actually exists on the endpoint (and/or a loud repeated warning event when planner calls consistently return zero tool calls), so a dead tier is visible in status/attach instead of silent.
2. Default alignment: decide whether the shipped default should match the documented cogito:3b + tool_mode json guidance (docs/llm-providers.md currently shows gemma4:12b-mlx in its example), or keep gemma4 and document the pull requirement prominently at `promptworld new` time.

Evidence: TASK-73 eval driver had to set model cogito:3b + tool_mode json on every eval world before ANY planner call succeeded (see specs/027-villager-prompt-quality/eval/decision.md caveats and TASK-73 notes, 2026-07-23).

Spec: specs/034-llm-defaults-preflight
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Fresh world on a machine without the default local model surfaces the dead tier loudly (status/attach/event), not silently
- [ ] #2 Default local model + tool_mode decision made and aligned across DefaultConfig, docs/llm-providers.md, and README
- [ ] #3 Spec phase: Setup
- [ ] #4 Spec phase: Foundational (Blocking Prerequisites) — tier: Opus 4.8 (orchestrator internals)
- [ ] #5 Spec phase: User Story 1 — A dead local tier is loud, not silent (Priority: P1) 🎯 MVP
- [ ] #6 Spec phase: User Story 2 — Consistently tool-silent planner calls are loud (Priority: P2)
- [ ] #7 Spec phase: User Story 3 — Fresh-world defaults work out of the box (Priority: P2)
- [ ] #8 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete (spec 034, specs/034-llm-defaults-preflight/: spec, plan, research, data-model, contracts, quickstart, tasks — 19 tasks, 6 phases). Approach: (1) net-new internal/llm/preflight.go probes GET {endpoint}/models per openai_compat provider at boot + 60s re-probe while unhealthy; (2) per-provider condition slot (model-missing/endpoint-unreachable/tool-silent) beside tierHealth, exported via ProviderStatus -> status/TUI, transitions logged + daemon.llm_warning event; (3) worker-side consecutive zero-tool-call detector (threshold 8, scoped to tool-carrying calls); (4) DefaultConfig local -> cogito:3b + tool_mode json + parallel 4 (decision grounded in TASK-73 eval record), promptworld new prints pull command, docs/llm-providers.md + README aligned.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: verified intact — DefaultConfig at config.go:448, gemma4:12b-mlx with no ToolMode at :454; resolveToolMode defaults native at :310-313; docs/llm-providers.md:32 still shows gemma4; no startup preflight (reachability is lazy via llm/health.go circuit breaker).

Tier decision (constitution V rubric): Phases 2-4 (condition plumbing, preflight lifecycle, worker hot-path detector — internal/llm orchestration + daemon wiring) = Opus 4.8: concurrency/scheduling logic in internal/llm explicitly named in the rubric. Phase 3 rendering slices (status/TUI) and Phase 5 (defaults + CLI output + doc reconciliation) = Sonnet: view/rendering code and doc reconciliation. Recorded 2026-07-24 at spec-link time.
<!-- SECTION:NOTES:END -->
