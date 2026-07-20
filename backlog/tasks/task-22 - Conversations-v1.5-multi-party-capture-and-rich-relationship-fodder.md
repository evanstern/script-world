---
id: TASK-22
title: 'Conversations v1.5: multi-party capture and rich relationship fodder'
status: In Progress
assignee: []
created_date: '2026-07-19 22:27'
updated_date: '2026-07-20 00:40'
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

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Grounding: today's driver (internal/mind/convo.go) is strictly pairwise ([2] arrays), one global slot, ≤5 memory lines each, and social.conversation is a reducer NO-OP — records evaporate; only gist memories (subject -1, not gossipable) and tone-based relation deltas persist.

Design:
1. N-party scenes (2..4): on agent.talked, adjacent live+awake villagers within radius 2 of the pair join the scene. convoCtx goes slice-based; round-robin turns (2 rounds); every participant hears every turn.
2. Durable record: social.conversation payload gains participants[], topics[], per-participant tones[]; reducer appends a bounded ring State.Conversations (cap 64) — the artifact future prompts read. Old two-party payloads keep applying (back-compat: empty participants => [a,b]).
3. Relationship fodder per participant×counterpart: gist memory now subject=counterpart with tone (=> TellableFor gossip seed), relation deltas per pair with reason "conversation: <topic>", and prompts (planner + convo snapshot) carry "last time you spoke with X: <gist>" pulled from the ring.
4. Optimization: conversations keep the prio lane and single slot; richer snapshot (relations both ways, open debts between participants, shared rumor knowledge, last-conversation callback); outcome call asks for gist + topics + per-participant tones in one JSON.
5. Observability: State.Conversations count + last gist surface via state (souls/TUI chronicle already shows the events); daemon log lines retained.
Tests: N-party scene formation from adjacency; reducer ring + back-compat; fodder events (subject-tagged gists tellable via TellableFor); prompt callback content; full suite. Live proof on muse-proof world at 4x.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Live proof paused (2026-07-19 evening): the operator's own world (second checkout) is running against the same local gemma — two daemons contending pushed calls past the 90s timeout and tripped both circuit breakers (filed as TASK-24). Proving world convo-proof stopped to free the model; db retained in session scratchpad. Unit/integration evidence complete (scene formation, record ring + back-compat, subject-tagged fodder via real loop + TellableFor, race-clean full suite). Live acceptance re-runs when the local model is free.
<!-- SECTION:NOTES:END -->
