# Research: Extract the Intent-Landing Ladder into Named Rungs

**Feature**: 022-landing-ladder-rungs | **Date**: 2026-07-23

No NEEDS CLARIFICATION markers existed in the Technical Context; research resolves the
extraction-shape decisions against the actual code (loop.go@71d876a, lines 437-632).

## D1 — Extraction shape: method on Loop + unexported rung funcs (not a new type)

**Decision**: extract the case body into `(*Loop).landIntent(...) error` in a new
`internal/sim/landing.go`; rungs are unexported package-level functions taking
`(*State, ...)` explicitly.

**Rationale**: the ladder reads `l.state`, `l.m`, and the loop's tick but mutates nothing
on `Loop` itself — a method keeps the call site trivial (`err = l.landIntent(cmd.inject,
emit)`) while rungs stay pure functions of state, which is exactly what makes them
unit-testable without a Loop or a command round-trip.

**Alternatives considered**: a dedicated `lander` struct type (over-engineering: no state
to carry between calls); exporting rungs for testing (unnecessary — tests are package-local
per existing sim convention).

## D2 — What stays in handleCommand: nothing but the dispatch

**Decision**: the whole case body moves, including the agent-index bounds check and the
staleness clamp; the switch case becomes `err = l.landIntent(cmd.inject, emit)`.

**Rationale**: splitting the bounds check from the ladder would create two homes for
"can this land at all"; the error (`"no such agent %d"`) emits nothing today and keeps
that exact behavior inside landIntent's first lines.

**Alternatives considered**: keeping validation in the case (rejected: leaves a residue of
the old shape and splits the testable surface).

## D3 — Flag replacement: one explicit decision value

**Decision**: a `landingDecision` (or equivalently named) value produced by the guard walk:
rejection outcome + reason, or accept with `adapted bool` and `hailTarget int` (−1 = none).
The `failed` flag disappears (rejection returns immediately from the walk); `adapted` and
`hailTarget` become fields written by named rungs and read in exactly one place each.

**Rationale**: this is the review's ask verbatim — "the flag soup replaced by explicit rung
outcomes". Behavior nuances that MUST survive, verified against the code:

- `adapted` can be set by three distinct rungs (mutual-hailer relax, hailable relax,
  moved-target adapt) — any one suffices; it only changes the final `cog.outcome` value
  (`landed` → `adapted`), never the landing events themselves (loop.go:621-631).
- `hailTarget` can be written by the out-of-radius relaxation (loop.go:513) and
  overwritten by the in-radius marking on a later guard (loop.go:534) — last write wins;
  preserved by keeping the walk order identical.
- `social.hailed` emits only on the goal path (inside the `else` at loop.go:570-620),
  never on the plan path — the decision value is consumed by the goal path only.
- A rejection both emits its records AND sets err (loop.go:452-471); the walk's rejection
  short-circuits the remaining guards exactly like today's `failed=true; break`.

**Alternatives considered**: error-typed outcomes (rejected: rejections are not Go errors
here — they emit metered events and carry doctrine outcome strings); per-rung interface
(over-abstraction for six rungs).

## D4 — Rung inventory pinned to doctrine names

**Decision**: the named units and their doctrine mapping:

| Doctrine word | Code unit | Source lines today |
|---|---|---|
| unavailable | `rungUnavailable` (dead, then asleep) | 472-479 |
| superseded | `rungSuperseded` (generation) | 483-486 |
| stale | `rungStale` (class budget) | 487-490 |
| guard-failed | `rungGuardFailed` (Eval reason) | 517-519 |
| hail-relaxed | `rungHailRelaxed` (mutual-hailer D6, then hailable) | 502-516 |
| adapted | `rungAdapted` (moved-target) + relax rungs setting adapted | 523-528 |
| fresh | fall-through (`OutcomeLanded`) | 622 |
| (in-radius hail) | in-radius marking inside the guard walk | 530-535 |

**Rationale**: matches the task/spec vocabulary; the in-radius hail is a marking (not an
outcome) and is named as such rather than forced into an outcome rung.

**Alternatives considered**: none material.

## D5 — No contracts/ directory

**Decision**: skip Phase 1 `contracts/`.

**Rationale**: the feature's hard constraint is that no external interface changes — event
schema, IPC command surface, and doctrine vocabulary are all frozen (FR-008). The
behavioral contract is already executable: the determinism/replay byte-identity suite.
Writing prose contracts would duplicate FR-004/FR-005 without adding a checkable artifact.

## D6 — Determinism gate procedure

**Decision**: the gate is: `go test -race ./...` green on the branch with zero edits to
existing test files or fixtures (verified by the diff touching only `loop.go`,
`landing.go`, `landing_test.go`). The determinism proof rides the existing suite —
same-seed timeline (`TestDeterminismSameSeedSameTimeline`), replay byte-identity
(`TestReplayByteIdentity*`, `TestReplayDeterminismWithHails`,
`TestReplayDeterminismWithLastGoal`, `TestReplayDeterminismWithQuarryAndWater`, …) —
which already compares full event timelines and marshalled state bytes across seeds.

**Rationale**: the spec's Assumption 1 operationalizes "bit-identical" this way; the suite
exercises the ladder end-to-end through real command round-trips (e.g. hail_test.go drives
inject_intent landings at lines 171, 207, 360-414).

**Alternatives considered**: a bespoke before/after golden-hash harness (rejected: the
replay tests already assert byte identity; a one-off harness would be discarded evidence).

## D7 — Unit-test seam for rungs

**Decision**: `landing_test.go` builds minimal `State` values (existing test-builder
conventions in hail_test.go / cognition_test.go) and calls rung functions and
`landIntent` directly with a capturing `emit`, asserting: decision outcomes per rung,
event sequences for accept/reject paths, and the three hail special-cases (mutual-hailer
→ adapted no-hail; in-radius → fresh + hailed; moved-target → adapted).

**Rationale**: rungs as pure `(*State, ...)` functions need no Loop, no store, no
goroutines — the isolation the review said the current shape makes impossible.

**Alternatives considered**: testing only through handleCommand (rejected: that is the
status quo the task exists to fix).
