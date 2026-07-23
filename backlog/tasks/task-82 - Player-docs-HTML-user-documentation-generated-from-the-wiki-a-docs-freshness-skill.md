---
id: TASK-82
title: >-
  Player docs: HTML user documentation generated from the wiki + a
  docs-freshness skill
status: In Progress
assignee: []
created_date: '2026-07-23 18:06'
updated_date: '2026-07-23 18:26'
labels:
  - docs
dependencies: []
priority: medium
ordinal: 72000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
User-level documentation for promptworld — what a PLAYER needs to start and enjoy the game (install, create/start a world, attach/TUI, Metatron and the charter, speed/pause, reading the chronicle, llm.json basics for non-engineers) — as HTML pages in a docs folder (proposed: docs/player/), distinct from the developer-facing docs/wiki corpus and from codebase-to-course's single-page course.

Source of truth: docs/wiki/ (the code-grounded corpus) plus README.md and docs/llm-providers.md — player docs are a curated, plain-language projection of that grounding, never independently asserted facts. Requested by the operator 2026-07-23 alongside the docs/llm-providers.md operator guide.

Mechanism (the deliverable beyond the pages): a project skill (.claude/skills/) that (a) generates/regenerates the player docs from the wiki corpus, and (b) keeps them fresh — each page carries the wiki notes + commit it was rendered from (frontmatter or HTML meta), and the skill detects when source notes have been re-pinned past a page's pin and refreshes only the stale pages. Freshness should be checkable (a --check mode reporting stale pages), mirroring the grounding-wiki update/gate pattern so 'docs are current' is provable, not asserted.

Open design points for the session that starts this task: static HTML vs generated-per-page styling (self-contained pages, shared minimal CSS, theme-aware); nav/index page; whether the skill runs standalone or is invoked from wiki-update as a follow-on; scope line between player docs and operator docs (llm-providers.md stays operator-level).

Spec: specs/026-player-docs
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A docs folder of self-contained HTML player pages exists, covering at minimum: getting started (install→new→start→attach), playing via Metatron + charter, time/speed/pause, reading the chronicle/story, and a plain-language 'the AI behind the village' page
- [ ] #2 Every page records the wiki notes + commit it was rendered from; a skill regenerates stale pages and offers a check mode that reports staleness without writing
- [ ] #3 The skill is planted as a project skill and documented (name + when to run) in CLAUDE.md or the skill's own description; running it twice in a row is a no-op
- [ ] #4 Player docs contain no facts that contradict docs/wiki at their pinned commit (spot-audit recorded on this task)
- [ ] #5 Spec phase: Setup
- [ ] #6 Spec phase: Foundational (blocks all user stories)
- [ ] #7 Spec phase: User Story 1 — new player gets to a running world (P1) 🎯 MVP
- [ ] #8 Spec phase: User Story 2 — player learns to play (P2)
- [ ] #9 Spec phase: User Story 3 — provable freshness (P3)
- [ ] #10 Spec phase: Polish & Validation
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Full Spec Kit per constitution v1.1.0: spec'd as specs/026-player-docs (spec/plan/tasks + research/data-model/contract/quickstart), linked via spec-bridge. Implementation in .worktrees/task-82 on branch task-82-player-docs, delegated to spec-implementer. 6 phases / 15 tasks: skill template first (SKILL.md), then pages US1 (index+getting-started) → US2 (4 gameplay pages), then US3 (check-freshness.mjs + CLAUDE.md pointer), then validation (quickstart V1–V6) incl. grounding spot-audit recorded here. One task, one PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Model tier: Sonnet (default implementation tier). Rubric: doc/rendering + a standalone zero-dependency Node script — single-package, no concurrency/scheduling/governor logic, not doctrine-adjacent, no prior failed attempt. No escalation triggers met.
<!-- SECTION:NOTES:END -->
