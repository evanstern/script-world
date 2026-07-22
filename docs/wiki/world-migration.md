---
name: world-migration
description: The v1→v2 snapshot-cut migration (spec 012 US6) — carries a stopped world's people across a save-format break while the map resets, as a single world.migrated event
kind: component
sources:
  - internal/sim/migrate.go
  - internal/world/migrate.go
verified_against: 1d1cc6ff8cad2414108f7e768f61eb0faaea3088
---

# World migration

Spec 012 (resources/food/crafting v2) is script-world's first save-format break:
`Inventory` widened from the legacy `{wood, food}` pair to the full v2 resource set,
and terrain generation gained rock outcrops — a v1 world's bytes simply don't mean
the same thing under a v2 build, so `internal/world.Open` refuses any manifest whose
`format_version` isn't the current one ([[world-save-directory]]). `scriptworld
migrate <world>` ([[cli-scriptworld]]) is the one-time, offline door a stopped v1
world walks through to keep running. The design pin (research R10): **"keep the
people, reset the land"** — every villager and the whole social/governance fabric
carry over verbatim; the map and everything bound to it is reborn under v2 rules.

## How it works

The work splits across two packages by concern:

- **`internal/world/migrate.go`** is the ceremony: resolve and validate the source
  world, refuse unsafe conditions, archive the original database, write the fresh
  log, bump the manifest. It never touches sim semantics directly.
- **`internal/sim/migrate.go`** is the pure transform: decode a v1 state shape,
  produce a v2 `sim.State`. It never runs on the live reducer path.

**Client-side and offline**: `Migrate(dir)` refuses a running daemon (a pidfile
liveness check duplicated from `internal/daemon` rather than imported, to avoid an
import cycle) and refuses if `world.v1.db` already exists (the already-migrated
guard — the archive is never overwritten). It never replays v1 events under v2
rules; it reads only the v1 world's **covering snapshot** — the clean-shutdown
guarantee (`CheckContiguity` + `LatestValidSnapshot`). A real v1 daemon appends a
`daemon.stopped` bookkeeping event after its shutdown snapshot, so a one-event tail
of pure `daemon.*` events past the snapshot is tolerated (they carry zero sim state
— nothing to lose); any sim-affecting event past the snapshot means an unclean stop
left un-snapshotted history, and migration refuses with a remedy: start and stop the
v1 world once cleanly, then retry.

**The transform** (`sim.TransformV1Snapshot` / `MigrateState`) decodes the v1
covering-snapshot JSON through a typed `legacyState`/`legacyAgent`/`legacyInventory`
shape — decoding straight into the v2 `Inventory` would silently drop `food` (the
one field where v1 and v2 diverge incompatibly; every other agent field either is
unchanged or is v2-added, so it decodes faithfully as its zero value from absent v1
JSON). The transform then:

- **Carries verbatim** (tick continuity intact): clock (`Tick`/`Paused`/`Speed`/
  `Night` — the migration tick *is* the carried v1 tick, so the clock simply
  continues), the whole social fabric (relations, debts, rumors + id counters), the
  conversation ring, the chronicle ring, Metatron's charge bank, and governance
  (norms + their id counters — the village's lived law); per-agent people-state
  (needs, memories, beliefs, narrative, generation, consolidation marks, talk/give
  cooldowns, known rumors, `NearDeath`, `Dead` — a villager who died in the old
  world stays dead, not resurrected by the break).
- **Resets** (map-/session-bound, nil/zero): `Structures`, `Cleared`, `Harvested`,
  `DenUses`, `Quarried`, `Gru`, `MeetingConvention`/`MeetingPlace`/`Meeting` (the
  in-flight session — re-seeded from `world.json` on next boot, or re-emerges), and
  per-agent `Intent`/`Plan`/`Hail`/`Asleep` (everyone wakes standing, freshly idle
  at the migration tick via `IdleSince`).
- **Re-places** every carried soul via `genesisPlacement` — the same deterministic
  placement `NewState` uses for a fresh genesis of that seed
  ([[sim-state-reducer]], [[deterministic-rng]]) — so migrated villagers land on
  passable v2 tiles (rock outcrops included) exactly where a brand-new world of the
  same seed would put them.
- **Re-expresses inventory**: `Wood` carries 1:1; the legacy `Food` count converts to
  `Meals` at a pinned rate (`legacyFoodToMeals` = 3 — a mild haircut, 350→300
  restore, flavored as preserved meals crossing the break); every new v2 kind
  (`Stone`/`Water`/`Planks`/`RefinedStone`/`FoodRaw`/`FoodCooked`/`Spears`) starts
  empty.

**Writing the result**: after the transform succeeds, `archiveDB` renames
`world.db` (and any `-wal`/`-shm` sidecars) to `world.v1.db` — the point of no easy
return, so every refusal has already run. A fresh `world.db` is opened and gets
exactly two events, both stamped at the continuation tick: `world.created` (same
name/seed) then `world.migrated`, whose payload (`WorldMigratedPayload` —
[[event-types]]) carries `FromFormat`, `SourceEvents` (the v1 log's last seq),
`SourceTick`, and the full transformed `State` embedded whole. A covering snapshot
is saved at the same tick — deleting it and replaying `world.created` →
`world.migrated` must reproduce the identical state (the determinism half of the
feature's SC-007). The manifest's `FormatVersion` is bumped **last**: a crash
between the archive and the manifest write leaves a recoverable state (`world.v1.db`
present, manifest still v1 — restore is the same rename-back).

## Connections

[[cli-scriptworld]]'s `migrate` command is the only caller; [[world-save-directory]]
defines the format-version gate this bridges and the `world.v1.db` archive artifact;
[[sim-state-reducer]]'s `Apply` applies `world.migrated` as a wholesale state
replace (validated by matching `Seed`); [[event-types]] catalogs the payload;
[[executor]] is what the migrated agents' widened `Inventory` belongs to;
[[snapshots]] is the general mechanism this borrows (a covering snapshot plus a
minimal event tail) to make the migrated log replay-provable with zero v1 history.

## Operational notes

Migration is irreversible in practice (though recoverable mid-crash, above):
`world.v1.db` is kept, never auto-deleted, as the human's escape hatch, but nothing
in the codebase restores from it automatically. A world with no valid v1 snapshot
(never cleanly stopped) cannot be migrated at all — there is no path that migrates
live event history directly. `migrate_test.go` (both packages) and
`whole_feature_test.go` exercise the transform and the full command against
fixture v1 worlds.
