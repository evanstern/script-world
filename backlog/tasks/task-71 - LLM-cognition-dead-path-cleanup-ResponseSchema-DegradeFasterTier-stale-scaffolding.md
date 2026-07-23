---
id: TASK-71
title: >-
  LLM/cognition dead-path cleanup (ResponseSchema, DegradeFasterTier, stale
  scaffolding)
status: To Do
assignee: []
created_date: '2026-07-23 06:34'
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
- [ ] #1 ResponseSchema/SchemaName removed from Request and providers; no production or test references remain
- [ ] #2 DegradeFasterTier enum removed or reduced to a comment; registry and docs consistent
- [ ] #3 Stale spec-012/013 no-op scaffolding comments deleted from the reducer
- [ ] #4 FutureDated untouched (verified live); go test -race ./... passes; wiki notes re-pinned (llm-orchestrator, cognition)
<!-- AC:END -->
