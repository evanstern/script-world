---
id: TASK-38
title: >-
  Docs drift audit 2026-07-21: reconcile wiki, README, and design notes with
  code
status: Done
assignee: []
created_date: '2026-07-21 13:18'
updated_date: '2026-07-21 13:26'
labels: []
dependencies: []
ordinal: 32000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Full-repo drift scan (wiki mechanical freshness gate + semantic verification of all 29 notes, README, docs/design, CLAUDE.md). Mechanical gate passed; 14 semantic drift items found. Fix on one branch, one PR.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 overview.md: Go version says 1.22+ but go.mod declares go 1.26.4
- [x] #2 overview.md: 'Placeholder simulation events flow until real village systems (TASK-4+) replace them' — stale; full village stack is implemented
- [x] #3 sim-loop.md: '~1.65M ticks/sec with the placeholder sim' — unqualified stale framing; mark as TASK-2-era measurement
- [x] #4 design-grounding.md: 'anticipates but does not implement: agent mind (agents/ dir exists empty), Metatron, social fabric, chronicle, gru' — all implemented since (TASK-5..13); rewrite tense
- [x] #5 deterministic-rng.md: purpose example '"move"' does not exist; movement uses "wander" (policy.go)
- [x] #6 cli-scriptworld.md: stop 'waits <=10 s for pidfile to clear' but commands.go:282 waits 30 s
- [x] #7 testing-strategy.md: 'over 10k ticks' stale; determinism harness runs 30,000 ticks (sim_test.go:69)
- [x] #8 cognition.md: sources list missing internal/sim/cognition.go (note describes OutcomeRejectedStale, cog.* payload types defined there)
- [x] #9 executor.md: '~4 needs events/game-minute' wrong; 1 per living agent per game-minute x 8 agents = ~8
- [x] #10 event-types.md: agent.talked emitter 'adjacent idle pair' wrong; chat-while-working, only dead/asleep excluded (executor.go:269-273)
- [x] #11 nightly-consolidation.md: prompt shows ordinal labels m1..mN, not (tick,hash) refs; (tick,hash) only used internally mapping accepted refs
- [x] #12 nightly-consolidation.md: anchor echo is normalized comparison (case/whitespace/trailing punct), not byte-for-byte (validate.go:71-75)
- [x] #13 README.md: 'daily noon meeting' stale post-TASK-36; meeting hour is per-world config or emergent convention
- [x] #14 README.md: quickstart omits resume; add it and point at scriptworld help for the full list
- [x] #15 repo hygiene: backlog task files task-34..37 untracked in git despite TASK-36/37 merged; commit board files
- [x] #16 edited wiki notes re-pinned (verified_against) to the audited commit 0264eb3
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1) Worktree .worktrees/task-38 off origin/main. 2) Delegate the 16 checklist edits to spec-implementer (constitution Principle V). 3) Verify: re-run mechanical freshness scan + grep spot checks. 4) Commit incl. untracked backlog files; PR.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Fixes applied by spec-implementer in .worktrees/task-38; verified by orchestrator (diff review + freshness re-scan green). Commit 004a430: README + board files; d38f4c3: 11 wiki fixes, 10 notes re-pinned to 004a430 (child of audited 0264eb3 — only README/board changed between, README being a source of overview.md, hence the newer pin). Batches B–E clean notes needed no re-pin.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Full drift scan 2026-07-21: mechanical freshness gate green; 6 parallel auditors semantically verified all 29 wiki notes + README + grounded-assumptions + CLAUDE.md against 0264eb3. 14 drift items found and fixed (11 wiki, 2 README, 1 board hygiene); 10 notes re-pinned; freshness gate re-verified green on merged main (4926cb3). PR #24 merged; worktree and branch cleaned up.
<!-- SECTION:FINAL_SUMMARY:END -->
