---
id: TASK-63
title: >-
  Decision-trace view: render the cog.tool_call verdict trail (why did my agent
  do that)
status: In Progress
assignee: []
created_date: '2026-07-23 03:26'
updated_date: '2026-07-23 05:15'
labels:
  - review-2026-07-22
  - teaching-game
dependencies: []
priority: high
ordinal: 28500
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team architecture review (new-ideas item 1): the prompt-engineering feedback signal already exists in the event log but nothing renders it. cog.tool_call events persist a {verdict, reason} CallRecord for EVERY model tool call on every termination path — landed, rejected_malformed, rejected_cardinality, capped, errored — i.e. "you called set_plan with these steps; the gate rejected it because X". The TUI digest has handlers for cog.thought / cog.outcome / cog.recalibration_recommended but NO cog.tool_call handler (internal/tui/digest.go:824-863), and the villager detail view shows beliefs/memories but not the decision trace.

This is the smallest change with the biggest payoff for the teaching goal: it turns the already-persisted event log into the feedback surface a learner iterates against. Scope: (a) a digest handler for cog.tool_call so the chronicle can show tool-call verdicts; (b) a "decisions" sub-view in the villager detail pane rendering the causal chain stimulus -> thought -> tool calls with verdicts -> landed action or terminal rejection, walkable per cognition (trigger_seq chaining already exists); (c) the same treatment for Metatron turns in the metatron pane (its tool calls and verdicts inline in the transcript). May need a small read-model/projection indexed by (agent, cognition) — the review noted the events exist but no per-agent causal projection does.

Note (2026-07-23): scope item (a) already landed via TASK-62 (PR #43) — this task's spec covers (b), (c), and plain-language rendering.

Spec: specs/020-decision-trace-view
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 digest.go handles cog.tool_call; the catalog sweep test (every event type has a digest) passes
- [ ] #2 Villager detail view has a decisions sub-view showing per-cognition chains: stimulus, thought, each tool call with verdict+reason, and the final outcome
- [ ] #3 Metatron pane shows its own tool-call verdicts inline in the transcript
- [ ] #4 A rejected tool call is legible to a non-engineer: verdict reason rendered in plain language, not raw enum strings
- [ ] #5 docs/wiki re-pinned for touched sources (tui-client, tool-loop notes)
- [ ] #6 Spec phase: Setup
- [ ] #7 Spec phase: Foundational (blocking all user stories)
- [ ] #8 Spec phase: User Story 1 — "Why did my villager do that?" (P1) 🎯 MVP
- [ ] #9 Spec phase: User Story 2 — Metatron's own verdict trail (P2)
- [ ] #10 Spec phase: User Story 3 — Legible to a non-engineer (P3)
- [ ] #11 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit run complete (specs/020-decision-trace-view: spec, plan, research, data-model, contracts/decision-trace-ui.md, quickstart, tasks — 19 tasks, 6 phases). Approach: pure internal/tui feature — (1) bounded per-agent decision-trace projection (map[job]chain + per-agent key list, cap 20/agent) fed from applyEvent, joining cog.thought/cog.tool_call/cog.outcome on job ID, stimulus resolved at ingest from the chronicle ring via the digest grammar; (2) decisions sub-view in villager detail (d toggles, j/k scroll, esc unwinds) rendering chains most-recent-first; (3) inline verdict rows in the metatron transcript for turn-metatron-* calls; (4) verdict glossary as single plain-language authority, sweep-tested against toolloop+sim constants. No daemon/sim/event changes. AC#1 already proven by TASK-62's digest handler. Worktree .worktrees/task-63, branch task-63-decision-trace-view, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Model tier: Sonnet (spec-implementer default) per constitution v1.1.0 Principle V rubric — routine profile: single-package (internal/tui) view/rendering feature, tests alongside code, no concurrency/scheduling/governor logic, no doctrine-adjacent behavior change (read-only projection over persisted events). Escalation to Opus 4.8 only if a Sonnet attempt fails gates.
<!-- SECTION:NOTES:END -->
