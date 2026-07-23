---
id: TASK-74
title: Behavioral test coverage for metatron and persona packages
status: Done
assignee: []
created_date: '2026-07-23 06:35'
updated_date: '2026-07-23 16:21'
labels:
  - review-2026-07-22
  - code-quality
dependencies: []
priority: medium
ordinal: 67000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (test thin spots), re-verified 2026-07-23: internal/metatron has 1 test file against 6 source files; internal/persona 1 against 3. Mitigation exists (sim/miracles_test.go covers the mutation arms; mind tests cover upstream) but the packages own seams those do not reach.

Scope — behavioral tests, not change-detectors, in the codebase style (scripted stubs, no network): metatron: turn dispatch and grant/charge accounting (charge decrement, banking cap, regen), charter provenance (default vs custom detection, per-read reload semantics), fixed-frame composition (the two non-negotiables present beneath ANY charter/skill text), transcript/soul tail windows, ErrTurnBusy serialization. persona: genesis-once semantics (second genesis is a no-op/error, 0444 mode), load behavior on missing/corrupt files, anchor/drift-marker alignment with Texts (index-aligned maps stay consistent — a sweep test).

Post-TASK-64 note: cover the new instruction-surface seams too (skill-file composition order, capability-manifest gating) if TASK-64 left them thin. [Grounding 2026-07-23: inventory in specs/023-metatron-persona-tests/research.md R1 found TASK-64 covered those seams thoroughly; the true gaps are tail windows, charge cap/regen mirror, concurrent ErrTurnBusy, Observe backpressure, absorb mirrors, and the persona sweep/lifecycle set.]

Spec: specs/023-metatron-persona-tests
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Every metatron source file has behavioral coverage of its exported seams; charge economy and fixed-frame composition tested explicitly
- [x] #2 Persona genesis-once, file modes, and cross-map alignment covered incl. a sweep test over the index-aligned maps
- [x] #3 All new tests run under -race with no network (scripted stubs only)
- [x] #4 Wiki testing-strategy note updated with the new coverage
- [x] #5 Spec phase: Setup (Shared Infrastructure)
- [x] #6 Spec phase: User Story 1 - Metatron economy and turn dispatch are provably correct (Priority: P1) 🎯 MVP
- [x] #7 Spec phase: User Story 2 - Instruction-surface composition is pinned by tests (Priority: P2)
- [x] #8 Spec phase: User Story 3 - Persona lifecycle guarantees are enforced by tests (Priority: P3)
- [x] #9 Spec phase: Polish & Cross-Cutting Concerns
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Full Spec Kit (not trivial-exempt: new coverage across two packages). Spec/plan/tasks at specs/023-metatron-persona-tests (spec-bridge linked; 15 tasks T001-T015).
2. Grounding 2026-07-23: existing-coverage inventory in research.md R1 — TASK-64 instruction surface already covered; do NOT duplicate. True gaps: tail windows (soulTail/transcriptTail/tailOfFile), charge cap+regen replica mirror, concurrent ErrTurnBusy, Observe backpressure, absorb mirrors; persona 4-map sweep, anchor≡temperament, unreadable-load degrade, genesis charter/journal seeding, SecretEvents.
3. Worktree .worktrees/task-74, branch task-74-metatron-persona-tests off origin/main.
4. Delegate to spec-implementer, tier=Sonnet. Rubric justification: tests alongside code in two leaf packages, no production changes (FR-011), no concurrency/scheduling/governor production logic touched (writing a race TEST is not governor logic); no prior failed attempt. No Opus trigger applies.
5. Gate on implementer report: go test -race ./... in worktree; R1 anti-duplication review of the diff; quickstart scenarios 3+5.
6. One PR; after merge: /grounding-wiki:wiki-update (testing-strategy re-pin), spec-bridge:sync, Done.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
2026-07-23: Spec Kit flow complete (specify → plan → tasks), spec linked via spec-bridge. Clarify skipped: board card detailed + re-verified, zero NEEDS CLARIFICATION markers survived spec validation. Implementation starting on Sonnet spec-implementer.

2026-07-23: Implementation complete on Sonnet spec-implementer (3 commits on task-74-metatron-persona-tests), gated by planning session: diff scope verified (2 test files + wiki note only, 602 insertions, zero production changes), fresh go test -race -count=1 green on both packages, full suite green, R1 anti-duplication review of the diff passed. 12 new tests: charge mirror accrual/cap, concurrent ErrTurnBusy, Observe backpressure, absorb mirrors, tailOfFile/soulTail/transcriptTail; persona 4-map sweep, anchor≡temperament, unreadable-load degrade, genesis charter/journal seeding, SecretEvents. No production defects surfaced. tasks.md T001-T015 ticked. PR #49 open and mergeable — merge blocked by session permissions, awaiting operator. AC #4 completes with post-merge wiki-update re-pin. Implementer flagged: quickstart/plan '~25s' full-suite baseline is stale — real baseline ~190s (e2e dominates); pre-existing, noted for future planning docs.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Behavioral coverage delivered via spec 023 (full Spec Kit, Sonnet spec-implementer, PR #49 merged as eddbce1). 12 new behavioral tests close the R1-verified gaps: metatron charge-mirror accrual/cap through Observe→run()→mirrorState, true concurrent ErrTurnBusy (two goroutines on the CAS), Observe backpressure, absorb-mirror refresh (alive map + 8-entry story tail), and the previously-untested tail windows (tailOfFile/soulTail 4000B/transcriptTail 6 turns); persona 4-map alignment sweep (Texts/Anchors/DriftMarkers/Secrets vs AgentNames), anchor≡temperament invariant, unreadable-load degrade, genesis charter/journal seeding + never-overwrite, SecretEvents. Tests-and-docs only (zero production changes); full suite green under -race; no production defects surfaced. Wiki testing-strategy note updated, re-pinned to eddbce1, freshness gate green (36 notes). Bonus: stale ~25s suite-baseline corrected to ~3min in wiki + spec docs. TASK-64 instruction surface confirmed already-covered by inventory — documented in research.md R1 rather than duplicated.
<!-- SECTION:FINAL_SUMMARY:END -->
