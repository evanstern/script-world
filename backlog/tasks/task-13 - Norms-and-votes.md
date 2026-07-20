---
id: TASK-13
title: Norms and votes
status: Done
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-20 19:50'
labels:
  - social
dependencies:
  - TASK-8
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The village legislates itself: agents propose rules, votes follow the relationship graph, passed norms become world constraints agents obey, skirt, or defy. Also the substrate for possible exile-by-vote (miscast valve of last resort). Grounding: grounded-assumptions.md (The world).

Governance happens at a daily village meeting, not ad hoc: villagers are coordinated to physically gather at a meeting place, agreed rules live in a persistent charter, and the meeting is the venue for proposing new rules, amending or removing existing ones, and voting.

Spec: specs/006-norms-and-votes
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 An agent can propose a norm; votes resolve via relationships; passed norms constrain behavior
- [x] #2 A coordination mechanism convenes the villagers: they break from their routines and gather at a meeting place so votes happen together, not scattered
- [x] #3 A charter persists the rules the village has agreed to; rules can be amended or removed via vote, and the charter reflects the change
- [x] #4 A village meeting runs once per game-day at noon: each villager gets a chance to speak (raise issues, propose/amend/remove rules), timeboxed to ~1 game-hour with grace to let an in-flight conversation finish
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: Foundational (blocking prerequisites)
- [x] #7 Spec phase: User Story 1 — The village convenes at noon (P1) 🎯 MVP
- [x] #8 Spec phase: User Story 2 — Propose, vote, pass (P1)
- [x] #9 Spec phase: User Story 3 — The charter remembers (P2)
- [x] #10 Spec phase: User Story 4 — Norms bind (and get broken) (P2)
- [x] #11 Spec phase: User Story 5 — Exile is on the table (P3)
- [x] #12 Spec phase: Polish & cross-cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Ground: grounded-assumptions.md (The world), docs/wiki/social-fabric.md, existing sim/mind internals
2. Spec: speckit-specify → specs/006-norms-and-votes (governance: proposals, relationship-weighted votes, charter persistence, daily noon meeting with convening + timebox)
3. speckit-plan → design artifacts; speckit-tasks → tasks.md
4. spec-bridge:link TASK-13 ↔ specs/006-norms-and-votes
5. Implement on branch task-13-norms-and-votes (one PR)
6. wiki-update re-ground; spec-bridge:sync; PR
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Setup: 2/2 · Foundational (blocking prerequisites): 5/5 · User Story 1 — The village convenes at noon (P1) 🎯 MVP: 6/6 · User Story 2 — Propose, vote, pass (P1): 7/7 · User Story 3 — The charter remembers (P2): 3/3 · User Story 4 — Norms bind (and get broken) (P2): 5/5 · User Story 5 — Exile is on the table (P3): 4/4 · Polish & cross-cutting: 4/4 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Setup: 2/2 · Foundational (blocking prerequisites): 5/5 · User Story 1 — The village convenes at noon (P1) 🎯 MVP: 6/6 · User Story 2 — Propose, vote, pass (P1): 7/7 · User Story 3 — The charter remembers (P2): 3/3 · User Story 4 — Norms bind (and get broken) (P2): 5/5 · User Story 5 — Exile is on the table (P3): 4/4 · Polish & cross-cutting: 4/4). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
