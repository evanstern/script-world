---
name: overview
description: System shape of script-world — an always-on simulation daemon with attachable terminal clients, event-sourced per-world state
kind: concept
sources:
  - README.md
  - cmd/scriptworld/main.go
  - go.mod
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
---

# Overview

script-world is an ambient, always-on village simulation: a Go daemon advances the world
24/7 whether or not anyone is watching, and terminal clients attach and detach without
affecting it. The current codebase is the **time substrate** (TASK-2 / spec
`specs/001-world-daemon`): tick loop, clock, persistence, and client protocol. The
village systems — agent minds, the social fabric, the chronicle, Metatron, and
village self-governance (norms and votes, [[governance]]) — arrived in later
tasks and plug into this substrate. Because model turns take real wall time while
game time keeps flowing, the **cognition horizon** ([[cognition]], TASK-32 / spec
`specs/007-cognition-horizon`) deterministically gates every model-reaching
decision by how stale its answer will be when it lands.

## How it works

One Go module (`github.com/evanstern/script-world`, Go 1.22+; external deps: pure-Go
SQLite, Bubble Tea/Lipgloss for the TUI, and the official Anthropic Go SDK for the
cloud inference tier) builds one binary, `cmd/scriptworld`, which is both the daemon
and every client tool. Data planes:

- **Simulation plane**: a single goroutine in `internal/sim` owns all world state and
  advances it in deterministic ticks (1 tick = 1 game second). All external input enters
  as commands applied at tick boundaries and recorded as events.
- **Persistence plane**: `internal/store` writes every event to an append-only SQLite
  log in the world's save directory; snapshots bound recovery time. The log is the
  source of truth; state is a reducer over it.
- **Interface plane**: `internal/ipc` serves a JSON-lines protocol over a Unix domain
  socket inside the save directory; `internal/tui` is the Bubble Tea full-screen
  client over that protocol; `internal/daemon` wires the planes together and owns
  process lifecycle.

Layering (imports point downward only):

```
cmd/scriptworld → daemon → ipc → sim → store
                → tui   ↗       clock ┘   world
```

`internal/cognition` is a further stdlib-only leaf below all of these: the sim loop,
the minds, the LLM layer, and the daemon import it (decision-class registry,
deterministic router, latency calibration); it imports none of them. The
`scriptworld calibrate` subcommand benchmarks the host+model to a seconds-per-point
profile for that layer.

Each world run is one save directory and at most one daemon process; multiple worlds
mean multiple daemons. There is no global state anywhere.

## Connections

[[design-grounding]] records why the system has this shape. [[sim-loop]] is the heart;
the [[llm-orchestrator]] is the (strictly quarantined) voice of the models, and
[[cognition]] decides deterministically when that voice may speak at all;
[[event-log]] and [[snapshots]] its memory; [[ipc-server]], [[tui-client]], and
[[cli-scriptworld]] its face. [[daemon-lifecycle]] ties them into a process.

## Operational notes

Local, single-operator posture: daemon and clients share a trusted host; the protocol
has no authentication. Target platform is darwin/arm64 (homelab MacBook); nothing is
platform-specific beyond Unix sockets. Placeholder simulation events flow until real
village systems (TASK-4+) replace them.
