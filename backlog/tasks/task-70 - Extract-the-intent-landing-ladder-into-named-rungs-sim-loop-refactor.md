---
id: TASK-70
title: Extract the intent-landing ladder into named rungs (sim loop refactor)
status: To Do
assignee: []
created_date: '2026-07-23 06:34'
labels:
  - review-2026-07-22
  - code-quality
dependencies: []
priority: high
ordinal: 63000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (improvement 1) — the worst complexity hotspot in the core, re-verified present 2026-07-23. The intent-landing ladder lives inline in handleCommand (internal/sim/loop.go, ~lines 430-619): the hail-rung relaxation is spliced INSIDE the guard-evaluation loop with three interacting flags (adapted, failed, hailTarget — see loop.go:481,513,534,616) and nested special-cases (mutual-hailer, in-radius, moved-target). The review judged it correct as far as traceable, but it is the least testable-in-isolation code in the core and the most likely to grow a bug on the next change.

Scope: extract into a landIntent method (or small type) with NAMED rungs — each rung a function with a name matching the doctrine (fresh / adapted / hail-relaxed / superseded / guard-failed / stale), the flag soup replaced by explicit rung outcomes. Pure refactor: behavior must be bit-identical. The determinism harness is the safety net — same seeds must replay to byte-identical state hashes before and after. Add unit tests exercising each rung in isolation (the thing the current shape makes impossible). No event-schema or doctrine changes.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Landing logic extracted from handleCommand into named rungs; the adapted/failed/hailTarget flag interplay is gone
- [ ] #2 Determinism harness proves bit-identical replay on existing seeds across the refactor
- [ ] #3 Each rung has isolated unit tests, including the hail special-cases (mutual-hailer, in-radius, moved-target)
- [ ] #4 go test -race ./... passes; docs/wiki sim-loop note re-pinned
<!-- AC:END -->
