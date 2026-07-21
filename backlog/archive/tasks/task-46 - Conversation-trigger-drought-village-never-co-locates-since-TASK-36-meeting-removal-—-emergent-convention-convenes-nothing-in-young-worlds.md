---
id: TASK-46
title: >-
  Conversation trigger drought: village never co-locates since TASK-36 meeting
  removal — emergent convention convenes nothing in young worlds
status: To Do
assignee: []
created_date: '2026-07-21 14:24'
updated_date: '2026-07-21 15:07'
labels:
  - cognition
  - social
dependencies: []
priority: high
ordinal: 40000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The strongest candidate for the user-felt 'not seeing conversations as much' (2026-07-21). Evidence from world-demo (seed 7): 5 agent.talked events in the entire 36k-tick day-1 history, all in early morning; ZERO adjacency events and ZERO conversation-class cog.thought jobs in 4+ game-hours after a calibrated restart with an exclusive, fast LLM and 8x margins — the first-adjacency conversation trigger never fires because agents never meet. TASK-36 (merged 2026-07-21, PR #22) removed the hard-coded 11:30 village meeting — previously the daily proximity pump — in favor of an emergent convention; in young worlds nothing has emerged, so the social graph starves upstream of everything TASK-39 examined (TASK-39's H1-H3 were governor-scoped and remain correct; this is a fourth, world-behavior channel). Investigate: does the emergent convention ever convene in a fresh world (what seeds it)? Do agents have any other convergence behavior (fire at dusk, shelter at night)? Options: seed the convention with one bootstrap meeting; add need-driven convergence (shared fire/food sites); or accept and document slower social warm-up. Full Spec Kit per constitution v1.1.0 if the fix is non-trivial.
<!-- SECTION:DESCRIPTION:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Conclusive negative (2026-07-21): watched world-demo through the entire game evening (~16:12 to past 21:30 game time at 8x, 40 wall-minutes) — zero agent.talked, zero rumor, zero conversation events. No dusk/fire/shelter convergence behavior brings agents adjacent; the village stays dispersed indefinitely once morning spawn-adjacency disperses. The TASK-34 speech-formatting demo is BLOCKED on this task (formatting itself is unit-tested via grammar_test.go; a live capture needs agents that actually meet). Demo world stopped and torn down; recipe to rerun post-fix is on TASK-34 notes (calibrate first, 8-16x, exclusive LLM).

RETRACTION (2026-07-21): premise falsified by offline log audit. The village DOES co-locate and converse without the hard-coded meeting: 4 conversation jobs scheduled across the afternoon/evening (13:05/13:58/18:04/19:04), ALL landed within budget (staleness 2475-3210 vs 7200), with real multi-turn dialogue. The 'zero contact' evidence was measurement error: (a) watcher seeded --since with TICK numbers instead of SEQ numbers, querying past end-of-log; (b) tail output while the daemon was live undercounted history vs the same query offline (live tail appears to serve a bounded window — worth a look as a tooling sharp edge, tracked in this note). TASK-36 exonerated for this world. Archiving.
<!-- SECTION:NOTES:END -->
