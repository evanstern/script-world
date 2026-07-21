---
name: instance-manager
description: World instance management — machine-wide ps with live-proven state, the default worlds home, name-or-path addressing, the advisory known-worlds registry
kind: component
sources:
  - internal/worlds/home.go
  - internal/worlds/registry.go
  - internal/worlds/resolve.go
  - internal/worlds/discover.go
  - internal/worlds/probe.go
  - cmd/scriptworld/ps.go
verified_against: 1434b65a74598495e01b2a8f5c0bbe8d1ad9722b
---

# Instance manager

`internal/worlds` is the client-side manager over many worlds (spec
`specs/008-instance-manager`, TASK-43): it gives `scriptworld` docker/ollama ergonomics
— `ps` across the machine, creation by bare name, name-or-path addressing on every
per-world command — without weakening the grounded "one directory = one world, never
global" decision ([[design-grounding]], [[world-save-directory]]). Everything it keeps
outside a world directory is advisory and self-healing; a world runs with none of it
present.

## How it works

**Home** (`home.go`): the scriptworld root is `~/.scriptworld`, overridden by
`SCRIPTWORLD_HOME`; derived paths are the worlds home `<root>/worlds` (where
`new <name>` creates) and the registry `<root>/known_worlds.json`. `ValidateName`
enforces creation-time name rules: non-empty, no `/`, no leading `-` or `.`.

**Registry** (`registry.go`): `known_worlds.json` is a `{"worlds":{name:{path}}}`
pointer cache for worlds living *outside* the worlds home (the home itself is
scan-owned). Reads tolerate a missing or corrupt file (⇒ empty, never an error); writes
are atomic (temp + rename) and prune entries whose directory no longer holds a readable
`world.json` or whose path is inside the current home. It is upserted from two places:
`daemon.Run` at boot ([[daemon-lifecycle]]) and `new --at`. Never authoritative for
liveness or content.

**Resolution** (`resolve.go`): an argument is a **path** iff it contains `/` or starts
with `.` or `~` — paths bypass the manager entirely and keep their historical behavior.
A bare **name** resolves worlds-home-first, then registry; a home/registry collision is
refused as ambiguous (both paths printed), an unknown name fails naming the home
searched and suggesting `ps --all`, and a registry entry whose directory vanished gets
a dedicated "last known at …, but that directory is gone" error (`ErrMissing`).

**Discovery + probe** (`discover.go`, `probe.go`): `ps` candidates are the home scan ∪
registry entries. Each candidate is classified concurrently under a ~1s per-world
budget (`probeBudget`): a live pidfile pid (same `kill(pid, 0)` check as the daemon's,
duplicated locally to avoid a daemon↔worlds import cycle) is only a pre-filter — a
world is `running`/`paused` solely when its daemon answers a `status` round trip within
budget; a live pid that doesn't answer is `unresponsive`, never running. Dead-pid
candidates become `stopped` (last-known clock via `OfflineSnapshot`, the extracted
offline read `status` also uses), `unreadable` (corrupt manifest), or `missing`
(registry path gone). `cmd/scriptworld/ps.go` renders the table
(`NAME STATE PID TICK GAME TIME SPEED LLM PATH`) and the `--json` array reusing the
`status --json` vocabulary; default shows live-pid states, `--all` adds the rest;
inference on/off comes from `StatusData.LLM != nil` live, `llm.json` presence stopped.

## Connections

[[cli-scriptworld]] threads every per-world command through `resolveWorld` and hosts
`ps`/`new`; [[daemon-lifecycle]] performs the boot-time registry upsert; probing rides
[[ipc-client]] `status` and [[world-save-directory]]'s pidfile/socket layout; offline
rows read [[snapshots]] + [[event-log]] via `OfflineSnapshot`.

## Operational notes

`SCRIPTWORLD_HOME` moves home + registry together; all discovery and resolution honor
it. Registry write failures inside the daemon are logged and never block boot. `ps`
whole-listing time stays under ~2s regardless of wedged daemons (parallel probes). A
copied world directory needs no manager state anywhere — starting it (re)registers it.
