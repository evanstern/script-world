---
id: TASK-3
title: Terminal client shell (Bubble Tea)
status: Done
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 02:36'
labels:
  - ui
dependencies:
  - TASK-2
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Attachable TUI client: pane framework with map (default), chronicle, Metatron console, soul reader; map renders live world state; other panes may stub until their systems land. Grounding: grounded-assumptions.md (Terminal UI).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Client attaches to a running daemon and renders the live map by default
- [x] #2 All four panes navigable; detach leaves the world running
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Branch task-3-terminal-client off 001-world-daemon (stacked; PR base = 001-world-daemon until PR #1 merges)
2. Protocol: add 'state' command returning canonical sim.State JSON + last_seq (loop DoState, server, client, contract doc update)
3. internal/tui: Bubble Tea model — four panes (map default / chronicle / metatron stub / souls stub), status bar, keys (1-4/tab panes, space pause, [/] speed, q detach)
4. Map renders live replica: initial state via 'state' cmd, then subscribe(since=last_seq) applying events through sim.State.Apply (same reducer as daemon)
5. scriptworld ui <dir> subcommand
6. Tests: model unit tests (pane nav, event application, render), ipc test for state cmd; full -race suite
7. Re-pin wiki notes whose sources changed (wiki-update), commit, PR, board close-out
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented on branch task-3-terminal-client (stacked on 001-world-daemon). scriptworld ui: Bubble Tea four-pane client — map (default, live 16x16 grid), chronicle (event feed), metatron + souls stubs. Map runs on a log-shipped replica: new 'state' protocol cmd returns canonical sim.State + last_seq, then subscribe(since) applies pushes through the daemon's own Apply reducer. Verified: go test -race ./... green (TUI model units + state-cmd coherence integration test); expect-driven PTY smoke against a live daemon — all four panes rendered (AC#2), map default with live header (AC#1), space paused the daemon, ] changed speed, q detached with world still running (AC#2). Wiki re-pinned (18 notes fresh). Note: an apparent q-hang in early smoke runs was a test-harness artifact (expect not draining the PTY between sends; app blocked on stdout) — reproduced, root-caused, not a product bug.

PR: https://github.com/evanstern/script-world/pull/2 (base 001-world-daemon; retargets to main when PR #1 merges)
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Bubble Tea TUI client shipped: four navigable panes (map default, chronicle, metatron stub, soul reader stub) over the daemon protocol; live map via event-sourced client replica ('state' cmd + subscribe through the shared sim reducer); pause/speed/detach controls verified against a running daemon end-to-end. Both ACs proven; wiki updated (tui-client note added, 10 notes re-pinned).
<!-- SECTION:FINAL_SUMMARY:END -->
