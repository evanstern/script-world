# Feature Specification: Resources, Food, and Crafting v1

**Feature Branch**: `012-resources-food-crafting`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "Resources, food, and crafting v1 — the resource economy layer designed in TASK-25's design session: stone and water as new gatherable resources, food as a fine-grained abstract unit with a raw/cooked distinction, and a Minecraft-ish crafting layer (two intermediates, five end items: fire, basic shelter, oven, spear, food) on top of the deterministic executor."

## Session Decisions (TASK-25, 2026-07-21)

All directional decisions below were settled in the TASK-25 Socratic design session and
recorded on the board task; this spec turns them into requirements. Numeric values are
pinned as tunable defaults in the Assumptions section.

1. **Stone** enters the world as a new rock-outcrop terrain kind, noise-placed on dry
   land; agents quarry an adjacent outcrop like they chop a tree. This is a
   format-versioned terrain change.
2. **Water** is a gatherable crafting/usage ingredient only — **no thirst need in v1**.
   No container is required to carry water in v1.
3. **Food** becomes a fine-grained abstract unit (berries ≈ 1–2, a rabbit ≈ 8) with a
   raw/cooked distinction; cooking roughly doubles nutritional value.
4. **Crafting** has exactly two intermediates: planks (wood side) and refined stone
   (stone side).
5. **Fire** needs fuel: it burns out unless refueled with wood, and cooks raw food at a
   modest multiplier. Warmth behavior is preserved while lit.
6. **Basic Shelter** is re-costed in planks, keeps its warmth role, adds a rest bonus
   when sleeping there, and is communal (no ownership in v1).
7. **Oven** is a placed station that consumes wood fuel per batch from day one; cooking
   does not require water in v1, but the oven can heat water for a bath (+morale,
   +warmth) — water's only v1 consumer, with future recipes intended.
8. **Spear** is the first carried tool: hunting works bare-handed at modest yield; a
   spear raises the yield and cuts the time, and breaks after a fixed number of hunts.
9. **Execution model**: portable things (planks, refined stone, spear) are hand-crafted
   anywhere as timed work intents; structures (fire, shelter, oven) remain
   build-on-site intents. No craft-then-place mechanic.
10. **Reflex / degraded-mode contract**: the no-planner fallback keeps only the survival
    raw-loop — forage/hunt bare-handed, eat raw, chop, build + refuel fire. Crafting,
    cooking, oven, spear, and shelter are planner-initiated only: degraded mode is
    subsistence living; civilization requires minds.
11. **Storage is out of scope**: carry capacity, stacking, chests, and stockpiles are
    deferred to the TASK-26 inventory & storage spec; this feature treats inventory as
    an abstract "agents hold items" interface.
12. **Migration keeps the people, resets the land** (user, 2026-07-22): existing v1
    worlds (specifically `myworld-01`) get a migration path rather than refusal-only.
    A full map reset is acceptable; the agents are not reset — souls, memories,
    relationships, and the village's lived history carry across the format break.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Stone and water enter the world (Priority: P1)

Every village map now generates rocky outcrops alongside water, woods, and forage, and
villagers can gather both new base resources: quarrying stone from an adjacent outcrop
tile (which depletes it) and collecting water while standing beside a water tile.
Gathered stone and water appear in the villager's carried inventory and are visible to
the player.

**Why this priority**: every crafting recipe downstream depends on stone (refined stone,
oven, spear) or water (bath) existing as gatherable resources. Nothing else in this
feature can function without this layer.

**Independent Test**: start a fresh world; confirm outcrops generate on every seed and
the same seed yields an identical map; direct a villager to quarry and to collect water;
confirm their inventory gains stone and water and the quarried outcrop is depleted.

**Acceptance Scenarios**:

1. **Given** a freshly generated world of any seed, **When** the map is inspected,
   **Then** rock outcrops are present on dry land alongside water, trees, forage, and
   dens, and at least 25% of the map remains open buildable grass.
2. **Given** the same seed, width, and height, **When** the map is generated twice
   (including on different machines), **Then** the two maps are identical.
3. **Given** a villager standing beside a rock outcrop, **When** they quarry it,
   **Then** after the work duration their inventory gains the stone yield, the outcrop
   tile is depleted (no longer quarryable), and the depletion survives replay.
4. **Given** a villager standing beside a water tile, **When** they collect water,
   **Then** after a short work duration their inventory gains one water; the water tile
   is unaffected (water sources are inexhaustible).
5. **Given** two villagers targeting the same outcrop, **When** the first completes the
   quarry, **Then** the second's work re-validates, finds the outcrop gone, and their
   intent resolves without yield (matching today's contested-resource behavior).

---

### User Story 2 - Fine-grained food and cooking at the fire (Priority: P2)

Food stops being "meals" and becomes fine-grained units: foraging yields a couple of
berry-sized units, a hunt yields a rabbit-sized batch. Food is either raw or cooked.
Villagers eat units until sated — raw food restores little per unit, fire-cooked food
about twice as much. A lit fire can cook carried raw food. Fires now consume fuel: a
fire burns for a bounded time and goes cold unless refueled with wood; a cold fire gives
no warmth and cannot cook, and refueling relights it.

**Why this priority**: this is the survival-economy rebalance the rest of the feature
hangs off — the raw/cooked distinction is what makes fire (and later the oven)
mechanically meaningful, and fuel gives wood an ongoing sink.

**Independent Test**: run a village with no crafting: confirm forage/hunt yield the new
unit amounts, eating consumes multiple units to satiety, cooking at a lit fire converts
raw units to cooked units worth double, and an untended fire goes cold on schedule and
resumes on refuel.

**Acceptance Scenarios**:

1. **Given** a hungry villager carrying raw food, **When** they eat, **Then** they
   consume units one meal's worth at a time at the raw per-unit value, stopping when
   sated or out of food, and the resulting need value is recorded absolutely.
2. **Given** a villager carrying raw food beside a lit fire, **When** they cook,
   **Then** after the work duration their raw units convert to fire-cooked units, each
   restoring about double the raw value when eaten.
3. **Given** a fire whose fuel window has elapsed with no refuel, **When** the burnout
   moment passes, **Then** the fire goes cold: it stops granting warmth and refuses
   cooking, and the change is visible to the player and the chronicle.
4. **Given** a cold fire and a villager carrying wood, **When** they refuel it, **Then**
   the fire relights and its fuel window extends by the per-wood burn time, up to a cap.
5. **Given** a village running with no planner (degraded mode), **When** several game
   days pass, **Then** villagers still survive on the raw loop: forage/hunt bare-handed,
   eat raw, chop wood, build and refuel fires. No crafting or cooking occurs.

---

### User Story 3 - The crafting chain: planks, refined stone, spear (Priority: P3)

Villagers can refine raw resources into intermediates anywhere, as timed work: wood into
planks, stone into refined stone. With intermediates in hand they can craft the first
carried tool, the spear. Hunting still works bare-handed at a modest yield; a villager
carrying a spear hunts faster and brings back more, and the spear breaks after a fixed
number of hunts — creating a re-craft loop.

**Why this priority**: this proves the two-step crafting chain (raw → intermediate →
item) end to end with the smallest item, and gives hunting its tool progression.

**Independent Test**: direct a villager to craft planks, refined stone, then a spear;
confirm inventory conversions at each step, the hunt bonus while carrying the spear, and
the spear breaking after its durability is spent.

**Acceptance Scenarios**:

1. **Given** a villager carrying wood, **When** they craft planks, **Then** after the
   work duration one wood converts to four planks, anywhere on the map.
2. **Given** a villager carrying stone, **When** they craft refined stone, **Then**
   after the work duration stone converts to refined stone at the pinned ratio.
3. **Given** a villager carrying the spear recipe's inputs, **When** they craft a spear,
   **Then** the inputs are consumed and a spear with full durability appears in their
   inventory.
4. **Given** a villager carrying a spear, **When** they hunt, **Then** the hunt takes
   less time and yields more raw food than bare-handed, and the spear's remaining uses
   decrease by one; on the hunt that spends its last use, the spear breaks and is
   removed, and the moment is memorable to the villager.
5. **Given** a villager with no spear, **When** they hunt, **Then** the hunt succeeds at
   the modest bare-handed yield (no tool gate on survival).
6. **Given** a village in degraded mode (no planner), **When** any amount of time
   passes, **Then** no crafting intents ever originate from the reflex.

---

### User Story 4 - The oven: meals and baths (Priority: P4)

Villagers can build an oven — the first stone-cost station — on open ground from refined
stone and planks. The oven consumes wood fuel per use. Cooking a batch at the oven turns
raw food into meals, the best food in the game. The oven can also heat water: a villager
with carried water takes a bath, consuming the water and fuel, and comes away warmer and
happier — water's only consumer in v1.

**Why this priority**: the oven is the flagship item that pulls the whole economy
together (stone chain + fuel + food chain + water's first consumer), but everything
about it layers on stories 1–3.

**Acceptance Scenarios**:

1. **Given** a villager carrying the oven recipe's inputs beside open buildable ground,
   **When** they build an oven, **Then** the inputs are consumed and an oven structure
   appears at the site.
2. **Given** a villager at an oven carrying raw food and wood, **When** they cook a
   batch, **Then** one wood fuel is consumed and up to a batch-size of raw units convert
   to meals, each restoring the best per-unit value when eaten.
3. **Given** a villager at an oven carrying water and wood, **When** they bathe, **Then**
   one water and one wood fuel are consumed and the villager gains the pinned morale and
   warmth bumps (capped at full), and the moment is visible to the chronicle.
4. **Given** a villager at an oven with no wood, **When** they attempt to cook or bathe,
   **Then** the intent resolves without effect (fuel is required from day one).

---

### User Story 5 - Shelter joins the plank economy (Priority: P5)

The basic shelter is re-costed from raw wood to planks and becomes worth sleeping in: it
keeps its warmth role and now speeds rest recovery for a villager sleeping there.
Shelters remain communal — anyone may use any shelter.

**Why this priority**: smallest behavioral delta; it makes the plank chain load-bearing
for an existing structure and completes the five-item roster.

**Acceptance Scenarios**:

1. **Given** a villager carrying enough planks, **When** they build a shelter, **Then**
   planks (not raw wood) are consumed and the shelter appears at the site.
2. **Given** a villager sleeping on a shelter, **When** game minutes pass, **Then**
   their rest recovers at the boosted rate (versus the normal rate sleeping rough), and
   warmth behaves as today.
3. **Given** any villager and any shelter, **When** they choose to sleep there, **Then**
   no ownership rule prevents it.

---

### User Story 6 - An old world's people survive the new world (Priority: P6)

The player migrates an existing pre-feature world (the reference case: `myworld-01`,
~3 game days of lived history) into the new format with one command. The villagers —
their memories, beliefs, relationships, debts, rumors, chronicle, and the clock — carry
over intact; the land is reborn under the new generation rules (outcrops included), and
map-bound state (structures, terrain overlays, positions, in-flight intents) resets
with it. The old database is archived in place, so the migration is inspectable and
reversible by restoring the archive.

**Why this priority**: the format break is this feature's own doing; giving a loved
world a door through it is part of shipping the break honestly. Priced last among the
stories because every mechanic must exist before a world can be migrated into them.

**Independent Test**: copy a real v1 world; run the migrate command; confirm the people
and their histories are intact, the map is the new-format regeneration, the archive
exists, and the migrated world runs and replays deterministically.

**Acceptance Scenarios**:

1. **Given** a cleanly-stopped v1 world, **When** the player runs the migrate command,
   **Then** the world opens under the new format with every villager's needs, memories,
   beliefs, narratives, relationships, debts, rumors, secrets, conversation records,
   norms, village charter, Metatron state, chronicle, and current tick/day preserved.
2. **Given** the migrated world, **When** the map is inspected, **Then** it is the
   new-format regeneration of the same seed (outcrops present); terrain overlays,
   structures, and the gru are reset; villagers stand on valid passable tiles with
   cleared intents/plans; carried wood is preserved and legacy food is converted at the
   pinned rate.
3. **Given** the migration ran, **Then** the original database is archived alongside
   the world (restorable), and the migration itself is recorded in the new event log
   such that replay from genesis — with no snapshots at all — reproduces the migrated
   state byte-identically.
4. **Given** a v1 world with events newer than its latest valid snapshot (unclean
   stop), **When** migration is attempted, **Then** it refuses with instructions to
   start+stop the world once under the old binary (producing a covering shutdown
   snapshot) — migration never guesses at un-snapshotted history.
5. **Given** an already-migrated world, **When** migration is attempted again, **Then**
   it refuses (nothing to do; the archive is not overwritten).

---

### Edge Cases

- **Fire burns out mid-cook**: the cook re-validates its station at completion; a fire
  that went cold during the work yields no cooked food (matching the contested-resource
  pattern).
- **Eating overshoot**: eating stops at the need cap; a unit is never consumed if the
  villager is already sated.
- **Stone is finite**: depleted outcrops never regrow in v1. Outcrop coverage is tuned
  so a village cannot realistically exhaust the map's stone; if it happens, crafting
  stalls but survival (which needs no stone) is unaffected.
- **Mixed inventory at death**: a villager dies carrying new resource kinds — same rule
  as today (inventory is lost with the body) until TASK-26 decides otherwise.
- **Old worlds**: the terrain change and food rescale invalidate existing worlds — see
  Assumptions (compatibility story).
- **Fuel cap**: refueling a fire already at its fuel cap consumes nothing and extends
  nothing.
- **Bath while already warm/happy**: effects cap at full; the water and fuel are still
  consumed (villagers may enjoy a bath they didn't strictly need).

## Requirements *(mandatory)*

### Functional Requirements

**Resources & terrain**

- **FR-001**: World generation MUST place rock-outcrop terrain on dry land as coherent
  patches (correlated, not salt-and-pepper), on every seed, while preserving: identical
  maps for identical (seed, width, height) across platforms; presence of water, trees,
  forage, and dens; and ≥25% open buildable grass.
- **FR-002**: Outcrop tiles MUST block movement while standing (like trees) and MUST be
  quarryable by an adjacent villager as a timed work intent yielding the pinned stone
  amount; a quarried outcrop is depleted (passable, not quarryable) permanently in v1,
  and the depletion MUST be event-sourced dynamic state over the static map.
- **FR-003**: A villager adjacent to a water tile MUST be able to collect water as a
  short timed work intent yielding one water; water sources are inexhaustible and
  require no container in v1.
- **FR-004**: Villager inventory MUST track the new resource and item kinds — stone,
  water, planks, refined stone, spear (with remaining uses), and raw/cooked food forms —
  alongside wood, with no carry limits in v1 (capacity is TASK-26's decision).

**Food**

- **FR-005**: Food MUST be denominated in fine-grained abstract units with three
  nutritional forms: raw, fire-cooked, and oven-cooked (meals), restoring the pinned
  per-unit values (raw < fire-cooked < meal, with cooking roughly doubling raw value).
- **FR-006**: Foraging MUST yield the pinned small number of raw units; hunting MUST
  yield the pinned batch of raw units (bare-handed) or the boosted batch (with spear).
- **FR-007**: Eating MUST consume carried units — preferring the most nutritious form
  first — until the villager is sated or out of food, in one action; the outcome MUST be
  recorded as absolute need values (no dice rolls, no deltas in payloads).
- **FR-008**: The hunger threshold and eating behavior MUST keep the degraded-mode
  survival guarantee: a planner-less village survives multiple game days on the raw loop
  alone.

**Fire & fuel**

- **FR-009**: A fire MUST have a bounded fuel window: building it grants the pinned burn
  time; each refuel (a new interaction consuming carried wood) extends it by the
  per-wood burn time up to the pinned cap. Burnout and relight MUST be visible,
  event-sourced happenings.
- **FR-010**: A lit fire MUST grant warmth exactly as today; a cold fire MUST grant
  nothing and refuse cooking. Cold fires persist as structures and relight on refuel.
- **FR-011**: A villager beside a lit fire MUST be able to cook carried raw food into
  fire-cooked food as a timed work intent (no extra fuel cost beyond the fire's burning
  fuel).
- **FR-012**: The reflex (no-planner fallback) MUST keep fires alive: build a fire when
  the cold-night rule triggers (as today) and refuel a dying fire when carrying wood —
  and MUST do nothing else new (no crafting, no cooking, no oven, no shelter, no
  spear).

**Crafting & items**

- **FR-013**: Recipes MUST exist exactly as pinned: wood → planks and stone → refined
  stone (hand-crafted anywhere, timed); spear from wood + refined stone (hand-crafted);
  shelter from planks (build-on-site); oven from refined stone + planks
  (build-on-site); fire from wood (build-on-site, as today).
- **FR-014**: Hand-crafting MUST execute as a timed work intent that re-validates inputs
  at completion, consumes them, and adds outputs to the crafter's inventory — all
  event-sourced with outcome payloads.
- **FR-015**: The spear MUST carry per-item durability: each completed hunt with a spear
  spends one use; spending the last use breaks the spear (removed from inventory) and
  MUST leave the villager a memory of it.
- **FR-016**: A carried spear MUST make hunts strictly better: more yield and less time
  than bare-handed, per the pinned numbers; hunting MUST remain possible bare-handed.
- **FR-017**: The oven MUST consume one wood fuel per batch action (cook or bathe) from
  day one; a batch with no wood MUST resolve without effect. Oven cooking MUST convert
  up to the pinned batch size of raw units into meals in one action.
- **FR-018**: Bathing at an oven MUST consume one water plus the fuel and grant the
  bather the pinned morale and warmth increases, capped at full — water's only consumer
  in v1.
- **FR-019**: Sleeping on a shelter MUST recover rest at the pinned boosted rate;
  shelters MUST remain usable by any villager (no ownership).

**Migration (v1 → v2 worlds)**

- **FR-023**: A migrate command MUST convert a stopped v1 world in place: people-state
  carried (agents' needs/memories/beliefs/narratives/generation, relations, ledger,
  rumors, secrets, conversation records, norms, village charter, Metatron state,
  chronicle ring, tick/day/speed), map-bound state reset (terrain overlays, structures,
  gru, meeting-convention coordinates, intents/plans/hails), villagers re-placed
  deterministically on passable tiles of the regenerated map, wood carried 1:1, legacy
  meal-food converted at the pinned rate.
- **FR-024**: Migration MUST require a latest valid snapshot covering the entire event
  log — where trailing process-bookkeeping events (`daemon.*`, reducer no-ops excluded
  from determinism comparisons) after the snapshot are tolerated, since they carry no
  sim state — and MUST refuse any tail containing sim-affecting events, with
  instructions; v1 events are never replayed under v2 rules. (Amended 2026-07-22:
  the v1 daemon appends `daemon.stopped` after its shutdown snapshot, so exact
  coverage is unsatisfiable for every real cleanly-stopped world — found migrating
  `myworld-01`.)
- **FR-025**: Migration MUST archive the original database intact next to the world
  (the directory remains a complete restorable archive) and MUST refuse to run twice.
- **FR-026**: The migration MUST be recorded as an event in the new log carrying the
  full transformed state, applied wholesale by the reducer — so the log alone (no
  snapshots) reproduces the migrated world byte-identically, preserving the
  log-is-truth invariant across the format break.
- **FR-027**: An un-migrated v1 world MUST still be refused by the v2 daemon (the
  existing unsupported-version error), with the error message naming the migrate
  command as the remedy.

**Minds, events & observability**

- **FR-020**: Every new goal (quarry, collect water, craft planks, craft refined stone,
  craft spear, cook, bathe, refuel, build oven) MUST be choosable by the planner and
  expressible as a guarded plan step; none except fire building/refueling may originate
  from the reflex.
- **FR-021**: Every new happening MUST be a namespaced, canonically-serialized event
  applied through the reducer; unknown new types MUST be no-ops for old replay code;
  payloads MUST record outcomes only. Replay of a new-format log MUST reproduce
  byte-identical state.
- **FR-022**: The player-facing views MUST surface the new layer: outcrops and depleted
  outcrops on the map, fire lit/cold state, ovens, the expanded inventory kinds, and
  chronicle-worthy moments (first oven, spear breaking, baths).

### Key Entities

- **Resource**: a countable inventory kind — wood, stone, water, plus food forms.
  Gathered from terrain (chop/quarry/collect) or converted by recipes.
- **Food unit**: the abstract nutrition currency in three forms (raw, fire-cooked,
  meal), each with a fixed per-unit restore value; yields and values are the economy's
  tuning surface.
- **Intermediate**: a refined resource (planks, refined stone) produced from a raw
  resource by a hand-craft recipe; consumed by end-item recipes.
- **Recipe**: a fixed mapping inputs → outputs with a work duration and an execution
  site rule (anywhere vs. build-on-site vs. at-station).
- **Tool (spear)**: a carried item with per-item durability spent by use; modifies the
  hunt action while carried.
- **Station structure**: a placed structure with usage rules — fire (fuel window,
  warmth, basic cooking), oven (fuel per batch, meals, baths), shelter (warmth, rest
  bonus). Structures exist only as event-sourced state, never in the static map.
- **Rock outcrop**: static terrain kind whose depletion is dynamic event-sourced state,
  parallel to trees/cleared and forage/harvested.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of tested seeds generate maps containing all six terrain features
  (water, trees, forage, dens, outcrops, open grass ≥25%), and identical inputs produce
  identical maps on every platform tested.
- **SC-002**: A planner-less (degraded-mode) village of 8 survives at least 3 full game
  days with zero crafting/cooking events in the log — the subsistence contract holds
  under the new food numbers.
- **SC-003**: A planner-driven village can progress from cold start to a working oven
  serving meals within 2 game days, exercising every recipe in the chain at least once.
- **SC-004**: Replaying any new-format event log reproduces byte-identical world state;
  100% of new event types are no-ops under pre-feature replay code.
- **SC-005**: The cooked pipeline at least doubles nutrition per gathered unit (meal
  restore ÷ raw restore ≥ 2.5, fire-cooked ÷ raw = 2), making cooking observably worth
  the fuel across a day's food intake.
- **SC-006**: A player watching the TUI can distinguish, without reading raw logs: an
  outcrop from a depleted one, a lit fire from a cold one, and what a villager is
  carrying across all new kinds.
- **SC-007**: `myworld-01` (107k+ events, ~3 game days) migrates successfully: 100% of
  villagers retain their memories, beliefs, relationships, and debts; the chronicle and
  clock continue; the migrated world runs under the new rules and replays
  byte-identically from its new log with all snapshots deleted.

## Assumptions

**Pinned tuning defaults** (the spec's single tuning surface; plan phase may adjust
within the decided ratios — cooking ≈ doubles raw, spear strictly better, degraded-mode
survival holds):

- Food restore per unit: raw **+40**, fire-cooked **+80**, meal (oven) **+100** on the
  0..1000 need scale (today: one meal-item = +350). Eating consumes units until food
  need ≥ **900** or inventory empty.
- Yields: forage **2 raw units**; hunt bare-handed **8 raw units** at today's duration;
  hunt with spear **12 raw units** at two-thirds the duration.
- Fire: built with 2 wood as today → **8 game-hours** of fuel (4 per wood); refuel = 1
  wood = +4 game-hours, capped at **12 game-hours** remaining.
- Recipes: 1 wood → **4 planks**; 1 stone → **1 refined stone**; spear = **1 wood + 1
  refined stone**; shelter = **8 planks**; oven = **4 refined stone + 2 planks**; oven
  batch size = **8 raw units** per 1 wood fuel.
- Spear durability: **3 hunts**.
- Bath: 1 water + 1 wood fuel → **+150 morale, +300 warmth** (capped).
- Quarry: yield **2 stone**, duration comparable to chopping; outcrops cover roughly
  **6%** of dry land; depleted outcrops never regrow in v1.
- Water collection: **1 water** per short work action.

**Scope & compatibility**:

- Carry capacity, stacking, storage containers, and stockpiles are out of scope
  (TASK-26); v1 inventories are unbounded and lost on death, as today.
- No thirst need, no new death causes, no water containers in v1.
- **Compatibility story**: this feature is a world-format break — the terrain change and
  food rescale invalidate pre-feature worlds. The format version bumps and the daemon
  refuses un-migrated v1 worlds; a **migrate command** (US6, decision #12) carries a
  world's people across the break while the land resets: snapshot-cut (clean-shutdown
  snapshot required, v1 events never replayed under v2 rules), original database
  archived in place, migration recorded as a full-state event so the new log alone is
  sufficient truth. Legacy meal-food converts at **1 old food → 3 Meals**; wood 1:1.
  Cross-version replay equivalence (replaying v1 events with v2 code) is explicitly
  not promised — the archive preserves the old history verbatim instead.
- The planner's action vocabulary and prompt grow to cover the new goals; prompt-side
  budget/shaping details are the plan phase's concern.
- The gru, social fabric, governance, and consolidation systems are untouched except
  where new memories/chronicle moments naturally feed them.
