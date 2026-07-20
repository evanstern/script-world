---
id: TASK-19
title: >-
  IPC: full-state reply can exceed maxLineBytes on long runs (client bufio
  overflow)
status: To Do
assignee: []
created_date: '2026-07-19 21:57'
labels:
  - engine
dependencies: []
ordinal: 15000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Surfaced during TASK-10 live proving (gru-proof, 1257 game days without TASK-9 consolidation): the state command's reply line grew past maxLineBytes (1<<20), so scriptworld ui/attach fail with 'bufio.Scanner: token too long (retrying...)' forever while the daemon is healthy. TASK-9's nightly consolidation bounds memory growth and mitigates, but the protocol still has a hard 1MB line ceiling on an unbounded payload. Consider chunked/streamed state replies, a larger cap, or state-size telemetry + graceful client error. Labels: engine.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A client connecting to a world whose state exceeds 1MB either succeeds or fails with an actionable error, not an endless retry loop
<!-- AC:END -->
