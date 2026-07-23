---
id: TASK-75
title: 'Doctrine and docs: determinism scope note + reducer-constants replay hazard'
status: To Do
assignee: []
created_date: '2026-07-23 06:35'
labels:
  - review-2026-07-22
  - code-quality
  - docs
dependencies: []
priority: low
ordinal: 68000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (improvements 5 and the replay-hazard removal note). Docs/doctrine PR, minimal code:

(a) Determinism scope: determinism is PER-LOG, not per-seed across machines — EffectiveRate is wall-clock-measured (loop.go ~658), lands in clock.degraded events, and is baked into the canonical state hash. Replay of a given log is exact; two machines on the same seed diverge. Nothing documents this limit; someone will eventually try to build a cross-machine determinism check on the stronger claim. State it explicitly in docs/wiki (deterministic-rng and/or sim-loop, plus the README determinism paragraph).

(b) Reducer-constants replay hazard: the pattern of the reducer re-deriving outcomes from mutable gameplay constants during replay (e.g. hunt spear yield, state.go ~504-511) means changing a constant silently breaks old-log replay unless format_version is bumped. Record it as doctrine: emitter-computes / payload-carries-the-outcome is the default; reducer-re-derives is the exception requiring an explicit format_version note. Add the doctrine to the wiki (event-log or sim-state-reducer note) and a code comment at the existing sites. Optionally: audit the reducer for other instances and list them in the note (audit only — migrating them is future work, not this task).
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Per-log vs per-seed determinism limit documented in wiki and README
- [ ] #2 Reducer-constants hazard recorded as doctrine with the emitter-computes default named; existing sites commented
- [ ] #3 Audit list of reducer-re-derives sites included in the wiki note
- [ ] #4 Wiki freshness gate passes (notes re-pinned)
<!-- AC:END -->
