---
id: TASK-43
title: 'World instance manager — ps/start/stop by name, default worlds home'
status: In Progress
assignee: []
created_date: '2026-07-21 14:01'
updated_date: '2026-07-21 15:58'
labels: []
dependencies: []
ordinal: 37500
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Users lose track of concurrently running world daemons and clobber the shared LLM host. Deliver docker/ollama-style instance management: scriptworld ps lists every running world machine-wide (name, state, pid, tick, game time, speed, LLM on/off); new <worldname> creates in ~/.scriptworld/worlds by default with an explicit-path escape hatch; every per-world command accepts a name or a path (paths keep today's exact behavior). Worlds stay self-contained copyable directories; any manager state is advisory and self-healing, liveness always re-proven from live evidence.

Spec: specs/008-instance-manager
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 scriptworld ps lists every running world machine-wide with name, state, pid, tick, game time, speed, LLM on/off — no false 'running' from stale files
- [ ] #2 new <worldname> creates in the default worlds home; explicit paths still work everywhere; worlds remain self-contained copyable directories
- [ ] #3 All per-world commands accept a name or a path; path invocations behave exactly as before
- [ ] #4 Manager state is advisory and self-healing; a world runs with no manager state present
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
spec-bridge sync:  — status To Do → In Progress
<!-- SECTION:NOTES:END -->
