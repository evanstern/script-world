---
id: TASK-70
title: Extract the intent-landing ladder into named rungs (sim loop refactor)
status: In Progress
assignee: []
created_date: '2026-07-23 06:34'
updated_date: '2026-07-23 09:04'
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

Spec: specs/022-landing-ladder-rungs
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Landing logic extracted from handleCommand into named rungs; the adapted/failed/hailTarget flag interplay is gone
- [x] #2 Determinism harness proves bit-identical replay on existing seeds across the refactor
- [x] #3 Each rung has isolated unit tests, including the hail special-cases (mutual-hailer, in-radius, moved-target)
- [ ] #4 go test -race ./... passes; docs/wiki sim-loop note re-pinned
- [x] #5 Spec phase: Setup
- [x] #6 Spec phase: User Story 1 — named-rung extraction (P1) 🎯 MVP
- [x] #7 Spec phase: User Story 2 — behavior-identity proof (P1)
- [x] #8 Spec phase: User Story 3 — rung isolation tests (P2)
- [ ] #9 Spec phase: Polish & Cross-Cutting
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit at specs/022-landing-ladder-rungs (spec/plan/research/data-model/quickstart/tasks; clarify skipped — no material ambiguity, scope pinned by the board task). Extraction: new internal/sim/landing.go with (*Loop).landIntent + doctrine-named rung funcs and an explicit landingDecision value (research.md D1-D4); inject_intent case shrinks to the dispatch. Gate: existing determinism/replay suite unedited + go test -race ./... (D6); new landing_test.go rung-isolation tests (D7). One PR from .worktrees/task-70.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Tier decision (constitution V rubric): Opus 4.8 via spec-implementer. Justification: core sim-loop change in the review-flagged worst complexity hotspot; doctrine-adjacent (the ladder is doctrine enforcement — outcome vocabulary, hail relaxation D6, metered rejection pairing); a behavioral slip ships a live defect. Not routine/single-mechanism work, so the Sonnet default does not apply.

spec-bridge sync: Setup: 2/2 · User Story 1 — named-rung extraction (P1) 🎯 MVP: 4/4 · User Story 2 — behavior-identity proof (P1): 3/3 · User Story 3 — rung isolation tests (P2): 3/3 · Polish & Cross-Cutting: 1/2

Implementation merged-ready: PR #47 open (branch task-70-landing-ladder-rungs, commits a981a7b extraction + 7540186 tests, forked from 213a6fa). Gates re-verified by orchestrator: determinism/replay subset + TestLanding green (9.2s), go vet clean, full go test -race ./... green (19 pkgs, exit 0), diff surface = internal/sim/{loop.go,landing.go,landing_test.go} only. AC #4 pending merge + wiki re-pin (T014). PR merge blocked by session permissions — awaiting user merge of PR #47.
<!-- SECTION:NOTES:END -->
