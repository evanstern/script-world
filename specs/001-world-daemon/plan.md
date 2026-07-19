# Implementation Plan: World Daemon & Time Substrate

**Branch**: `001-world-daemon` | **Date**: 2026-07-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-world-daemon/spec.md`

## Summary

Build the always-on substrate every later script-world feature stands on: a Go daemon
that advances a deterministic fixed-timestep simulation loop, keeps a game clock with
runtime-adjustable compression (default 1 game-min = 15 real-sec), treats pause as a
first-class command, records every simulation event in an append-only SQLite log with
periodic snapshots, confines each world run to its own save directory, and exposes a
Unix-domain-socket JSON-lines protocol so clients can attach/detach, observe events, and
drive time controls without ever interrupting the world. A placeholder simulation
(seeded wanderer entities + day/night boundary events) exercises the whole substrate
end-to-end until real systems (TASK-4..12) plug in.

## Technical Context

**Language/Version**: Go 1.22+ (`math/rand/v2` for seedable PCG; single static binary)

**Primary Dependencies**: standard library first; `modernc.org/sqlite` (pure-Go SQLite,
no cgo) as the only storage dependency. No framework for the IPC layer — `net` Unix
sockets + `encoding/json`.

**Storage**: SQLite database `world.db` per save directory — append-only `events` table,
`snapshots` table, `meta` key/value table (clock state, seed, format version). Flat
files in the same directory belong to later features; this feature creates the layout.

**Testing**: `go test` — unit tests (clock math, determinism, reducer), integration
tests that build the binary, run a real daemon against a temp save dir, attach over the
socket, kill -9 it, and verify lossless resume.

**Target Platform**: macOS homelab (darwin/arm64) primary; nothing platform-specific
beyond Unix domain sockets, so linux/amd64 works for CI.

**Project Type**: single CLI binary `scriptworld` with subcommands (daemon + client +
world management) — one Go module at repo root.

**Performance Goals**: placeholder sim sustains ≥ 1,000 ticks/sec at uncapped speed on
the target machine; attach-to-status round trip < 2 s (SC-002); restart recovery < 10 s
for a one-game-day run (SC-003).

**Constraints**: deterministic replay (same seed + same command timeline → byte-identical
event history, SC-006); append-only event log (no UPDATE/DELETE); daemon must survive
abrupt client death; graceful auto-slow instead of dropped ticks when overrunning.

**Scale/Scope**: one daemon process = one world run. Event volume at v1 scale (8 agents,
later features) is trivially within SQLite range (< 1M events per 30-day run).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` is an unfilled template — no ratified principles
exist yet. **No constitutional gates to evaluate; gate passes vacuously.** (Recorded so
a future constitution knows this plan predates it.)

## Project Structure

### Documentation (this feature)

```text
specs/001-world-daemon/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output — decisions + rationale
├── data-model.md        # Phase 1 output — entities, schema, state machine
├── quickstart.md        # Phase 1 output — end-to-end validation guide
├── contracts/
│   ├── cli.md           # scriptworld subcommand contract
│   ├── client-protocol.md  # JSON-lines socket protocol
│   └── storage.md       # SQLite DDL + save-directory layout
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
go.mod                       # module github.com/evanstern/script-world
cmd/
└── scriptworld/
    └── main.go              # CLI dispatch: new|start|stop|status|attach|daemon|pause|resume|speed
internal/
├── world/                   # save-directory layout: manifest, paths, create/open/validate
│   ├── world.go
│   └── world_test.go
├── clock/                   # game clock: tick↔game-time math, speed, pause state
│   ├── clock.go
│   └── clock_test.go
├── sim/                     # deterministic loop: fixed timestep, seeded RNG, reducer,
│   │                        #   placeholder systems (wanderers, day/night)
│   ├── loop.go
│   ├── state.go             # world state + event reducer (replayable)
│   ├── placeholder.go
│   └── sim_test.go          # determinism harness (SC-006)
├── store/                   # SQLite: events append, snapshots, meta, replay iterator
│   ├── store.go
│   ├── schema.go
│   └── store_test.go
├── ipc/                     # protocol types + server (daemon side) + client (attach side)
│   ├── protocol.go
│   ├── server.go
│   ├── client.go
│   └── ipc_test.go
└── daemon/                  # lifecycle: run loop wiring, pidfile, logfile, detach start,
    │                        #   signal handling, graceful + crash recovery paths
    ├── daemon.go
    └── daemon_test.go
e2e/
└── daemon_e2e_test.go       # builds binary; full attach/kill/resume scenarios (SC-001..005)
```

**Structure Decision**: single Go module at repo root, one binary under `cmd/scriptworld`,
all logic in `internal/` packages layered store→clock→sim→ipc→daemon so later features
(agents, map, LLM orchestrator) mount as new `internal/` packages feeding events into the
same loop. End-to-end scenarios live in `e2e/` because they exec the built binary rather
than import packages.

## Complexity Tracking

No constitution violations to justify (no constitution). One deliberate scope guard:
the placeholder simulation stays under ~150 lines — it exists only to push real events
through the substrate, not to preview TASK-5 gameplay.
