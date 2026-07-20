---
id: TASK-19
title: >-
  IPC: full-state reply can exceed maxLineBytes on long runs (client bufio
  overflow)
status: In Progress
assignee: []
created_date: '2026-07-19 21:57'
updated_date: '2026-07-20 19:55'
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

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Root cause: server writes the state reply as ONE JSON line of unbounded size; both client and server bufio.Scanners cap lines at maxLineBytes (1MiB). A state reply >1MiB kills the client read loop with bufio.ErrTooLong ('token too long'), and the TUI's retryLater() reconnects every 2s forever.

Fix (both directions, per task guidance):
1. Split the single cap: keep 1MiB for client->server request lines (requests are small); introduce maxReplyBytes = 64MiB as the documented server->client line bound. Client scanner buffer sized to maxReplyBytes, so states up to 64MiB now just work (the gru-proof case succeeds).
2. Server guard: writeResponse checks marshaled size; a reply over maxReplyBytes is replaced by an ok:false error response with a recognizable 'reply too large' prefix + byte counts — the server can never emit a line a current client cannot read.
3. Client fail-fast: exported sentinel ipc.ErrReplyTooLarge. Scanner ErrTooLong is mapped to it with an actionable message (version-skew safety net); Call() wraps server 'reply too large' errors in the same sentinel.
4. TUI: disconnectedMsg with errors.Is(err, ipc.ErrReplyTooLarge) is fatal — quit with the message (surfaced via Model.FatalErr() -> cmdUI returns it) instead of the endless retry loop.
5. Tests: fake-server test proving FetchState succeeds on a >1MiB state; oversized-line test proving Call fails fast with ErrReplyTooLarge (no hang); net.Pipe session test proving the server substitutes the actionable error; TUI Update test proving the fatal path quits instead of retrying.
<!-- SECTION:PLAN:END -->
