---
id: TASK-17
title: Event payloads name their agents (chronicle legibility)
status: To Do
assignee: []
created_date: '2026-07-19 15:56'
updated_date: '2026-07-22 04:34'
labels:
  - events
  - tui
dependencies: []
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Out-of-sim consumers of the event log (webhook sinks TASK-18, exported logs, external tools) see agents only by index — {"agent":2}, {"from":0,"to":3} — and cannot resolve names without a replica. Re-grounded 2026-07-22: the TUI is no longer the motivating case — eventRow is gone; the chronicle now resolves names post-hoc via formatChronicleLine(e, names) / m.agentNames() (internal/tui/views.go, chronicleRawBody), and narration (TASK-11, Done) ships in chronicleNarratedBody. But that is exactly the post-hoc-lookup pattern this task makes unnecessary at the format level; the primary driver is now external/out-of-sim readers, pairing this task with TASK-18. Requirement: the log format itself carries names, enforced at emission. Approach: an AgentRef that marshals {id, name} (names are fixed per agent, so the denormalization is replay-safe) used for every agent-referencing payload field across sim/mind/social emitters (agent, subject, speaker, listener, from/to, creditor/debtor, witnesses). Enforce mechanically: payload constructors take refs, plus append-time validation or a test sweep over all registered payload types that rejects agent-bearing payloads lacking names. Define the back-compat story for historic events without names (reducer accepts both; renderers fall back gracefully). Bonus once landed: the TUI lookup layer can shrink.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every recorded event payload that references an agent carries both the index and the agent's name in its JSON
- [ ] #2 The TUI chronicle shows agent names with no replica/post-hoc lookup
- [ ] #3 The format is enforced mechanically (typed ref + append validation or exhaustive payload test), not by convention
- [ ] #4 Replay of pre-change worlds still works; back-compat behavior is documented
<!-- AC:END -->
