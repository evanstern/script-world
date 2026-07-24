---
id: TASK-14
title: The 30-day proving run
status: To Do
assignee: []
created_date: '2026-07-19 01:14'
updated_date: '2026-07-24 02:42'
labels:
  - milestone
dependencies:
  - TASK-9
  - TASK-10
  - TASK-11
  - TASK-12
  - TASK-13
ordinal: 22000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Seed 8 authored personas, run 30+ game days always-on, and answer the v1 question: does a nudge, filtered through a fixed persona and a lived soul, produce a legible story you feel you authored? Also observe the recorded open questions: does social pressure quarantine a miscast agent; does the persona firewall hold against soul drift; does cost stay under $100/month. Findings written up and fed back into decisions/specs. Grounding: decision-2, grounded-assumptions.md.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A 30+ game-day always-on run completes with 8 agents
- [ ] #2 Findings doc answers the v1 sentence plus the three observation questions, with evidence from the chronicle/souls/event log
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: all five dependencies (TASK-9/10/11/12/13) are Done — nominally unblocked. BUT specs 012/013 (TASK-50/51, in progress) are a world-format break (spec 012 US6: migrate command, reset the land), and the tool stack (TASK-53/52/16) plus survival retunes will materially change the world being proven. Deliberately ordered LAST: run the proving run on the post-012/013, post-tool-stack world so its findings are about the world we intend to keep.

Drift audit 2026-07-23: stale refs fixed — specs 012 (47/47) and 013 (41/41) are DONE and merged (TASK-50/51 Done), tool stack (TASK-52/53/16) Done. The remaining reason to hold: the survival labor-budget retune chain (TASK-28 -> TASK-30) has not run, and it will materially retune the world being proven. Still deliberately ordered last.
<!-- SECTION:NOTES:END -->
