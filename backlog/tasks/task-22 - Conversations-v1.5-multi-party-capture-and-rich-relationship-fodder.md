---
id: TASK-22
title: 'Conversations v1.5: multi-party capture and rich relationship fodder'
status: To Do
assignee: []
created_date: '2026-07-19 22:27'
labels:
  - sim
  - llm
dependencies: []
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
First slice of the interaction-system overhaul (user request 2026-07-19; full redesign parked as its own design task). Today's driver (internal/mind/convo.go) is strictly pairwise, single-flight, thin snapshots. This task: (1) single out conversations as first-class — 2..N adjacent participants (3+ join the scene), conversation calls prioritized over musings and never dropped silently; (2) optimize the calls — richer snapshot context (relationship history both ways, open debts, shared rumors, prior conversation callbacks); (3) store as much useful outcome as we can: per-participant structured fodder — gist memories with subject/tone per counterpart, relation deltas with reasons, topic tags, and a durable conversation record linkable from future prompts (relationship fodder). All effects stay one atomic inject_social batch.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Conversations support 2..N adjacent participants; a third villager arriving can join and is captured in the record
- [ ] #2 Each participant stores structured fodder about each counterpart (gist memory with subject+tone, relation delta with reason, topic tags) retrievable by future prompts
- [ ] #3 Conversation calls are prioritized over musings and observable (status/telemetry shows conversation activity)
<!-- AC:END -->
