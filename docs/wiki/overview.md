---
name: overview
description: System shape of promptworld — an always-on simulation daemon with attachable terminal clients, event-sourced per-world state
kind: concept
sources:
  - README.md
  - cmd/promptworld/main.go
  - go.mod
verified_against: 8be4440aae8d108884080cb6476782d2f11ad165
---

# Overview

promptworld is an ambient, always-on village simulation: a Go daemon advances the world
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

One Go module (`github.com/evanstern/promptworld`, Go 1.26; external deps: pure-Go
SQLite, Bubble Tea/Lipgloss for the TUI, and the official Anthropic Go SDK for the
cloud inference tier) builds one binary, `cmd/promptworld`, which is both the daemon
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
cmd/promptworld → daemon → ipc → sim → store
                → tui   ↗       clock ┘   world
```

`internal/cognition` is a further stdlib-only leaf below all of these: the sim loop,
the minds, the LLM layer, and the daemon import it (decision-class registry,
deterministic router, latency calibration); it imports none of them. The
`promptworld calibrate` subcommand benchmarks the host+model to a seconds-per-point
profile for that layer. `internal/worlds` ([[instance-manager]]) sits beside the
client tools — imported by `cmd/promptworld` and (for boot registration) by
`internal/daemon` — providing the worlds home, name resolution, and the `ps` probe.

Each world run is one save directory and at most one daemon process; multiple worlds
mean multiple daemons. The world directory is the sole source of truth; the only
machine-level state is the instance manager's ([[instance-manager]], TASK-43) —
a default worlds home (`~/.promptworld/worlds`, where `new <name>` creates) and an
advisory known-worlds pointer cache — both strictly optional: every command still
takes a plain path, and a world runs and copies with no manager state present.
`promptworld ps` enumerates every running world machine-wide from live evidence.
The save format has broken twice so far: spec 012 (resources/food/crafting) bumped
`format_version` to 2, and spec 013 (inventory & storage) bumped it again to 3;
`promptworld migrate <world>` ([[world-migration]]) is the door a stopped older
(v1 or v2) world walks through to keep running under a newer binary, chaining
both steps in one run for a v1 source.

## Connections

[[design-grounding]] records why the system has this shape. [[sim-loop]] is the heart;
the [[llm-orchestrator]] is the (strictly quarantined) voice of the models, and
[[cognition]] decides deterministically when that voice may speak at all;
[[event-log]] and [[snapshots]] its memory; [[ipc-server]], [[tui-client]], and
[[cli-promptworld]] its face. [[daemon-lifecycle]] ties them into a process;
[[instance-manager]] keeps many such processes legible (`ps`, names, worlds home);
[[world-migration]] carries a world across a format break.

## Operational notes

Local, single-operator posture: daemon and clients share a trusted host; the protocol
has no authentication. Target platform is darwin/arm64 (homelab MacBook); nothing is
platform-specific beyond Unix sockets. The full village stack — executor, minds, social
fabric, chronicle, Metatron, governance — now runs on the substrate (see [[executor]],
[[agent-mind]]).
