---
id: TASK-16
title: 'Grounded memories: context-rich episodic capture'
status: To Do
assignee: []
created_date: '2026-07-19 15:56'
labels:
  - memory
  - agent-mind
dependencies: []
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Memories are terse strings baked at emission — executor templates like 'Built a fire.' / 'Talked with %s.' (internal/sim/executor.go, internal/sim/memory.go) and convo gists (internal/mind/convo.go). soul.md is a faithful view over them (internal/scribe/scribe.go); there is NO richer store behind it — the events table payload IS the whole memory. Yet the grounding detail already exists as sibling events that memories never reference: agent.thought carries the planner's reason (sim/loop.go:371), social.conversation_turn carries full dialogue text (mind/convo.go:135), agent.built/foraged/hunted carry X/Y coords. Task: enrich episodic capture so each memory is situated — extend MemoryAddedPayload (internal/sim/agents.go:218) with structured context (place/coords, driving intent reason when one exists, refs to underlying events e.g. conv id) and enrich the deterministic templates with where/why. All detail must be baked at emission and reducer-applied so replay stays model-free and deterministic. This directly upgrades the input TASK-9 consolidation digests and what prompts/soul.md show.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Memory payloads carry structured context — place, cause/intent reason when present, and refs to source events (e.g. conversation id) — and soul.md renders it
- [ ] #2 Deterministic executor memories are situated (where + why), not bare verbs like 'Built a fire.'
- [ ] #3 Conversation memories reference their transcript so what was said is retrievable from the memory, not just a gist
- [ ] #4 Replay from the event log reproduces identical souls; no model calls or out-of-band lookups
<!-- AC:END -->
