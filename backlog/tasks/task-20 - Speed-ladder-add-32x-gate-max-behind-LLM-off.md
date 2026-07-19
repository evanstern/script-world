---
id: TASK-20
title: 'Speed ladder: add 32x; gate max behind LLM-off'
status: In Progress
assignee: []
created_date: '2026-07-19 22:27'
updated_date: '2026-07-19 22:28'
labels:
  - engine
dependencies: []
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
32x becomes the top watchable speed (user decision 2026-07-19): LLM turns cannot keep pace with uncapped ticking (proving run hit 47k ticks/s), and runaway runs also feed the TASK-19 state-size failure. Add Speed32x to the clock ladder (CLI parse, TUI [/] steps, docs); keep 'max' as the headless/proving escape hatch but REFUSE it (actionable error at the set_speed door) when the world has LLM configured (llm.json present) — max is only legal for pure-sim worlds. Grounding: internal/clock/clock.go speeds map, tui speedSteps, daemon llm.json wiring.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 32x parses, paces at 32 ticks/sec, and is the top of the TUI speed ladder
- [ ] #2 A world with LLM configured refuses 'max' with an actionable error; an LLM-off world still accepts it
- [ ] #3 Full suite green — determinism/replay unaffected
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. clock: Speed32x ("32x", 32 t/s) in the speeds map + ParseSpeed error text
2. tui: speedSteps ladder ends at 32x (max leaves the watchable path)
3. ipc server set_speed: refuse max when srv.llm != nil with an actionable error naming 32x and the llm.json escape hatch (llm_call already models this gate)
4. cmd usage text; tests: clock 32x parse/interval, ipc gate both ways
5. suite + wiki (game-clock, ipc-protocol/server, tui-client notes as touched)
<!-- SECTION:PLAN:END -->
