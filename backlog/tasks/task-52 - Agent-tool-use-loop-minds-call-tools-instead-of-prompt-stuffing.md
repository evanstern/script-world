---
id: TASK-52
title: 'Agent tool-use loop: minds call tools instead of prompt stuffing'
status: To Do
assignee: []
created_date: '2026-07-22 02:20'
updated_date: '2026-07-22 02:20'
labels:
  - agent-mind
  - llm
dependencies: []
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Prerequisite for the agent-authored journal in TASK-16 and any future agent-callable capability.

Current state: the LLM layer is strictly single-shot — internal/llm Orchestrator.Submit(ctx, Request) returns one Response (llm.go:254), providers send one messages array with no tools parameter (providers.go), and internal/mind parses free-text replies (parse.go). There is no tool schema, no tool-call parsing, no multi-turn loop anywhere in the codebase.

Needed: an agentic loop for agent minds — a mind call can declare a set of tools; the model may respond with tool calls; the loop executes them and feeds results back until the model produces a final answer (with a hard iteration/budget cap).

Design considerations to resolve in the spec:
- Tool declaration + dispatch: a small registry mapping tool name -> handler; handlers are ordinary Go funcs. Read-only tools (search_journal, read_journal) just return data; mutating tools (write_journal_entry, delete_from_journal) must emit events and be reducer-applied so replay stays deterministic (the model transcript is not replayed — only the emitted events are).
- Provider support across tiers: Anthropic SDK has native tool use; local tier (gemma via OpenAI-compat / 9router) has varying function-calling quality — spec must decide between native tool-calling APIs per provider vs a provider-agnostic structured-output convention, and what the fallback is when a tier cannot tool-call reliably.
- Metering/governor: today one Submit = one metered call; a tool loop is N calls per cognition — cognition estimates, calibration, and the governor (internal/cognition, llm/meter.go) must account for multi-call cognitions.
- Determinism boundary: tool loops happen at decision time (like existing planner calls); everything durable they cause lands as events. Replay never re-runs the loop.

First consumer: TASK-16 journal tools (write_journal_entry, search_journal, optional read_journal / delete_from_journal). Spec via Spec Kit before implementation per constitution.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Mind LLM calls can declare tools; a loop executes model tool calls via a registry and feeds results back until a final answer, with a hard iteration/budget cap
- [ ] #2 Mutating tool handlers emit events and are reducer-applied; replay never re-runs the tool loop and reproduces identical state
- [ ] #3 Works on at least one local-tier and the cloud-tier provider, with an explicit documented fallback for tiers that cannot tool-call reliably
- [ ] #4 Metering/governor accounts for multi-call cognitions (estimates + calibration remain sane)
<!-- AC:END -->
