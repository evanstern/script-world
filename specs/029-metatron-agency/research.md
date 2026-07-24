# Phase 0 Research: Metatron Agency

All unknowns from Technical Context resolved. Each decision is grounded in the
shipped code (commit at branch point) and TASK-27's design record.

## R1 — Taxonomy migration & replay compatibility

**Decision**: `metatron.nudged` remains the one influence event; its `form` field
gains `"vision"` and keeps `"omen"`; `"dream"` is grandfathered. The reducer arm
(`internal/sim/metatron.go applyMetatron`) validates: `vision` = exactly one living
target, any time; `omen` = ≥1 living targets, `State.Night` must be true; `dream` =
legacy shape (exactly one target) accepted so historical events replay — but no
tool, handler, or roster entry can produce a new one. The roster check
`tool.OnRoster(RosterMetatron, "nudge_"+form)` is replaced by a form-set check that
includes the grandfathered `dream`.

**Rationale**: replay and the dry-run share `applyMetatron`; removing `dream`
acceptance would break from-genesis replay of existing worlds (constitutional
regression). Structural absence of any dream-producing tool is the guarantee new
dreams cannot land — same pattern as `gratis`'s structural absence.

**Alternatives considered**: (a) new event types `metatron.omen_sent`/
`metatron.vision_sent` — rejected: doubles reducer arms and whitelist entries for
no semantic gain; the `form` field already discriminates. (b) Event-log migration
rewriting dream events — rejected: the log is append-only and immutable by
doctrine.

## R2 — Registry entries, cap literals, and manifest compatibility

**Decision**: retire `nudge_dream`/`nudge_omen` registry entries; add `send_vision`
(Params: `target` AgentName required, `text` Text required cap 400) and `send_omen`
(`targets` — see R3, `text` Text required cap 400), Gate Charge, Effect Expressive,
Events `metatron.nudged` + `agent.memory_added`, Cost.Charges 1. The two cap
readers (`metatron.nudgeTextMax`, `sim.NudgeTextMax`) re-point at `send_vision`'s
entry (single source preserved). `capabilities.json` manifests naming the retired
tools: spec 021's loader already treats unknown names as ungranted-without-error;
document that legacy manifests granting `nudge_*` now grant nothing and the status
surface makes the effective roster visible. `LoopRosterMetatron()` becomes
`send_omen`, `send_vision`, `monitor_and_act`, `cancel_order`, `work_miracle`, plus
granted meta tools; `RosterMetatron` (the door-side name set) follows.

**Rationale**: renaming at the registry keeps described ≡ declared ≡ priced by
construction; the cap literal stays single-sourced.

**Alternatives considered**: aliasing old manifest names to new tools — rejected:
silent grant translation contradicts the manifest-as-explicit-grant design; the
status surface already tells the player what is effective.

## R3 — send_omen group targeting within the Param model

**Decision**: `send_omen` takes `targets` as a required Text param carrying a
comma-separated list of villager names or the word `everyone` (documented in the
param Description and the derived guidance). The handler splits/validates against
the alive set; the landed payload keeps `Targets []int` exactly as today.

**Rationale**: the registry's scalar Param model has no array kind; the only
authored-schema escape hatch (`InputSchemaJSON`) is being spent on
`monitor_and_act` (R5), and a comma list over 8 known names is trivially parseable
and self-describing in guidance prose. Villager-name text is already the pattern
for `nudge_dream`'s `target`.

**Alternatives considered**: (a) authored `InputSchemaJSON` with a string array —
viable but forces the R5 validator generalization to cover two tools and buys
little; (b) repeated single-target omens — rejected: violates "one act, one
charge, one atomic batch".

## R4 — Standing-order state, events, and expiry determinism

**Decision**: `State.MetatronOrders []MetatronOrder` (json `metatron_orders,omitempty`
— empty on pre-agency snapshots, upgrade-free). Four whitelisted event types with
reducer arms that validate at the door (dry-run) and replay identically:

- `metatron.order_placed` — full order payload (R7 for id); rejects when the
  player-origin active count is already 3 (system-origin exempt), when predicates
  are empty, or when TTL is outside 1..7 game days.
- `metatron.order_cancelled` / `metatron.order_expired` — reject unknown or
  non-active order ids; transition state and free the slot.
- `metatron.order_triggered` — rejects unknown/non-active id; marks the order
  consumed (one-shot). Carries the matched event's type + tick for the trail.

Expiry is emitted by the **executor** (`internal/sim/executor.go`), exactly like
`metatron.charge_regenerated`: a pure function of state + tick — when
`tick ≥ order.ExpiresTick` for an active order, emit `order_expired`. Deterministic
in replay by construction. Triggering is emitted only by live Metatron via
`InjectSocial` (never during replay — replay merely applies the recorded event).

**Rationale**: mirrors the proven charge-economy split: deterministic bookkeeping
in the executor, model-driven acts as injected recorded events.

**Alternatives considered**: metatron-side expiry emission — rejected: the angel's
absorb goroutine is not deterministic space; expiry must survive replay without
the angel running.

## R5 — monitor_and_act schema and the authored-override validator

**Decision**: the turn model itself is the compiler — `monitor_and_act`'s schema
requires the model to supply the compiled structure in the tool call: `condition`
(NL, ≤300 chars), `action` (NL instruction, ≤400), `event_types` (array of strings
from the observable-event enum), optional `agent` (villager name), optional
`keywords` (array of strings, coarse filter), optional `confirm` (boolean: fuzzy —
needs the watch confirm), optional `ttl_days` (integer 1..7, default 3). This
needs arrays ⇒ authored `InputSchemaJSON`. The toolloop driver's `validateArgs`
currently routes every authored-override tool through `validateSetPlan`; it is
generalized to a small schema-lite walker (required keys, scalar types, string
arrays + enum membership, integer bounds, string maxLength) driven by the authored
schema itself; `set_plan` validates identically through it (equivalence pinned by
existing driver tests).

A condition the model cannot structure (empty `event_types`) is refused at the
gate with counsel (FR-005) — the placement handler, not the driver, owns that
refusal so it lands as `rejected_gate` with in-fiction wording.

**Rationale**: "one model call at placement" is satisfied by zero *extra* calls —
the turn call that composes the tool call is the compile. The enum of observable
event types rides the schema so the model picks from real vocabulary
(`agent.slept`, `agent.woke`, `agent.died`, `social.promise_broken`, …, curated
list in contracts/tools.md).

**Alternatives considered**: (a) separate compile Submit inside the handler —
rejected: an extra cloud call per placement for strictly less context than the
turn already has; (b) scalar comma-lists to dodge the validator work — rejected:
`event_types` enum membership is exactly what the driver's schema validation
exists to enforce; hand-parsing would bypass `rejected_malformed`.

## R6 — Predicate evaluation & trigger execution path

**Decision**: the absorb goroutine (`metatron.run`) matches each observed event
against a mirror of active orders (refreshed in `mirrorState` from the replica —
the replica's `MetatronOrders` is the authority). A structural hit for a
non-fuzzy order enqueues a trigger job on a small buffered channel consumed by a
dedicated trigger worker goroutine. The worker: (1) lands
`metatron.order_triggered` through `InjectSocial` (dry-run enforces the order is
still active — the cancel/expiry race resolves at the door, edge case covered);
(2) runs a **system-authored turn**: `Turn`'s body refactored into an internal
`runTurn(origin, seed)` shared by console and trigger paths — same single-flight
`turnBusy` guard, same roster/handler/gate composition, same telemetry, same
transcript append (rendered as `[watch]` origin), same moment queueing. The
trigger worker WAITS on the busy flag (bounded retry loop) rather than failing
fast — ErrTurnBusy stays console-only. Trigger jobs serialize FIFO on the channel;
multiple orders matching one batch each fire once, in order-id order.

Self/cascade suppression: events landed by a triggered turn carry through the
normal Observe path, but a triggered order is already consumed (one-shot) before
its turn runs, and system deferral orders match only `sim.night_started` (executor-
emitted), so no order's own output can re-trigger it. Depth is structurally ≤1.

**Rationale**: reuses the single-flight path per FR-010 verbatim; the door
resolves all lifecycle races; no new concurrency primitives beyond one channel +
worker.

**Alternatives considered**: matching inside the sim loop (deterministic space) —
rejected: triggering requires model calls and wall-clock work; the sim must never
block on cognition (the mind/scribe notify pattern exists precisely for this).

## R7 — Order identity

**Decision**: `OrderID = fmt.Sprintf("ord-%d-%d", placedTick, seqWithinTick)` —
assigned by the placement handler from the mirrored tick, uniqueness enforced by
the reducer (rejects duplicate active id; seq disambiguates same-tick placements).

**Rationale**: deterministic, human-readable, replayable; no RNG draw (keeps the
deterministic-rng stream untouched).

## R8 — KindMetatronWatch routing & config backfill

**Decision**: add `KindMetatronWatch = "metatron_watch"` to `acceptedKinds` and
`defaultRoutes` with chain `["local", "cloud"]` (cheap-first, reliable fallback).
Config load compatibility: `validateV2`'s both-direction completeness stays for
all pre-existing kinds, but a missing route for `metatron_watch` is **backfilled
from the default** with a boot log line instead of erroring — a narrow, documented
carve-out so every existing v2 `llm.json` keeps booting (upgrade honesty).
`llm.Kinds()` picks the new kind up automatically for calibrate/status surfaces.

**Rationale**: strict completeness exists to catch typos and dead routes, not to
brick worlds on upgrade; the backfill preserves the invariant post-load (every
kind has a route).

**Alternatives considered**: boot error demanding a manual config edit — rejected:
violates the project's world-upgrade discipline (pre-existing worlds keep working);
routing `metatron_watch` through `KindMetatron`'s chain — rejected: the whole point
is independent cheap routing + independent metering.

## R9 — Watch confirm mechanics & rate cap

**Decision**: fuzzy orders (`confirm: true`) on a structural hit enqueue a confirm
job instead of a trigger. The confirm is ONE bounded `Submit` on
`KindMetatronWatch` (not a tool loop): system prompt fixed; user prompt = the
order's condition + the matched event rendered by the existing digest vocabulary;
reply contract = single token `yes`/`no` (anything else = no). Rate cap: per order,
at most one confirm per 30 game minutes (1800 ticks), tracked in the order mirror
(`lastConfirmTick`, not event-sourced — a skipped confirm is not world state);
skipped hits and negative verdicts land a `cog.tool_call`-style trail via the
existing `cog.outcome` vocabulary? — No: they land nothing in the world; they are
logged and counted in the angel's soul digest line (model-free), keeping the trail
countable (SC-001's zero-call claim is verified from the *absence* of metatron_watch
meter entries; the meter itself is the countable trail for confirms). Confirm
failures (budget, tier, transport) = unconfirmed, no retry (the orchestrator's
single-call semantics; no loop, so no loop retry either).

**Rationale**: a single cheap call with a binary contract is the design record's
"rate-capped confirmation call"; metering (existing per-kind ledger) is the
audit trail without new event types.

**Alternatives considered**: confirm as a toolloop run — rejected: nothing to
dispatch; a bare Submit is the cheapest honest shape.

## R10 — Meta tools & the loop-control seam

**Decision**: three registry entries — `pause` (no params), `start` (optional
`speed` Enum over clock speeds), `adjust_speed` (required `speed` Enum) — Effect
**Expressive with empty Events** (the converse precedent: acting cardinality
applies, nothing injected), Gate None, Cost zero charges. Metatron gains a
`LoopControl` seam: `type LoopControl interface { Do(name string, speed clock.Speed) (sim.Status, error) }`
— satisfied by `*sim.Loop`; the daemon already passes the loop (as `Injector`);
`metatron.New` gains the seam parameter (same value, second interface). Handlers
map: `pause`→`Do("pause","")`, `start`→`Do("resume",speed)` (empty speed = resume
at current), `adjust_speed`→`Do("set_speed",speed)`. The clock events these land
(`clock.paused`/`clock.resumed`) are the loop's own, unchanged. The fixed frame
gains one sentence pinning meta tools (and all standing orders) to
player-requested or pre-authorized use. `capabilities.json` gates each meta tool
individually like any other (structural absence when ungranted); **default grant
includes them** (missing manifest = full roster, unchanged rule).

**Rationale**: `Loop.Do` is exactly the surface the IPC server drives — same
functions, per the design record; Expressive-empty-Events avoids a new effect
class while keeping meta acts inside the one-act cardinality (pausing is an act).

**Alternatives considered**: a new `EffectClass Meta` — rejected: touches
Validate/coverage/isActing for zero behavioral difference from
Expressive-with-no-Events; revisit only if a future meta tool must not consume
the turn's one act.

## R11 — Daytime omen deferral

**Decision**: `landOmen` checks the mirrored night flag (`State.Night` via
`mirrorState`); when day, instead of landing it places a **system-origin** standing
order: predicates = `event_types: ["sim.night_started"]`, action = a fixed
rendering ("deliver this omen: <text> to <targets>"), TTL 1 game day (nightfall
always arrives within it), origin `system` (cap-exempt, cancellable, visible).
The handler returns Verdict landed-equivalent? No — nothing landed: the handler
returns `VerdictLanded`? **Decision**: the order placement IS the landed act
(`order_placed` is a recorded world event through the door): the tool call lands,
`ResultForModel` says the omen is deferred to nightfall, the reply/moment wording
covers the player. At nightfall the trigger worker's system turn re-runs
`send_omen` (now night: lands, spends the charge at landing time per FR-012/SC-004).

**Rationale**: unifies the machinery exactly as the design record intends — the
deferral is literally a standing order, exercising placement, trigger, and
budget-honesty paths.

## R12 — Budget/degradation honesty for triggered turns

**Decision**: the system turn path reuses `runTurn` verbatim, so
`ErrBudgetExhausted`/`ErrTierDown`/transport failures surface as the same error
returns console turns get. The trigger worker maps ANY failed system turn to one
queued honest moment (fixed model-free wording per failure family: "strength was
spent" for empty bank — detected pre-turn; "my sight dimmed" for budget/tier) and
never retries (the in-loop single transport retry inside toolloop still applies,
as on console turns). Empty-bank short-circuit: if the order's action requires a
charge and the bank is 0 at trigger time, skip the model call entirely and queue
the moment — no spend, no cloud cost.

**Rationale**: FR-011/SC-005 verbatim; the empty-bank precheck avoids paying for
a turn that cannot act. (The model could still choose converse-only output — but
a triggered turn with no possible act is noise; the precheck is the honest cheap
path. The precheck applies only to omen/vision-bearing deferral orders where the
act is known; free-form monitor orders still run — their action may be advisory or
meta.)

## R13 — Prompt, status, and moments surface

**Decision**: the turn user prompt gains an "Standing orders you keep watch over"
block (id, condition, remaining days, fuzzy marker); `Status` gains
`Orders []OrderStatus` (id, condition, origin, expires-day, fuzzy) — additive,
omitempty; expiry/deferral/degradation moments are model-free lines through the
existing moment queue. Placement/cancel/trigger also append soul lines (same
`appendFile` discipline).

**Rationale**: FR-016/FR-017; additive JSON keeps IPC clients compatible
(the spec-021 precedent).

## R14 — Sentinel/audit extension

**Decision**: extend `metatron_test.go`'s structural firewall audit: the handler
map for any turn is built ONLY from `grantedRoster`; assert no code path from
model output reaches `InjectSocial`, `InjectIntent`, or `LoopControl.Do` outside
registered-tool handlers; assert `converse` remains undeclared; assert the meta
tools' handlers are absent when ungranted. Plus a static assertion that
`send_omen`'s and `send_vision`'s events ⊆ whitelist rides the existing
`sim.ValidateToolCoverage` boot gate automatically.

**Rationale**: FR-015/SC-007 — the registry stays the sole world-action path,
now including clock control.
