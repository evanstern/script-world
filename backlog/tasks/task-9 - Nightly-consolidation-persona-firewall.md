---
id: TASK-9
title: Nightly consolidation + persona firewall
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 21:05'
labels:
  - agents
  - llm
dependencies:
  - TASK-7
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
At each agent's sleep: one cloud-tier call compresses the day's episodic buffer into soul.md (memories promoted/faded, beliefs revised with confidence+provenance, self-narrative rewritten in the agent's voice). Firewall mechanized: consolidator cannot write persona.md (structural) and an automated validator rejects temperament drift in its output. Grounding: grounded-assumptions.md (Agent mind).

Spec: specs/004-nightly-consolidation
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Consolidation runs per agent per game night on the cloud tier
- [x] #2 Validator demonstrably rejects a temperament-drifting consolidation
- [x] #3 Souls visibly grow across a multi-day run
- [x] #4 Spec phase: Foundational sim core (blocking)
- [x] #5 Spec phase: US1 — Sleep consolidates the day (P1)
- [x] #6 Spec phase: US3 — The firewall holds (P2, blocking US2's live claim)
- [x] #7 Spec phase: US2 — Souls that grow (P2)
- [x] #8 Spec phase: Polish
<!-- AC:END -->



## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Foundational sim core (blocking): 4/4 · US1 — Sleep consolidates the day (P1): 4/4 · US3 — The firewall holds (P2, blocking US2's live claim): 3/3 · US2 — Souls that grow (P2): 2/2 · Polish: 0/2

spec-bridge sync: Foundational sim core (blocking): 4/4 · US1 — Sleep consolidates the day (P1): 4/4 · US3 — The firewall holds (P2, blocking US2's live claim): 3/3 · US2 — Souls that grow (P2): 2/2 · Polish: 2/2 — status In Progress → Done
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
All spec tasks complete (Foundational sim core (blocking): 4/4 · US1 — Sleep consolidates the day (P1): 4/4 · US3 — The firewall holds (P2, blocking US2's live claim): 3/3 · US2 — Souls that grow (P2): 2/2 · Polish: 2/2). Derived Done by spec-bridge sync.
<!-- SECTION:FINAL_SUMMARY:END -->
