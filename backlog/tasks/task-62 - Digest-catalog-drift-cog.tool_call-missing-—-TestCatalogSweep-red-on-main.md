---
id: TASK-62
title: 'Digest catalog drift: cog.tool_call missing — TestCatalogSweep red on main'
status: To Do
assignee: []
created_date: '2026-07-23 03:09'
labels:
  - events
  - tui
dependencies: []
priority: medium
ordinal: 55000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
internal/tui TestCatalogSweep (digest_test.go:183) fails on clean main: docs/wiki/event-types.md backticks cog.tool_call (added by TASK-52's wiki re-pin, 7ea819f) but the TUI digest catalog fixture doesn't cover it — TASK-60's catalog was pinned before that event type existed in the wiki note. Pre-existing vs task-16 branch (verified 2026-07-23 at 6c6b5a4). Trivial exemption per constitution: surgical fix (add a cog.tool_call digest template + fixture coverage in internal/tui), complete file:line diagnosis pinned here.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 TestCatalogSweep green on main
- [ ] #2 cog.tool_call renders a readable digest line consistent with the spec-018 grammar
<!-- AC:END -->
