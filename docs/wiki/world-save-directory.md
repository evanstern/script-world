---
name: world-save-directory
description: One directory = one world run — manifest (world.json), path helpers, create/open validation, clean separability
kind: component
sources:
  - internal/world/world.go
verified_against: 5f1c2894075ef128b627d38198bd2cd69876c5ac
---

# World save directory

`internal/world` defines the save-directory contract: one directory is one world run,
containing everything that run owns and nothing any other run touches. Copying a
stopped world's directory is a complete, restorable archive.

## How it works

`Manifest` (serialized as `world.json` at the dir root) carries `name`, `seed`
(uint64), `created_at` (RFC3339, metadata only — wall time never enters sim state),
`format_version` (currently 1), `tick_game_seconds` (fixed 1),
`map_width`/`map_height` (default 64×64; zero/absent values from older saves default
on `Open`), and an optional `meeting` block (TASK-36, `MeetingConfig`:
`convene`/`open` as "HH:MM" 24-hour game clock times, optional `x`/`y` meeting
place) — the per-world meeting convention the daemon seeds on boot
([[governance]], [[daemon-lifecycle]]); `scriptworld new` never writes it, so
emergent is the default. `World.Map()` regenerates the terrain from the seed and
dimensions — deterministic, so the map is never stored ([[worldmap-generation]]).

- `Create(dir, name, seed)` refuses any existing non-empty directory, creates
  `agents/` (empty — flat files for later features live there), and writes the
  manifest. The genesis `world.created` event is appended by the CLI `new` command,
  not here.
- `Open(dir)` reads and validates the manifest: unknown `format_version`, a
  `tick_game_seconds` other than 1, or a malformed `meeting` block (bad "HH:MM",
  or convene not strictly before open) is a hard error, so an old binary can
  never half-load a newer world.
- Path helpers centralize layout: `DBPath()` → `world.db`, `LLMConfigPath()` →
  `llm.json` (the [[llm-orchestrator]] config, written by `new`, deletable to
  disable inference), `CalibrationPath()` → `calibration.json` (the
  seconds-per-point profile written only by `scriptworld calibrate` —
  [[cognition]]; an absent file is legal, pessimistic bootstrap defaults apply),
  `SockPath()` → `daemon.sock`, `PidPath()` → `daemon.pid`,
  `LogPath()` → `daemon.log`, `CharterPath()` → `charter.md` (the player-editable
  prompt), `MetatronDir()` → `metatron/` (the angel's soul and transcript —
  [[metatron]]), and `VillageCharterPath()` → `village_charter.md` (the village's
  scribe-rendered law, deliberately distinct from Metatron's charter —
  [[governance]], TASK-13).

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
