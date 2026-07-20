---
id: TASK-17
title: Event payloads name their agents (chronicle legibility)
status: To Do
assignee: []
created_date: '2026-07-19 15:56'
labels:
  - events
  - tui
dependencies: []
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The chronicle pane prints raw event payloads (internal/tui/views.go eventRow, :255) and every payload references agents only by index — {"agent":2}, {"from":0,"to":3} — so the feed is unreadable without a replica lookup. Requirement: the log format itself carries names, enforced at emission rather than patched by post-hoc lookups. Approach: introduce an AgentRef that marshals {id, name} (names are fixed per agent, so the denormalization is replay-safe) and use it for every agent-referencing payload field across sim/mind/social emitters (agent, subject, speaker, listener, from/to, creditor/debtor, witnesses). Enforce mechanically: payload constructors take refs, plus a validation at store append or a test sweep over all registered payload types that rejects agent-bearing payloads lacking names. Define the back-compat story for existing worlds whose historic events lack names (reducer accepts both; chronicle falls back gracefully). Precursor to TASK-11 narration — the narrator reads the same payloads.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every recorded event payload that references an agent carries both the index and the agent's name in its JSON
- [ ] #2 The TUI chronicle shows agent names with no replica/post-hoc lookup
- [ ] #3 The format is enforced mechanically (typed ref + append validation or exhaustive payload test), not by convention
- [ ] #4 Replay of pre-change worlds still works; back-compat behavior is documented
<!-- AC:END -->
