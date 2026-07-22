# Data Model: Metatron Miracles

No new persistent entities. The feature adds four event types (the log is the data
model), one derived cost table, and one state-transformation helper. State struct
changes: **none** — miracles mutate existing fields only.

## Event payloads (canonical JSON, struct-ordered)

### `metatron.time_snapped`

| Field | Type | Rules |
|---|---|---|
| `to_tick` | int64 | MUST be > current `State.Tick` (forward-only, FR-008) |
| `gratis` | bool | cost waived when true; settable only by the operator door |

Apply: `delta = to_tick − Tick`; `rebaseTicks(state, delta)`; `Tick = to_tick`;
spend 2 charges unless gratis (reject if bank < 2 and not gratis).

### `metatron.item_granted`

| Field | Type | Rules |
|---|---|---|
| `agent` | int | valid index, villager alive |
| `kind` | string | one of the Inventory keys: `wood, stone, water, planks, refined_stone, food_raw, food_cooked, meals, spear` |
| `qty` | int | > 0; for `spear`, each grant unit is a fresh full-use spear |
| `gratis` | bool | as above |

Apply: reject whole if `bulk(inv) + bulk(grant) > bulkCap` (24) — reject-never-clamp
(FR-011); add to `Agent.Inv`; spend 1 charge unless gratis.

### `metatron.entity_moved`

| Field | Type | Rules |
|---|---|---|
| `class` | string | `villager \| structure \| pile` |
| `x`,`y` | int | an entity of `class` MUST be here (FR-014) |
| `to_x`,`to_y` | int | villager: `passable`; structure: `buildSite`; pile: `passable` |
| `gratis` | bool | as above |

Apply per class: villager → set X/Y, `Intent = nil`, `IdleSince = landing tick`
(clarified: cancel-and-replan); structure → relocate whole (FuelUntil/Owner/Store ride);
pile → relocate, merging onto an existing destination pile (one-pile-per-tile doctrine).
Spend 1 charge unless gratis.

### `metatron.entity_removed`

| Field | Type | Rules |
|---|---|---|
| `class` | string | `structure \| pile \| terrain` — `villager` MUST be rejected (v1) |
| `x`,`y` | int | an entity of `class` MUST be here; already-overlaid terrain is a no-op target → reject |
| `gratis` | bool | as above |

Apply per class: structure → remove; a chest's `Store` spills to a ground pile on the
tile first (clarified: never silently destroyed); pile → remove with contents (explicit
destruction of the named target); terrain → tree appends `Cleared` (standard regrow),
forage appends `Harvested` (standard regrow), rock appends `Quarried` (permanent).
Spend 1 charge unless gratis.

## Cost table (doctrine constant, `internal/sim/miracles.go`)

| Miracle | Charges |
|---|---|
| time_snapped | 2 |
| item_granted | 1 |
| entity_moved | 1 |
| entity_removed | 1 |

Bank cap (3) and regeneration (1 per 6 game-hours, absolute boundaries) unchanged.

## `rebaseTicks(s *State, delta int64)` — the shift taxonomy

The single authority for shift semantics (FR-009). Every tick-anchored field in the
state tree is classified; the guard test fails on any unclassified field.

**SHIFT (+delta)** — remaining duration preserved:
`Intent.WorkStart` (when ≠ 0) · `Agent.IdleSince` (unconditional — its zero is
genesis-idle, a real tick, not a sentinel) · `Agent.LastTalk` / `Agent.LastGive`
(when ≠ 0; zero is a "never" sentinel) · `AgentHail.Until` · `PlanStep.Until`
(when > 0; zero = no expiry) · `Guard.Tick` (when ≠ 0; timed-guard boundary) ·
`Structure.FuelUntil` (when ≠ 0) · pile `FoodBatch.SpoilAt` · `Harvest.Regrow` ·
`DenUse.Ready` · `Gru.LastAttack` (when ≠ 0, gru present) ·
`Meeting.OpenedTick` / `Meeting.GatherStart` (when ≠ 0, in-flight meeting) · `Debt.Due`

*(`PlanStep.Until` and `Guard.Tick` were added during implementation under FR-009's
catch-all — both are genuine future deadlines reachable from `State` via `Agent.Plan`;
left unshifted, a snap would expire pending plan steps / fire timed guards instantly.
General rule proven out in review: any SHIFT field whose zero value means "unset/never"
shifts only when non-zero.)*

**KEEP** — history and identity, never rewritten:
`Memory.Tick` · `Belief.Tick` · `KnownRumor.Tick` · `Rumor.OriginTick` ·
`ConvoRecord.Conv` (identity!) / `.Tick` · `Chronicle` tick/day fields ·
`Agent.LastGoalTick` · `Agent.Generation` / `Guard.Generation` (counters, not ticks) ·
`LastConsolidatedNight` / `ConsolidatedUpTo` / `LastConsolidateMark` ·
day-denominated governance fields (`DayPassed`, `DayRepealed`, `DayAmended`,
`EstablishedDay`, `LastMeetingDay`) · `NormViolation.Tick`

**PHASE-ANCHORED** — pure functions of the absolute clock, no stored state, follow the
new time by construction: night/day, meeting-convention times of day,
charge-regeneration boundaries.

## Perception memories (FR-018)

Not a new entity — `agent.memory_added` events (existing type, already whitelisted)
appended to the miracle's batch, salience `SalDream`, fixed deterministic templates:

| Miracle | Recipients |
|---|---|
| villager moved | the moved villager |
| item granted | the granted villager |
| time snapped | every living villager |
| structure/pile/terrain move/remove | none (v1) |

## State transitions

All transitions are reducer-arm applications of the four events above; the door
(`InjectSocial`) dry-runs each batch on a state copy, so an invalid miracle is rejected
before recording and a recorded miracle always re-applies in replay. No new lifecycle,
no new indexes, no schema/format-version bump (payloads are additive event types;
`State` marshals unchanged).
