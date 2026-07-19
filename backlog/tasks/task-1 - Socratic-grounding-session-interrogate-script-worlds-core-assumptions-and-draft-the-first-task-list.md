---
id: TASK-1
title: >-
  Socratic grounding session: interrogate script-world's core assumptions and
  draft the first task list
status: Done
assignee: []
created_date: '2026-07-18 15:09'
updated_date: '2026-07-19 01:14'
labels: []
dependencies: []
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Run a full Q/A session using the socratic loop from /educate:lesson to ground the assumptions in README.md before any design or code.

The README pitches: a terminal UI, open-world, top-down game on a procedurally generated map, populated by 10-20 AI agents whose behavior is fully programmable via AI prompting (Dwarf Fortress / RimWorld, but you tweak your dwarf by prompting it).

Areas to interrogate (non-exhaustive):
- What does "programmable via AI prompting" mean mechanically? Live LLM calls per agent per tick, compiled behavior trees from prompts, or something in between?
- Cost/latency model: 10-20 concurrent agents against an LLM - what cadence is affordable and responsive?
- Simulation model: tick rate, world state representation, what agents can perceive and act on.
- Terminal UI: rendering approach, library/stack, input model.
- Procedural generation: what the map needs to contain for agent jobs to be meaningful.
- What the player actually does moment-to-moment; the core loop.
- Tech stack and language choice.

Output: grounded assumptions written down, plus a first-pass task list captured as Backlog.md tasks (and candidate Spec Kit specs for the larger features).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Socratic Q/A session completed using the /educate:lesson socratic loop
- [x] #2 Key assumptions from the README are each examined and either confirmed, revised, or rejected, with the outcome written down
- [x] #3 A first-pass task list exists on the Backlog.md board derived from the session
- [x] #4 Candidate features needing full specs are identified for Spec Kit
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Load /educate:lesson socratic loop
2. Interrogate the Rumor Mill hybrid (decision-1) area by area: nudge economy, soul consolidation, miscast valve, sim model, cost/latency, terminal UI, procgen, core loop, stack
3. Record each assumption as confirmed/revised/rejected in a durable artifact
4. Derive first-pass Backlog tasks + candidate Spec Kit specs
5. Sync AC and finish with final summary
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Direction chosen before the session (decision-1): build toward Rumor Mill as a hybrid — ~20 agents, fixed authored personas + dynamic soul.md (memories/beliefs that grow), no direct persona edits, influence only via nudges/whispers. The session should interrogate THIS design, not the three open concepts. Input: research/project-plans.html ('Chosen direction' section) + backlog/decisions/decision-1.

Scaffolding done: educate structure planted (topics/.template, progress.schema.json, CLAUDE.md educate block appended), lesson topics/script-world-design/101-rumor-mill-grounding scaffolded (checklist.md tailored to decision-1 areas, raw-notes.md live log ready), progress.json synced and gate-green. Socratic session starting.

Session complete: 12 Q/A exchanges logged live in topics/script-world-design/101-rumor-mill-grounding/raw-notes.md; lesson at 'taught', educate gate green. Outcomes: docs/design/grounded-assumptions.md (every README/decision-1 assumption confirmed/revised/rejected with citations), decision-2 (v1 shape), TASK-2..TASK-14 created with dependencies, 4 spec candidates labeled 'spec-candidate' on the board (world daemon, agent mind, Metatron, social fabric) + village survival sim named in the assumptions doc.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Socratic grounding session run via educate:lesson (12 exchanges, live-logged). Key revisions vs README/decision-1: script-world is an ambient ALWAYS-ON world (1 game-min = 15 real-sec, daemon + attachable Go/Bubble Tea TUI); all influence mediated by Metatron, a gatekeeper agent whose charter is the game's only player-editable prompt; nudges = dream + omen at 1 charge/6h cap 3; all seven social systems in v1 as the conflict engine; death by neglect added (deathless stance reversed under interrogation); the gru night predator added; memory = top-K reranked window; persona firewall mechanized; local-first inference under $100/mo. Artifacts: docs/design/grounded-assumptions.md, decision-2, raw-notes.md/checklist.md in the lesson folder, TASK-2..14 on the board, 5 candidate specs identified.
<!-- SECTION:FINAL_SUMMARY:END -->
