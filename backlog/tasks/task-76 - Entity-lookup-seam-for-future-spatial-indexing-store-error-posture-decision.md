---
id: TASK-76
title: >-
  Entity-lookup seam for future spatial indexing (+ store-error posture
  decision)
status: To Do
assignee: []
created_date: '2026-07-23 06:35'
labels:
  - review-2026-07-22
  - code-quality
dependencies: []
priority: low
ordinal: 69000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
From the 2026-07-22 team review (improvement 6 — latent scaling walls). Not urgent at 8 villagers; cheap to seam now, expensive to retrofit later.

(a) Entity-lookup seam: pileAt/chestAt/structureAt are O(n) slice scans (internal/sim/state.go:220-227) called from stepEvents inside per-agent and per-structure loops (7 call sites in executor.go), plus the rot sweep is O(piles x foodKinds x batches) every 60 ticks. Scope is the SEAM, not the index: route all positional lookups through one accessor type so a grid/spatial index can drop in behind it later without touching call sites. Must be determinism-neutral (accessor returns identical results incl. tie-break ordering; harness proves bit-identical replay).

(b) Store-error posture: a transient store write error is fatal to the daemon (loop.go ~352) — defensible doctrine ("an unwritable log must never silently diverge from state") but harsh for an always-on process, and there is no retry seam. Deliverable here is a recorded DECISION (wiki operational note or docs/design): keep fatal-by-doctrine, or add a small bounded-retry-then-fatal. Implement only if the decision says yes; otherwise document why fatal stands.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 All positional entity lookups routed through one accessor seam; zero raw slice scans at former call sites
- [ ] #2 Determinism harness proves bit-identical replay across the seam refactor
- [ ] #3 Store-error posture decision recorded durably; bounded retry implemented only if chosen
- [ ] #4 go test -race ./... passes; affected wiki notes re-pinned
<!-- AC:END -->
