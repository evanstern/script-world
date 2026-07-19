---
id: TASK-3
title: Terminal client shell (Bubble Tea)
status: In Progress
assignee: []
created_date: '2026-07-19 01:13'
updated_date: '2026-07-19 02:21'
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
- [ ] #1 Client attaches to a running daemon and renders the live map by default
- [ ] #2 All four panes navigable; detach leaves the world running
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
