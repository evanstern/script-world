---
name: metatron-orders
description: The event-sourced standing-orders subsystem (spec 029, TASK-27) — monitor_and_act watches compiled once into free structural predicates, matched live in the absorb path, fired as system-authored turns through the single-flight door, with fuzzy confirm, budget-honest degradation, and daytime-omen deferral as a system-origin order
kind: component
sources:
  - internal/metatron/orders.go
  - internal/sim/metatron.go
  - internal/sim/executor.go
  - internal/sim/loop.go
  - internal/sim/state.go
  - internal/metatron/turn.go
  - internal/metatron/toolcalls.go
  - internal/metatron/digest.go
  - internal/tool/registry.go
  - internal/llm/llm.go
  - internal/llm/config.go
verified_against: e9213e17e6e48cf30da802949d9b59e0e3d78370
---

# Metatron's standing orders

A standing order is a pre-authorized watch-and-act instruction (spec 029, TASK-27):
the player tells the angel "when Rowan next falls asleep, send her a comforting
vision" and walks away. The condition is compiled ONCE at placement into structural
predicates evaluated for free as world events stream past; when it matches, the angel
wakes and performs the pre-authorized action through exactly the [[metatron]] console
turn's guarded machinery. Orders are **one-shot** (fire once, consumed), event-sourced
(they ride `sim.State` through snapshots and replay), and never fire during
reconstruction — replay rebuilds their state but only live observation triggers them.

## The entity and its lifecycle

`sim.MetatronOrder` (`internal/sim/metatron.go`, data-model §1) is the event-sourced
record: `ID` (`"ord-<placedTick>-<seq>"`, deterministic, no RNG — `nextOrderID` in
`orders.go`), `Origin` (`"player"` | `"system"`), `Condition` (the original NL, ≤300
runes), `Action` (the NL action instruction, ≤400 runes), `EventTypes` (the structural
predicate — non-empty, drawn from the observable vocabulary), `Agent` (a villager index,
or `-1` for any), `Keywords` (a lowercase coarse text filter, ≤6), `Confirm` (fuzzy —
needs the watch confirm), `PlacedTick`, `ExpiresTick`, and `Status`
(`active` → `triggered` | `cancelled` | `expired`, one-way).

**Caps and bounds** (`sim` constants): at most `MetatronPlayerOrderCap` (3) ACTIVE
**player-origin** orders may stand concurrently — system-origin deferral orders are
exempt (they are bookkeeping for an already-authorized act, FR-012). Every order carries
a TTL in game days, player-specifiable, default 3, bounded `MetatronOrderTTLMinDays`..
`MetatronOrderTTLMaxDays` (1..7); the reducer validates `ExpiresTick - PlacedTick`
against the same `ticksPerGameDay` (`24*3600`) literal the turn side computes from
(mirrored in `orders.go` so the door and the placer can never diverge). The
`MetatronOrders` slice is pruned to retain every active order plus the most recent
`metatronOrderRetain` (32) non-active ones (`pruneMetatronOrders` — deterministic,
order-preserving, so replay prunes identically), giving the status/trail recent history
without unbounded growth.

## Event sourcing

Four event types carry the lifecycle, all cataloged in [[event-types]] and dispatched
through [[sim-state-reducer]]'s `applyMetatron` arm:

- **`metatron.order_placed`** (payload = the whole `MetatronOrder`) — landed through
  the [[sim-loop]] `InjectSocial` door by `placeOrder` (a `monitor_and_act` call) or by
  `deferOmen` (a system deferral). The reducer dry-run is the door authority: it rejects
  a duplicate id in ANY status, an unknown `Origin`, empty `EventTypes` (an uncompilable
  condition), a TTL outside 1..7 days, an out-of-range `Agent`, an over-long
  condition/action, and a player placement beyond the cap. The payload's `Status` is
  IGNORED — an order always lands `active`, then the retention prune runs.
- **`metatron.order_triggered`** (`OrderTriggeredPayload{id, matched_type, matched_tick}`)
  — injected by the trigger worker when a match fires; NEVER emitted during replay.
- **`metatron.order_cancelled`** (`OrderIDPayload{id}`) — injected by a `cancel_order`
  call.
- **`metatron.order_expired`** (`OrderIDPayload{id}`) — **executor-emitted**, a pure
  function of `(state, tick)` exactly like `metatron.charge_regenerated`: the [[executor]]
  emits it once when an active order's `ExpiresTick` elapses, so it is reproduced
  deterministically in replay without the angel running (unlike a trigger). It is NOT on
  the `injectSocialWhitelist` — `order_placed`/`order_cancelled`/`order_triggered` are
  the injected three; expiry is produced sim-side.

`transitionMetatronOrder` performs every active→terminal move and rejects an unknown or
already-non-active id — this is where the **cancel/expiry/trigger race resolves**:
exactly one terminal lands, and the loser hits a non-active order and refuses at the
door. Replay reconstructs order state through `json.Unmarshal` + `Apply` alone;
`matchOrders` runs only in the absorb goroutine, so a predicate can never match during
reconstruction (the edge-case guarantee — triggering is a live-observation behavior).
`MetatronOrder.ExpiresTick` is a SHIFT field in the miracle `rebaseTicks` taxonomy
(shifted only while active; `PlacedTick` is KEEP) — see [[metatron-miracles]].

## Live matching (the absorb path)

`matchOrders` runs in [[metatron]]'s `run()` loop AFTER the replica applies each batch
and the mirror refreshes, so it is live-only by construction. `orderMatches` is a PURE
predicate — no state, no model call, evaluated free per event (SC-001): the event type
is one of the order's `EventTypes`; if the order pins an agent (`>= 0`) the event
concerns that villager (`eventConcernsAgent` probes the `agent`/`from`/`to` payload
fields — a best-effort structural match, never a false positive); if the order lists
keywords the lowercased payload contains at least one. Only active orders match.

Orders fire in **order-id order** within a batch, at most once: `pendingTrigger`
(stateMu-guarded) dedups an order already queued but not yet resolved, and one job is
enqueued per order per batch. A structural hit enqueues a `triggerJob` onto the buffered
`triggerQ` (a full queue logs and drops the order). A **fuzzy** order (Confirm) matches
structurally here too, but its hit is routed as a CONFIRM job and is rate-capped to one
confirm per `confirmRateTicks` (1800 ticks = 30 game minutes) per order via the
absorb-owned `lastConfirmTick` map — NOT event-sourced (a skipped confirm is an economy
decision, never world history); a rate-capped hit is logged and skipped so a storm of
matching events never triggers a flood of watch calls (FR-009, SC-008).

## Trigger execution

`triggerWorker` consumes `triggerQ` FIFO (one worker, so triggered turns and confirms
serialize with each other and — via the shared `turnBusy` — with console turns). A
structural job fires straight through `runTrigger`; a fuzzy job first runs `runConfirm`.

**The fuzzy confirm** (`confirmOrder`, spec 029 US6, `contracts/routing.md`): ONE bare
`Submit` on [[llm-orchestrator]]'s new `llm.KindMetatronWatch` kind (routed to a cheap
default chain — `local`→`cloud` in `internal/llm/config.go`), `MaxTokens` 16, a fixed
yes/no system prompt (`confirmSystem`), and a user prompt rendering the order's condition
plus the matched event in the digest vocabulary (`describeEvent`, reading static
`sim.AgentNames`). Reply contract (`confirmYes`): the first token, lowercased and
stripped of punctuation, must be exactly `"yes"` — anything else, empty, garbage, or an
error is a NO. A no/failed verdict leaves the order armed with NO retry (a single call,
not a loop); only the in-flight marker is cleared so a later hit can confirm again,
subject to the rate cap.

**`runTrigger`** fires one matched order:

1. Land `metatron.order_triggered` through the door — the dry-run enforces the order is
   STILL active, so a cancel/expiry that raced the match wins here and the trigger is
   abandoned silently.
2. **Empty-bank precheck** (`knownActEmptyBank`): a system-origin (deferral) order's
   action is a known charge spend, so an empty bank short-circuits to an honest moment —
   no model call, no cloud cost. A free-form player order's action may be advisory or a
   meta act, so it still runs the turn.
3. Acquire `turnBusy` with a **bounded wait** (`acquireTurnBusy`, `systemTurnBusyWait`
   90s): system turns WAIT for the single-flight slot (unlike the console's fail-fast
   `ErrTurnBusy`), but a wedged console turn degrades the trigger to an honest moment
   rather than hanging.
4. Run the pre-authorized action as a **system-authored turn**: `runTurn` with
   `turnOrigin{system: true, jobPrefix: "watch", seed: order.Action}` — the SAME
   [[tool-loop]]-driven turn body a console message uses (same roster/handler/gate
   composition, same `cog.tool_call` telemetry, same retry marker). The framing differs:
   the transcript opens with a `[watch]` origin marker over the order's action (never a
   player-text line — a triggered turn has no player text), the correlation id is
   `watch-metatron-<tick>`, and moment consumption is suppressed (the player-facing queue
   stays intact for the next console open; the trigger worker queues the turn's OWN
   moment).
5. `queueMoment` from the outcome: `triggeredMoment` names the landed act on success
   (omen/vision/miracle), `degradedMoment` maps a failed turn to ONE model-free honest
   moment per failure family — `ErrBudgetExhausted`/`ErrTierDown`/`ErrTierBusy` →
   "my sight dimmed", otherwise "I faltered" — never a retry (FR-011). Moments accrete to
   `metatron/soul.md` and the queue so the next console reply leads with what the angel
   did while the player was away (SC-003).

## Daytime-omen deferral

A daytime `send_omen` never lands and never refuses (FR-012): `landOmen`'s day path calls
`deferOmen` ([[metatron]]'s `turn.go`), which places a **system-origin** standing order —
`EventTypes` `["sim.night_started"]`, TTL 1 game day, cap-exempt — whose one-shot trigger
re-runs the omen the instant night falls. Placement is FREE; the charge is spent at
trigger-time landing, not at placement (SC-004). The action seed leads the night system
turn back to `send_omen` with the promised targets and text (terse framing keeps typical
renderings within the 400-rune action cap; a near-cap omen can exceed it and is refused
at placement with counsel to shorten). `"everyone"` is preserved as the target word so
the night turn re-resolves against whoever lives THEN; a named list re-sends to those
still living. The `monitor_and_act` grant is NOT required — a deferral carries `send_omen`'s
gate, so a world granting `send_omen` but withholding `monitor_and_act` can still defer.
Cancelling the deferral before nightfall wins: the omen never lands and no charge is spent.

## Surfaces

The player reads and cancels orders through the angel. `monitor_and_act` and
`cancel_order` are registered [[tool-registry]] tools (`monitor_and_act` uses a hand-built
`monitorAndActSchema` — arrays are unrepresentable in the scalar Param model, like
`set_plan` — with `event_types` an enum over `observableEventTypes`, the curated
vocabulary of genuinely-emitted types); `toolcalls.go`'s `handleMonitor`/
`handleCancelOrder` wrap `placeOrder`/`cancelOrder`, mapping a door rejection to
in-fiction counsel fed back
as a `rejected_gate` the model may repair. The turn prompt carries active orders
(`writeStandingOrders` — id, condition, days-left, fuzzy/structural — FR-017) so the
angel's counsel stays truthful to live state, and the model-free `metatron.Status`
surface lists them (`Status.Orders`, `OrderStatus{id, condition, origin, fuzzy,
expires_day, status}`, FR-016). The fixed frame's `metatronInitiativeFrame` (a
compile-time constant appended last, beneath any charter/skill) binds standing-order and
meta-tool use to player-requested or pre-authorized action only — never the angel's own
initiative — with the door-side grant gate backing it independently.

## Connections

[[metatron]] hosts the trigger worker, the turn body (`runTurn`) both console and system
paths share, and the meta-tool/deferral wiring; [[sim-state-reducer]] holds the reducer
arms (`applyMetatron`, `transitionMetatronOrder`, `pruneMetatronOrders`) and the
`State.MetatronOrders` field; [[executor]] emits `metatron.order_expired` as a pure
function of state and tick; [[event-types]] catalogs the four order events; [[tool-loop]]
drives the system-authored turn exactly as it drives the console turn; [[llm-orchestrator]]
routes the fuzzy `KindMetatronWatch` confirm to its cheap chain; [[tool-registry]]
declares `monitor_and_act`/`cancel_order` and the observable-event vocabulary;
[[metatron-miracles]] shares the `rebaseTicks` taxonomy that shifts an active order's
expiry across a time snap. Spec: `specs/029-metatron-agency/` (TASK-27) — `spec.md` US2/
US3/US4/US6, `data-model.md`, `contracts/events.md`, `contracts/routing.md`.

## Operational notes

Orders are one-shot by doctrine — recurring watches are out of scope; the player re-places
if wanted. A triggered turn's own emissions must not re-trigger the order that produced
them (bounded cascade — an order fires at most once). A confirmed trigger that races its
own TTL expiry resolves at the door: exactly one of triggered/expired lands, never both.
A full game-day of unattended watching with ≤3 active orders adds at most the placement
call plus rate-capped confirms (≤48 cheap calls/order/day worst case) — no unbounded
per-event model cost shape exists (SC-008).
</content>
</invoke>
