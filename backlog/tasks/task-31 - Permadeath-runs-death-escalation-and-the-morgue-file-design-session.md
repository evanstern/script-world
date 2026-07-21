---
id: TASK-31
title: 'Permadeath runs, death escalation, and the morgue file: design session'
status: To Do
assignee: []
created_date: '2026-07-20 19:55'
labels:
  - design
dependencies: []
ordinal: 26000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Roguelike survival design (user, 2026-07-20; see decision-3 strife doctrine). Per-agent permadeath already exists: Health 0 fires agent.died with a cause (starvation/exposure/collapse), the reducer marks the agent Dead forever, nothing respawns, and witnesses within radius 8 form memories (docs/wiki/executor.md). But nothing is at stake at the run level: a world where all 8 agents die just keeps ticking, and the only lethal threats are neglect — the gru wounds (-250 health) but floors health at 1 and never kills (docs/wiki/gru.md). Socratic/spec session covering: (1) Run outcomes — define the end of a run: all agents dead emits a run.ended event; decide what the daemon does after (keep ticking an empty world? go idle? mark the save dir closed?). A save directory is already one run with no reset command (docs/wiki/world-save-directory.md), which fits roguelike semantics: new run = new world dir, old runs are archives. (2) The morgue file — the roguelike epitaph, written into the save dir as a durable artifact: per-death and at run end, days survived, cause, notable memories, relationships, debts owed and owing, deeds; the chronicle narrates the moment, the morgue file is the legacy document a human reads afterward. (3) Graves — a grave overlay where an agent fell; candidates: mourning morale effects, agents visiting graves, grief entering rumors. (4) Death escalation — decide whether the gru can kill (remove the health-1 floor entirely, or only for wounded/NearDeath agents, or only in the cold season per TASK-28), and whether untreated wounds fester without sleep (with TASK-30 healing). (5) Candidate: a difficulty preset recorded in the world.json manifest so hard runs are reproducible and comparable by seed. Output: a spec under specs/ linked to the board via spec-bridge.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for run outcomes, death escalation, and the morgue file, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pre-session decisions (user, 2026-07-20): (1) Permadeath enabled per agent — already true in code; this task makes it consequential at run level. (2) Death should be a real risk from more than neglect: the gru or wounds should be able to finish someone. (3) Per decision-3, deaths should generate social material (grief, blame, inheritance of stockpiles), not just remove an agent.
<!-- SECTION:NOTES:END -->
