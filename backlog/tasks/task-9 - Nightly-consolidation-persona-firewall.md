---
id: TASK-9
title: Nightly consolidation + persona firewall
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Consolidation runs per agent per game night on the cloud tier
- [ ] #2 Validator demonstrably rejects a temperament-drifting consolidation
- [ ] #3 Souls visibly grow across a multi-day run
<!-- AC:END -->
