# Tasks: Metatron Agency — Standing Orders, Omens & Visions

**Input**: Design documents from `specs/029-metatron-agency/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: included — this project's testing strategy runs tests alongside every
slice (reducer replay/determinism, driver equivalence, sentinel audit, boot gates).

**Organization**: Phase 2 is the cross-package substrate (registry, driver, llm
kind, sim state) that every story consumes; stories then land as independent
increments in priority order. All paths are worktree-relative
(`.worktrees/task-27/`).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

*(no setup tasks — existing repo, no new packages or tooling)*

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: the substrate every user story consumes. These four tasks are
strongly coupled (the registry retirement breaks compiles in sim/metatron until
their readers re-point) — execute sequentially in this order, keeping the build
green at each commit.

- [X] T001 Add `KindMetatronWatch` to `internal/llm/llm.go` (acceptedKinds,
  comment) and `internal/llm/config.go` (defaultRoutes chain `["local","cloud"]`,
  missing-route backfill per contracts/routing.md — unknown keys still error;
  extend `internal/llm/config_test.go` with: v2 config without the new route boots
  with backfill + log, unknown route key still errors, defaults include the kind.
- [X] T002 Generalize authored-schema validation in `internal/toolloop/loop.go`:
  replace the hardcoded validateSetPlan dispatch with a schema-lite walker
  (required keys; string/integer/boolean scalars; string arrays with enum,
  maxLength, minItems/maxItems; integer minimum/maximum; string maxLength) driven
  by the tool's `InputSchemaJSON`; `set_plan` must validate byte-identically
  (existing `loop_test.go`/`equivalence_test.go` pass unchanged); add walker unit
  tests in `internal/toolloop/loop_test.go` covering monitor_and_act-shaped calls
  (missing required, bad enum member, array over maxItems, wrong scalar type).
- [X] T003 Registry migration in `internal/tool/registry.go` + `roster.go`:
  retire `nudge_dream`/`nudge_omen`; add `send_vision`, `send_omen`,
  `monitor_and_act` (authored InputSchemaJSON + curated event_types enum pinned
  against real emitted types), `cancel_order`, `pause`, `start`, `adjust_speed`
  per contracts/tools.md; update `LoopRosterMetatron()`/`RosterMetatron`;
  re-point cap readers (`internal/sim/metatron.go` `NudgeTextMax`,
  `internal/metatron/turn.go` `nudgeTextMax`) at `send_vision`; keep
  `tool.Validate()` green; extend `internal/tool/registry_test.go`/
  `derive_test.go`: roster contents, guidance renders the new tools, InputSchema
  for monitor_and_act returns the authored schema verbatim, retired names miss.
- [X] T004 Sim substrate in `internal/sim/`: `state.go` gains
  `MetatronOrders []MetatronOrder` (data-model §1, `metatron_orders,omitempty`,
  prune-to-32 non-active); `metatron.go` gains the four order reducer arms
  (validation matrix per contracts/events.md) and the `metatron.nudged` form
  migration (vision/omen/dream-grandfathered, omen requires `State.Night`,
  form-set check replaces the OnRoster check); `loop.go` whitelist gains the three
  injected order types; `executor.go` emits `metatron.order_expired` at
  `tick ≥ expires_tick` (charge_regenerated pattern); `toolcheck.go` coverage
  stays green over the new Expressive tools. Tests in
  `internal/sim/metatron_test.go` (or the reducer suite's home): every reducer
  rejection row, dream-event replay compatibility, omen night gate, vision
  single-target, order lifecycle transitions, executor expiry determinism,
  pre-029 snapshot upgrade (nil orders), replay of a recorded order lifecycle.

**Checkpoint**: `go test ./...` green; boot gates pass; no consumer wired yet.

---

## Phase 3: User Story 1 — Omens and visions replace dreams (P1)

**Goal**: the angel's act vocabulary is send_omen/send_vision; dream retired.

**Independent Test**: quickstart Scenario 1 steps 1 and 4 (vision lands, roster
shows new tools); daytime omen behavior arrives with US4 — until then the
landOmen day path refuses with counsel (temporary, replaced in Phase 6).

- [ ] T005 [US1] Rework `internal/metatron/turn.go`: replace `landNudge` with
  `landVision(target, text, …)` and `landOmen(targets, text, …)` (comma-list/
  `everyone` parsing, alive-set validation, night check against a new mirrored
  `night` flag — add `State.Night` to `mirrorState` in
  `internal/metatron/metatron.go`; day path: temporary refusal-with-counsel,
  superseded in Phase 6); memory prefixes per contracts/events.md; soul-append
  lines; keep the atomic batch shape.
- [ ] T006 [US1] Rework `internal/metatron/toolcalls.go` handlers: `send_vision`/
  `send_omen` replace `nudge_dream`/`nudge_omen` in `turnHandlers` (grant-gated
  as before); update `grantedRoster`/`grantSet` plumbing in
  `internal/metatron/charter.go` if tool-name literals appear there.
- [ ] T007 [US1] Update `internal/metatron/metatron_test.go` +
  `metatron_gaps_test.go`: sentinel/firewall audit covers the new handler names;
  adversarial rows (dead target, two-villager vision, empty text, ungranted tool
  structurally absent); determinism fixtures; retire dream-path tests that assert
  the old tool exists (keep dream REPLAY tests in sim).

**Checkpoint**: visions land live; omens land at night; dream unreachable.

---

## Phase 4: User Story 2 — Standing orders via monitor_and_act (P1)

**Goal**: placement, visibility, cancellation, expiry — the order lifecycle.

**Independent Test**: quickstart Scenario 2 steps 1, 3 (status/restart/replay),
4 (cap), 5 (cancel), 6 (TTL) — trigger execution itself is US3.

- [ ] T008 [US2] Create `internal/metatron/orders.go`: order mirror (mirrorState
  copies `replica.MetatronOrders`), `placeOrder` (id `ord-<tick>-<seq>` per
  research R7, lands `metatron.order_placed` via InjectSocial, in-fiction refusal
  mapping for cap/uncompilable), `cancelOrder` (lands `order_cancelled`), and the
  pure predicate matcher `orderMatches(order, event) bool` (event_types set,
  agent index, lowercase keyword search over payload text) with unit tests in
  `internal/metatron/orders_test.go` (match/no-match matrix, replay-never-matches
  guard is US3).
- [ ] T009 [US2] Wire `monitor_and_act` + `cancel_order` handlers into
  `internal/metatron/toolcalls.go` (grant-gated; empty event_types → gate refusal
  with counsel per research R5); thread order placement through `turnDispatch`.
- [ ] T010 [US2] Surfaces in `internal/metatron/turn.go` + `metatron.go`:
  `Status.Orders []OrderStatus` (additive, omitempty); standing-orders block in
  `turnUserPrompt` (id, condition, remaining game-days, fuzzy marker); expiry
  moment line in `internal/metatron/digest.go`'s observeMoment path
  (`metatron.order_expired` → model-free moment); soul lines for place/cancel.
- [ ] T011 [US2] Tests in `internal/metatron/metatron_test.go`: placement lands
  and mirrors, 4th player order refused, cancel frees slot, status lists orders,
  prompt block renders, expiry queues a moment; extend the sentinel audit to
  `monitor_and_act`/`cancel_order` handler gating.

**Checkpoint**: orders place, show, cancel, expire — nothing fires yet.

---

## Phase 5: User Story 3 — Triggered orders act while away (P1)

**Goal**: a matched order executes as a system-authored turn with full trail.

**Independent Test**: quickstart Scenario 2 step 2 and Scenario 5.

- [ ] T012 [US3] Refactor `internal/metatron/turn.go`: extract the shared body of
  `Turn` into `runTurn(origin turnOrigin, seed string, …)` — console path
  unchanged byte-for-byte in behavior (jobID `turn-metatron-<tick>`); system path
  (jobID `watch-metatron-<tick>`, transcript rendered with a `[watch]` origin
  marker, no player-text sink) per research R6.
- [ ] T013 [US3] Trigger pipeline in `internal/metatron/orders.go` +
  `metatron.go`: absorb-path matching (live events only — `run()` matches AFTER
  replica apply; nothing matches during construction from snapshot), buffered
  trigger channel + worker goroutine (FIFO, order-id order within a batch),
  land `metatron.order_triggered` via InjectSocial (door resolves cancel/expiry
  races), bounded-wait acquisition of `turnBusy` for system turns (console keeps
  fail-fast ErrTurnBusy), moment queued from the system turn's outcome.
- [ ] T014 [US3] Budget/degradation honesty in `internal/metatron/orders.go`:
  empty-bank precheck for known-act (deferral) orders skips the model call and
  queues "strength was spent"; failed system turns (ErrBudgetExhausted /
  ErrTierDown / transport / cap-dry) map each failure family to one model-free
  honest moment, never retry (research R12).
- [ ] T015 [US3] Tests in `internal/metatron/metatron_test.go` +
  `orders_test.go` (scripted runLoop seam): trigger fires → order_triggered +
  system turn + moment; trigger while console turn in flight serializes; cancelled
  order racing its trigger resolves at the door (exactly one of
  triggered/cancelled); empty-bank precheck spends nothing and calls no model;
  budget-exhausted system turn yields one moment and zero retries; replay of a
  world containing order_triggered events reconstructs without any live firing.

**Checkpoint**: the P1 core is complete — agency while away works end-to-end.

---

## Phase 6: User Story 4 — Daytime omens defer to nightfall (P2)

**Goal**: daytime send_omen places a system-origin nightfall order.

**Independent Test**: quickstart Scenario 1 steps 2–3.

- [ ] T016 [US4] Replace the Phase 3 temporary day-refusal in `landOmen`
  (`internal/metatron/turn.go`): day path places a system-origin order
  (event_types `["sim.night_started"]`, action = fixed deliver-omen rendering,
  TTL 1 game day, cap-exempt) via `placeOrder`; `ResultForModel` + reply wording
  promise nightfall; trigger-time landing spends the charge (research R11).
- [ ] T017 [US4] Tests: daytime omen → order_placed(origin system) + no nudged +
  no spend; night trigger → omen lands + one charge + moment; deferred omen
  cancelled before nightfall never lands; deferral order visible in status;
  system-origin orders don't count against the player cap.

---

## Phase 7: User Story 5 — Meta tools: pause, start, adjust speed (P2)

**Goal**: charge-free clock control through registered tools.

**Independent Test**: quickstart Scenario 4.

- [ ] T018 [US5] LoopControl seam: define
  `type LoopControl interface { Do(name string, speed clock.Speed) (sim.Status, error) }`
  in `internal/metatron/metatron.go`, accept it in `metatron.New`, pass the loop
  in `internal/daemon/daemon.go:209`; handlers for `pause`/`start`/`adjust_speed`
  in `internal/metatron/toolcalls.go` mapping to Do("pause"/"resume"/"set_speed")
  with in-fiction ResultForModel; grant-gated like every tool.
- [ ] T019 [US5] Fixed-frame sentence in `internal/metatron/turn.go`
  (`metatronNonNegotiables` or the frame block): meta tools + standing orders
  only on player request or player-placed authorization (contracts/tools.md).
- [ ] T020 [US5] Tests: pause/start/adjust land through a stubbed LoopControl,
  spend nothing, respect grant gating (ungranted ⇒ handler absent + declaration
  absent); sentinel audit extended to assert LoopControl is unreachable outside
  registered handlers; frame determinism fixtures updated.

---

## Phase 8: User Story 6 — Fuzzy conditions confirmed cheaply (P3)

**Goal**: coarse filter + rate-capped KindMetatronWatch confirm.

**Independent Test**: quickstart Scenario 3.

- [ ] T021 [US6] Confirm path in `internal/metatron/orders.go`: fuzzy orders
  (`confirm: true`) route filter hits to a confirm step — one bare
  `Submitter.Submit` on `llm.KindMetatronWatch` (MaxTokens 16, yes/no contract
  per contracts/routing.md), per-order rate cap 1/1800 ticks via
  `lastConfirmTick` (absorb-owned, not event-sourced), skipped hits logged;
  positive verdict → normal trigger pipeline; negative/failed → order stays
  armed, no retry.
- [ ] T022 [US6] Tests in `internal/metatron/orders_test.go`: no confirm calls
  without a filter hit; rate cap skips excess hits; `no`/garbage/error verdicts
  leave the order active; `yes` triggers; confirm failure families
  (budget/tier/transport) are unconfirmed without retry.

---

## Phase 9: Polish & Cross-Cutting

- [ ] T023 [P] Additive client surfaces: render `Status.Orders` where
  metatron status shows (TUI metatron pane `internal/tui/`, CLI
  `promptworld metatron` status output) — additive JSON already tolerated;
  verify `promptworld calibrate`/llm-status enumerate `metatron_watch` via
  `llm.Kinds()` (contracts/routing.md).
- [ ] T024 [P] Docs reconciliation: `docs/llm-providers.md` gains the
  `metatron_watch` kind + backfill note; README capability mentions if the
  angel's tool list appears there.
- [ ] T025 Full-suite + quickstart live validation: `go test ./...`, then run
  quickstart.md Scenarios 1–5 against a throwaway world; record outcomes in the
  implementer report (evidence for the board).
- [ ] T026 Post-merge (root, planning session): `/grounding-wiki:wiki-update`
  re-pin (metatron, tool-registry, tool-loop, llm-orchestrator, event-types,
  sim-loop, sim-state-reducer, executor, ipc-protocol if status contract noted)
  + player-docs freshness check — constitution Principle IV; tracked on TASK-27
  AC #9, not part of the implementer's worktree scope.

---

## Dependencies

- Phase 2 blocks everything; execute T001→T004 sequentially (coupled compiles).
- US1 (Phase 3) ← Phase 2. US2 (Phase 4) ← Phase 2 (independent of US1 except
  shared files — serialize phases). US3 (Phase 5) ← US2 + US1 (its acts are
  omens/visions). US4 (Phase 6) ← US3. US5 (Phase 7) ← Phase 2 only. US6
  (Phase 8) ← US3.
- Suggested delegation batches for spec-implementer: **Batch A** = Phase 2;
  **Batch B** = Phases 3–5 (the P1 core, one agent, sequential);
  **Batch C** = Phases 6–8; **Batch D** = Phase 9 (T023–T025).

## Parallel Opportunities

- Within Phase 9: T023 ∥ T024.
- Phases 3–8 all touch `internal/metatron/` — do NOT parallelize across agents;
  the batches above are sequential handoffs on one branch (one task, one PR).

## Implementation Strategy

MVP = Phases 2–5 (US1+US2+US3): the angel acts while the player is away.
Phases 6–8 are independent increments on top; Phase 9 closes the trail. Every
batch ends with `go test ./...` green and a commit on `task-27-metatron-agency`.
