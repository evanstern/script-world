---
id: TASK-82
title: >-
  Player docs: HTML user documentation generated from the wiki + a
  docs-freshness skill
status: To Do
assignee: []
created_date: '2026-07-23 18:06'
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
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A docs folder of self-contained HTML player pages exists, covering at minimum: getting started (install→new→start→attach), playing via Metatron + charter, time/speed/pause, reading the chronicle/story, and a plain-language 'the AI behind the village' page
- [ ] #2 Every page records the wiki notes + commit it was rendered from; a skill regenerates stale pages and offers a check mode that reports staleness without writing
- [ ] #3 The skill is planted as a project skill and documented (name + when to run) in CLAUDE.md or the skill's own description; running it twice in a row is a no-op
- [ ] #4 Player docs contain no facts that contradict docs/wiki at their pinned commit (spot-audit recorded on this task)
<!-- AC:END -->
