---
id: TASK-16
title: 'Grounded memories: context-rich episodic capture'
status: To Do
assignee: []
created_date: '2026-07-19 15:56'
updated_date: '2026-07-22 02:18'
labels:
  - memory
  - agent-mind
dependencies: []
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Memories are terse strings baked at emission — executor templates like 'Built a fire.' / 'Talked with %s.' (internal/sim/executor.go, internal/sim/memory.go) and convo gists (internal/mind/convo.go). soul.md is a faithful view over them (internal/scribe/scribe.go); there is NO richer store behind it — the events table payload IS the whole memory. Yet the grounding detail already exists as sibling events that memories never reference: agent.thought carries the planner's reason (sim/loop.go:371), social.conversation_turn carries full dialogue text (mind/convo.go:135), agent.built/foraged/hunted carry X/Y coords. Task: enrich episodic capture so each memory is situated — extend MemoryAddedPayload (internal/sim/agents.go:218) with structured context (place/coords, driving intent reason when one exists, refs to underlying events e.g. conv id) and enrich the deterministic templates with where/why. All detail must be baked at emission and reducer-applied so replay stays model-free and deterministic. This directly upgrades the input TASK-9 consolidation digests and what prompts/soul.md show.

## Agent-authored journal (added 2026-07-21)

Second layer on top of the deterministic episodic capture: give each agent a personal journal — a tiny markdown wiki the agent writes itself. Where the memory stream is system-authored (baked at emission), the journal is agent-authored: the agent decides what is worth noting for later.

- Tools/skills exposed to the agent mind: write_journal_entry and search_journal as the core pair; optionally read_journal (fetch a specific page/entry) and delete_from_journal (remove text) so the agent can curate.
- The only imposed rule is a hard size cap (characters/pages) enforced at write time — no guidance on how or when to use it. The point is to observe what usage behavior emerges.
- Journal mutations must be event-sourced like everything else: each write/delete lands as an event and is reducer-applied, so replay reproduces identical journals with no model calls (same invariant as AC #4).
- Journal content becomes retrievable context for the mind (search results fed into prompts), complementing the situated memories above.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Memory payloads carry structured context — place, cause/intent reason when present, and refs to source events (e.g. conversation id) — and soul.md renders it
- [ ] #2 Deterministic executor memories are situated (where + why), not bare verbs like 'Built a fire.'
- [ ] #3 Conversation memories reference their transcript so what was said is retrievable from the memory, not just a gist
- [ ] #4 Replay from the event log reproduces identical souls; no model calls or out-of-band lookups
- [ ] #5 Each agent has a personal markdown journal with write_journal_entry and search_journal tools exposed to the mind (read_journal / delete_from_journal optional)
- [ ] #6 Journal is size-capped (fixed character/page budget) enforced at write time; no other usage rules imposed on the agent
- [ ] #7 Journal mutations are event-sourced and reducer-applied — replay reproduces identical journals with no model calls
<!-- AC:END -->
