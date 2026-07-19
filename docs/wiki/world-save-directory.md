---
name: world-save-directory
description: One directory = one world run — manifest (world.json), path helpers, create/open validation, clean separability
kind: component
sources:
  - internal/world/world.go
verified_against: 08d8c70e23c104a4c61df1749c00cb315f5c643d
---

# World save directory

`internal/world` defines the save-directory contract: one directory is one world run,
containing everything that run owns and nothing any other run touches. Copying a
stopped world's directory is a complete, restorable archive.

## How it works

`Manifest` (serialized as `world.json` at the dir root) carries `name`, `seed`
(uint64), `created_at` (RFC3339, metadata only — wall time never enters sim state),
`format_version` (currently 1), and `tick_game_seconds` (fixed 1).

- `Create(dir, name, seed)` refuses any existing non-empty directory, creates
  `agents/` (empty — flat files for later features live there), and writes the
  manifest. The genesis `world.created` event is appended by the CLI `new` command,
  not here.
- `Open(dir)` reads and validates the manifest: unknown `format_version` or a
  `tick_game_seconds` other than 1 is a hard error, so an old binary can never
  half-load a newer world.
- Path helpers centralize layout: `DBPath()` → `world.db`, `SockPath()` →
  `daemon.sock`, `PidPath()` → `daemon.pid`, `LogPath()` → `daemon.log`.

Runtime files (`daemon.sock`, `daemon.pid`) exist only while a daemon runs and are
swept by [[daemon-lifecycle]] when stale. The full layout is documented in
`specs/001-world-daemon/contracts/storage.md`.

## Connections

[[daemon-lifecycle]] opens the world and cross-checks the manifest against store meta;
[[event-log]] and [[snapshots]] live inside `world.db`; [[ipc-server]] binds the socket
at `SockPath()`. [[cli-scriptworld]]'s `new` creates worlds.

## Operational notes

Seed and format version are immutable after creation. There is deliberately no global
registry of worlds — the directory is the identity, per the grounding decision "never
global; runs cleanly separable" ([[design-grounding]]). Archiving = stop the daemon,
`cp -R` the directory.
