---
id: TASK-80
title: 'Perception of absence: grounded arrival observations make beliefs falsifiable'
status: To Do
assignee: []
created_date: '2026-07-23 17:50'
updated_date: '2026-07-24 02:42'
labels:
  - emergent-lore
  - epistemics
  - design-session
dependencies:
  - TASK-79
priority: medium
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Architectural root of the Thornspire finding (2026-07-23, world-01): the sim only tells agents what happened, never what is NOT there. Memories are emitted from executed actions (internal/sim/memory.go situated constructors); there is no perception channel that reports what actually exists at a location. So when a villager plans "search Thornspire gardens for the stones" the steps compile to real verbs (wander/forage/talk_to — e.g. seqs 73399, 74138: set_plan reasons cite Thornspire, steps are generic), every step lands successfully, and no event records that the tendrils/stones do not exist. Confabulated beliefs are unfalsifiable by construction; the myth can only ever be socially reinforced, never tested.

Scope: give reality a voice — a grounded observation channel emitted when an agent arrives somewhere, reporting what is actually present at/near those tiles (the describePlace / placeScanRadius machinery in the situated-memory layer is the natural substrate). Feed confirming/disconfirming observations into belief confidence via TASK-79's reinforcement seam.

Design questions for the spec (this needs Spec Kit, not a surgical fix):
- Trigger: always-on arrival observations vs. only when the driving intent's reason names a place/phenomenon (the latter needs reason interpretation — keep any LLM-side interpretation in the mind layer, never the reducer).
- Event shape + salience: an observation of absence must be memorable enough to surface in the working window without flooding it (every wander step is an arrival).
- Belief interaction: what counts as disconfirmation ("I looked and saw nothing") vs. mere silence; how much confidence moves; myths should die slowly, not instantly.
- Determinism: observation emission is executor/reducer-side and replay-deterministic.
- Flavor risk: falsifiability shortens myth lifetimes — pairs deliberately with the canonization miracle (TASK-81) so the god can choose to make a myth true before reality debunks it.

Sequencing per plan of record: after TASK-79 (uses its reinforcement seam); runs in parallel with TASK-81.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Spec Kit spec produced and linked via spec-bridge before implementation (constitution rigor)
- [ ] #2 A grounded observation event exists reporting actual local world state on arrival; emission is deterministic and replay-safe
- [ ] #3 Belief confidence responds to confirming/disconfirming observations through the TASK-79 reinforcement seam; disconfirmation decays faster than silence
- [ ] #4 Working-memory window is not flooded: observation salience/dedup tuned and demonstrated on a live soak
- [ ] #5 Chronicle/TUI legibility: grounded observations are visible in the decision trail
<!-- AC:END -->
