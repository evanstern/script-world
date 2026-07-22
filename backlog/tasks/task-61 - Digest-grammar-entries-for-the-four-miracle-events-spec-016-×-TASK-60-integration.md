---
id: TASK-61
title: >-
  Digest grammar entries for the four miracle events (spec 016 × TASK-60
  integration)
status: Done
assignee: []
created_date: '2026-07-22 20:32'
updated_date: '2026-07-22 21:46'
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
- [x] #1 All four metatron miracle events render readable digest summaries (no raw-payload fallback)
- [x] #2 Gratis miracles are visibly marked in the digest line
- [x] #3 Grammar/digest tests cover the four new types
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Trivial exemption applies (constitution: surgical fix, single mechanism internal/tui digest summary table, diagnosis pinned digest.go:741 area, ACs on task). Tier decision (Principle V rubric): Sonnet — single-package view/rendering code with tests alongside; no doctrine surface. One PR from .worktrees/task-61 branch task-61-digest-grammar-miracles.

Implemented on branch task-61-digest-grammar-miracles (commit 40198f3, Sonnet tier): four digest registry entries + gratisMark helper; per-class phrasing (payloads carry class+coords, not names — 'moved the villager at (x,y)' rather than a resolved name; terrain removal reads 'cleared'). Cured TestCatalogSweep, which is RED on main since TASK-59's wiki re-pin backticked the new event types; added 4 catalog fixture rows + 2 dedicated tests; appended the 4 template rows to specs/018 digest-grammar contract (declared enumeration, cross-spec precedent TASK-53 whitelist tripwire). Full gate green. PR #39: https://github.com/evanstern/promptworld/pull/39
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Four miracle event types render readable, per-class digest summaries with a styled (forced) marker on gratis payloads; catalog sweep cured (was red on main after TASK-59's wiki re-pin), 4 fixture rows + 2 dedicated tests added, specs/018 digest-grammar contract enumeration extended. Merged as PR #39.
<!-- SECTION:FINAL_SUMMARY:END -->
