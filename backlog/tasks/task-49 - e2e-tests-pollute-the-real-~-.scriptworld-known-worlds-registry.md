---
id: TASK-49
title: e2e tests pollute the real ~/.scriptworld known-worlds registry
status: Done
assignee: []
created_date: '2026-07-21 20:30'
updated_date: '2026-07-22 23:21'
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
- [x] #1 All e2e daemon-spawning helpers run with a hermetic SCRIPTWORLD_HOME (no test writes ~/.scriptworld)
- [x] #2 Full e2e suite green; a run against a seeded real-registry fixture leaves it byte-identical
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Trivial-exemption surgical fix (file:line diagnosis + ACs on task; no spec). Env var is PROMPTWORLD_HOME (task text's SCRIPTWORLD_HOME is stale naming; resolution in internal/worlds/home.go:19-32). Fix: set a hermetic PROMPTWORLD_HOME (temp dir) in package e2e's TestMain (e2e/daemon_e2e_test.go:21) so every helper/subprocess inherits it; manager_e2e_test.go's per-test isolatedHome keeps layering via t.Setenv. Verify AC#2 by running the suite with HOME pointed at a temp dir seeded with .promptworld/known_worlds.json and diffing byte-identical. Implementation delegated to spec-implementer on Sonnet (routine single-package test fix; no escalation triggers). Wiki: docs/wiki/testing-strategy.md pins daemon_e2e_test.go — re-pin after merge.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Re-grounding 2026-07-22: refs drifted ~2 lines — run/runErr helpers at daemon_e2e_test.go:~37/46 (exec real binary, no .Env, inherit os.Environ); registerWorld at daemon boot daemon.go:60; only manager_e2e_test.go isolates SCRIPTWORLD_HOME (:18-27). Diagnosis and fix (hermetic SCRIPTWORLD_HOME in shared helpers / TestMain) unchanged.

Live symptom observed + cleaned during TASK-50's myworld-01 discoverability check (2026-07-22): known_worlds.json contained a stale 'w' entry pointing at a deleted TestDeterminism_FullBinary temp dir, showing as 'missing' in ps --all. Entry removed by hand (registry is a client-side advisory file). Root cause — e2e tests writing to the real registry — remains this task's scope.

2026-07-22 Sonnet (spec-implementer) landed the fix: package e2e TestMain sets a hermetic PROMPTWORLD_HOME temp dir before m.Run() (e2e/daemon_e2e_test.go, +10 lines); all exec sites inherit it via os.Environ(), manager_e2e_test.go's isolatedHome layers on top unchanged. Note: real env var is PROMPTWORLD_HOME — task text's SCRIPTWORLD_HOME was stale naming. Verified: go vet clean; go test ./e2e/ -count=1 green (23 tests, 183s) = AC#1; seeded known_worlds.json fixture under fake HOME byte-identical (sha256 b30f27fb…f0b0) before/after full suite run, no new files under fake .promptworld = AC#2. Branch task-49-hermetic-e2e-home rebased onto b7d3c03 (concurrent TASK-52 board push) and pushed; PR #40 open. Merge pending user action (classifier blocked gh pr merge). Post-merge remaining: worktree cleanup, root ff-pull, wiki re-pin (docs/wiki/testing-strategy.md pins daemon_e2e_test.go), status Done.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed: package e2e's TestMain sets a hermetic PROMPTWORLD_HOME temp dir before m.Run() (e2e/daemon_e2e_test.go), so every subprocess the suite execs inherits it and no test can write the real ~/.promptworld/known_worlds.json; manager_e2e_test.go's per-test isolatedHome layers on top unchanged. (Task text's SCRIPTWORLD_HOME was stale naming — actual env var is PROMPTWORLD_HOME, internal/worlds/home.go.) Verified: go vet clean; full e2e suite green (23 tests); seeded registry fixture under a fake HOME byte-identical (sha256-matched) after a full suite run with no new files under the fake .promptworld. Merged as PR #40 (8a5604f); worktree/branch cleaned up; docs/wiki/testing-strategy.md re-pinned to 8a5604f.
<!-- SECTION:FINAL_SUMMARY:END -->
