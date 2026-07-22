# Feature Specification: Inventory & Storage v1

**Feature Branch**: `013-inventory-storage`

**Created**: 2026-07-21

**Status**: Draft

**Input**: User description: "Inventory and storage v1 — the companion layer to resources/food/crafting (specs/012): a single bulk carry cap on every villager, emergent ground piles and stockpile zones (no player zoning), builder-owned chests with recorded-but-unprevented theft, death drops, and food rot that gives chests a mechanical job."

## Session Decisions (TASK-26, 2026-07-21)

All directional decisions were settled in the TASK-26 Socratic design session (recorded
on the board task). Two were pre-decided when the task was filed: **both chests and
stockpile zones exist**, and **there is no player zoning** — agents place and organize
storage themselves; the player is never in charge of layout.

1. **Carry capacity** is a single integer bulk cap: every resource unit and every item
   costs 1 bulk; a villager carries at most the cap. One number to tune, trivially
   deterministic.
2. **Stockpiles are emergent from drops**: a new drop/deposit-on-ground action creates a
   ground pile on the villager's tile; piles on adjacent tiles read as one stockpile
   zone. Where agents choose to drop IS the zoning — no governance machinery, no player
   input.
3. **Ownership**: a chest remembers its builder as owner (personal); ground piles are
   commons — anyone takes freely.
4. **Theft is recorded, never prevented**: taking from another villager's chest always
   works mechanically, but emits a witnessed-style happening — the owner and nearby
   witnesses remember it (gossip seeds), and trust toward the taker drops through the
   existing relation machinery. No permission gates; anti-theft norms can emerge later
   through governance.
5. **The reflex ignores storage**: deposits and withdrawals are planner-only, matching
   spec 012's degraded-mode contract (subsistence living needs no infrastructure). The
   bulk cap is sized so the raw survival loop never jams.
6. **Chests are built on site** (consistent with 012's no craft-then-place rule) from
   planks, with **finite capacity** — "the chest is full" is a real state.
7. **Death drops**: a villager's carried bulk spills as a ground pile at the death site,
   recoverable by anyone.
8. **Food rots on the ground but not in chests**: food in ground piles spoils after a
   rot window; chests preserve it; non-food never decays. Chests thereby have a
   mechanical job (the larder) beyond ownership.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Villagers can only carry so much (Priority: P1)

Every villager has a single bulk capacity covering everything they hold — resources,
food in all forms, and tools alike, one bulk each. Gathering that would exceed the cap
simply doesn't happen (the work completes with no yield or isn't chosen), eating frees
space, and the survival loop is never jammed by a full pouch.

**Why this priority**: the cap is the constraint that makes every other storage decision
meaningful — without it, piles and chests are decoration.

**Independent Test**: fill a villager to the cap; confirm further gathering yields
nothing, eating reduces carried bulk, and a planner-less village still survives multiple
game days under the cap.

**Acceptance Scenarios**:

1. **Given** a villager at the bulk cap, **When** a gather work completes, **Then** no
   yield is added (the resource is not consumed/depleted) and the intent resolves.
2. **Given** a villager near the cap, **When** a gather completes whose yield would
   overflow, **Then** the yield is truncated to the remaining space and the remainder
   stays in the world (tree/outcrop yields that don't fit are forfeit; the den/patch
   behaves as harvested).
3. **Given** a hungry villager at the cap carrying food, **When** they eat, **Then**
   consumed units free bulk immediately.
4. **Given** a planner-less (degraded-mode) village, **When** 3+ game days pass,
   **Then** all villagers survive on the raw loop without any storage interaction — the
   cap never deadlocks survival.

---

### User Story 2 - Ground piles and emergent stockpiles (Priority: P2)

A villager can put down part of what they carry, creating a ground pile on their tile;
anyone can pick up from any pile. Villagers dropping near each other create adjacent
piles that read as one stockpile zone — the village's storage layout emerges from where
its people choose to drop things. When a villager dies, everything they carried spills
as a pile where they fell.

**Why this priority**: piles are the commons and the substrate chests build on; death
drops reuse the same mechanic for the game's most dramatic recoveries.

**Independent Test**: direct a villager to drop goods; confirm a pile appears, another
villager can take from it, adjacent drops cluster, and a death spills a lootable pile.

**Acceptance Scenarios**:

1. **Given** a villager carrying goods on a passable tile, **When** they drop a chosen
   amount, **Then** a ground pile holding those goods exists on that tile and the
   villager's bulk decreases accordingly.
2. **Given** an existing pile on a tile, **When** more goods are dropped there (by
   anyone), **Then** the pile accumulates them (one pile per tile).
3. **Given** a pile, **When** any villager standing on/adjacent to it picks up goods
   (within their bulk space), **Then** the goods move to their inventory; an emptied
   pile disappears.
4. **Given** a villager carrying goods, **When** they die (any cause), **Then** a pile
   holding their full carried inventory appears at the death site.
5. **Given** piles on adjacent tiles, **When** the player views the map, **Then** they
   read as one stockpile zone (a rendering/observability grouping — piles need no zone
   entity in state).

---

### User Story 3 - Chests: the village learns to keep things (Priority: P3)

A villager can build a chest from planks. The chest remembers its builder as its owner.
Anyone can deposit into or withdraw from any chest — but a chest has finite room, and
what happens socially depends on whose chest it is (US4). Food stored in a chest keeps
indefinitely.

**Why this priority**: the first owned container; needs piles (P2) conceptually and the
planks chain from spec 012.

**Acceptance Scenarios**:

1. **Given** a villager carrying the chest recipe's planks beside buildable ground,
   **When** they build a chest, **Then** the planks are consumed and a chest structure
   owned by the builder appears at the site.
2. **Given** a chest with space and a villager with goods, **When** they deposit,
   **Then** chosen goods move from inventory to chest, bounded by chest capacity
   (partial deposits allowed).
3. **Given** a chest holding goods, **When** a villager withdraws, **Then** chosen goods
   move to their inventory, bounded by their bulk space.
4. **Given** a full chest, **When** a deposit is attempted, **Then** nothing moves
   beyond capacity and the excess stays carried.
5. **Given** food in a chest, **When** any amount of time passes, **Then** it never
   spoils.

---

### User Story 4 - Theft is a story, not an error (Priority: P4)

Taking from someone else's chest always works — and always leaves a mark. The owner and
any witnesses remember it; word spreads through the existing rumor machinery; trust
toward the taker drops. The village decides what to do about thieves itself (perhaps,
one day, with a norm).

**Why this priority**: the social payoff of ownership; layers on US3 and the existing
social fabric without new enforcement machinery.

**Acceptance Scenarios**:

1. **Given** a villager withdrawing from a chest they don't own, **When** the withdrawal
   completes, **Then** a distinct taking-from-owned-chest happening is recorded, with
   the owner and taker identified.
2. **Given** such a taking, **When** it lands, **Then** the owner gains a memory of it
   (gossip-seeded: subject = taker, negative tone) regardless of distance, and any
   villager within witness range gains a witness memory.
3. **Given** such a taking, **When** relations update, **Then** owner→taker trust drops
   through the existing relation-change machinery with a theft reason.
4. **Given** an owner withdrawing from their own chest, **When** it completes, **Then**
   no social happening is recorded (it's just fetching your things).

---

### User Story 5 - Rot: the ground is not a larder (Priority: P5)

Food left in ground piles spoils after a couple of game days and vanishes from the pile;
non-food (wood, stone, water, planks, refined stone, tools) never decays. The player can
see piles, chests, and what's in them, and the chronicle can tell storage stories (the
first chest, a theft, a friend's effects recovered).

**Why this priority**: closes the loop that gives chests their mechanical purpose;
pure pressure/observability layered on everything prior.

**Acceptance Scenarios**:

1. **Given** food dropped in a ground pile, **When** the rot window elapses, **Then**
   that food batch is removed from the pile as a visible, event-sourced happening.
2. **Given** non-food goods in any pile, **When** any amount of time passes, **Then**
   they remain.
3. **Given** the TUI, **When** the player inspects the world, **Then** piles and chests
   are visible on the map, their contents and (for chests) owner are inspectable, and
   carried bulk per villager is visible.

---

### Edge Cases

- **Drop on an occupied tile**: piles coexist with agents and with structures' tiles
  only where passable; dropping is allowed anywhere the villager can stand (one pile
  per tile, accumulating).
- **Building on a pile's tile**: build-site validation treats a tile with a pile as
  blocked for construction (goods aren't buried).
- **Withdraw more than fits**: transfers truncate to available space (carry or chest) —
  never an error, never a loss.
- **Chest owner dies**: the chest keeps its owner record; taking from a dead owner's
  chest still records the happening (the village remembers whose it was). No
  inheritance in v1.
- **Two villagers, one pile, same tick**: deterministic agent-order arbitration, as
  with every contested resource today; a second taker finds whatever remains.
- **Rot mid-pickup**: rot applies on its timer tick; a pickup that lands first wins the
  food — same re-validation pattern as contested gathers.
- **Spear in storage**: tools carry their remaining durability with them through
  piles/chests (durability is the item's, not the holder's).
- **Cap smaller than a hunt yield**: yields truncate (US1); tuning keeps the cap
  comfortably above the largest single yield.

## Requirements *(mandatory)*

### Functional Requirements

**Carry capacity**

- **FR-001**: Every villager MUST have a single integer bulk capacity; every resource
  unit and every item MUST cost exactly 1 bulk; carried bulk MUST never exceed the cap.
- **FR-002**: Gathering MUST truncate yields to the taker's free bulk (forfeiting the
  remainder) and MUST resolve without error at zero space; eating MUST free the bulk of
  consumed units.
- **FR-003**: The cap MUST be tuned so the degraded-mode raw loop (spec 012's reflex
  vocabulary) never deadlocks: a planner-less village survives 3+ game days with no
  storage interactions.

**Piles & stockpiles**

- **FR-004**: A villager MUST be able to drop a chosen subset of carried goods as a
  timed-instant action, creating or adding to the one ground pile on their tile; any
  villager MUST be able to pick up from a pile on/adjacent to their tile, bounded by
  free bulk. Both are planner-only goals (never reflex).
- **FR-005**: Ground piles are commons: no ownership, no social effects from taking.
- **FR-006**: A villager's death MUST spill their entire carried inventory into the
  pile at their death tile (creating it if absent).
- **FR-007**: Piles MUST be event-sourced state over the map (no static-map changes);
  an emptied pile MUST cease to exist; construction MUST NOT be sited on a pile's tile.

**Chests**

- **FR-008**: A chest MUST be a build-on-site structure costing the pinned planks,
  recording its builder as owner permanently (no transfer, no inheritance in v1).
- **FR-009**: A chest MUST have the pinned finite capacity in bulk; deposits and
  withdrawals (planner-only goals) MUST truncate to available space on either side and
  never destroy goods.
- **FR-010**: Food in a chest MUST never spoil.

**Theft & social fabric**

- **FR-011**: A withdrawal from a chest by a non-owner MUST be recorded as a distinct
  taking happening identifying owner and taker; owner-from-own-chest MUST NOT be.
- **FR-012**: The taking happening MUST leave the owner a negative, gossip-seeded
  memory (subject = taker), give witnesses in range a witness memory, and drop
  owner→taker trust via the existing relation-change machinery with a theft reason —
  and MUST NOT block or undo the transfer. No permission system exists in v1.

**Rot**

- **FR-013**: Each food batch dropped in a ground pile MUST spoil (be removed, visibly
  and event-sourced) after the pinned rot window; non-food MUST never decay anywhere.

**Minds, events & observability**

- **FR-014**: All storage goals (drop, pick up, deposit, withdraw, build chest) MUST be
  planner-choosable and guarded-plan-expressible; NONE may originate from the reflex.
- **FR-015**: Every storage happening MUST be a namespaced event through the reducer
  with outcome-only payloads; unknown types MUST no-op under old replay code; replay
  MUST reproduce byte-identical state, including pile contents, chest contents/owner,
  and rot timers.
- **FR-016**: The player-facing views MUST show piles and chests on the map, their
  contents and chest ownership on inspection, and each villager's carried bulk; theft
  and rot happenings MUST be chronicle-visible.

### Key Entities

- **Bulk**: the single carry currency; 1 per resource unit or item, capped per villager.
- **Ground pile**: per-tile commons container of goods; created by drops or death;
  vanishes when emptied; food batches inside carry rot deadlines.
- **Stockpile zone**: an observability grouping of adjacent piles — a rendering concept,
  not a state entity.
- **Chest**: built structure with finite bulk capacity and a permanent owner (builder);
  preserves food; contents are event-sourced state.
- **Taking happening**: the recorded theft-flavored event (owner, taker, goods) feeding
  memories, rumors, and trust via existing social machinery.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A planner-less village of 8 survives 3+ full game days under the bulk cap
  with zero storage events in the log — the cap never breaks the subsistence contract.
- **SC-002**: A planner-driven village demonstrably uses the full storage loop within 2
  game days of having planks: at least one chest built, deposits, withdrawals, and at
  least one ground pile/stockpile in active use.
- **SC-003**: 100% of non-owner chest withdrawals produce the taking record, an owner
  memory, and an owner→taker trust drop; 0% are blocked.
- **SC-004**: Food in ground piles is gone within the rot window +1 game-minute in 100%
  of cases; food in chests persists indefinitely; non-food never decays.
- **SC-005**: Replay of any log reproduces byte-identical state including every pile,
  chest, owner, and rot deadline; all new event types no-op under pre-feature replay
  code.
- **SC-006**: A player can answer, from the TUI alone: where the village stores things,
  what's in a given pile/chest, who owns a chest, and how full a villager's hands are.

## Assumptions

**Pinned tuning defaults** (tunable in plan phase within the decided shape):

- Bulk cap: **24** per villager (comfortably above the largest single yield — a spear
  hunt's 12 raw food — and roomy enough for the raw loop's wood + food juggling).
- Chest: recipe **6 planks**; capacity **48 bulk**; build duration comparable to a fire.
- Rot window: **2 game days** per food batch on the ground, denominated in ticks.
- Drop/pick-up/deposit/withdraw: instant-on-arrival actions (like eating), not timed
  work.
- Witness range for takings: the existing witness radius.

**Scope & dependencies**:

- Depends on spec 012 (planks for chests; the expanded inventory kinds; the fine-grained
  food units that make the cap meaningful). The chest is one new craftable beyond 012's
  five items, owned by this feature.
- The bulk cap lands as part of this feature, not 012 — 012's inventories stay
  unbounded until this layer arrives (012 ships first).
- No permission systems, no zoning UI, no inheritance, no water containers, no per-kind
  weights in v1. Norms about theft are governance's future business, not this spec's.
- Carrying-capacity effects on movement speed are out of scope (bulk never slows you).
