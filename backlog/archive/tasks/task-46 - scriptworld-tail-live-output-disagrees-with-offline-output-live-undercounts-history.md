---
id: TASK-46
title: >-
  scriptworld tail: live output disagrees with offline output (live undercounts
  history)
status: To Do
assignee: []
created_date: '2026-07-21 15:08'
updated_date: '2026-07-24 02:39'
labels:
  - tooling
dependencies: []
priority: medium
ordinal: 40000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Observed 2026-07-21 during the TASK-39/46 forensics: with the daemon RUNNING, 'tail --since 1 | grep -c agent.talked' returned 5 while the same query offline (daemon stopped) shows many more across the full 24961-seq log — live tail appears to serve a bounded window (replica ring?) rather than the persisted log, silently. This misattributed a 'conversation trigger drought' (archived TASK-46) and nearly shipped a wrong verdict. Fix: live tail should read the store (or explicitly say it's windowed); at minimum document the boundary. Verify mechanism first — evidence is one live/offline discrepancy. Also note the operator-error sibling: --since takes SEQ, not TICK; consider warning when --since exceeds last seq (blind-watcher failure mode).
<!-- SECTION:DESCRIPTION:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Drift audit 2026-07-23: headline premise NO LONGER TRUE on main — cmdTail reads history unconditionally from the persisted store, daemon or not (cmd/promptworld/commands.go:676-698; --follow only adds a live IPC subscription after store catch-up, :700-721; wiki cli-promptworld.md:88 and event-log.md:34 agree). The live/offline discrepancy this task was built on cannot recur via that path. Residual niceties (still true, judged too small to keep a card): --since takes SEQ not TICK (commands.go:665) and a --since beyond LastSeq yields silent empty output. Archiving as fixed-by-drift.
<!-- SECTION:NOTES:END -->
