---
id: TASK-54
title: >-
  resolveWorld swallows manifest-open errors — version mismatch reports as 'no
  world named'
status: To Do
assignee: []
created_date: '2026-07-22 04:54'
updated_date: '2026-07-24 02:42'
labels:
  - bug
dependencies: []
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found live (2026-07-22, post-PR-#31): a stale v1 binary running 'scriptworld ui myworld-01' against the migrated v2 world reported "no world named myworld-01" while ps (IPC-based, no manifest read) listed it fine. Root cause: name resolution treats any world.Open failure as not-a-world and falls through to the not-found message, hiding the real 'format_version N unsupported (run scriptworld migrate)' error. Fix: resolution should distinguish 'directory exists but unopenable' from 'no such world' and surface the Open error verbatim. Applies both directions (old binary/new world, new binary/future-format world). Surgical: internal/worlds resolution path + a test.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Resolution distinguishes 'directory exists but unopenable' from 'no such world' and surfaces the world.Open error verbatim (both directions: old binary/new world, new binary/future world)
- [ ] #2 Test covering a format_version-mismatch world resolving to the migrate-hint error, not not-found
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: still real. Fresh pins — isReadableWorld returns world.Open(dir)==nil at internal/worlds/resolve.go:106-109, so a format-version-mismatch world is indistinguishable from a missing one; falls through to ErrNotFound ('no world named %q', resolve.go:44-45, 82-103). CLI path via cmd/promptworld/commands.go:68-73.
<!-- SECTION:NOTES:END -->
