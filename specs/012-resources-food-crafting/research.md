# Phase 0 Research: Resources, Food, and Crafting v1

All Technical Context unknowns resolved against the actual codebase (commit c43ebf2).
Format: Decision / Rationale / Alternatives considered.

## R1. Rock outcrops in worldmap generation

**Decision**: Add `Rock` as a fifth `TileKind` in `internal/worldmap` (`uint8` enum after
`Forage`). Place outcrops in the `Generate` pipeline after trees, before forage: claim
the highest-elevation ~6% of remaining dry grass using the existing elevation `fbm`
field with a new purpose tag (`"rock"`) for tie-breaking hash jitter — correlated noise
⇒ coherent patches, mirroring how trees claim the moistest fraction. `Buildable` stays
plain grass; `Passable` excludes `Rock` (standing rock blocks like trees). Update
`Hash()` coverage implicitly (it fingerprints all tiles) and extend generation tests to
assert outcrops present on every seed alongside the existing water/trees/forage/dens
assertions and the ≥25% buildable floor.

**Rationale**: elevation-correlated placement is geologically plausible (rock shows
where land is high), reuses an existing field (no new noise pass), and the
percentile-threshold pattern is proven by water (every seed gets some). 6% of dry land
on a 64×64 map ≈ 150–200 tiles ≈ 300–400 stone at 2/quarry — practically inexhaustible
for 8 villagers, satisfying the spec's "finite but effectively plentiful" edge case.

**Alternatives**: separate noise field (extra purpose tag, no benefit over reusing
elevation); scattered singles like forage (spec decided coherent patches); den-style
point sites (spec decided terrain kind).

## R2. Inventory representation

**Decision**: Extend the existing fixed-field `Inventory` struct:

```
Wood, Stone, Water, Planks, RefinedStone int
FoodRaw, FoodCooked, Meals int
Spears []int   // remaining uses per spear, sorted ascending, spend index 0
```

All `omitempty` where zero-valued so pre-feature snapshot bytes for untouched agents
stay stable in shape (the format bump makes this a courtesy, not a requirement). The
legacy `Food int` field is REMOVED — the format break (R7) covers it.

**Rationale**: structs-never-maps is the codebase's canonical-JSON determinism rule
(event-types.md); fixed fields keep byte-determinism trivial. Per-spear durability as a
sorted int slice is the smallest representation that survives TASK-26's storage
transfers (each spear keeps its wear); ascending order + spend-lowest = use the
most-worn spear first, deterministic and thrifty.

**Alternatives**: `map[string]int` (breaks canonical-JSON discipline); a fungible
"spear-uses pool" (loses per-item identity that spec 013 storage transfer needs);
generic `[]Item` list (over-engineered for 9 kinds).

## R3. Food forms and eating

**Decision**: Three integer food kinds (R2): `FoodRaw` (+40), `FoodCooked` (+80,
fire-produced), `Meals` (+100, oven-produced). Eating is one instant action consuming
units most-nutritious-first (Meals → FoodCooked → FoodRaw) until `Food need ≥ 900` or
inventory empty. The `agent.ate` event payload changes to
`AtePayload{Agent, Meals, Cooked, Raw int, FoodAfter int}` — consumed counts plus the
absolute resulting need value (outcome-payload convention; reducer sets `FoodAfter`
absolutely and decrements the counts).

**Rationale**: three ints keep "cooked quality" out of the item model (fire output and
oven output are simply different kinds, per the design session); absolute `FoodAfter`
follows the `gru.attacked` absolute-post-wound-health pattern so replay never recomputes
satiety arithmetic. Reflex `eat` rule keys on the same threshold constants as today
(`hungryAt` 350), so degraded-mode behavior only changes in unit granularity.

**Alternatives**: per-unit eat events (event-volume explosion: ~10 events per meal);
carrying a quality field per unit (a map, or parallel arrays — worse than 3 ints).

## R4. Fire fuel, burnout, and cooking

**Decision**: `Structure` gains `FuelUntil int64` (game-tick deadline; 0 = the
pre-feature "always lit" is gone — a fire is lit iff `tick < FuelUntil`). Build sets
`FuelUntil = tick + 2×fireBurnPerWood` (2 wood × 4 game-hours). New executor beat
(inside `stepEvents`, with the other per-tick sweeps) emits `sim.fire_burned_out{X, Y}`
exactly once when a lit fire's deadline passes (pure function of state + tick).
`agent.refueled{Agent, X, Y, FuelUntil}` carries the absolute new deadline (cap: now +
12 game-hours), emitted on the refuel goal's completion; refueling a cold fire relights
it (same event). `warmAt` and cook-site validation check lit-ness. Oven has no
`FuelUntil` — its fuel is consumed per batch from carried wood, not stored.

**Rationale**: absolute deadline in state + absolute deadline in the payload is the
established replay-safe shape (hail's `Until`, den `Ready`, harvest `Regrow` all work
this way). One sweep, no per-fire timers.

**Alternatives**: fuel as an integer level decremented per minute (needs a per-minute
event per fire — volume for nothing); removing burned-out fires from state (loses the
relight-in-place story and churns structure indices).

## R5. Crafting, cooking, and bathing as intents

**Decision**: New goals, all through the existing `Intent` state machine:

- Hand-crafts (anywhere, `Target` = current tile): `craft_planks`, `craft_stone`,
  `craft_spear`. Completion events `agent.crafted{Agent, Kind, X, Y}` with reducer
  applying the fixed recipe delta (inputs re-validated at completion; insufficient
  inputs ⇒ intent resolves without effect, the contested-resource pattern).
- Site works: `quarry` (adjacent Rock, like chop), `collect_water` (adjacent Water),
  `build_oven` (buildable tile, like build_shelter), `refuel_fire` (adjacent/on fire),
  `cook` (adjacent/on lit fire or oven — station inferred from target structure),
  `bathe` (adjacent/on oven). Completion events: `agent.quarried`,
  `agent.collected_water` (HarvestPayload shape), `agent.built{kind: "oven"}`,
  `agent.refueled` (R4), `agent.cooked{Agent, Station, Consumed, Produced, Kind}`,
  `agent.bathed{Agent, MoraleAfter, WarmthAfter}` (absolute, gru-pattern).
- `resolveGoal` (policy.go) gains cases for each, reusing `nearest`/`nearestAdjacentTo`
  helpers; `intentDuration` gains the durations; `goalVocabulary` (mind/prompt.go)
  gains the planner-visible names. The reflex ladder gains exactly one new rule —
  refuel a dying/cold fire when carrying wood (slotted into the existing night-cold
  step 3 and prep step 6) — and nothing else.

**Rationale**: every new mechanic lands as (goal, duration, completion event, reducer
case) — the exact shape of the five existing work goals, so the executor state machine,
guarded plans, hail interactions, and cognition landing ladder all work unchanged.

**Alternatives**: a generic `craft{recipe}` goal with payload-carried recipe ids
(planner must emit structured args the validate layer doesn't support today; per-goal
names match the existing vocabulary pattern and prompt shape).

## R6. Recipe table as data

**Decision**: Recipes live as a package-level table in a new `internal/sim/recipes.go`
(inputs, outputs, duration, site rule), consumed by resolveGoal validation, completion
handling, and the reducer. The table is code (constants), not config — mirrored
human-readably in `contracts/recipes.md`.

**Rationale**: one authoritative table keeps the five recipe call-sites (validate,
duration, completion, reducer, prompt help) from drifting; constants-as-code is how all
tuning lives today (top of agents.go).

**Alternatives**: recipes in world.json (config-driven behavior changes would break
replay-vs-code assumptions; nothing needs per-world recipes).

## R7. Compatibility: format version bump + migration door

**Decision**: Bump `internal/world.FormatVersion` 1 → 2. The existing manifest check
(`world.go:125`) refuses un-migrated worlds — its error message gains a pointer to
`scriptworld migrate`. New event types are reducer no-ops for old code by the existing
unknown-type convention; changed semantics (`agent.ate` payload, yields, `agent.built`
oven kind) are shielded by the version gate. Existing worlds get the snapshot-cut
migration in R10 (spec decision #12: keep the people, reset the land).

**Rationale**: refusal machinery already exists and is tested (`world_test.go`);
migration rides beside it rather than replacing it — an un-migrated world is still
never half-loaded.

**Alternatives**: dual-path reducer keyed on log version (real work; still couldn't
survive the terrain change); refuse-only (original pin — revised 2026-07-22 when the
user asked for a path for `myworld-01`, which holds ~3 game days of lived history).

## R10. Snapshot-cut world migration (v1 → v2)

**Decision**: `scriptworld migrate <world>` (client-side, daemon must be stopped):

1. **Read**: `Store.LatestValidSnapshot` must cover the whole log, tolerating a
   trailing tail of `daemon.*` process-bookkeeping events only (reducer no-ops,
   excluded from determinism comparisons — the v1 daemon appends `daemon.stopped`
   AFTER its shutdown snapshot, so exact `snapshot.seq == max(events.seq)` coverage
   is unsatisfiable for every real cleanly-stopped world; amended 2026-07-22 during
   the live myworld-01 migration). Any sim-affecting event past the snapshot ⇒
   refuse with "start and stop this world once with the v1 binary". v1 events are
   NEVER replayed by v2 code. The v1 state JSON is decoded via a small legacy-shape
   reader (notably `Inventory.Food int`), not the v2 structs.
2. **Transform** (`internal/sim/migrate.go`, pure function v1-state → v2-state):
   - *Carried*: Tick/Day/Night/Speed/pause flags; per-agent Name, Needs, Memories,
     Beliefs, Narrative, Generation, consolidation marks, LastTalk/LastGive, Known
     rumors; Relations, Ledger, Rumors, Secrets, Conversations ring; Norms +
     governance charter state; MetatronCharges; Chronicle ring. Inventory: Wood 1:1,
     legacy Food × 3 → Meals (350→300 mild haircut, flavored as preserved meals),
     new fields zero.
   - *Reset*: Cleared/Harvested/Quarried empty; Structures empty; Gru zeroed;
     MeetingConvention + MeetingPlace cleared (re-seeded from world.json `meeting`
     block on next boot, or re-emerges); all Intents/Plans/Hails/WorkStart/Asleep
     cleared (everyone wakes standing); IdleSince = migration tick.
   - *Re-place*: agents positioned genesis-style (the existing deterministic
     passable-tile placement) on the v2 regeneration of the same seed.
3. **Write**: archive `world.db` → `world.v1.db` (refuse if it already exists —
   idempotence guard); create fresh `world.db`; append `world.created` (same
   name/seed) then `world.migrated{from_format: 1, source_events, source_tick,
   state: <full canonical v2 state>}`; save an initial snapshot; bump manifest
   `format_version` to 2.
4. **Reducer**: `world.migrated` replaces state wholesale (validating name/seed
   match). This keeps the snapshots-are-never-authority invariant: with every
   snapshot deleted, replay-from-genesis (`world.created` → `world.migrated`) still
   reproduces the migrated world byte-identically.

**Rationale**: the one-directory-one-world archive philosophy (world-save-directory
note: "archiving = cp -R") extends naturally to `world.v1.db` — the old history stays
verbatim and restorable, instead of being lossily rewritten. Carrying tick continuity
keeps memory ticks, consolidation marks, and day counts meaningful. The full-state
event is ~1–2 MB once — trivial for SQLite, priceless for the determinism contract.

**Alternatives**: event-log rewrite (transform 107k v1 events into v2 equivalents —
enormous surface, and the map change invalidates every coordinate anyway); migration
snapshot as replay root without a state event (breaks "all snapshots can be discarded";
a pruned/corrupt snapshot chain would silently resurrect a pre-migration world);
carry structures by relocating them (user explicitly accepted a full map reset —
fires/shelters are cheap to rebuild and relocation heuristics are guess-work).

## R8. TUI and observability

**Decision**: `internal/tui/views.go` — new glyph/style for `worldmap.Rock` (and a
distinct one for quarried-out overlay via effectiveKind), oven structure glyph, cold
vs lit fire styling (fire's lit-ness derivable from `FuelUntil` vs current tick),
inventory pane extended to the new kinds (compact: wood/stone/water/planks/rstone +
food triplet + spear count with min uses). Chronicle needs no code change — new events
flow to the narrator as material automatically; salience-table additions in
`internal/sim/memory.go` cover the memorable moments (spear broke, first bath, oven
built).

**Rationale**: follows the existing render path (effectiveKind merge, structure switch);
memory salience entries are how happenings become soul/chronicle material today.

## R9. Model-tier assignment (constitution V)

**Decision**: recommend to the orchestrator: **Opus 4.8** for the cross-package
foundational slice (worldmap Rock + format bump + inventory/state/reducer/event types —
US1's substrate and every payload struct) and the executor/reflex behavior slice
(fuel sweep, eat rewrite, reflex refuel rule — doctrine-adjacent degraded-mode
contract). **Sonnet** for the TUI slice, the planner-vocabulary/prompt slice, recipe
table + hand-craft goals once the substrate exists, and test-alongside work. Record
tier + rubric justification on TASK-50 when dispatching.

**Rationale**: the substrate slice is exactly the rubric's "cross-package/architectural
+ determinism-critical" case; rendering and vocabulary additions are routine
single-package work.
