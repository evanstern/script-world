---
name: metatron-miracles
description: The four charge-priced world-edit events (time snap, item grant, entity move, entity remove) — cost table, operator-only gratis doctrine, shift-semantics re-base taxonomy, perception memories, and the two doors
kind: component
sources:
  - internal/sim/miracles.go
  - internal/metatron/miracle_batch.go
  - internal/metatron/turn.go
  - internal/ipc/server.go
  - cmd/promptworld/miracle.go
verified_against: c8fe41323c1155e8fda1619e4e0ed70ff3f37645
---

# Metatron's miracles

Miracles (spec 016) are four direct, charge-priced world edits — spent from the same
bank as a [[metatron]] nudge, but landing a concrete change rather than a villager's
subjective experience. Like a nudge, a miracle lands through `Loop.InjectSocial` as
one atomic, whitelisted batch; the reducer validates rather than clamps, so an
invalid miracle is rejected wholesale before recording and a recorded miracle always
re-applies cleanly in replay (spec 016 R1). No new persistent entities exist —
miracles only mutate fields already in `sim.State`.

## How it works

**The four event types** (`internal/sim/miracles.go`, canonical JSON, struct-ordered):

| Event | Payload | Effect |
|---|---|---|
| `metatron.time_snapped` | `TimeSnappedPayload{to_tick, gratis}` | jumps `State.Tick` forward to `to_tick`, forward-only (a target at or before the current tick is rejected whole, before any spend); shifts every relative-duration field via `rebaseTicks` first |
| `metatron.item_granted` | `ItemGrantedPayload{agent, kind, qty, gratis}` | provisions a living villager with `qty` known items, reject-whole (never clamp) if it would exceed the carry cap |
| `metatron.entity_moved` | `EntityMovedPayload{class, x, y, to_x, to_y, gratis}` (`class` ∈ villager\|structure\|pile) | relocates the entity from `(x,y)` to `(to_x,to_y)` |
| `metatron.entity_removed` | `EntityRemovedPayload{class, x, y, gratis}` (`class` ∈ structure\|pile\|terrain; villager is always rejected) | deletes the entity or overlays the terrain |

`applyMiracle` in `miracles.go` is the reducer dispatcher `sim.State.Apply` routes
these four types to (alongside `applyMetatron` for `metatron.charge_regenerated`/
`metatron.nudged` — [[sim-state-reducer]]). Every arm's validation — presence at the
source, the destination's placement rule, item kind/quantity — precedes both the
charge spend and the mutation, so a rejected miracle spends nothing and leaves no
partial application (validate-not-clamp, reject-whole):

- **`applyEntityMoved`**: `villager`/`pile` destinations must be `passable`;
  `structure` destinations must satisfy `buildSite`. A moved structure carries its
  `FuelUntil`/`Owner`/`Store` along whole; a moved pile merges onto any pile already
  at the destination (`movePile`); a moved villager drops its intent and goes idle
  at the landing tick (cancel-and-replan) — villagers may share a tile, so no
  destination-exclusivity check applies to a villager move.
- **`applyEntityRemoved`**: a villager is always rejected ("a villager can never be
  removed" — v1 doctrine). A removed chest first spills its `Store` to a ground pile
  via `spillInventory` (the same death-spill vocabulary `agent.died` uses) before
  deletion, so goods are never silently destroyed; a removed pile is destroyed with
  its contents (the explicit, operator-visible destruction the miracle names).
  `removeTerrain` overlays a tree/forage/rock tile through the SAME vocabulary the
  executor's own harvest completions use (chop→`Cleared`, forage→`Harvested` with a
  regrow deadline, quarry→`Quarried`, permanent) — a removed tile is a state the
  executor could already have produced on its own; an already-overlaid tile is
  rejected as a no-op target.
- **`applyItemGranted`**: validates a living, in-range agent index, a `grantableKind`
  (the `Inventory` key vocabulary plus `"spear"` singular), and a positive quantity.
  One bulk per granted unit, exactly like a carried item — a grant of `qty` items
  always costs `qty` bulk regardless of kind, so the cap check is `bulk(*inv)+qty >
  bulkCap`. A spear grant appends `qty` fresh `spearDurability` entries to
  `Inv.Spears`, kept sorted ascending (hunts spend the most-worn first).
- **`applyTimeSnapped`**: rejects a non-forward target before any spend or mutation;
  spends 2 charges (the dearest miracle) unless gratis; calls `rebaseTicks`, then
  sets `State.Tick = to_tick`. FR-010 (a snap mints no charges across the skipped
  regeneration boundaries) needs no code of its own — regeneration only fires when
  the executor *processes* a boundary crossing, and a snap processes no interval.

**Cost table and gratis doctrine** (`miracleCost`, a keyed map — never iterated into
state, for determinism): the time snap costs 2 charges; every other miracle costs 1.
Pricing is doctrine fixed in the reducer, not caller input — a payload never carries
its own price, so replay re-validates every spend identically (R2).
`spendMiracleCharge(eventType, gratis)` is the shared validate/spend helper every
arm calls last, after all other validation passes: with `gratis` it returns
immediately, waiving ONLY the charge (every other validation still runs in full);
without it, it errors if the bank can't pay and decrements it otherwise. `gratis` is
reachable from exactly one surface: the `promptworld miracle --force` CLI/IPC door
([[cli-promptworld]], [[ipc-protocol]], [[ipc-server]]) — the operator's cheat door
the angel structurally cannot reach. The angel's turn contract (`turnReply.Miracle`
in `internal/metatron/turn.go`) is an anonymous struct with **no gratis field at
all** — a model can emit `"gratis":true` in its JSON and it is simply dropped at
unmarshal, nothing to sanitize or forget. `landMiracle` (the angel's landing path)
calls the shared builder with `gratis` hardcoded `false`, so a model-driven miracle
is unconditionally charged (contracts §1, FR-007/SC-005).

**Shift-semantics re-base taxonomy** (`rebaseTicks` in `miracles.go`): the SINGLE
authority for how a time snap preserves in-flight durations while history stays
put (FR-009). Every tick-anchored `int64` field anywhere in the state tree MUST be
classified SHIFT or KEEP in its doc comment:

- **SHIFT** (`+delta`) — a future deadline, or an anchor from which an elapsed/
  remaining duration is measured (shifting preserves that duration across the
  jump). A SHIFT field whose zero value means "unset/never" is shifted only when
  non-zero. SHIFT fields: `Agent.IdleSince` (shifted unconditionally — its zero is
  genesis-idle, a real tick, not a "never" sentinel), `Agent.LastTalk`/`LastGive`,
  `Intent.WorkStart`, `AgentHail.Until`, `PlanStep.Until`, `Guard.Tick`,
  `Structure.FuelUntil`, `Harvest.Regrow`, `DenUse.Ready`, `FoodBatch.SpoilAt`,
  `Debt.Due`, `Gru.LastAttack`, `Meeting.OpenedTick`, `Meeting.GatherStart`.
- **KEEP** — a historical timestamp or an identity/counter; rewriting it would
  rewrite history or break a reference. `Agent.Generation`, `Agent.LastGoalTick`,
  `Memory.Tick`, `Belief.Tick`, `ChronicleEntry.Tick`/`Day`/`FromTick`/`ToTick`,
  and every other identity/history field — see the doc comment for the full list.

`TestRebaseTaxonomyComplete` (`internal/sim/miracles_test.go`) is the taxonomy guard:
it fails the build when a new tick-anchored `int64` field appears in the state
structs without a classification entry here, so the taxonomy can never silently
drift from the struct definitions. `PlanStep.Until` and `Guard.Tick` are shifted even
though `specs/016-metatron-miracles/data-model.md` did not list them — a deviation
recorded in `rebaseTicks`'s doc comment: both are genuine future deadlines FR-009's
catch-all ("any future duration-anchored state") requires shifting, since leaving
them unshifted would expire a pending plan step or fire a timed guard the instant a
snap jumped past its absolute tick.

**Perception memories** (`BuildMiracleBatch` in `internal/metatron/miracle_batch.go`):
the shared, door-neutral batch-builder both channels call, so the miracle event and
its perception memories can never drift between the operator and angel paths. It
only COMPOSES — validation lives entirely in the reducer arms above, enforced by the
`InjectSocial` dry-run, so both doors reject identically and a recorded miracle
always re-applies in replay. `MiracleParams` is the door-neutral, already-resolved
input (villager names resolved to indices, day/`HH:MM` resolved to a tick, by the
caller). Fixed, deterministic memory templates land at `SalDream` — miracles are
exactly as memorable as an angelic dream:

- `time_snap` touches every living villager (`s.LivingAgents()`) with
  `"The light lurched across the sky; a great span of time passed in a single
  breath."`
- `give_item` touches only the granted villager with a rendered
  `"You found N <item> in your hands, as if set there by an unseen giver."`
- `move` touches the moved villager only when `class == "villager"`, resolved via
  `s.VillagerAt(x,y)` — the SAME helper the reducer's `applyEntityMoved` and this
  builder both call, so a tile-addressed move and its memory can never name
  different villagers — with `"An unseen hand lifted you and set you down in a
  strange place."`
- `remove` touches nobody in v1 (no villager is directly affected by a structure/
  pile/terrain removal).

**The two doors**: both are thin translators onto the SAME `BuildMiracleBatch` +
`InjectSocial` path, so they cannot drift.

- **The angel's turn** (`internal/metatron/turn.go`): at most one mediated act per
  turn — a `nudge` takes the turn if the model returns one, otherwise a `miracle`
  may land. `landMiracle` resolves `MiracleParams` from an `agentXY` snapshot
  (`mt.agentXY`, mirrored per batch by the absorb goroutine in `mirrorState`, so the
  turn worker never races the live replica), builds a probe `sim.State` from it, and
  calls `BuildMiracleBatch` with `gratis=false`. A reducer rejection becomes an
  in-fiction refusal appended to the reply (`"(No miracle landed: …)"`), exactly like
  a refused nudge; a landed miracle appends a soul-file line and is recorded in the
  transcript with a `✨` prefix.
  Output contract: `turnReply.Miracle` — `{kind, day, time, villager, item, qty,
  class, x, y, to_x, to_y}`, no gratis. `TurnResult.Miracle` (`{kind, summary}`) is
  what the console surfaces.
- **The operator CLI/IPC door** (`cmd/promptworld/miracle.go` → IPC `miracle`
  command → `internal/ipc/server.go`'s `handleMiracle`): `promptworld miracle
  <world> <snap-time|give|move|remove> ... [--force]`. `handleMiracle` needs only
  `srv.loop` — never `srv.llm` or `srv.metatron` — so it works on pure-sim worlds
  with no angel or orchestrator configured. It fetches the current state via
  `loop.DoState` (to resolve door-side name/tile lookups — `give_item`'s villager
  name through `sim.AgentIndexByName`, `time_snap`'s day/`HH:MM` through
  [[game-clock]]'s `clock.ParseTimeOfDay`/`clock.TickAt`), builds `MiracleParams`,
  calls `BuildMiracleBatch`, and lands it through `loop.InjectSocial`. `--force`
  sets `MiracleArgs.Gratis`, the one field that reaches `gratis=true`. Replies with
  `MiracleData{kind, charges, gratis, summary}`.

**Replay determinism**: a miracle event carries only door-resolved, already-decided
values (a tick, an index, a kind, a coordinate) — never a name or a day/HH:MM string
— so `Apply` re-derives nothing at replay time; the same event applied to the same
prior state always produces the same result. `TestMiracleReplayByteIdentity`,
`TestMiracleSnapReplayByteIdentity`, and `TestMiracleGrantReplayByteIdentity`
(`internal/sim/miracles_test.go`) prove each type replays to the same state hash as
live application. `sim.State.m` (the unexported, unserialized static map attached by
`SetMap`/`NewState`/`MigrateState` — [[sim-state-reducer]]) makes the terrain
vocabulary (`passable`/`buildSite`/`effectiveKind`) available identically live, in
the `InjectSocial` dry-run (`probe.SetMap(l.m)` in [[sim-loop]]'s `handleCommand`),
and in replay, so the map-dependent move/remove-terrain checks can never diverge
between the three contexts.

## Connections

[[metatron]] hosts the angel's door (`landMiracle`, `turnReply.Miracle`);
[[sim-loop]] whitelists the four event types in `injectSocialWhitelist` and
reattaches the static map to the dry-run probe; [[sim-state-reducer]] dispatches to
`applyMiracle` and carries the unexported `m *worldmap.Map` field the reducer arms
need; [[event-types]] catalogs the four payload shapes; [[ipc-protocol]] and
[[ipc-server]] define and implement the `miracle` wire command; [[cli-promptworld]]
is the `promptworld miracle` operator door; [[game-clock]]'s `TickAt`/
`ParseTimeOfDay` resolve a time-snap target; [[world-migration]]'s `MigrateState`
attaches the map so a migrated state is miracle-ready like a fresh genesis.

## Operational notes

A miracle never mints a new persistent entity — it edits fields already in
`sim.State`. A villager is the one class that can never be removed by any door
(v1 doctrine); this is enforced in the reducer, not just at the doors, so it holds
even against a forged event. The gratis flag's only reachable path is the CLI/IPC
`--force` flag — if a future surface needs to grant it, that is a deliberate design
decision to record, not a default to fall into.
