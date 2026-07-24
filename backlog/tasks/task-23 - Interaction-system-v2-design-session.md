---
id: TASK-23
title: 'Interaction system v2: design session'
status: To Do
assignee: []
created_date: '2026-07-19 22:27'
updated_date: '2026-07-24 02:42'
labels:
  - design
dependencies: []
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The full agent<=>agent interaction system needs a ground-up design (user, 2026-07-19: 'that's a whole system we need to design'). Socratic/spec session covering: interaction primitives beyond talk (argue, trade, teach, comfort, conspire), scene formation and dissolution for groups, how conversation records become long-term relationship memory (interplay with TASK-9 consolidation), LLM budget shaping across interaction kinds, and what the chronicle (TASK-11) needs from interactions. Output: a spec under specs/ linked to the board via spec-bridge. Builds on evidence from Conversations v1.5.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A grounding/design session produces a spec directory for interactions v2, linked on the board via spec-bridge
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: reframed onto the tool substrate — new interaction primitives (argue/trade/teach/comfort/conspire) should be authored as tool-registry entries (TASK-53) with per-agent rosters, invoked through the TASK-52 loop, not as a bespoke parallel system. The design session should start from the registry's tool classes (world/expressive/read). Ordered after Metatron v2 (TASK-27), which exercises the same substrate first.
<!-- SECTION:NOTES:END -->
