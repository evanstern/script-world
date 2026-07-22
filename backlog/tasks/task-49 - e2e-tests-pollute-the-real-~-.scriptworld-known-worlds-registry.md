---
id: TASK-49
title: e2e tests pollute the real ~/.scriptworld known-worlds registry
status: To Do
assignee: []
created_date: '2026-07-21 20:30'
updated_date: '2026-07-22 04:38'
labels: []
dependencies: []
priority: medium
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Found live on 2026-07-21 while auditing a migrated world: ~/.scriptworld/known_worlds.json contained a stale entry 'w' -> /var/folders/.../TestDeterminism_FullBinary.../002/w. Root cause: since TASK-43, daemon boot upserts outside-home worlds into the registry (internal/daemon/daemon.go registerWorld), and the shared e2e helpers exec the real binary with the inherited environment — no hermetic SCRIPTWORLD_HOME (e2e/daemon_e2e_test.go:39 and :47, used by daemon/determinism/cognition e2e; only e2e/manager_e2e_test.go isolates it). Every e2e run therefore writes temp-dir worlds into the developer's real registry. Damage is self-healing (entries show missing in ps --all and prune on next registry write) but tests must never touch real user state. Fix: set SCRIPTWORLD_HOME to a per-test/per-process temp dir in the shared e2e exec helpers (or package TestMain).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 All e2e daemon-spawning helpers run with a hermetic SCRIPTWORLD_HOME (no test writes ~/.scriptworld)
- [ ] #2 Full e2e suite green; a run against a seeded real-registry fixture leaves it byte-identical
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: refs drifted ~2 lines — run/runErr helpers at daemon_e2e_test.go:~37/46 (exec real binary, no .Env, inherit os.Environ); registerWorld at daemon boot daemon.go:60; only manager_e2e_test.go isolates SCRIPTWORLD_HOME (:18-27). Diagnosis and fix (hermetic SCRIPTWORLD_HOME in shared helpers / TestMain) unchanged.

Live symptom observed + cleaned during TASK-50's myworld-01 discoverability check (2026-07-22): known_worlds.json contained a stale 'w' entry pointing at a deleted TestDeterminism_FullBinary temp dir, showing as 'missing' in ps --all. Entry removed by hand (registry is a client-side advisory file). Root cause — e2e tests writing to the real registry — remains this task's scope.
<!-- SECTION:NOTES:END -->
