---
id: TASK-54
title: >-
  resolveWorld swallows manifest-open errors — version mismatch reports as 'no
  world named'
status: To Do
assignee: []
created_date: '2026-07-22 04:54'
labels:
  - bug
dependencies: []
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found live (2026-07-22, post-PR-#31): a stale v1 binary running 'scriptworld ui myworld-01' against the migrated v2 world reported "no world named myworld-01" while ps (IPC-based, no manifest read) listed it fine. Root cause: name resolution treats any world.Open failure as not-a-world and falls through to the not-found message, hiding the real 'format_version N unsupported (run scriptworld migrate)' error. Fix: resolution should distinguish 'directory exists but unopenable' from 'no such world' and surface the Open error verbatim. Applies both directions (old binary/new world, new binary/future-format world). Surgical: internal/worlds resolution path + a test.
<!-- SECTION:DESCRIPTION:END -->
