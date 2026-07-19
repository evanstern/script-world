---
id: TASK-8
title: 'Social fabric: relationships, rumors, debts, secrets, conversations'
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 05:10'
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

Spec: specs/003-social-fabric
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Social encounters read/write relationship edges; rumors mutate and carry provenance
- [ ] #2 Broken promises persist in the ledger and move reputation
- [ ] #3 Conversations run multi-turn within the cap and land in both souls
- [ ] #4 Spec phase: Foundational sim core (blocking)
- [ ] #5 Spec phase: Secrets + genesis (US3)
- [ ] #6 Spec phase: Conversations (US4)
- [ ] #7 Spec phase: Polish
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-8-social-fabric off main (now complete through TASK-7)
2. Spec Kit spec (specs/003-social-fabric), spec-bridge link, plan/tasks
3. Sim core (deterministic): relationship graph (directed trust/affection edges, event-sourced), promises/debts ledger (give act creates debt; repay settles; overdue breaks -> trust hit), computed reputation, seeded secrets (one per persona, genesis events), rumor registry + per-agent known-rumor variants with provenance chain + per-hop confidence decay
4. Conversation driver (mind extension): on adjacency talks, run multi-turn LLM exchanges (<=3 turns each way, local tier, one convo at a time in its own goroutine with an immutable context), outcome call yields gist + tones + rumor paraphrase; effects enter ONLY as inject_social recorded events (replay stays model-free)
5. Deterministic fallbacks everywhere: primitive talk stays; verbatim rumor retell when LLM absent; reflex give/repay
6. Planner prompt gains social context (bonds, debts, reputation, top rumor); scribe adds Bonds section to soul.md
7. Tests: relation/ledger/reputation mechanics (AC#2), rumor provenance + mutation + decay (AC#1), convo cap + dual-soul memories (AC#3), determinism/replay re-proven; -race suite; live Ollama smoke
8. Wiki, board close-out, PR (base main)
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync: Foundational sim core (blocking): 0/6 · Secrets + genesis (US3): 0/1 · Conversations (US4): 0/4 · Polish: 0/2
<!-- SECTION:NOTES:END -->
