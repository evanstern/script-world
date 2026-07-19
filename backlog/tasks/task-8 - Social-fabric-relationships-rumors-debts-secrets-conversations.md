---
id: TASK-8
title: 'Social fabric: relationships, rumors, debts, secrets, conversations'
status: To Do
assignee: []
created_date: '2026-07-19 01:13'
labels:
  - spec-candidate
  - agents
  - social
dependencies:
  - TASK-7
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The conflict engine: relationship graph (trust/affection/debt edges read+written by all social acts); rumor objects (content/source/confidence, mutate on retell via cheap paraphrase, provenance tracked); promises/debts ledger with computed reputation; one seeded secret per persona; agent-to-agent conversations capped at ~5 turns each way. Grounding: grounded-assumptions.md (The world, Agent mind). Spec candidate #4.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Social encounters read/write relationship edges; rumors mutate and carry provenance
- [ ] #2 Broken promises persist in the ledger and move reputation
- [ ] #3 Conversations run multi-turn within the cap and land in both souls
<!-- AC:END -->
