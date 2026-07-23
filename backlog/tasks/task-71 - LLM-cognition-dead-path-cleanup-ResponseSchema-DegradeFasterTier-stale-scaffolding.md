---
id: TASK-71
title: >-
  LLM/cognition dead-path cleanup (ResponseSchema, DegradeFasterTier, stale
  scaffolding)
status: Done
assignee: []
created_date: '2026-07-23 06:34'
updated_date: '2026-07-23 15:13'
labels:
  - review-2026-07-22
  - code-quality
dependencies: []
priority: medium
ordinal: 64000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (removal items), re-verified present 2026-07-23. Small, pure-deletion PR: (a) ResponseSchema/SchemaName — a dead production path: no request sets it in production (wiki llm-orchestrator.md:55-59 concedes this) yet it carries real branching in callNative (internal/llm/providers.go:152-169) and API surface (llm.go:97-103). Cut until a caller exists; git remembers. (b) DegradeFasterTier — a documented no-op enum for a model tier that does not exist (internal/cognition/registry.go:14-16); reduce to a one-line comment or delete. (c) The spec-012/013 "explicit no-op" reducer scaffolding comments (internal/sim/state.go ~572-576 and ~717-723) — phased-build collision guards whose phases have long since landed; the explanatory paragraphs outweigh the code.

VERIFIED EXCLUSION: FutureDated on DecisionClass was on the review removal list but is now LIVE (read at internal/mind/mind.go:341) — do NOT remove it.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 ResponseSchema/SchemaName removed from Request and providers; no production or test references remain
- [x] #2 DegradeFasterTier enum removed or reduced to a comment; registry and docs consistent
- [x] #3 Stale spec-012/013 no-op scaffolding comments deleted from the reducer
- [x] #4 FutureDated untouched (verified live); go test -race ./... passes; wiki notes re-pinned (llm-orchestrator, cognition)
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Trivial exemption from Spec Kit (constitution Dev Workflow): surgical pure-deletion, file:line diagnosis + ACs on this card. No spec dir.
2. Worktree .worktrees/task-71, branch task-71-dead-path-cleanup off origin/main.
3. Delegate to spec-implementer (Sonnet). Tier justification: pure deletion of dead paths — no concurrency/scheduling/governor logic touched; rubric Opus triggers do not apply. Line refs re-verified 2026-07-23: llm.go:97-104, providers.go:152-169, registry.go:14-16, state.go:609-613 & 754-760 (drifted from card's ~572/~717). FutureDated live at mind.go:341 — excluded.
4. Gate on implementer report: go build + go test -race ./... in worktree; review diff.
5. One PR; after merge: wiki-update re-pin (llm-orchestrator, cognition), tick ACs, Done.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented by spec-implementer (Sonnet per rubric: pure deletion, no concurrency/governor logic) as db45c44; merged to main via PR #48 (cabe1fb). 6 files, +3/-89. Gates: go build, go vet, go test -race ./... all green (18 pkgs). FutureDated verified live at mind.go:341, untouched. Worktree .worktrees/task-71 removed, branch deleted, root ff-pulled. Remaining for AC 4: wiki re-pin (llm-orchestrator, cognition).

Merged as PR #48 (cabe1fb). Wiki re-pinned in 3cf697f: cognition + llm-orchestrator prose updated, agent-journal/event-types/sim-state-reducer re-pinned verified (comment-only state.go diff); plan gate empty, freshness gate OK (36 notes). Worktree removed, branch deleted, root ff-pulled.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Pure-deletion cleanup landed via PR #48 (merge cabe1fb, commit db45c44; 6 files, +3/-89). (a) ResponseSchema/SchemaName removed from llm.Request, callNative's response_format branch, and both tests that exercised it — zero references remain. (b) DegradeFasterTier reduced to a one-line comment in cognition/registry.go. (c) spec-012/013 explicit-no-op scaffolding paragraphs deleted from the state.go reducer (section dividers kept). FutureDated verified live (mind.go:341) and untouched. Gates: go build, go vet, go test -race ./... green (18 pkgs). Implemented by spec-implementer on Sonnet (rubric: pure deletion, no concurrency/scheduling/governor logic). Trivial Spec Kit exemption applied (surgical fix, file:line diagnosis + ACs on card). Wiki re-pinned (3cf697f): llm-orchestrator + cognition prose updated, three sim notes verified comment-only.
<!-- SECTION:FINAL_SUMMARY:END -->
