---
id: TASK-74
title: Behavioral test coverage for metatron and persona packages
status: To Do
assignee: []
created_date: '2026-07-23 06:35'
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

Post-TASK-64 note: cover the new instruction-surface seams too (skill-file composition order, capability-manifest gating) if TASK-64 left them thin.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every metatron source file has behavioral coverage of its exported seams; charge economy and fixed-frame composition tested explicitly
- [ ] #2 Persona genesis-once, file modes, and cross-map alignment covered incl. a sweep test over the index-aligned maps
- [ ] #3 All new tests run under -race with no network (scripted stubs only)
- [ ] #4 Wiki testing-strategy note updated with the new coverage
<!-- AC:END -->
