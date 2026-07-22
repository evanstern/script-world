---
name: world-save-directory
description: One directory = one world run — manifest (world.json), path helpers, create/open validation, clean separability, v1→v2→v3 migration
kind: component
sources:
  - internal/world/world.go
  - internal/world/migrate.go
verified_against: d25ca1fdd87b128f7cbb4a44e31694e5cc5bf8f6
---

# World save directory

`internal/world` defines the save-directory contract: one directory is one world run,
containing everything that run owns and nothing any other run touches. Copying a
stopped world's directory is a complete, restorable archive.

## How it works

`Manifest` (serialized as `world.json` at the dir root) carries `name`, `seed`
(uint64), `created_at` (RFC3339, metadata only — wall time never enters sim state),
`format_version` (currently **3** — spec 013's inventory/storage break bumped it
from 2, on top of spec 012's resources/food/crafting break that bumped it from 1;
a v1 or v2 manifest is refused by `Open` with a pointer to
`scriptworld migrate <world>` — [[world-migration]]), `tick_game_seconds` (fixed 1),
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

**Migration** (`migrate.go`, spec 012 US6 for v1→v2 + spec 013 for v2→v3 —
[[world-migration]] has the full design): `OpenForMigration(dir)` loads a world
manifest without the current version gate — it admits `format_version` 1 or 2 (the
sole purpose is migrating an older world this build otherwise can't `Open`) and
refuses an already-current world outright; `Migrate(dir)` runs the whole ceremony —
refuse a live daemon or an already-migrated source (the guard is keyed to the
*source* format: `V1DBPath()` → `world.v1.db` for a v1 source, `V2DBPath()` →
`world.v2.db` for a v2 source), read the source world's covering snapshot,
transform it (`internal/sim` — a v1 source chains both transforms in one run,
1→2→3), archive the live `world.db` (and any `-wal`/`-shm` sidecars) to that
source-format archive **before** writing anything new (the archive is never
overwritten and never deleted), write a fresh log (`world.created` then
`world.migrated`) plus its covering snapshot, then bump `Manifest.FormatVersion` to
the current version last — a crash between the archive and the manifest bump
leaves a recoverable state (restore = rename the archive back, reset the
manifest).

## Connections

[[daemon-lifecycle]] opens the world and cross-checks the manifest against store meta;
[[event-log]] and [[snapshots]] live inside `world.db`; [[ipc-server]] binds the socket
at `SockPath()`. [[cli-scriptworld]]'s `new` creates worlds and `migrate` upgrades
an older one ([[world-migration]]).

## Operational notes

Seed and format version are immutable after creation (except across a migration,
which bumps `format_version` in place). There is deliberately no global
registry of worlds — the directory is the identity, per the grounding decision "never
global; runs cleanly separable" ([[design-grounding]]). Archiving = stop the daemon,
`cp -R` the directory. A migrated world's directory additionally carries the
source-format archive (`world.v1.db` and/or `world.v2.db`, depending how far it
chained), the untouched original database(s) — deleting one is a deliberate,
irreversible acceptance of that step of the migration; `Migrate` never removes
either itself.
