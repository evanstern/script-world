# Quickstart: Validating the Metatron/Persona Coverage Feature

**Feature**: 023-metatron-persona-tests

## Prerequisites

- Go toolchain (repo standard); no network, no API keys — the suites are hermetic.

## Validation scenarios

### 1. The two package suites pass under the race detector

```sh
go test -race ./internal/metatron/ ./internal/persona/
```

Expected: PASS for both packages; every new test green alongside the existing 26+5.

### 2. The whole suite stays green

```sh
go test -race ./...
```

Expected: PASS across the repo (~25 s, e2e dominates) — proves the new tests are
hermetic and race-clean in the full run (spec SC-003).

### 3. Coverage gaps are actually closed (spot-check)

```sh
go test -race ./internal/metatron/ -run 'Tail|ChargeMirror|TurnBusyConcurrent|ObserveNonBlocking' -v
go test -race ./internal/persona/ -run 'Sweep|Anchor|Unreadable|Charter|SecretEvents' -v
```

Expected: the named new tests run and pass (exact names per tasks.md).

### 4. Behavioral, not change-detector (review gate)

Reviewer check: each new test asserts an observable contract from
[data-model.md](data-model.md) §1–2 and would survive a legitimate refactor that
preserves behavior (e.g. renaming internals, reordering struct fields).

### 5. No production drift

```sh
git diff --stat main -- ':!*_test.go' ':!docs/' ':!specs/' ':!backlog/'
```

Expected: empty — the PR touches only test files, the wiki note, spec artifacts,
and board state (spec FR-011).

### 6. Wiki freshness

```sh
grep -n "metatron" docs/wiki/testing-strategy.md
```

Expected: the note narrates the new metatron/persona behavioral suites, lists the
test files in `sources:`, and `verified_against:` is re-pinned to the merge commit
(run `/grounding-wiki:wiki-update` as the final step).
