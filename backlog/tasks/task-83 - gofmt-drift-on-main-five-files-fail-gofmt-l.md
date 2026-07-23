---
id: TASK-83
title: 'gofmt drift on main: five files fail gofmt -l'
status: To Do
assignee: []
created_date: '2026-07-23 21:19'
labels: []
dependencies: []
ordinal: 75000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found by spec 028's T017 gate (2026-07-23): gofmt -l internal/ flags five files whose drift PRE-EXISTS task-33 (not in its diff): internal/metatron/digest.go, internal/sim/bulk_cap_test.go, internal/sim/ground_pile_test.go, internal/tui/grammar.go, internal/tui/render_test.go. Surgical fix: gofmt -w those files. Consider adding a gofmt check to the standing test/CI gate so drift can't accumulate silently. Trivial-exemption candidate (surgical, diagnosis pinned, ACs here).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 gofmt -l internal/ prints nothing on main
- [ ] #2 A standing gate (test or hook) fails on future gofmt drift, or a decision not to add one is recorded
<!-- AC:END -->
