---
name: executor
description: The deterministic agent-body layer — integer needs with death, multi-step intents (forage/chop/hunt/quarry/craft/cook/build/demolish/repair/eat/bathe/sleep), per-minute heartbeat, dynamic terrain overlays (including walls and paths), fire fuel
kind: component
sources:
  - internal/sim/executor.go
  - internal/sim/agents.go
  - internal/sim/plan.go
  - internal/sim/terrain.go
  - internal/sim/recipes.go
  - internal/sim/memory.go
verified_against: e9213e17e6e48cf30da802949d9b59e0e3d78370
---

# Executor

The executor (TASK-5) replaced the placeholder wanderers: agents are now
deterministic bodies with needs, inventories, and multi-step intents, run unattended
by `stepEvents` between planner calls. The LLM planner (TASK-7) will *choose* goals;
the executor is what makes goals physically happen — and it must keep bodies alive
with no planner at all (the degraded-mode contract from the grounding session).
Spec 012 (resources/food/crafting v2) widened the body's economy substantially:
finer-grained resources, a crafting chain, fire fuel with burnout, spear-armed
hunts, and a shelter rest bonus. Spec 013 (inventory & storage v1) added a carried
bulk cap, ground piles, builder-owned chests, and food rot — this note covers
that v3 shape. Spec 032 (walls, axes, paths) layered in a fifth harvest tool
(the axe, tripling chop/quarry yield), a new impassable-structure family
(player-built walls, multi-cycle demolish/repair), and a walkable tile
improvement (paths, which double movement speed) — all additive `omitempty`
fields, so `format_version` stays 3 and no migration is needed to carry them.

## How it works

**Agents** (`agents.go`): eight named bodies (`sim.AgentNames`) with authored
personas ([[agent-mind]]). `Needs{Health, Food, Rest, Warmth, Morale}` are integers 0..1000 —
integer math keeps decay byte-deterministic across platforms. `Inventory` (v2,
format_version 2 — [[world-save-directory]]) carries `Wood`, `Stone`, `Water`,
`Planks`, `RefinedStone`, `FoodRaw`, `FoodCooked`, `Meals` (all `omitempty` ints),
and `Spears []int` — remaining uses per carried spear, sorted ascending so a hunt
always spends the most-worn one first. `Axes []int` (spec 032) mirrors `Spears`
exactly: remaining harvest uses per carried axe, sorted ascending, spent
most-worn-first on chop/quarry. The legacy `Food int` field is gone; a v1
world must run `promptworld migrate` ([[world-migration]]) before it can boot under
this build. All tuning constants (decay rates, action durations, yields, costs,
thresholds) sit at the top of `agents.go`; the v2 economy's constants (food
restores, fire fuel, spear durability, gather/craft/build/station magnitudes) are
grouped under their own "spec 012" block there, and a separate "spec 032" block
holds the walls/axes/paths tuning surface (`wallPlankHP` 200, `wallStoneHP` 600 —
at least 2x the plank wall per FR-003 — `axeDurability` 10 harvest uses,
`chopYieldBare`/`chopYieldAxe` 1/3, `quarryYieldBare`/`quarryYieldAxe` 1/3,
`demolishChipHP` 100 per work cycle, `repairHPPerUnit` 100 per material spent,
`pathStoneCost` 1). The legacy flat `chopWood`/`quarryYield` (2) constants are
deleted, replaced by the bare/axe pairs. The recipe table itself lives
in `recipes.go` (mirroring `specs/012-resources-food-crafting/contracts/recipes.md`
and `specs/032-walls-axes-paths/contracts/recipes.md` — `recipes_test.go` asserts
the tables agree).

**Heartbeat**: every game-minute (`tick%60 == 0`) each living agent's needs decay via
`decayNeeds`: food always falls; rest falls awake (or recovers asleep — at
`restRegenShelter` (6/minute) on a shelter tile, `restRegenSleep` (4) otherwise, the
plank economy's payoff for building one); warmth falls at night outdoors, recovers
near a **lit** fire or in shelter, drifts up by day. A fire is lit iff
`tick < Structure.FuelUntil` — `warmAt` takes the tick and checks it, so a burned-out
fire grants no warmth. Zero food or zero warmth drains health; health at 0 emits
`agent.died` with cause `starvation` / `exposure` / `collapse`. The new values land
as one absolute `agent.needs_changed` event per agent per minute (absolute values =
replay-safe).

**Fire fuel** (T019/T020): `build_fire` (still 2 wood) lights a fire for
`2×fireBurnPerWood` (4 game-hours per wood, so 8 hours) from the build tick.
`refuel_fire` (instant on arrival, 1 wood) pushes `FuelUntil` forward by
`fireBurnPerWood`, capped at `now + fireFuelCap` (12 hours); relighting a cold fire
starts the window from now. Every tick, `stepEvents` sweeps `Structures` for a fire
whose `FuelUntil` falls in the tick's window (`tick-1 < FuelUntil <= tick`) and emits
`sim.fire_burned_out` exactly once on that transition (no state effect — lit-ness
stays derived), plus a low-salience witness memory ("Watched the fire burn out.")
for every living agent within `witnessRadius`.

**Intents**: `Intent{Goal, Target, Res, WorkStart}` executes as a state machine —
walk (one tile per 5 ticks, staggered per agent, next hop from [[reflex-policy]]'s
BFS), then on arrival: instant goals (`sleep`, `wander`, `goto_warmth`,
`refuel_fire`) complete immediately; work goals re-validate the resource or station
(someone may have taken it, or a fire may have gone cold — the contested-resource
pattern, spec 012 FR-002/FR-014), emit `agent.work_started`, and after the goal's
duration (`workDuration`, below) emit the completion event, which the reducer turns
into inventory, overlays, structures, or needs. Movement itself gets a second,
conditional cadence slot (spec 032 US3): the staggered phase-0 tick always steps,
but a phase-2 tick also steps when the agent is standing ON a path tile
(`pathAt`) — stepping FROM a paved tile doubles effective speed along it, while
an unpaved agent never sees the extra slot, so nothing else about movement changes.

**The v2 goal set** adds `quarry`/`collect_water` (gather, like forage/chop/hunt),
`craft_planks`/`craft_stone`/`craft_spear` (hand-crafts, `SiteAnywhere` — no travel,
work happens on the agent's own tile), `build_oven` (alongside `build_fire`/
`build_shelter`), and `cook`/`bathe`/`refuel_fire` (station actions at a fire or
oven). Spec 032 (walls, axes, paths) adds a fourth hand-craft, `craft_axe`
(alongside the other three), two ADJACENT-build goals — `build_wall_plank`/
`build_wall_stone` (the builder stands beside the tile the wall lands on, unlike
every stand-on-target build before it, so a wall can never entomb its own
builder), a stand-on-target build, `build_path` (the fire/oven/chest pattern), and
two multi-cycle work goals on an existing wall, `demolish` and `repair` — each
completion may re-arm the SAME intent for another work cycle rather than
finishing (below). Since spec 014 (TASK-53) `intentDuration` reads `intentDurations`, a table
built at init from the [[tool-registry]]'s per-tool `Cost.DurationTicks` (values
hand-equal to the sim constants, pinned by
`TestWorldToolDurationsMatchSimConstants`) — since spec 017, filtered to
GOAL-DOOR tools (`Effect World && PlanStep`, the same discriminator
[[tool-registry]]'s coverage check uses): `set_plan` is a World tool but never
reaches `intentDuration` by its own name (each of its plan steps names an
already-covered goal-door goal instead), so it is deliberately absent from this
table rather than carrying a meaningless zero-duration entry. Goals with no
registry duration — the instant verbs and the internal `seek` alias — complete
on arrival (0), exactly as the old switch's default did. `workDuration` overrides the plain
`intentDuration(goal)` lookup for two
context-dependent cases: a spear-carrying hunt takes `huntTicksSpear` (faster than
the bare-handed default) and cooking at an oven takes `cookOvenTicks` (slower than
at a fire) — both read off current state (`Agent.Inv.Spears`, the target structure),
never persisted on the `Intent`.

Completion behavior per goal:
- `chop` → `agent.chopped` (+`chopYieldBare` (1) Wood bare-handed, or
  `chopYieldAxe` (3) with a carried axe — spec 032 US2, replacing the old flat
  `chopWood` (2)). `quarry` → `agent.quarried` (+`quarryYieldBare`/`quarryYieldAxe`,
  same 1/3 split), and the outcrop is added to `State.Quarried` (below).
  `collect_water` → `agent.collected_water` (+`collectWaterYield` Water); water
  sources never deplete. A carried axe (`Axes[0]`, checked pre-mutation like the
  spear/hunt precedent) spends its last use on either harvest — spending it to
  zero emits a companion `agent.axe_broke` right after, in the same batch, plus
  a memory ("My axe broke at the work…"), the exact `agent.spear_broke` pattern.
- `hunt` → `agent.hunted`; a carried spear (`Spears[0]`, checked pre-mutation) raises
  the yield to `huntYieldSpear` (vs. `huntYieldBare` bare-handed) and spends that
  spear's last use — spending it to zero emits a companion `agent.spear_broke` right
  after, in the same batch, plus a memory ("My spear broke on the hunt…").
- `craft_planks`/`craft_stone`/`craft_spear`/`craft_axe` (spec 032 US2 adds the
  last: 1 plank + 1 stone → one axe holding `axeDurability` (10) uses) → inputs
  re-validated against `recipes.go`'s table at completion (`hasItems`);
  insufficient inputs resolve via `agent.intent_done` only (no craft). A
  satisfied craft emits `agent.crafted{Kind}`; the reducer applies the recipe's
  delta, appending to `Inv.Spears`/`Inv.Axes` (sorted ascending) for the two
  durability-slice crafts, or a flat `Inv` field otherwise.
- `build_oven` → `agent.built{Kind: "oven"}`; the first oven in the village gets
  distinct memory text ("Raised the village's first oven — meals and baths, at
  last."), and nearby living agents get a witness memory, same pattern as a
  witnessed death.
- `build_wall_plank`/`build_wall_stone` (spec 032 US1) → `agent.built{Kind:
  "wall_plank"|"wall_stone"}` landing on the intent's `Res` tile — walls are the
  one build family that lands ADJACENT to the builder (`Target`) rather than on
  it, so a builder can never wall itself in (FR-007); re-validated at completion
  that `Res` is still a build site AND holds no living agent (`agentAt`), else
  `agent.intent_done` only. The reducer stamps the new wall's `HP` at
  `wallMaxHP(kind)` (`wallPlankHP` 200, `wallStoneHP` 600 — derived from kind,
  never stored separately, the fire-lit-ness doctrine) and spends the recipe's
  planks/refined-stone. A builder memory rides at the shelter salience tier
  ("Built a wall."); walls emit no witness memory.
- `demolish` (spec 032 US1) → one chip per completed work cycle against the
  still-standing wall at `Res` (re-validated; a vanished wall — someone else
  finished it first — resolves via `agent.intent_done` only): `agent.wall_chipped`
  removes `demolishChipHP` (100) and the reducer re-arms the SAME intent
  (`WorkStart` reset to 0) for another cycle when the wall would still stand
  (`HP - chip >= 1`); the cycle that would take it to zero instead emits
  `agent.wall_destroyed`, which removes the structure and clears the intent. A
  plank wall takes 2 cycles, a stone wall 6. No memory (spam-avoidance, the
  forage/chop precedent).
- `repair` (spec 032 US1) → one cycle mends a still-damaged wall at `Res` with 1
  unit of its matching carried material (`wallRepairMaterial`: planks for
  `wall_plank`, refined stone for `wall_stone`), restoring `repairHPPerUnit`
  (100) HP clamped to `wallMaxHP`; `agent.wall_repaired` re-arms the same intent
  for another cycle if the wall is still damaged AND material remains, else
  clears it. Re-validated at completion (wall gone, already mended, or material
  spent all resolve via `agent.intent_done` only).
- `build_path` (spec 032 US3) → `agent.built{Kind: "path"}` landing on the
  intent's own `Target` tile (stand-on-target, like fire/oven/chest), spending
  `pathStoneCost` (1) raw stone; a path carries no `HP` (`isWall` is false for
  it) and emits no builder memory (not formative, same spam-avoidance
  precedent as forage/chop). Standing on a path tile is what grants the 2x
  movement bonus described above (`pathAt`), not the build itself.
- `cook` → up to `ovenBatchSize` FoodRaw converts to `agent.cooked`: at a fire,
  fuel-free, producing `food_cooked`; at an oven, additionally burning 1 carried
  wood, producing `meals` (mirrors the fire's own fuel — an oven with no carried
  wood or no raw food resolves via `agent.intent_done` only).
- `bathe` (oven only) → re-validates carried water + wood at completion (water's
  only consumer); emits `agent.bathed` with absolute post-cap Morale/Warmth
  (`bathMorale`/`bathWarmth` bumps, gru-pattern) and a positive-toned memory.
- `refuel_fire` → re-validated on arrival (fire still present, wood still carried);
  a refuel that would grant no gain over the current deadline (already at the fuel
  cap) is a no-op (`agent.intent_done` only).

**Eating** (T018, `eatOutcome`): the reflex's `agent.ate` direct-event path (and the
planner's guarded-plan equivalent) now computes an outcome rather than emitting a
bare marker. `eatOutcome` consumes the most-nutritious form first — `Meals` →
`FoodCooked` → `FoodRaw` — one unit at a time until `Needs.Food` reaches `satietyAt`
(900) or the inventory runs dry, and returns `false` (nothing eaten, no event) if
already sated or carrying no food at all. Each form restores a different amount
(`mealRestore` 100, `foodCookedRestore` 80, `foodRawRestore` 40 — cooking roughly
doubles raw, a meal is the best food); the payload carries counts consumed per form
plus the absolute post-eat food need, so the reducer never re-derives arithmetic.
`wakeReason`'s hunger-emergency wake check now looks for *any* carried food form,
not just raw. `canGive` (the give-to-starving social rule) checks `Inv.FoodRaw`
specifically — raw is deliberately the form a subsistence village shares.

**Carried bulk & the v1 storage economy** (spec 013): every kind of carried good
counts toward a per-villager `bulk` — one unit per inventory count, one per
carried spear or (since spec 032) axe — capped at `bulkCap` (24), derived via `bulk()`/`freeBulk()` and
never stored. Every gather completion (`forage`/`chop`/`hunt`/`quarry`/
`collect_water`) clamps its yield to the taker's pre-event free bulk and is
skipped entirely — no event at all, so no depletion — when free bulk is already
zero (US1-AS1/AS2); a hand-craft's completion additionally re-validates its net
output-minus-input bulk delta the same way (only `craft_planks` is net-positive;
crafts don't truncate, they simply don't happen if the net won't fit). The
give-to-starving social rule (`repayable`/`giveable`) likewise requires the
receiver have free bulk before a give is offered.

Ground goods live as `State.Piles`, one per tile (event-sourced overlay state,
like `Quarried`). `drop`/`pick_up` are instant-on-arrival, planner/plan-only
goals (never in the reflex ladder, FR-014): `drop` moves a named `Kind`/`Qty`
(`Qty` 0 = all carried) from inventory onto the agent's own tile, creating or
merging the tile's pile; `pick_up` targets the nearest pile (on or adjacent) and
moves goods in, truncated to free bulk, emitting one `agent.picked_up` per kind
actually moved — `Kind` "" sweeps every kind in canonical field order (wood,
stone, water, planks, refined_stone, food_raw, food_cooked, meals, spears, and
since spec 032, axes). Food
on the ground is batch-tracked (`FoodBatch{Kind, N, SpoilAt}`, drop order, same
`(Kind, SpoilAt)` merges); every non-food kind is a flat count; spears and axes
carry their remaining uses, always sorted ascending so the most-worn moves first
on either side of a transfer (axes move the exact same way as spears — build,
drop/pick-up, deposit/withdraw, and death-spill all clone the spear path).
`agent.died` additionally spills the dead agent's
entire inventory onto a pile at the death tile (reducer-internal, no new event —
research R7's debt-opening precedent), and `buildSite` (`terrain.go`) rejects any
tile already holding a pile (FR-007 — goods aren't buried).

**Builder-owned chests** (`build_chest`, spec 013 US3): a fifth structure kind
alongside fire/shelter/oven, gated on `chestPlankCost` (6) planks with a
fire-comparable build duration. The builder is recorded as the chest's `Owner`
permanently (no transfer or inheritance in v1) and the chest gets an empty
`Store`, capped at `chestCap` (48, the same derived `bulk()`). `deposit`/
`withdraw` are instant-on-arrival, planner/plan-only goals resolving to the
nearest chest (`withdraw` with a named `Kind` targets the nearest chest actually
holding it); their completions re-validate the chest still stands and truncate
the move to whichever side is tighter — the chest's free space on deposit, the
taker's free bulk on withdraw. A non-owner `withdraw` is theft: never blocked,
always marked — the executor co-emits a companion batch in the same tick
(`social.chest_taken`, a reason-`"theft"` `social.relation_changed`, the owner's
gossip-seed memory, and witness memories for nearby villagers — [[social-fabric]]
has the full mechanics).

**Food rot** (spec 013 US5): on the same per-game-minute boundary the needs
heartbeat uses, `stepEvents` also sweeps every pile's food batches for ones whose
`SpoilAt` has arrived, emitting one `sim.food_rotted` per (pile, kind) with
same-kind spoiled batches merged — a pure function of (state, tick), the
fuel-sweep pattern. Chest food carries no batches and never rots (FR-010).

**Guarded plans** (TASK-32, `plan.go`): a planner reply may land as a short
conditional plan — up to `PlanStepCap` (3) `PlanStep`s, each with a goal, an
optional `When` guard, and an `Until` validity deadline (default window
`PlanDefaultWindowTicks`, 2 game-hours). The steps live on `Agent.Plan` in
deterministic state (`agent.plan_set`); each idle tick the executor evaluates
the head step via `planStepEvents` *before* falling through to the reflex:
holding (guard false, window open) emits nothing, expiry or a failed goal
resolution clears the whole plan with `agent.plan_expired` (a broken sequence
is not resumed), and firing emits `agent.plan_step_started` plus the intent
with source `plan`. No model runs at firing time — timed guards are the sole
act-at-time-T mechanism. `Agent.Generation` (also TASK-32) counts
high-salience interrupts: the reducer bumps it on memories at or above
`GenerationBumpSalience` (9), and in-flight thoughts snapshotted under an
older generation are superseded when they land ([[cognition]]).

**Terrain overlays** (`terrain.go`): chopped trees and harvested forage are
event-sourced state over the static map — `effectiveKind`/`passable` merge
[[worldmap-generation]] with `State.Cleared`/`Harvested`/`Quarried`; forage regrows
after 12 game-hours (`sim.forage_regrown`), dens cool down 6 game-hours after a hunt.
A quarried rock outcrop (spec 012) is different from the other two: it does NOT
revert to Grass — `effectiveKind` renders it as `worldmap.Depleted` permanently (no
regrow in v1), `passable` allows walking across it, but it is neither buildable
(`buildSite`) nor quarryable again. Structures (`fire`, `shelter`, `oven`, `chest`)
exist only in state; `warmAt` is a *lit* fire within Manhattan radius 2, or standing on a
shelter (ovens grant no warmth). `fireStructAt`/`litFireAt` locate a fire by
coordinate and test lit-ness for the refuel/cook completion checks. Spec 032
adds two more structure kinds and the first one to affect pathing: `isWall`
names the wall family (`wall_plank`, `wall_stone`); `passable` now checks
`wallAt(s, x, y) != nil` FIRST and refuses the tile if so — a standing wall is
the one structure family that blocks movement (`buildSite`'s generic
"no structure on this tile" scan already kept walls, like every other
structure, un-buildable-over). `wallMaxHP(kind)` derives each kind's ceiling
(`wallPlankHP` 200, `wallStoneHP` 600) for the build stamp, the repair clamp,
and the TUI's damage styling — never stored separately (`WallMaxHP` exports it
for [[tui-client]]). `agentAt(s, x, y)` backs the wall-build occupancy guard
(FR-007: a wall may never land on a tile holding a living agent). `pathAt(s, x,
y)` reports a `path` structure underfoot — the movement dual-phase cadence's
per-step predicate (above); a path has no `HP` and never blocks (`isWall` is
false for it).

**Hails** (TASK-47, `hail.go`): a `talk_to` landing flags its target down —
`social.hailed` pauses the target for `hailWindowTicks` (480, 8 game-minutes) so
the hailer can close distance. The per-tick `hailStep` sweep runs *before* the
per-agent loop: a hailer within Manhattan 1 of its paused target founds the talk
deterministically (`social.hail_met` + the `talkEvents` shape, bypassing the
ambient `canTalk` cooldown — met is checked before expiry so an on-time arrival
wins the edge tick); otherwise the window closing emits `social.hail_expired`
and the target resumes untouched. A paused agent (`hailPaused`) skips the
reflex, plan-step evaluation, and en-route movement, but keeps decaying,
keeps its intent and plan exactly as they were, and still works if already
standing on its intent target. `hailable` (same file) is the exemption
predicate: dead, asleep, already-hailed, actively-hailing, meeting-pinned, or
beyond `hailRadius` (64) targets are never paused. A plan-step `talk_to` firing
hails exactly as a planner landing does. The ambient beat's talk founding is
shared with the sweep via `talkEvents` (`executor.go`).

The executor also emits `agent.memory_added` events from the salience table in
`memory.go` ([[agent-mind]]) alongside memorable happenings — and since spec 019
(US1) every one is *situated*. The emission sites now go through the situated
constructors (`situatedMemoryEvent`/`situatedMemoryToned`/`situatedMemoryAboutEvent`,
`memory.go`; T008b removed the pre-019 bare `memoryEvent`/`memoryEventToned`/
`memoryAboutEvent` once every site had migrated, so no sim memory can be emitted
unsituated). Each bakes a `Where` — the acting-or-witnessing agent's tile via
`PlaceAt` → `describePlace`, a deterministic Manhattan-radius nearest-feature scan
that names a station ("the fire") or terrain ("the woods") — and, for a driven
personal act, a `Why` (the completing intent's `Reason`, `""` for reflex/witness)
into the `MemoryAddedPayload`, and composes both into the memory text via
`situateText`; the [[chronicle]]/scribe render what the payload carries with no
re-derivation, so replay is byte-identical. Build completions situate through
`placeForBuild`, which excludes the just-built structure kind from the scan so
"Built a fire" resolves to the tile as it was ("at the woods (x,y)"), never
"at the fire" (T024). Gossip/witness memories carry no `Why` — a witness did not
drive the act.

Since spec 030, all three situated constructors also take a required `origin`
parameter — the emission-stamped provenance class the compiler now forces every
call site to declare, so no new memory site can land unstamped. `origin` is a
closed vocabulary (`memory.go`): `OriginAction` (an own executed act),
`OriginWitness` (a seen event — `situatedMemoryAboutEvent`'s usual value),
`OriginReport` (learned of at any distance, e.g. a chest-owner's theft
notification), `OriginOmen` (a delivered omen/dream/miracle), `OriginGist` (a
conversation summary written into memory), and `OriginDigest` (a nightly
day-gist); an absent/legacy origin (`""`, any pre-030 payload) classifies as
secondhand, the conservative direction. `DirectPerception(origin)` is the pure
helper — true only for `OriginAction`/`OriginWitness`/`OriginOmen` — that the
belief validator ([[nightly-consolidation]]) reads to decide whether a memory
counts as direct perception; it is the ONLY signal that decision uses, no text
inspection. `Memory.Origin` (`omitempty`) rides the same copied-at-Apply,
never-re-derived doctrine as `Where`/`Why`/`Conv`, so replay stays byte-identical
and a pre-030 Memory (field absent) reduces to `Origin` `""`. It also regenerates
Metatron's nudge charges (`metatron.charge_regenerated` at absolute 6-game-hour
tick boundaries while below the cap — [[metatron]]) and, per tick, sweeps
`State.MetatronOrders` for any active standing order whose `ExpiresTick` the
new tick has reached, emitting `metatron.order_expired` (spec 029, the
`charge_regenerated` pattern — a pure function of state + tick, so a
lapsed watch reproduces on replay with no angel running — [[metatron-orders]]);
its reflex fires only on agents idle past `reflexGraceTicks` (120). `stepEvents` also runs the
[[gru]]'s whole turn (`gruStep`) each tick, and the heartbeat's near-death memory
names "the gru" as the cause when the last wound was recent. The per-minute social beat
(`socialEvents`, [[social-fabric]]) runs the adjacency ladder — repay an open
debt, give to a starving neighbor, or talk (chat-while-working, cooldown-bounded)
with a verbatim rumor fallback — and the hourly due-check breaks overdue debts
(also emitting a `norm.violated` when a repay-debts norm is in force — [[governance]]).
`stepEvents` further runs the whole governance layer (TASK-13, `governanceEvents` in
`governance.go`): the daily meeting lifecycle — gated since TASK-36 on an
event-sourced meeting convention (convene at the convention's hour with attendee
intent pinning to `attend_meeting`, open, speaking-turn beats, timebox+grace
close; no convention → the per-minute emergent-gathering watch runs instead) —
and the per-minute curfew/exile violation detectors. `attend_meeting` is the one
intent goal the executor sets itself (never planner-choosable): arrival idles at
the meeting place until close, and stale pins clear when the meeting ends.
`stepEvents` stays a pure function of (pre-tick state, map, next tick);
every effect is an event through [[sim-state-reducer]] — the determinism and replay guarantees of
the substrate hold unchanged over the whole layer.

## Connections

[[reflex-policy]] decides what idle agents do (including the v2 fuel/craft/eat
ladder and, via `resolveGoal`, the spec 032 wall/axe/path goals); [[sim-loop]]
drives the tick; [[event-types]] catalogs the event families; the [[gru]] preys
on the bodies at night; [[tui-client]] renders bodies, needs gauges, structures
(including wall HP dimming and path tiles), fire lit/cold state, ground piles,
and chest contents; [[worldmap-generation]] supplies the Rock kind quarry sites
overlay onto; [[social-fabric]] carries the theft companion batch a non-owner
withdrawal triggers; [[nightly-consolidation]] hosts the belief validator that
reads `Origin`/`DirectPerception` off situated memories; [[world-migration]]
re-places carried souls on a fresh v2 map with empty overlays (v1→v2) and, for
the v2→v3 cut, spills any over-cap carry to a pile in place with no land reset.
TASK-7 replaces goal *selection*, never execution.

## Operational notes

A fresh village (seed 42) builds fires within the first game-hour and survives
multiple days unattended. Known day-1 quirk: agents can't see construction in
progress, so several may each build a fire in the same window — wasteful, harmless.
Event volume: ~8 needs events/game-minute (one per living agent) plus movement bursts;
a two-day run is ~100k events. The v2 economy adds a full crafting chain (wood/stone
→ planks/refined_stone → spears/shelter/oven) and a fire that must be refueled or it
goes cold — `whole_feature_test.go` and `food_fire_test.go` exercise the chain and
the fuel sweep end-to-end. The v3 storage economy (spec 013) is exercised by its own
suite — `bulk_cap_test.go`, `ground_pile_test.go`, `chest_test.go`, `theft_test.go`,
`rot_test.go`, `migrate_test.go` — plus an extended `whole_feature_test.go` pass.
Spec 032 (walls, axes, paths) adds `wall_test.go` (blocking/rerouting, occupancy
guard, HP stamping, multi-cycle demolish/repair math and re-arm, replay
determinism), `axe_test.go` (bare-vs-axe yield, ten-use breakage, bulk
truncation, storage round-trip, replay), and `path_speed_test.go` (a paved
corridor halves traversal ticks vs. unpaved) — plus an extended
`whole_feature_test.go` pass exercising all three together. Spec 030's `Origin`
stamping is exercised by `origin_test.go`.
