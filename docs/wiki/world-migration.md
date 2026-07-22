---
name: world-migration
description: The snapshot-cut migration chain (spec 012 US6 v1→v2, spec 013 v2→v3) — carries a stopped world's people (and, from v2 on, its land) across a save-format break as a single world.migrated event
kind: component
sources:
  - internal/sim/migrate.go
  - internal/world/migrate.go
verified_against: c8fe41323c1155e8fda1619e4e0ed70ff3f37645
---

# World migration

promptworld has broken its save format twice. Spec 012 (resources/food/crafting
v2) widened `Inventory` from the legacy `{wood, food}` pair to the full v2 resource
set and gave terrain generation rock outcrops — a v1 world's bytes simply don't mean
the same thing under a v2 build. Spec 013 (inventory/storage v3) added a bulk cap,
ground piles, chests, theft, and rot, which change how the reducer/executor treat
*existing* event shapes (yield truncation, death spill, the give-guard) — a v2 log
replayed under v3 code would diverge even though v2 and v3 land are identical.
Either way, `internal/world.Open` refuses any manifest whose `format_version` isn't
the current one ([[world-save-directory]]). `promptworld migrate <world>`
([[cli-promptworld]]) is the one-time, offline door a stopped older world walks
through to keep running; it admits a v1 **or** v2 source and refuses an
already-current world outright ("nothing to migrate").

The two steps have different design pins because their inputs differ:

- **v1→v2** (research R10): **"keep the people, reset the land"** — terrain
  generation itself changed (rock outcrops), so every villager and the whole
  social/governance fabric carry over verbatim while the map and everything bound
  to it (structures, overlays, in-flight intents/plans) is reborn under v2 rules —
  migrated souls are re-placed via `genesisPlacement`.
- **v2→v3** (research R3): people **and** land carry verbatim — spec 013 changed no
  terrain generation and no map inputs, so there is nothing to reset. Agents keep
  their exact coordinates (no re-placement), structures/overlays/mid-flight intents
  ride through unchanged, and the only adjustment is the new bulk-cap invariant.

A v1 world chains both steps in one `migrate` run (1→2→3); a v2 world runs only the
second step.

## How it works

The work splits across two packages by concern:

- **`internal/world/migrate.go`** is the ceremony: resolve and validate the source
  world (`OpenForMigration`, which admits `format_version` 1 or 2), refuse unsafe
  conditions, archive the original database, write the fresh log, bump the
  manifest. It never touches sim semantics directly.
- **`internal/sim/migrate.go`** is the pure transforms: decode a v1 state shape and
  produce a v2 `sim.State` (`TransformV1Snapshot`/`MigrateState`), and carry a v2
  `sim.State` into v3 (`TransformV2State`/`TransformV2Snapshot`). Neither runs on
  the live reducer path.

**Client-side and offline**: `Migrate(dir)` refuses a running daemon (a pidfile
liveness check duplicated from `internal/daemon` rather than imported, to avoid an
import cycle) and refuses if the source-format archive already exists — `world.v1.db`
for a v1 source, `world.v2.db` for a v2 source (`archiveDBPath`, keyed to
`Manifest.FormatVersion`; the already-migrated guard, never overwritten). Keying the
guard to the *source* format means a v2 world produced by an earlier v1→2 migration
— which still carries a stale `world.v1.db` from that run — remains migratable to
v3: its own guard is `world.v2.db`, untouched until this run. `Migrate` never
replays old events under new rules; it reads only the source world's **covering
snapshot** — the clean-shutdown guarantee (`CheckContiguity` +
`LatestValidSnapshot`). A real daemon appends a `daemon.stopped` bookkeeping event
after its shutdown snapshot, so a one-event tail of pure `daemon.*` events past the
snapshot is tolerated (they carry zero sim state — nothing to lose); any
sim-affecting event past the snapshot means an unclean stop left un-snapshotted
history, and migration refuses with a remedy: start and stop the world once cleanly
under its own binary, then retry.

**The v1→v2 transform** (`sim.TransformV1Snapshot` / `MigrateState`) decodes the v1
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
- **Attaches the map** (spec 016): `MigrateState` sets the resulting `State`'s
  unexported `m *worldmap.Map` field (via the same construction path `NewState`
  uses, [[sim-state-reducer]]), so a migrated state is map-aware for the
  miracle reducer arms exactly like a fresh genesis or a live replica.
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

**The v2→v3 transform** (`sim.TransformV2State` / `TransformV2Snapshot`) needs no
distinct legacy decoder: every v3 addition (`State.Piles`, `Structure.Owner`/
`Store`, `Intent.Kind`/`Qty`) is additive and `omitempty`, so a v2 snapshot's JSON
decodes straight into the current `sim.State`, new fields landing on their zero
values. The transform is a pure function of the input (it copies the `Agents` and
`Piles` slices before mutating, so the caller's state is never touched) that:

- **Carries everything verbatim**: positions (no re-placement — v3 changed no
  terrain), structures, overlays (`Quarried`/`Cleared`/`Harvested`), mid-flight
  intents/plans (unlike 1→2's wipe — no map inputs changed, so targets stay valid
  and the bulk cap simply applies at completion), the whole social/governance
  fabric, and the clock (`Degraded` resets to `false` and `EffectiveRate` is
  refreshed from `Speed`, exactly as the v1→v2 step does — a stopped world carries
  no live drift across the break).
- **Applies only the bulk-cap invariant** (research R3 "Decisions taken at
  implementation"): a living agent whose carried bulk exceeds `bulkCap` spills the
  excess to a ground pile at its own tile; a dead agent spills its *entire* frozen
  inventory — the v3 death-spill invariant (see [[executor]]) carried forward, so a
  migrated world matches what v3 itself would have produced from that death.
  Spilling removes goods in canonical kind order: within food, least-nutritious
  first (`food_raw` → `food_cooked` → `meals`, so a capped villager keeps its best
  food); spears spill most-worn-first, mirroring the give/drop transfer idiom.
  Spilled food batches are stamped `SpoilAt` = migration tick + `rotWindowTicks`,
  same as any other fresh drop.

**Writing the result**: after the transform succeeds, `archiveDB` renames
`world.db` (and any `-wal`/`-shm` sidecars) to the source-format archive —
`world.v1.db` for a v1 source, `world.v2.db` for a v2 source — the point of no easy
return, so every refusal has already run. A fresh `world.db` is opened and gets
exactly two events, both stamped at the continuation tick: `world.created` (same
name/seed) then `world.migrated`, whose payload (`WorldMigratedPayload` —
[[event-types]]) carries `FromFormat`, `SourceEvents` (the source log's last seq),
`SourceTick`, and the full transformed `State` embedded whole. A covering snapshot
is saved at the same tick — deleting it and replaying `world.created` →
`world.migrated` must reproduce the identical state (the determinism half of SC-007
in both specs). The manifest's `FormatVersion` is bumped to the current version
**last**: a crash between the archive and the manifest write leaves a recoverable
state (the source-format archive present, manifest still at the source version —
restore is the same rename-back).

## Connections

[[cli-promptworld]]'s `migrate` command is the only caller; [[world-save-directory]]
defines the format-version gate this bridges and the `world.v1.db`/`world.v2.db`
archive artifacts; [[sim-state-reducer]]'s `Apply` applies `world.migrated` as a
wholesale state replace (validated by matching `Seed`); [[event-types]] catalogs the
payload; [[executor]] is what the migrated agents' inventory (and, from v3, the bulk
cap and death-spill rule) belongs to; [[snapshots]] is the general mechanism this
borrows (a covering snapshot plus a minimal event tail) to make the migrated log
replay-provable with zero source-format history.

## Operational notes

Migration is irreversible in practice (though recoverable mid-crash, above): the
source-format archive (`world.v1.db` or `world.v2.db`) is kept, never
auto-deleted, as the human's escape hatch, but nothing in the codebase restores
from it automatically. A world with no valid covering snapshot (never cleanly
stopped) cannot be migrated at all — there is no path that migrates live event
history directly. `internal/sim/migrate_test.go` and `internal/world/migrate_test.go`
exercise both transforms and the full command against fixture v1 and v2 worlds,
including a v1 fixture that chains 1→2→3 in one run; `internal/sim/whole_feature_test.go`
covers the storage-surface event types this migration must also carry correctly.
