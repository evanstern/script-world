# Quickstart Validation: Extract the Intent-Landing Ladder into Named Rungs

**Feature**: 022-landing-ladder-rungs | **Date**: 2026-07-23

Prerequisites: Go toolchain per go.mod; run from the task worktree
(`.worktrees/task-70`). PATH note: `export PATH="/opt/homebrew/bin:$PATH"`.

## 1. Determinism gate (FR-005 / SC-002)

```sh
go test -race ./internal/sim/ -run 'TestDeterminism|TestReplay' -v
```

**Expected**: every determinism/replay test passes; `git diff --stat main` shows NO
existing `*_test.go` or fixture modified — only `internal/sim/loop.go`,
`internal/sim/landing.go`, `internal/sim/landing_test.go` (plus spec/board artifacts).

## 2. Full race-checked suite (FR-007 / SC-004)

```sh
go test -race ./...
```

**Expected**: all packages pass.

## 3. Rung isolation tests (FR-006 / SC-003)

```sh
go test ./internal/sim/ -run 'TestLanding' -v
```

**Expected**: named tests covering each rung — unavailable, superseded, stale,
guard-failed, hail-relaxed, adapted, fresh — plus the three hail special-cases
(mutual-hailer, in-radius, moved-target), all passing without a command round-trip.

## 4. Shape check (FR-001..003 / SC-001)

```sh
grep -n "adapted\s*:=\|failed\s*:=\|hailTarget\s*:=" internal/sim/loop.go
```

**Expected**: no matches in loop.go's inject_intent case (the flags are gone); the case
body dispatches to `landIntent`. `internal/sim/landing.go` contains the doctrine-named
rungs (research.md D4 table).

## 5. Wiki re-pin (FR-009, after merge)

Run `/grounding-wiki:wiki-update`; **expected**: `docs/wiki/sim-loop.md` re-verified and
re-pinned to the merge commit (landing.go added to its sources).
