# Feature Specification: Metatron Miracles

**Feature Branch**: `016-metatron-miracles`

**Created**: 2026-07-22

**Status**: Draft

**Input**: User description: "Metatron miracles: give the angel a small, mediated intervention vocabulary plus a player-side force door — per TASK-59 on the Backlog board. Metatron remains the only 'hands' in the system (no God panel). Three miracle abilities, all landed as recorded, replay-deterministic events through the existing injection door following the nudge pattern: (1) time snap with shift semantics, (2) item grant, (3) entity move/remove. The angel spends charges in normal play; the player console can land the same events gratis (cheat mode) — bypassing only the charge cost, never validation or the log."

## Clarifications

### Session 2026-07-22

- Q: How many charges should each miracle cost in normal play? → A: Tiered — time snap costs 2; item grant, entity move, and entity remove cost 1 each (same as a nudge).
- Q: What happens to the contents of a removed storage structure? → A: Contents spill to a ground pile on the same tile; nothing is silently destroyed.
- Q: What happens to a villager's in-flight intent when the angel moves them? → A: The move cancels the intent; the villager reassesses and replans from the new tile.
- Q: Do villagers perceive miracles that directly affect them (move, item grant)? → A: Yes — the miracle also lands a brief memory/observation for each directly affected villager, so minds stay coherent with the world. Applies to charged and gratis miracles alike.
- Q: What console surface does the player use to land miracles directly? → A: A dedicated operator subcommand (`promptworld miracle <world> …`), visibly separate from the in-fiction angel chat console; gratis/force is a flag on that subcommand only.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Rescue a stuck villager (move/remove an entity) (Priority: P1)

A villager is stuck — wedged by terrain, planner failure, or a live defect — and the player
wants them relocated without stopping the world or editing its history by hand. The player
asks Metatron to move them; the angel spends a charge and the villager *snaps* to the new
tile. The same ability moves structures and ground piles, and removes structures, piles,
and terrain features (trees, forage, rock outcrops). It can never remove a villager.

**Why this priority**: this is the incident that motivated the feature (2026-07-22: villager
Ash had to be rescued by stopping the daemon and hand-appending an event to the log). It is
the smallest slice that makes the system operable without out-of-band surgery.

**Independent Test**: with a world running, ask the angel to move a villager to a named
tile; observe the villager standing there on the next tick, a recorded miracle event in
the log, and one charge spent. Restart the world and observe the same position after
recovery replay.

**Acceptance Scenarios**:

1. **Given** a living villager at (44,8) and at least one banked charge, **When** the angel moves them to a passable, in-bounds tile, **Then** the villager occupies the destination on the next tick, any in-flight intent is cancelled (they replan from the new tile), one charge is spent, and the move is recorded in the event log.
2. **Given** a move targeting an impassable or out-of-bounds destination, **When** the miracle is attempted, **Then** it is rejected at the door with a reason, no state changes, and no charge is spent.
3. **Given** a structure or ground pile at (x,y), **When** the angel moves it to a valid site, **Then** it stands at the destination with its contents intact.
4. **Given** a removable entity (structure, pile, or terrain feature) at (x,y), **When** the angel removes it, **Then** the tile's effective terrain reflects the removal and the event is recorded.
5. **Given** a remove request targeting a villager, **When** the miracle is attempted, **Then** it is rejected — villagers cannot be removed in this version.
6. **Given** a tile holding more than one entity class (e.g. a villager standing on a pile), **When** a move or remove is requested, **Then** the request names the entity class explicitly and only that entity is affected.

---

### User Story 2 - Operator force door ("cheat" mode, gratis miracles) (Priority: P2)

The player, acting as operator, needs to land a miracle *right now* regardless of the
charge bank — e.g. rescuing a villager when the angel is out of charges. A player-only
console path lands the same miracle events marked gratis: the charge cost is waived, but
validation still applies and the act still lands in the permanent record. The angel's
model-mediated turn can never mint a gratis miracle.

**Why this priority**: without it, an out-of-charges emergency still forces the stop-the-world,
hand-edit-the-log surgery this feature exists to eliminate. It is also the boundary-critical
slice: the isolation rule (only the player may waive cost) must hold from day one.

**Independent Test**: with zero banked charges, land a gratis move from the player console
and observe it succeed with the charge bank untouched; then cause the angel's model output
to claim gratis and observe it stripped or rejected.

**Acceptance Scenarios**:

1. **Given** zero banked charges, **When** the player lands a gratis miracle from the console path, **Then** it applies, the charge bank stays at zero, and the recorded event is marked gratis.
2. **Given** a gratis miracle that fails validation (e.g. move to an impassable tile), **When** it is attempted, **Then** it is rejected exactly as a charged miracle would be — gratis waives cost only, never validity.
3. **Given** model output from the angel's turn that includes a gratis marker, **When** the turn lands, **Then** the gratis marker is stripped or the batch rejected; the model path can only ever spend charges.
4. **Given** any gratis miracle has landed, **When** the world's history is reviewed (log, chronicle digest), **Then** the gratis act is visible there like any other miracle.

---

### User Story 3 - Time snap (Priority: P2)

The player asks the angel to jump the world clock to a specific day/hour/minute — *snap*,
and it is now 11:30 tomorrow. Nothing is simulated for the skipped interval: the world is
genuinely frozen and only the clock label changes ("shift" semantics). Fires do not burn
out, food does not rot, hunger does not advance, and the skipped hours mint no regeneration
charges. Afterward the world behaves exactly as it would have, offset in time.

**Why this priority**: high-value for pacing and demos, but the world stays operable
without it, and it carries the largest correctness surface (every time-anchored behavior
must survive the jump).

**Independent Test**: run two copies of a world from the same state — one snapped forward,
one not — and verify their subsequent behavior is identical except for the clock offset.

**Acceptance Scenarios**:

1. **Given** a running world at day 1, 21:57, **When** the angel snaps time to day 2, 11:30, **Then** the clock reads day 2, 11:30 and no villager has moved, eaten, hungered, worked, or spoken on account of the skipped interval.
2. **Given** in-flight timed activity (a lit fire, ripening rot, an intent mid-work), **When** time snaps forward, **Then** every such activity retains exactly the remaining duration it had before the snap.
3. **Given** a snap that skips across one or more charge-regeneration boundaries, **When** it lands, **Then** the charge bank is unchanged by the skipped boundaries — a snap never pays for itself.
4. **Given** a snap targeting a time at or before the current moment, **When** it is attempted, **Then** it is rejected — the clock only moves forward.
5. **Given** a world that snapped forward, **When** the world restarts and rebuilds from its history, **Then** it recovers to the identical post-snap state.

---

### User Story 4 - Item grant (Priority: P3)

The player asks the angel to provision a villager — give Ash two rations, give Sage a
spear. The angel spends a charge and the items appear in the villager's carry. Grants
respect the world's rules: only known item kinds, only living villagers, never beyond
what the villager can carry.

**Why this priority**: useful for unblocking scenarios (a crafter missing inputs, a
starving villager), but narrower in impact than rescue, force door, or time control.

**Independent Test**: grant a known item to a living villager and observe it in their
inventory on the next tick; attempt an over-capacity grant and observe rejection with
no partial delivery.

**Acceptance Scenarios**:

1. **Given** a living villager with free carrying capacity and a banked charge, **When** the angel grants them a known item and quantity, **Then** the items are in the villager's possession on the next tick and one charge is spent.
2. **Given** a grant that would exceed the villager's carrying capacity, **When** it is attempted, **Then** it is rejected whole — no partial delivery, no charge spent.
3. **Given** a grant naming an unknown item kind or a dead villager, **When** it is attempted, **Then** it is rejected with a reason.

---

### Edge Cases

- Move destination is momentarily occupied by another villager: villagers may share a tile (movement rules do not reserve tiles), so the move lands; the spec imposes no exclusivity.
- Removing a storage structure that holds goods: the goods spill to a ground pile on the same tile (clarified 2026-07-22; never silently destroyed).
- Removing a terrain feature that already has a depletion/clearing overlay (e.g. removing an already-cleared tree): rejected as a no-op target — there is nothing there to remove.
- Time snap requested while the world is paused: allowed; the clock label changes and the world stays paused.
- Time snap landing exactly on a regeneration boundary: the boundary belongs to the skipped interval and mints nothing.
- Two miracles race (angel turn and player console at the same moment): they serialize through the single injection door; each validates against the state the previous one produced.
- The angel is asked for a miracle with no charges banked (non-gratis): the attempt is refused before anything lands, and the angel can say so in-fiction.
- A miracle names a tile holding nothing of the named entity class: rejected with a reason, no charge spent.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST support three miracle families — time snap, item grant, and entity move/remove — each landed as a recorded, replayable event through the same validated injection door as existing angel nudges.
- **FR-002**: Every miracle MUST be validated before it lands (dry-run at the door) and MUST be rejected whole on any validation failure: no partial application, no charge spent, and a stated reason returned to the invoker.
- **FR-003**: Every landed miracle — charged or gratis — MUST appear in the permanent event record and be visible to the world's history surfaces (log, chronicle digest).
- **FR-004**: A world rebuilt from its record (snapshot + replay) MUST reach a state identical to the live world after any sequence of miracles.
- **FR-005**: Charged miracles MUST spend from the existing charge bank under the existing regeneration rules, at tiered prices — time snap costs 2 charges; item grant, entity move, and entity remove cost 1 charge each; a miracle attempted with an insufficient bank MUST be rejected.
- **FR-006**: A gratis marker on a miracle MUST waive only the charge cost. Validation, recording, and replay behavior MUST be identical to a charged miracle.
- **FR-007**: Only the player-facing console/operator path MAY set the gratis marker. The model-mediated angel turn MUST strip or reject any gratis marker present in model output — there MUST be no path from model output to a cost-free miracle.
- **FR-008**: Time snap MUST move the clock only forward, to an explicit day/hour/minute; a target at or before the current moment MUST be rejected.
- **FR-009**: Time snap MUST use shift semantics: no simulation occurs for the skipped interval, and every relative duration in progress (work underway, idle grace, cooldowns, fire life, ripening/rot, regrowth, courtesy pauses, debt deadlines, and any future duration-anchored state) retains exactly its pre-snap remaining duration. Historical stamps (memories, chronicle, conversation records) keep their original times. Clock-phase-derived behavior (day/night, scheduled gathering times, charge-regeneration boundaries) follows the new clock time. For a snap whose distance is a whole number of days (clock phase preserved), the world's subsequent behavior MUST be identical to its un-snapped behavior modulo the clock offset.
- **FR-010**: Time snap MUST NOT change the charge bank: regeneration boundaries inside the skipped interval mint nothing.
- **FR-011**: Item grant MUST target a living villager with a known item kind and positive quantity, and MUST be rejected whole if the grant would exceed the villager's carrying capacity (reject, never clamp).
- **FR-012**: Entity move MUST support villagers, structures, and ground piles; the destination MUST be validated by the same rules the world already uses for that entity class (walkability for villagers, siting rules for structures and piles), and moved containers keep their contents. Moving a villager MUST cancel their in-flight intent so they reassess and replan from the new tile.
- **FR-013**: Entity remove MUST support structures, ground piles, and terrain features (routing terrain removal through the world's existing clearing/harvest/depletion vocabulary), and MUST NOT accept a villager as a target in this version. Removing a storage structure that holds goods MUST spill those goods to a ground pile on the same tile — contents are never silently destroyed.
- **FR-014**: Move and remove requests MUST identify their target unambiguously as (coordinates + entity class); a request whose named class is absent at the coordinates MUST be rejected.
- **FR-015**: The angel MUST be able to invoke all three miracle families from its normal mediated turn (spending charges), and MUST be able to decline or report failure in-fiction when a miracle is rejected or unaffordable.
- **FR-016**: The player MUST have a direct operator path — a dedicated console subcommand, separate from the in-fiction angel chat — that can land any of the three miracle families, with or without the gratis marker, without going through the model.
- **FR-017**: Miracles MUST serialize through the single existing injection door so that concurrent invocations validate against a coherent, current state.
- **FR-018**: A miracle that directly affects a villager (moving them, granting them items) MUST also land a brief recorded memory/observation for that villager, for charged and gratis miracles alike, so the villager's mind stays coherent with the changed world.

### Key Entities

- **Miracle**: a recorded intervention event — one of time-snapped, item-granted, entity-moved, entity-removed — carrying its parameters, the gratis marker (player path only), and landing tick.
- **Charge bank**: the existing bounded pool the angel spends from; regenerates on fixed clock boundaries; unchanged in cap or regeneration rules by this feature.
- **Gratis marker**: a flag on a miracle meaning "cost waived by the operator"; settable only from the player console path; visible in the record.
- **Entity class**: the addressing category for move/remove — villager, structure, ground pile, terrain feature — disambiguating what "the thing at (x,y)" means.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A stuck villager can be relocated in under one minute of player effort, with the world running — no daemon stop, no manual record surgery (versus the 2026-07-22 incident, which required both).
- **SC-002**: After any sequence of miracles (charged and gratis), a world restarted from its record recovers byte-identical state, demonstrated by the project's existing replay-determinism test pattern extended to every new miracle type.
- **SC-003**: For a whole-day snap, a snapped world and an un-snapped control world, driven identically from the same starting state, exhibit identical behavior modulo the clock offset (drift test passes); for arbitrary snaps, every relative duration in progress is proven to retain its remaining time.
- **SC-004**: 100% of landed miracles are discoverable in the world's history surfaces; a reviewer can enumerate every gratis act after the fact.
- **SC-005**: Zero paths exist from model output to a cost-free miracle: adversarial model output claiming gratis is demonstrably stripped or rejected in tests.
- **SC-006**: All existing world behavior is unchanged when no miracle is invoked: the full pre-existing test suite passes without modification (beyond additions).

## Assumptions

- **Backward time travel is out of scope permanently**, not just v1: the record is append-only and the clock monotonic; "undo" is a different feature if it ever exists.
- **Villager despawn/removal is a future feature** with its own spec (narrative weight: relations, conversations, inventory disposition). This spec only guarantees move-not-remove for villagers.
- **How the angel's model output expresses a miracle** (extension of its existing structured reply contract) is a design-phase decision; this spec requires only that all three families are expressible and gratis is not.
- **Charge regeneration boundaries stay anchored to the absolute clock** (a pure function of game time, unchanged by this feature). Boundaries inside the skipped interval mint nothing (FR-010); the next regeneration after a snap arrives at the next absolute boundary, which may be sooner than a full regeneration period after the snap.
- The existing charge cap (3) and regeneration cadence (one per 6 game hours) are unchanged.
