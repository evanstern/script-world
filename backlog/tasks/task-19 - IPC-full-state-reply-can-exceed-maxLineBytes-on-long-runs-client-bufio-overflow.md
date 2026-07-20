---
id: TASK-19
title: >-
  IPC: full-state reply can exceed maxLineBytes on long runs (client bufio
  overflow)
status: Done
assignee: []
created_date: '2026-07-19 21:57'
updated_date: '2026-07-20 20:04'
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
- [x] #1 A client connecting to a world whose state exceeds 1MB either succeeds or fails with an actionable error, not an endless retry loop
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

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-19-ipc-state-reply (rebased onto origin/main after TASK-13 merged; no overlap).

Root cause: state reply is one JSON line of unbounded size; client+server shared maxLineBytes=1MiB scanner cap, so a >1MiB state killed the client read loop (bufio.ErrTooLong) and the TUI's retryLater() reconnected every 2s forever.

Fix landed in two commits:
- b95ee64 ipc: maxRequestBytes 1MiB / maxReplyBytes 64MiB split; client scanner sized to the reply cap (>1MiB states now just work); server writeResponse substitutes an ok:false 'reply too large' error (same ID, byte counts) so the wire never carries an unreadable line; exported ipc.ErrReplyTooLarge classifies both server refusals and raw ErrTooLong (version skew) as fatal. Contract doc (specs/001-world-daemon/contracts/client-protocol.md) documents the caps.
- 14c63fd tui: disconnectedMsg with errors.Is(err, ipc.ErrReplyTooLarge) quits with the reason (Model.FatalErr -> cmdUI returns non-zero) instead of retrying; transient disconnects still retry.

Proof (all pass, full go test ./... green):
- TestFetchStateOver1MiBSucceeds — 2MiB state round-trips (AC success arm)
- TestServerSubstitutesActionableErrorForOversizedReply — server caps at 64MiB with actionable error
- TestClientClassifiesServerRefusalAsFatal / TestOversizedRawLineFailsFastNotForever — fail fast, no hang (AC failure arm)
- TestReplyTooLargeQuitsInsteadOfRetrying — TUI quits, no retry loop; transient errors still retry

Wiki notes needing re-pin before PR: ipc-protocol.md, ipc-server.md, ipc-client.md, tui-client.md, cli-scriptworld.md
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed the 1MiB IPC line ceiling that left clients in an endless 'token too long (retrying...)' loop on long-run worlds. Split the shared cap: requests stay 1MiB, daemon->client replies get a documented 64MiB ceiling the client's scanner matches — so the gru-proof case (state >1MiB) now succeeds. The server never emits a line over the cap (oversized replies become an ok:false 'reply too large' error with byte counts), and the client classifies both that refusal and raw over-long lines as ipc.ErrReplyTooLarge — fatal, so the TUI quits with the actionable message (non-zero exit via cmdUI) instead of retrying forever. AC#1 proven by 5 new tests across ipc and tui; full go test ./... green. Commits b95ee64 + 14c63fd on task-19-ipc-state-reply. Wiki re-pin needed: ipc-protocol, ipc-server, ipc-client, tui-client, cli-scriptworld.
<!-- SECTION:FINAL_SUMMARY:END -->
