---
id: TASK-63
title: >-
  Decision-trace view: render the cog.tool_call verdict trail (why did my agent
  do that)
status: To Do
assignee: []
created_date: '2026-07-23 03:26'
labels:
  - review-2026-07-22
  - teaching-game
dependencies: []
priority: high
ordinal: 56000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team architecture review (new-ideas item 1): the prompt-engineering feedback signal already exists in the event log but nothing renders it. cog.tool_call events persist a {verdict, reason} CallRecord for EVERY model tool call on every termination path — landed, rejected_malformed, rejected_cardinality, capped, errored — i.e. "you called set_plan with these steps; the gate rejected it because X". The TUI digest has handlers for cog.thought / cog.outcome / cog.recalibration_recommended but NO cog.tool_call handler (internal/tui/digest.go:824-863), and the villager detail view shows beliefs/memories but not the decision trace.

This is the smallest change with the biggest payoff for the teaching goal: it turns the already-persisted event log into the feedback surface a learner iterates against. Scope: (a) a digest handler for cog.tool_call so the chronicle can show tool-call verdicts; (b) a "decisions" sub-view in the villager detail pane rendering the causal chain stimulus -> thought -> tool calls with verdicts -> landed action or terminal rejection, walkable per cognition (trigger_seq chaining already exists); (c) the same treatment for Metatron turns in the metatron pane (its tool calls and verdicts inline in the transcript). May need a small read-model/projection indexed by (agent, cognition) — the review noted the events exist but no per-agent causal projection does.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 digest.go handles cog.tool_call; the catalog sweep test (every event type has a digest) passes
- [ ] #2 Villager detail view has a decisions sub-view showing per-cognition chains: stimulus, thought, each tool call with verdict+reason, and the final outcome
- [ ] #3 Metatron pane shows its own tool-call verdicts inline in the transcript
- [ ] #4 A rejected tool call is legible to a non-engineer: verdict reason rendered in plain language, not raw enum strings
- [ ] #5 docs/wiki re-pinned for touched sources (tui-client, tool-loop notes)
<!-- AC:END -->
