---
id: TASK-44
title: >-
  Musing cadence phase collapse: re-arm from now destroys boot stagger — whole
  village muses in lockstep
status: Done
assignee: []
created_date: '2026-07-21 14:05'
updated_date: '2026-07-21 15:11'
labels:
  - bug
  - cognition
dependencies: []
priority: high
ordinal: 38000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Live finding (demo world, day 1 13:34): all 8 agents fired musing jobs at the same tick (identical snapshot_tick/predicted_land_tick) and all dropped 'tier busy; best-effort call dropped'. Root cause: boot stagger (mind.go:152, per-agent offsets interleaved with planner stagger) is destroyed at runtime — after any shared stall makes multiple agents overdue, the drain (mind.go:474-486) fires them all in one tick and re-arms each to the IDENTICAL tick+museCadenceTicks (mind.go:486, re-arm from now instead of from the agent's phase). Phases lock permanently; every 900 ticks the herd collides with the tier and drops as a block (only museStarveWindow rescues occasional musings). Fix options: advance museDue in cadence multiples from the ORIGINAL due (preserves phase), or re-jitter per-agent on re-arm, or rate-limit the drain to one musing per tick. Evidence: event log seqs 12153-12168 in the TASK-34 demo world; cross-refs TASK-40 concurrency finding and TASK-32 idea E.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Phase-preserving re-arm: after any shared stall, per-agent musing dues remain pairwise distinct and preserve boot offset mod cadence
- [x] #2 Planner nextDue re-arm audited; same mechanism fixed the same way (both sites) without touching the separate lastPlanned/planDebounceTicks debounce
- [x] #3 Regression tests cover the pure function, the 8-agent shared-stall collapse, and multi-cadence skip without drift
- [x] #4 go build, go vet, gofmt (touched files), full go test suite green
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation started: worktree .worktrees/task-44, branch task-44-musing-phase-collapse (forked from 655b019). Delegated to implementing tier (Sonnet — low-complexity surgical fix per constitution tiering). Scope: phase-preserving re-arm for museDue; audit planner nextDue for the same class of bug (fix if identical, report if intentionally different); regression tests for phase preservation across stalls.

Implementer (Sonnet) delivered e140f7e: extracted nextPhasePreservingDue(due,tick,cadence); applied at the musing re-arm and BOTH planner nextDue re-arm sites (audit confirmed identical mechanism; night_started arms all agents in one tick — same collapse class; debounce untouched). Tests: TestNextPhasePreservingDue (table), TestMusingCadenceSurvivesSharedStall (8-agent repro), TestNextPhasePreservingDueSkipsWithoutDrift (37-cadence stall, zero drift). Orchestrator independently verified: build clean, internal/mind green uncached (67.7s); agent ran full suite incl e2e green. Trivial-exemption task per constitution v1.1.0 (surgical + pinned diagnosis + these ACs).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase-preserving cadence re-arm shipped: nextPhasePreservingDue(due,tick,cadence) applied at the musing re-arm and both planner nextDue re-arm sites; boot stagger now survives shared stalls permanently. PR #25 merged as acd2cd9. All 4 ACs verified (implementer report + orchestrator independent build/test); tests TestNextPhasePreservingDue, TestMusingCadenceSurvivesSharedStall, TestNextPhasePreservingDueSkipsWithoutDrift. Wiki re-grounded: docs/wiki/agent-mind.md re-verified and re-pinned to acd2cd9 (freshness gate green, 29 notes). Worktree and branch cleaned; root ff-pulled. First task through constitution v1.1.0's trivial-exemption path.
<!-- SECTION:FINAL_SUMMARY:END -->
