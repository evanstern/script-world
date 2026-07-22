# Phase 0 Research: Metatron Miracles

All unknowns from the Technical Context are resolved here. Grounding: direct code reading
of `internal/sim` (state, reducer, loop, terrain), `internal/metatron` (turn contract,
nudge landing), `internal/ipc` (server dispatch), `cmd/promptworld` (CLI), on 2026-07-22.

## R1 — Event vocabulary and the door

**Decision**: four new event types, landed through the existing `InjectSocial` door and
added to `injectSocialWhitelist` (`internal/sim/loop.go:151`):

- `metatron.time_snapped` — `{to_tick, gratis}`
- `metatron.item_granted` — `{agent, kind, qty, gratis}`
- `metatron.entity_moved` — `{class, x, y, to_x, to_y, gratis}`
- `metatron.entity_removed` — `{class, x, y, gratis}`

Reducer arms live in a new `internal/sim/miracles.go`, mirroring `applyMetatron`
(`internal/sim/metatron.go`): validate-not-clamp, error on any invalid payload so the
door's dry-run rejects before recording, and recorded events always re-apply cleanly in
replay.

**Rationale**: `metatron.nudged` already proves this exact shape — charge check in the
reducer, dry-run at the door, atomic batch landing, whitelist as the isolation boundary.
No new infrastructure is needed; the whitelist addition IS the feature's security model.

**Alternatives considered**: a separate "admin" IPC path writing events directly to the
store (rejected: bypasses the dry-run and the single-mutation-path doctrine; exactly the
hand-surgery this feature exists to eliminate); reusing `agent.moved` for villager moves
(rejected: miracle moves carry gratis/charge semantics and intent-cancel behavior that
executor moves must not have).

## R2 — Charge pricing in the reducer

**Decision**: a package-level cost table in `internal/sim/miracles.go`:
`time_snapped: 2; item_granted, entity_moved, entity_removed: 1` (clarified 2026-07-22).
Each payload carries `gratis bool`. The reducer arm checks `MetatronCharges >= cost`
unless gratis, and decrements unless gratis. Gratis never skips any other validation.

**Rationale**: keeping cost enforcement in the reducer (not the console) means replay
re-validates spends identically at the same log position — the established
`metatron.nudged` invariant ("no charges banked" is a reducer error there today).

**Alternatives considered**: enforcing charges door-side only (rejected: replay could
then diverge from live validation); a Charges field in the payload (rejected: pricing is
doctrine, not caller input — a payload-supplied price would let callers set their own).

## R3 — Time-snap shift semantics: the re-base taxonomy

**Decision**: `metatron.time_snapped` sets `State.Tick = to_tick` and re-bases exactly
the *relative-duration* fields by `delta = to_tick − old tick`, via one helper
(`rebaseTicks(s *State, delta int64)`) with an exhaustive field walk. Taxonomy (from a
complete `int64` field inventory of the sim state structs, 2026-07-22):

**Shift (+delta)** — future deadlines and duration anchors, so remaining time is preserved:

| Field | Location | Anchors |
|---|---|---|
| `Intent.WorkStart` (when non-zero) | agents.go | work-in-progress duration |
| `Agent.IdleSince` | agents.go | reflex grace window |
| `Agent.LastTalk`, `Agent.LastGive` | agents.go | social cooldowns |
| `AgentHail.Until` | agents.go | courtesy pause |
| `Structure.FuelUntil` | agents.go | fire remaining burn |
| `FoodBatch.SpoilAt` (in piles) | agents.go | rot remaining window |
| `Harvest.Regrow` | agents.go | forage regrowth |
| `DenUse.Ready` / den cooldown fields | agents.go | den availability |
| `Gru.LastAttack` | gru.go | attack cooldown |
| `Meeting.OpenedTick`, `Meeting.GatherStart` (when a meeting is in flight) | governance.go | meeting phase durations |
| `Debt.Due` | social.go | repayment deadline |

**Keep (never shift)** — historical stamps and identities; rewriting them would rewrite
history or break identity references:

`Memory.Tick`, `Rumor.OriginTick`, `ConvoRecord.Conv` (tick doubles as conversation id)
and `ConvoRecord.Tick`, all `Chronicle` tick/day fields, `Agent.LastGoalTick` (display
history), consolidation marks (`LastConsolidatedNight`, `ConsolidatedUpTo`,
`LastConsolidateMark` — they reference past event/memory ticks that do not move), all
day-denominated governance fields (`Norm.DayPassed/DayRepealed/DayAmended`,
`EstablishedDay`, `LastMeetingDay` — day-cadence anchors that should naturally re-arm
under the new clock).

**Phase-anchored (intentionally follow the new clock)** — pure functions of absolute
game time, no stored state to touch: night/day derivation, meeting-convention times of
day, charge-regeneration boundaries (multiples of 21600 ticks).

**FR-010 (no minting) falls out structurally**: regeneration is emitted by the executor
when a *processed* tick crosses a boundary; a snap jumps `Tick` without processing the
interval, so skipped boundaries simply never fire. No suppression code needed — but the
drift/regen test must assert it.

**Maintenance hazard control**: `rebaseTicks` is accompanied by a doctrine comment and a
guard test that fails when a new `int64` tick-anchored field appears in the state structs
without a taxonomy entry (reflective walk over the state JSON comparing against an
explicit allow/shift/keep list). Every future field must be classified deliberately.

**Drift test (SC-003)**: whole-day variant — copy a mid-activity state, snap +86400
ticks, drive both worlds through the same scripted executor ticks, assert event streams
and final state are identical modulo the tick offset (reuses the replay byte-identity
harness pattern from `chest_test.go` / `craft_test.go`). Arbitrary-delta variant — per-field
unit assertions that remaining durations are preserved.

**Snap target validation**: reducer rejects `to_tick <= Tick` (forward-only, FR-008) and
door-side the console/angel translate day/HH:MM to ticks via `internal/clock`.

## R4 — Entity addressing, move and remove semantics

**Decision**: `class` ∈ `villager | structure | pile | terrain`; target resolved at
(x,y); request rejected if no entity of that class is there (FR-014).

- **Move villager**: destination must satisfy `passable(m, s, x, y)`
  (`internal/sim/terrain.go:38`); sets X/Y, clears `Intent` and stamps `IdleSince` to the
  landing tick (clarified: intent cancelled, villager replans).
- **Move structure**: destination must satisfy `buildSite` (effective grass, no
  structure, no pile); the struct moves whole — `FuelUntil`, `Owner`, `Store` ride along.
- **Move pile**: destination must be passable; if a pile already exists there, contents
  merge (the reducer's existing one-pile-per-tile merge doctrine); food batches keep
  their `SpoilAt`.
- **Remove structure**: chest contents spill to a ground pile on the same tile
  (clarified 2026-07-22; reuses the existing spill/pile vocabulary); non-chest structures
  are simply removed.
- **Remove pile**: the pile and its contents are removed — explicit, operator-visible
  destruction of the named target is the point of the ability (distinct from *silent*
  side-channel destruction, which the spill rule prevents).
- **Remove terrain**: routed through the existing overlay vocabulary — tree → `Cleared`
  (with standard regrow), forage → `Harvested` (standard regrow), rock → `Quarried`
  (permanent). Removing an already-overlaid tile is rejected as a no-op target.
- **Villager remove**: rejected in the reducer (v1 doctrine).

**Rationale**: reusing `passable`/`buildSite`/overlay vocabulary means miracles cannot
create states the executor couldn't — no new invariants to defend.

## R5 — The angel's output contract

**Decision**: extend `turnReply` (`internal/metatron/turn.go:46`) with an optional
`Miracle` field:

```json
"miracle": {"kind": "time_snap"|"give_item"|"move"|"remove",
            "day": 2, "time": "11:30",
            "villager": "Ash", "item": "food_raw", "qty": 2,
            "class": "villager", "x": 44, "y": 8, "to_x": 45, "to_y": 32} or null
```

(kind-dependent subset of fields; exactly one of `nudge`/`miracle` may land per turn,
keeping the existing "at most one mediated act per turn" shape). A `landMiracle`
counterpart to `landNudge` validates, prices, builds the batch (miracle event + FR-018
memory events), and lands it via `InjectSocial`.

**Gratis is structurally unreachable from the model**: `turnReply`'s miracle struct has
no gratis field, so any `"gratis": true` in model output is dropped by unmarshalling
before it can exist in-process. The adversarial test (SC-005) feeds a crafted model reply
claiming gratis and asserts the landed event is charged.

**Rationale**: mirrors the nudge contract exactly; the system prompt gains a cost table
and the miracle grammar. Structural stripping beats sanitization — there is nothing to
forget to strip.

## R6 — Operator surface

**Decision**: new CLI subcommand family (dedicated, per clarification):

```
promptworld miracle <world> snap-time <day> <HH:MM>            [--force]
promptworld miracle <world> give <villager> <item> <qty>       [--force]
promptworld miracle <world> move <class> <x,y> <x1,y1>         [--force]
promptworld miracle <world> remove <class> <x,y>               [--force]
```

backed by a new IPC command `miracle` (`internal/ipc`): args
`{kind, params…, gratis bool}`. The server builds the same batch the angel path builds
(shared batch-builder so the two doors cannot drift) and lands it via `InjectSocial`.
`--force` sets gratis; without it the console spends charges like the angel. The IPC arm
does NOT require the LLM orchestrator or metatron presence — miracles work on pure-sim
worlds (the charge bank is sim state).

**Alternatives considered**: flags on `promptworld metatron` (rejected in clarify: blurs
the fiction/operator boundary the gratis rule enforces); attach/TUI-only (rejected:
clumsy for scripted/emergency use).

## R7 — Villager perception (FR-018)

**Decision**: door-side batch construction appends `agent.memory_added` events (already
whitelisted; exact pattern of `landNudge`'s dream/omen memories) for directly affected
villagers, charged and gratis alike:

- moved villager → one memory ("an unseen hand set you down elsewhere" rendering);
- granted villager → one memory naming the items;
- time snap → one memory for every living villager (all directly experience the lurch),
  omen-style;
- structure/pile/terrain move/remove → no memories in v1 (no villager directly affected).

Salience: the existing dream salience constant (`SalDream`) — miracles are exactly as
memorable as angelic dreams. Deterministic fixed-template texts (no LLM in the path).

## R8 — Determinism, replay, observability

**Decision**: no new mechanisms. Events land through `InjectSocial` (dry-run on a state
copy, atomic batch, recorded); recovery replays them through the same reducer
(`internal/daemon/daemon.go:246`). New tests follow the established replay byte-identity
pattern (SC-002): scripted miracle sequences replay from genesis/log to byte-identical
state hashes. Chronicle/digest visibility is automatic — the digest already notes
recorded events, and gratis appears in payloads for after-the-fact enumeration (SC-004).
