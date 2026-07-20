---
id: TASK-12
title: 'Metatron v1: the editable angel'
status: In Progress
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-20 13:06'
labels:
  - spec-candidate
  - metatron
  - llm
dependencies:
  - TASK-8
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The player's sole verb: a singular long-running gatekeeper agent — watches via periodic event-stream digests, keeps notes, converses in the Metatron console (primary interface). Mediates all nudges: player intent -> persuadability/impact/method judgment -> dream or omen in agent-comprehensible form (raw player text never reaches a villager). Charge economy: 1 per 6 game hours, max 3 banked. Its charter is the game's ONLY player-editable prompt (persona: faithful, professional, near-robotic); it has a soul that starts empty. V1 contract: acts only when told, one prompt = one mediated turn. Owns the drama-router rule (parked open question). Grounding: grounded-assumptions.md (The player's verb). Spec candidate #3.

Spec: specs/005-metatron
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 All influence flows through Metatron; raw player text never enters a villager context
- [ ] #2 Dream and omen nudges land as provenance-unknown memories interpreted in persona
- [ ] #3 Charter is editable at any time and observably changes Metatron's behavior
- [ ] #4 Charge economy enforced (1/6h, cap 3); gatekeeper can refuse with counsel
- [ ] #5 Spec phase: Setup
- [ ] #6 Spec phase: Foundational (blocking prerequisites)
- [ ] #7 Spec phase: User Story 1 — Converse with the angel that watches (P1) 🎯 MVP
- [ ] #8 Spec phase: User Story 2 — Nudge the world through a gatekeeper (P2)
- [ ] #9 Spec phase: User Story 3 — Edit the charter, change the angel (P3)
- [ ] #10 Spec phase: User Story 4 — The angel keeps watch (P4)
- [ ] #11 Spec phase: Polish & cross-cutting
<!-- AC:END -->



## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Started 2026-07-20 on branch task-12-metatron (stacked on task-11-chronicle, PR #14). Spec-candidate #3 -> full Spec Kit flow: specify -> plan -> tasks under specs/005-metatron/, then spec-bridge:link, then build. Grounding read: grounded-assumptions 'The player's verb — Metatron' (gatekeeper, editable charter, dream+omen, 1-per-6h charge cap 3, acts only when told; world-tools/regency parked; drama-router rule owned here but was parked as an open question needing a concrete rule).
<!-- SECTION:NOTES:END -->
