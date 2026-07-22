---
id: TASK-52
title: 'Agent tool-use loop: minds call tools instead of prompt stuffing'
status: To Do
assignee: []
created_date: '2026-07-22 02:20'
updated_date: '2026-07-22 18:34'
labels:
  - agent-mind
  - llm
dependencies:
  - TASK-53
priority: high
ordinal: 2000
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
- [ ] #5 Tool-call trace is first-class and correlatable end-to-end: every tool call is a recorded artifact (including rejected/never-grounded calls), and downstream grounding events link back to the causing call — e.g. JobID carried into IntentSetPayload — so 'tool call → verdict → grounding chain' is queryable from the event log without adjacency inference
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
2026-07-21 design exploration decisions (with Evan):
1. Layer split confirmed — TASK-53 (tool registry, behavior-identical formalization) stands alone and precedes this task; TASK-52 is now specifically the agentic loop (Layer 2): per-provider native tool calling + bounded execute-and-feed-back loop.
2. Cardinality: ONE acting tool per cognition (world or expressive) — read tools (search_journal/read_journal) are exempt, they are mid-loop lookups that inform the cognition, not actions. Journal writes therefore carry opportunity cost: a cognition spent journaling is not spent acting.
3. muse merges into the tool roster (no separate scheduled musing channel long-term); agents choosing to muse via tool call lands with this task's loop.
4. Core principle to preserve verbatim in the spec: a tool call is a REQUEST; an event is the FACT; the gate decides; the executor grounds work in time and space. Speaking/musing/thinking are tools too — game-state integrity applies to expression, not just world mutation.

2026-07-22 (with Evan): added the tool-call observability AC. Today (post-TASK-53) tool usage is visible only as the landing (agent.intent_set{goal, source} / agent.plan_set); the call itself has no independent record, and correlating a completion back to its causing thought requires agent+adjacency inference (cog.outcome carries the job id, IntentSetPayload does not). Fold the cure into this task's loop design: the request artifact plus JobID threading on IntentSetPayload (additive payload field — verify snapshot/replay byte-stability for old logs via omitempty, the TASK-32 pattern). Related registry note: a numeric ParamKind (for storage-verb qty) is also owed to this task — recorded in specs/014-tool-registry/contracts/tool-catalog.md.
<!-- SECTION:NOTES:END -->
