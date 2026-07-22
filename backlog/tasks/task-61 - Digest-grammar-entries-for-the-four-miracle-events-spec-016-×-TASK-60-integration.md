---
id: TASK-61
title: >-
  Digest grammar entries for the four miracle events (spec 016 × TASK-60
  integration)
status: To Do
assignee: []
created_date: '2026-07-22 20:32'
labels: []
dependencies: []
ordinal: 54000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
PR #37 (TASK-60, chronicle digest grammar) and PR #38 (TASK-59, metatron miracles) were developed concurrently; the digest's per-type summary table (internal/tui/digest.go:741 area) has an entry for metatron.nudged but none for the four new miracle events, so metatron.time_snapped / item_granted / entity_moved / entity_removed render via the untyped fallback in the chronicle digest.

Add readable per-event summary functions for the four miracle types in the metatron family style, including the gratis marker (an operator force should be visible in the digest, SC-004 of spec 016). Follow the existing summary-func table pattern and grammar tests.

Trivial-exemption candidate (surgical, single mechanism, diagnosis pinned): internal/tui/digest.go summary table + tests. Blocked until PR #38 merges.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 All four metatron miracle events render readable digest summaries (no raw-payload fallback)
- [ ] #2 Gratis miracles are visibly marked in the digest line
- [ ] #3 Grammar/digest tests cover the four new types
<!-- AC:END -->
