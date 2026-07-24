# Feature Specification: Metatron Agency — Standing Orders, Omens & Visions

**Feature Branch**: `task-27-metatron-agency`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "Metatron v2 agency: evolve the angel from a single-turn console responder into a long-running agent whose only world-action paths are registered tools. Scope per TASK-27's recorded design decisions, re-grounded on the shipped spec 014/017/021 substrate: (1) nudge taxonomy becomes send_omen (night-only, one villager or a named group) + send_vision (one villager, any time); the dream form retires; a daytime send_omen auto-defers to the next sim.night_started as a system-generated standing order; 1 charge per landed omen/vision including triggered ones. (2) monitor_and_act(condition, action_prompt) places an event-sourced standing order: one model call at placement compiles the NL condition to structural predicates (event_types, agent, keywords) evaluated free in Go inside Metatron's Observe path — never per-event model evaluation without a structural filter; fuzzy conditions compile to a coarse filter plus a rate-capped confirm call on a new KindMetatronWatch routed cheap; uncompilable conditions are refused with counsel. Standing orders are event-sourced state (metatron.order_placed/triggered/cancelled/expired) riding State through snapshots/replay, concurrent cap ~3, TTL in game-days. (3) A triggered order executes as a system-authored turn through the same single-flight turn path -> normal tool loop -> lands as a recorded injection, appends to transcript, and queues as a moment so the player sees what the angel did while away. (4) Meta tools pause/start/adjust_speed wrap the existing loop controls via a small loop-control interface; charge-free; the fixed frame pins them to player-requested or standing-order-authorized use only. (5) Budget honesty: triggered turns respect ErrBudgetExhausted/ErrTierDown like console turns — an order firing on an empty charge bank or exhausted budget queues an honest moment ('strength was spent') instead of acting or retry-looping."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Omens and visions replace dreams (Priority: P1)

The player asks the angel to influence the village and the angel now acts through
two clearly distinct mediated forms: an **omen** — a portent that lands at night on
one villager, a named group of villagers, or everyone — and a **vision** — a waking
revelation delivered to exactly one villager at any hour. The old "dream" form
disappears from the angel's vocabulary. Each landed omen or vision costs exactly one
charge, no matter how many villagers an omen reaches.

**Why this priority**: this is the angel's core act vocabulary — every other story
(deferral, standing orders, triggered turns) delivers its payoff through these two
forms. Nothing else in the feature can be demonstrated without them.

**Independent Test**: on a running world at night, ask the angel to send an omen to
two named villagers and a vision to a third during the day; verify both land as
recorded influences, each spends one charge, and the villagers' memories carry the
correct prefixed form. Ask for a "dream" and verify the angel maps the request onto
one of the two living forms rather than a retired one.

**Acceptance Scenarios**:

1. **Given** a world where it is currently night and the charge bank holds ≥1,
   **When** the angel sends an omen naming two living villagers, **Then** one
   influence event lands atomically covering both villagers, exactly one charge is
   spent, and each named villager gains an omen-prefixed memory.
2. **Given** a world at any time of day with charges available, **When** the angel
   sends a vision to one living villager, **Then** the vision lands, one charge is
   spent, and only that villager gains a vision-prefixed memory.
3. **Given** the angel attempts a vision naming two villagers or an omen naming a
   dead villager, **When** the act is validated, **Then** it is refused with
   in-fiction counsel, nothing lands, and no charge is spent — and the refusal is
   fed back so the angel may repair the call within the same console turn.
4. **Given** a world upgraded from an earlier save whose history contains landed
   dream nudges, **When** the world replays from genesis, **Then** replay reproduces
   the historical state exactly (old dream events still count and spend as they
   did), while the angel's current vocabulary offers only omen and vision.

---

### User Story 2 - Standing orders via monitor_and_act (Priority: P1)

The player tells the angel "when Rowan next falls asleep, send her a comforting
vision" and walks away. The angel places a **standing order**: the condition is
compiled once, at placement, into structural filters (event types, subject villager,
keywords) that are checked for free as world events stream past. The player can ask
what orders stand and cancel them. Orders expire on their own after a bounded number
of game days, and only a small number may stand at once.

**Why this priority**: this is the feature's namesake — long-running agency. The
placement/lifecycle machinery is the substrate the triggered-execution story and the
deferral story both ride.

**Independent Test**: place an order with a structurally compilable condition;
verify the order appears in the angel's status, survives a daemon restart and a
from-genesis replay, expires after its TTL, and that a fourth concurrent order is
refused with counsel. Verify by inspecting the recorded trail that zero model calls
were made while events streamed past that did not match the structural filter.

**Acceptance Scenarios**:

1. **Given** a console conversation, **When** the player pre-authorizes "when X
   happens, do Y" and the condition compiles to structural predicates, **Then** an
   order-placed record lands in world history carrying the condition, its compiled
   predicates, the action instruction, and its expiry — and the angel's reply
   confirms the order in-fiction.
2. **Given** three standing orders already active, **When** a fourth placement is
   attempted, **Then** it is refused with counsel and nothing is recorded as placed.
3. **Given** a condition the compiler cannot turn into any structural filter,
   **When** placement is attempted, **Then** the angel refuses with counsel
   explaining what it can watch for, and no order is placed.
4. **Given** a standing order, **When** the world restarts or replays from genesis,
   **Then** the order (and its remaining lifetime) is reconstructed exactly from
   recorded history.
5. **Given** a standing order whose TTL has elapsed with no trigger, **When** the
   expiry boundary passes, **Then** an expiry record lands, the slot frees, and the
   next console reply mentions the lapsed watch.
6. **Given** an active order the player no longer wants, **When** the player asks
   the angel to cancel it, **Then** a cancellation record lands and the order stops
   matching immediately.

---

### User Story 3 - Triggered orders act while the player is away (Priority: P1)

While the player is away, a standing order's condition comes true. The angel wakes,
performs the pre-authorized action through exactly the same guarded turn machinery a
console message uses — same serialization, same tool loop, same gates, same charge
economy — and leaves a visible trail: the act lands as recorded influence, the
exchange appends to the console transcript, and a moment is queued so the next time
the player opens the console, the reply leads with what the angel did.

**Why this priority**: triggering is the payoff of story 2; without it standing
orders are inert bookkeeping. It is separately testable because it consumes the
placement machinery rather than defining it.

**Independent Test**: place an order conditioned on a schedulable world event (e.g.
a villager falling asleep), let the world run until the event fires, then open the
console: the reply leads with the moment describing the triggered act, the
transcript shows the system-authored exchange, and the influence landed with one
charge spent.

**Acceptance Scenarios**:

1. **Given** an active order whose structural predicates match an observed event,
   **When** the trigger fires, **Then** a trigger record lands, the pre-authorized
   action executes as a system-authored turn through the single-flight turn path and
   the normal tool loop, the resulting act lands through the standard doors, and a
   moment is queued for the next console reply.
2. **Given** a triggered turn whose act is an omen or vision, **When** it lands,
   **Then** it spends one charge exactly as a console-initiated act would.
3. **Given** an order fires while the charge bank is empty, **When** the triggered
   turn attempts its act, **Then** nothing lands, nothing retries, and an honest
   moment is queued telling the player the angel's strength was spent.
4. **Given** an order fires while the model budget is exhausted or the provider
   tier is down, **When** the triggered turn is attempted, **Then** the angel
   degrades to a queued honest moment — never a retry loop — exactly as console
   turns degrade.
5. **Given** an order triggers while a console turn is in flight, **When** both
   contend, **Then** they serialize through the same single-flight path and both
   leave complete trails; neither is dropped silently.

---

### User Story 4 - Daytime omens defer to nightfall (Priority: P2)

The player asks for an omen during the day. Rather than refusing, the angel accepts
and defers: a system-generated standing order is placed that fires at the next
nightfall and delivers the omen then. The player is told the portent will come with
the dark.

**Why this priority**: quality-of-life on top of stories 1–3; it reuses the standing
order machinery end-to-end (it IS a standing order) and proves the unification.

**Independent Test**: during game daytime, ask for an omen; verify the angel's reply
promises deferral, an order-placed record lands conditioned on nightfall, and when
night begins the omen lands (spending its one charge at landing time, not placement
time) with a moment queued.

**Acceptance Scenarios**:

1. **Given** game daytime, **When** the angel calls send_omen, **Then** no omen
   lands immediately; a system-generated standing order conditioned on the next
   nightfall is placed (exempt from the player-order concurrent cap or counted
   within it per the documented policy), and the reply tells the player the omen is
   deferred.
2. **Given** a deferred omen order, **When** night begins, **Then** the omen lands
   through the normal triggered-turn path, spends one charge, and queues a moment.
3. **Given** a deferred omen whose trigger arrives when the bank is empty, **Then**
   the honest "strength was spent" moment is queued instead, consistent with story 3.

---

### User Story 5 - Meta tools: pause, start, adjust speed (Priority: P2)

The player can ask the angel to pause the world, start it again, or change its
speed, and the angel does so through registered tools that wrap the same controls
the operator surfaces already use. These meta acts are free — no charge — but the
angel's fixed frame binds it to use them only when the player asked or a standing
order pre-authorized it.

**Why this priority**: an independent, small capability; valuable alone but not the
heart of agency.

**Independent Test**: from the console, ask the angel to pause the world and verify
the clock stops and a pause record lands; ask it to resume at a different speed and
verify. Verify no charge was spent, that the tools appear in the angel's declared
roster (and can be withheld per-world through the existing capability manifest), and
that unprompted use is pinned against by the fixed frame.

**Acceptance Scenarios**:

1. **Given** a running world, **When** the player asks the angel to pause, **Then**
   the world clock pauses via the same control the operator commands use, the act is
   recorded, and no charge is spent.
2. **Given** a paused world, **When** the player asks the angel to start it at a
   named speed, **Then** the clock resumes at that speed.
3. **Given** a world whose capability manifest omits the meta tools, **When** the
   angel's roster is composed, **Then** the meta tools are structurally absent from
   its declared tools, not merely forbidden in prose.

---

### User Story 6 - Fuzzy conditions confirmed cheaply (Priority: P3)

The player asks the angel to watch for something no structural filter can decide
alone ("when Rowan seems truly heartbroken, comfort her"). The condition compiles to
a coarse structural filter plus a confirmation step: each filter hit may spend one
small, cheap, rate-capped model call to decide whether the fuzzy condition truly
holds; only a confirmed hit triggers the order. Between filter hits, watching is
free.

**Why this priority**: an accuracy refinement of story 2. The structural-only path
must ship first; fuzzy confirmation layers on without changing the lifecycle.

**Independent Test**: place a fuzzy order; verify the placement records both the
coarse filter and the confirm requirement, that non-matching events cause zero model
calls, that a filter hit causes at most one cheap confirmation call within the rate
cap, and that an unconfirmed hit does not trigger the order.

**Acceptance Scenarios**:

1. **Given** a fuzzy condition, **When** it is compiled at placement, **Then** the
   recorded order carries both the coarse structural filter and a confirmation
   instruction, and the angel's reply sets expectations honestly.
2. **Given** a fuzzy order and a stream of events that do not pass the coarse
   filter, **Then** zero model calls are made on their account.
3. **Given** a filter hit, **When** confirmation runs, **Then** it uses the
   dedicated cheap watch-call class, is bounded by a per-order rate cap, and a
   negative verdict leaves the order armed without triggering.
4. **Given** filter hits arriving faster than the rate cap, **Then** excess hits
   are skipped without model calls and the skip is visible in the recorded trail.

---

### Edge Cases

- A structural predicate matches an event emitted during replay/catch-up rather
  than live play: standing orders must never trigger during replay — replay only
  reconstructs order state; triggering is a live-observation behavior.
- Multiple active orders match the same event batch: each fires; their triggered
  turns serialize through the single-flight path in a deterministic order.
- An order's own triggered turn emits events that match another order (or itself):
  a triggered turn's emissions must not re-trigger the order that produced them;
  cascade depth is bounded (an order fires at most once — orders are one-shot).
- The world is paused when an order's condition would fire: no events flow while
  paused, so nothing matches; behavior resumes with the event stream.
- TTL expiry lands while the expiring order's trigger is racing it: exactly one of
  triggered/expired lands, never both.
- The angel calls monitor_and_act to place an order whose action is itself a
  daytime omen: the action defers again at execution time through the same rule.
- A deferred-omen order is cancelled before nightfall: cancellation wins; the omen
  never lands and no charge is spent.
- The capability manifest revokes send_omen after an omen-bearing order was placed:
  the triggered turn's act is gated at landing time by the then-current grant and
  degrades to an honest moment if revoked.
- The watch confirm call itself fails (budget, tier down, transport): the hit is
  treated as unconfirmed — no trigger, no retry loop, trail records the outcome.
- Legacy worlds: histories containing dream-form nudges and pre-agency snapshots
  load, replay, and upgrade without error; the new order state starts empty.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The angel's mediated influence forms MUST be exactly two: an omen
  (deliverable only during game night, to one living villager, a named group of
  living villagers, or all living villagers) and a vision (deliverable at any time,
  to exactly one living villager). The dream form MUST no longer be offered,
  declared, or accepted for new acts.
- **FR-002**: Each landed omen or vision MUST spend exactly one charge regardless
  of recipient count and regardless of whether it was console-initiated or
  triggered by a standing order. Refused or deferred acts MUST spend nothing.
- **FR-003**: A landed omen or vision MUST land as one atomic recorded batch: the
  influence record plus one prefixed memory per recipient, exactly as the existing
  nudge machinery lands its batches, and MUST remain subject to the existing
  structural firewall (no path from player text to villager surfaces).
- **FR-004**: Replay compatibility MUST hold: histories containing retired
  dream-form acts MUST replay to identical state, and pre-agency snapshots MUST
  upgrade cleanly with empty standing-order state.
- **FR-005**: The angel MUST offer a monitor_and_act capability taking a
  natural-language condition and an action instruction. Placement MUST compile the
  condition ONCE, via a single model call, into structural predicates (event types,
  optional subject villager, optional keywords) plus an optional fuzzy-confirmation
  instruction. A condition yielding no usable structural filter MUST be refused
  with counsel and place nothing.
- **FR-006**: Standing orders MUST be event-sourced world state: placement,
  trigger, cancellation, and expiry MUST each land as recorded events
  (order placed / triggered / cancelled / expired) that reconstruct the full order
  set through snapshots, restart, and from-genesis replay. Replay MUST reconstruct
  state only — it MUST never execute triggers.
- **FR-007**: At most 3 player-placed standing orders may be active concurrently;
  a placement beyond the cap MUST be refused with counsel. Every order MUST carry a
  TTL expressed in game days (player-specifiable, defaulting to 3 game days, capped
  at 7); expiry MUST land an expiry event, free the slot, and surface a moment.
- **FR-008**: Live event observation MUST evaluate structural predicates in native
  code at zero model cost per event. A model call on account of a watched event
  MUST be possible only after a structural filter hit, and only for orders whose
  compiled condition requires fuzzy confirmation.
- **FR-009**: Fuzzy confirmation MUST run on a new dedicated call class
  (KindMetatronWatch) routed to a cheap tier, MUST be rate-capped per order (at
  most one confirmation per order per 30 game minutes; excess filter hits are
  skipped and countable from the trail), and an unconfirmed or failed confirmation
  MUST leave the order armed without triggering and without retrying.
- **FR-010**: A confirmed trigger MUST execute the order's action instruction as a
  system-authored turn through the same single-flight turn path and bounded tool
  loop console turns use — same gates, same charge economy, same telemetry (every
  tool call recorded), same transcript append — and MUST queue a moment describing
  what was done so the next console reply leads with it. Orders are one-shot: a
  triggered order is consumed.
- **FR-011**: Triggered turns MUST respect budget and tier-degradation honesty
  identically to console turns: on exhausted budget, downed tier, or an empty
  charge bank at act time, the turn MUST degrade to a queued honest moment (e.g.
  "strength was spent") — never silent drop, never a retry loop.
- **FR-012**: A daytime send_omen call MUST NOT land immediately and MUST NOT be
  refused: it MUST place a system-generated standing order conditioned on the next
  nightfall that delivers the omen through the triggered-turn path, spending its
  charge at landing time. System-generated deferral orders MUST NOT count against
  the player's concurrent cap but MUST be event-sourced, cancellable, and visible
  like any order.
- **FR-013**: The angel MUST offer charge-free meta tools wrapping the existing
  world clock controls: pause, start (resume at a named speed), and adjust_speed.
  These MUST reuse the same control path the operator surfaces use, MUST land the
  same clock records, and MUST be individually withholdable per world through the
  existing capability manifest (structurally absent when ungranted).
- **FR-014**: The fixed, non-editable frame MUST bind meta-tool and standing-order
  use to player-requested or pre-authorized action only, beneath any player-edited
  charter or skill text.
- **FR-015**: Every new capability (send_omen, send_vision, monitor_and_act,
  cancel_order, pause, start, adjust_speed) MUST be a registered tool on the
  angel's declared roster — registry-declared schema, gate, effect class, and cost
  — and the existing structural guarantee MUST extend: no code path from model
  output to world action or clock control exists outside registered tools;
  conversation remains the tool-free final-answer channel.
- **FR-016**: The player MUST be able to see standing orders (condition, remaining
  lifetime, fuzzy/structural kind) and cancel them by asking the angel; the
  model-free status surface MUST list active orders alongside the charge bank.
- **FR-017**: The angel's turn prompt MUST carry its active standing orders and
  the deferral rule so its counsel and confirmations stay truthful to live state.

### Key Entities

- **Standing order**: a pre-authorized watch-and-act instruction. Attributes:
  identity, origin (player-placed vs system-generated deferral), the original
  natural-language condition, compiled structural predicates (event types, optional
  subject, optional keywords), optional fuzzy-confirmation instruction, action
  instruction, placement time, TTL in game days, lifecycle state
  (active → triggered | cancelled | expired).
- **Omen**: night-only influence to one, several, or all living villagers; one
  charge; lands as an atomic influence-plus-memories batch with an omen prefix.
- **Vision**: any-time influence to exactly one living villager; one charge; lands
  as an atomic batch with a vision prefix.
- **Watch confirmation**: a cheap, rate-capped model verdict on whether a fuzzy
  condition truly holds for a filter-hit event; positive verdicts trigger, negative
  or failed verdicts leave the order armed.
- **Moment**: the existing queued console-surface line; extended to carry triggered
  acts, deferrals, expiries, and honest degradations.
- **Meta tool**: a charge-free registered tool wrapping an existing world clock
  control (pause / start / adjust_speed).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: While a world with active structural-only standing orders runs
  unattended, the recorded trail shows zero watch-related model calls for event
  batches that produce no filter hit — verifiable from the trail alone.
- **SC-002**: A standing order placed, then observed through a daemon restart and a
  from-genesis replay, reconstructs identically (same identity, predicates,
  remaining TTL) in 100% of runs; replay executes zero triggers.
- **SC-003**: A pre-authorized action whose condition fires while the player is
  away is visible to the player entirely from durable surfaces: the next console
  reply leads with the moment, the transcript shows the system-authored exchange,
  and world history shows the trigger and the landed act — no hidden state.
- **SC-004**: 100% of landed omens/visions (console-initiated and triggered) spend
  exactly one charge; 100% of refused, deferred-at-placement, or degraded acts
  spend zero.
- **SC-005**: An order firing under empty bank, exhausted budget, or downed tier
  produces exactly one honest queued moment and zero retries in 100% of cases.
- **SC-006**: With all new tools granted, the angel's declared roster and its
  prompt guidance describe exactly the same capability set (described ≡ declared);
  with a manifest withholding any subset, the withheld tools are absent from
  declaration, guidance, and handling simultaneously.
- **SC-007**: The structural-firewall audit extends to the new surface: the
  sentinel proves no path from model output to villager influence or clock control
  outside registered tools, and it fails if one is introduced.
- **SC-008**: A full game-day of unattended watching with ≤3 active orders adds at
  most the placement call plus rate-capped confirms (≤48 cheap calls/order/day
  worst case) — no unbounded per-event model cost shape exists.

## Assumptions

- Standing orders are **one-shot**: an order fires once and is consumed. Recurring
  watches are out of scope; the player re-places if wanted.
- Default TTL is 3 game days, player-specifiable up to a cap of 7 game days.
- The fuzzy-confirm rate cap is one confirmation per order per 30 game minutes;
  skipped hits leave a countable trace. The exact knob value may be tuned at
  implementation without spec change as long as a per-order cap exists.
- System-generated deferral orders sit outside the player cap of 3 (they are
  bookkeeping for an already-authorized act), but share every other lifecycle rule.
- "start" resumes the world at a named speed and "adjust_speed" changes speed while
  running; both map onto the existing pause/resume/set-speed control commands, and
  their recorded clock events are the existing ones (no new event vocabulary for
  clock acts).
- Group targeting for omens is a list of living villager names or the special
  "everyone"; a group omen is still one act, one charge, one atomic batch.
- The trigger-time roster/grant check governs (placement-time grants do not carry
  forward): an act ungranted at landing time degrades to an honest moment.
- Villager-side interpretation of omens/visions continues to ride the existing
  provenance-unknown memory mechanism; no villager-side changes are in scope.
- The existing single-flight turn serialization is sufficient for triggered turns;
  no parallel turn execution is introduced.
- KindMetatronWatch joins the existing per-kind routing configuration with a cheap
  default chain; operators may re-route it like any other kind.
- A daytime omen whose text approaches the 400-byte cap (≳ ~370 chars) can exceed
  the deferral order's 400-rune action cap and is refused at placement with
  counsel to shorten — typical sentence-length omens defer fine (T016 finding,
  accepted as a documented edge).
