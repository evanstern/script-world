---
id: TASK-62
title: 'Digest catalog drift: cog.tool_call missing — TestCatalogSweep red on main'
status: Done
assignee: []
created_date: '2026-07-23 03:09'
updated_date: '2026-07-23 05:05'
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
- [x] #1 TestCatalogSweep green on main
- [x] #2 cog.tool_call renders a readable digest line consistent with the spec-018 grammar
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Worktree .worktrees/task-62 (branch task-62-cog-tool-call-digest) off origin/main.
2. Add cog.tool_call digest to internal/tui/digest.go cog (labeled) block — labeled voice per spec-018 grammar §2: job=… ord=… tool=… <verdict> tier=… [reason=…]; args elided (detail pane bounds them, world.migrated precedent).
3. Add catalogFixture row + cog.tool_call to TestDigestRoleSpans labeled list in digest_test.go.
4. go test ./internal/tui/ green; PR, merge, tick ACs.
Tier: Sonnet spec-implementer — routine single-package view/rendering change per constitution Principle V rubric; no concurrency/cross-package surface.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-62-cog-tool-call-digest (worktree .worktrees/task-62), commit 6bcda8d, PR #43 open: https://github.com/evanstern/promptworld/pull/43. Sonnet spec-implementer per Principle V rubric (routine single-package view/rendering slice). Labeled-voice digest job/ord/tool/verdict/tier + conditional reason; args+snapshot_tick elided (detail pane bounds them, world.migrated precedent). go test ./... fully green in worktree; go vet clean. AC#2 proven by TestCatalogSweep+TestDigestRoleSpans on the branch. AC#1 (green on main) pends the PR merge — merge blocked by permission classifier, awaiting user.

PR #43 merged (4d9088a). TestCatalogSweep verified green on merged main. Worktree .worktrees/task-62 removed, branch deleted, root ff-pulled. Wiki re-pinned: docs/wiki/tui-client.md 9495150 → d38330a after reading the diff (registry-only addition; note claims unchanged); plan + freshness gates pass (36 notes fresh).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
cog.tool_call now renders a labeled-voice digest (job/ord/tool/verdict/tier, reason when present; args+snapshot_tick elided to the detail pane) with a catalog fixture row + TestDigestRoleSpans coverage. TestCatalogSweep green on main (PR #43, merge 4d9088a). Wiki tui-client.md re-pinned; freshness gate green.
<!-- SECTION:FINAL_SUMMARY:END -->
