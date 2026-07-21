# Implementation Plan: World Instance Manager

**Branch**: `task-43-instance-manager` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/008-instance-manager/spec.md`

## Summary

Give scriptworld docker/ollama-style instance management without breaking the "one
directory = one world" grounding: a machine-wide `scriptworld ps` that re-proves
liveness from pidfile + a bounded daemon `status` round-trip (never from records), a
default worlds home at `~/.scriptworld/worlds` that `new <name>` creates into, and
name-or-path acceptance on every per-world command via one shared resolution helper.
All manager state (a `known_worlds.json` pointer cache) is advisory and self-healing;
worlds stay fully self-contained. Approach: a new `internal/worlds` package (home,
registry, resolution, parallel probing) consumed by `cmd/scriptworld`, plus a one-line
registration hook in `daemon.Run`.

## Technical Context

**Language/Version**: Go 1.26.4 (single module `github.com/evanstern/script-world`)

**Primary Dependencies**: stdlib only for this feature (`flag` CLI, `net` unix sockets,
`encoding/json`); existing internal packages `internal/world` (save-dir layout),
`internal/daemon` (pidfile/liveness), `internal/ipc` (client + `StatusData`),
`internal/store` (offline last-known state), `internal/llm` (`Status` presence = LLM on)

**Storage**: Files — per-world dirs with `world.json` manifest (unchanged); new advisory
`$SCRIPTWORLD_HOME/known_worlds.json` (atomic write, tolerated-corrupt); worlds home dir
scan. No schema changes to the SQLite store.

**Testing**: `go test ./...` unit tests beside code (`internal/worlds`,
`cmd/scriptworld`); black-box e2e in `e2e/` (pattern: `daemon_e2e_test.go` builds the
binary and drives real daemons)

**Target Platform**: darwin + linux (unix domain sockets, `syscall.Kill` liveness —
same portability envelope as today)

**Project Type**: single Go module, one CLI binary (`cmd/scriptworld`)

**Performance Goals**: `ps` completes machine-wide in < 2s with zero false "running"
(SC-001) — parallel per-world probes, ~1s per-world dial+call budget

**Constraints**: worlds remain self-contained copyable directories (SC-004); all
path-based invocations byte-compatible (SC-003); manager state never required for a
world to run (FR-008); `SCRIPTWORLD_HOME` env override honored everywhere (edge case)

**Scale/Scope**: single user, single machine, O(tens) of worlds; 13 CLI commands gain
name resolution; 1 new package + 1 new subcommand + `new` argument change

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*
*Constitution v1.1.0.*

- **I. Artifact-Grounded Action — PASS.** Spec 008 + requirements checklist committed
  (`112deea`); this plan, research.md, data-model.md, contracts/, quickstart.md are the
  new artifacts; TASK-43 is the board anchor (linked via spec-bridge). Decisions D1–D7
  in research.md derive from the grounded wiki decision ("never global, cleanly
  separable") rather than re-asking it.
- **II. One Task, One PR — PASS.** TASK-43 → one branch `task-43-instance-manager` in
  `.worktrees/task-43`, one PR. Spec phases are internal breakdown only.
- **III. Gates Over Assertions — PASS.** Board status moves only via `spec-bridge:sync`
  from spec artifacts; ACs tick only against merged, tested behavior.
- **IV. Grounding Freshness — PASS (obligation registered).** This feature edits files
  pinned by wiki notes (`cmd/scriptworld/*`, `internal/daemon/daemon.go`,
  `internal/world/world.go` sources of `world-save-directory`, `daemon-lifecycle`,
  `design-grounding` notes at minimum) — `/grounding-wiki:wiki-update` is a required
  post-merge step before the task is Done.
- **V. Model-Tiered Workflow — PASS.** This plan and the coming tasks.md are produced on
  the planning tier; implementation is delegated to the `spec-implementer` agent.
  Recommended tier: **Sonnet** (default) — the work is CLI/plumbing plus a new leaf
  package; the only concurrency is a bounded fan-out probe with no shared mutable state,
  well below the governor/scheduler bar the rubric reserves for Opus. Escalation stays
  available one-way if a slice fails gates; tier choice + justification will be recorded
  on TASK-43 at implementation start.

**Post-Phase-1 re-check: PASS** — design adds exactly one new package (`internal/worlds`)
and no new dependencies, patterns, or stores beyond the advisory JSON file the spec
itself mandates. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/008-instance-manager/
├── plan.md              # This file
├── research.md          # Phase 0 output — decisions D1–D7
├── data-model.md        # Phase 1 output — entities & states
├── quickstart.md        # Phase 1 output — validation walkthrough
├── contracts/
│   └── cli.md           # Phase 1 output — CLI surface contract (ps, new, resolution)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
cmd/scriptworld/
├── main.go              # + "ps" dispatch; usage text: names-or-paths, new <name>
├── commands.go          # per-world commands resolve args via internal/worlds
├── ps.go                # NEW — cmdPs: discovery + parallel probe + table/JSON render
├── ps_test.go           # NEW — output shaping, state classification
└── commands_test.go     # NEW/extended — new-arg semantics, resolution plumbing

internal/worlds/         # NEW package — the manager (client-side only)
├── home.go              # SCRIPTWORLD_HOME root, worlds-home path, name validation
├── registry.go          # known_worlds.json: load-tolerant, atomic upsert/prune
├── resolve.go           # name-vs-path rule, resolution order, ambiguity/missing errors
├── discover.go          # candidate enumeration: home scan ∪ registry
├── probe.go             # bounded parallel liveness+status probes; offline last-known
└── *_test.go            # unit tests for all of the above

internal/daemon/
└── daemon.go            # Run(): upsert registry entry on boot (advisory, error-tolerant)

internal/world/
└── world.go             # unchanged layout; (possibly) small helper reuse only

e2e/
└── manager_e2e_test.go  # NEW — US1/US2/US3 end-to-end: ps, new-by-name, stale pidfile
```

**Structure Decision**: single-project layout (existing). One new leaf package
`internal/worlds` holds every manager concern so `cmd/scriptworld` stays thin and the
daemon's only new duty is a best-effort registry upsert. No changes to the IPC protocol,
store schema, or world directory layout.

## Complexity Tracking

> No Constitution Check violations — table intentionally empty.
