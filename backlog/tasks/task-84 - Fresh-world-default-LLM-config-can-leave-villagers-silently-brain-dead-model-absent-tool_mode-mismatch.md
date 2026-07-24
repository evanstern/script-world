---
id: TASK-84
title: >-
  Fresh-world default LLM config can leave villagers silently brain-dead (model
  absent / tool_mode mismatch)
status: To Do
assignee: []
created_date: '2026-07-23 23:17'
labels:
  - onboarding
  - llm
dependencies: []
ordinal: 75000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found during TASK-73's eval bring-up (spec 027, implementer finding, 2026-07-23): DefaultConfig() (internal/llm/config.go:454) writes a fresh world's local tier as model gemma4:12b-mlx with tool_mode unset, which resolveToolMode (config.go:310-318) resolves to "native". On a machine without that model pulled, a brand-new world's villagers make ZERO successful planner tool calls — and even with cogito:3b (the model the operator guide and wiki document as the working local choice) native OpenAI-compat function-calling emits no tool calls; it needs tool_mode: json. Nothing fails loudly: the world runs, minds plan, every call comes back empty — reflex floor forever.

Two threads to pull (scope for spec/clarify):
1. Silent failure: consider a startup/daemon preflight that checks the declared local model actually exists on the endpoint (and/or a loud repeated warning event when planner calls consistently return zero tool calls), so a dead tier is visible in status/attach instead of silent.
2. Default alignment: decide whether the shipped default should match the documented cogito:3b + tool_mode json guidance (docs/llm-providers.md currently shows gemma4:12b-mlx in its example), or keep gemma4 and document the pull requirement prominently at `promptworld new` time.

Evidence: TASK-73 eval driver had to set model cogito:3b + tool_mode json on every eval world before ANY planner call succeeded (see specs/027-villager-prompt-quality/eval/decision.md caveats and TASK-73 notes, 2026-07-23).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Fresh world on a machine without the default local model surfaces the dead tier loudly (status/attach/event), not silently
- [ ] #2 Default local model + tool_mode decision made and aligned across DefaultConfig, docs/llm-providers.md, and README
<!-- AC:END -->
