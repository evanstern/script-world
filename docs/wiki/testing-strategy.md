---
name: testing-strategy
description: How correctness is proven — unit determinism harness, in-process IPC integration, binary-level e2e quickstart scenarios, race detector
kind: pattern
sources:
  - internal/sim/sim_test.go
  - internal/sim/migrate_test.go
  - internal/sim/whole_feature_test.go
  - internal/sim/miracles_test.go
  - internal/sim/metatron_test.go
  - internal/sim/origin_test.go
  - internal/sim/belief_evidence_test.go
  - internal/sim/belief_decay_test.go
  - internal/sim/belief_reinforced_test.go
  - internal/sim/wall_test.go
  - internal/sim/axe_test.go
  - internal/sim/path_speed_test.go
  - internal/world/migrate_test.go
  - internal/ipc/ipc_test.go
  - internal/mind/replay_test.go
  - internal/mind/provenance_test.go
  - internal/mind/belief_read_sites_test.go
  - internal/metatron/metatron_test.go
  - internal/metatron/metatron_gaps_test.go
  - internal/metatron/orders_test.go
  - internal/persona/persona_test.go
  - e2e/daemon_e2e_test.go
  - e2e/determinism_e2e_test.go
verified_against: e9213e17e6e48cf30da802949d9b59e0e3d78370
---

# Testing strategy

The spec's success criteria (determinism, crash-lossless resume, detach-isolation)
are only provable by tests, so the suite is layered: pure-logic harnesses at the
package level, an in-process integration layer, and binary-level e2e that execs the
real `promptworld`.

## How it works

**Unit determinism harness** (`internal/sim/sim_test.go`): `driveTicks` replicates
the loop's semantics minus the real-time scheduler — commands injected at exact tick
boundaries, terrain threaded through exactly as the live loop does. Now proven over
the full [[executor]]: 30k–40k-tick determinism and replay harnesses, plus behavior
suites — multi-step intent chains with zero input (AC#1), needs decay + self-feeding
and starvation death with recorded cause (AC#2), night warmth mechanics and exposure
death (AC#3), and a two-day unattended village survival run on multiple seeds.
(Terrain generation has its own determinism/AC suite in `internal/worldmap`, covered
by [[worldmap-generation]].)

Spec 012 and spec 013 each added their own fixture suite spanning both save-format
packages, all in [[world-migration]]'s territory: `internal/sim/migrate_test.go`
builds representative v1 and v2 states and proves both pure transforms'
carry/reset/re-place/spill rules directly, including a v1 fixture that chains both
transforms (1→2→3) in one call; `internal/world/migrate_test.go` drives the full
`Migrate(dir)` ceremony end-to-end against on-disk v1 and v2 fixture worlds (happy
path, replay-from-zero-snapshots determinism, the already-migrated and
already-current guards, uncovered/tolerated event tails, a running-daemon refusal)
for both the v1→v2 and v2→v3 steps.

`internal/sim/whole_feature_test.go` carries several byte-identity suites (SC-004/SC-005):
the original spec-012 run, a single scripted-agent script chaining every
resources/food/crafting event kind (quarrying — five bare quarries at
`quarryYieldBare` (1) each, since spec 032 T014 split the old flat `quarryYield`
(2) into bare/axe-assisted tiers and dropped the bare yield to 1 — water, the full
craft chain, both cook stations, bathing, refueling, a spear breaking, a fire
burning out) — rebalanced under spec 013's bulk cap (24) to consume-as-it-goes
rather than hoard a large seeded larder — that replays from genesis to a
byte-identical state hash; and a spec-013 storage suite
(`TestReplayByteIdentityWholeFeatureStorage`) exercising every new
013 event type in one run — `agent.dropped`, `agent.picked_up`, `agent.deposited`,
`agent.withdrew` (both an owner fetch and a non-owner theft with its full companion
batch: `social.chest_taken`, a reason-`theft` `social.relation_changed`, and owner +
witness `agent.memory_added`), `sim.food_rotted`, `agent.built{kind: chest}`, and a
death spill — that also replays to a byte-identical hash. The same file also proves
every new 013 event type no-ops under a pre-013 reducer stub (the unknown-type
convention: an event type the reducer's switch doesn't match falls through to a
total no-op, never an error), so old logs stay safely replayable by builds that
predate a given event kind. `TestReplayByteIdentityWallsAxesPaths` (spec 032 T021,
quickstart scenario 7) is the walls/axes/paths counterpart: one scripted session
chains `craft_axe`, an axe-assisted chop, a `build_wall_plank`, a full
`demolish` (chip then destroy), and a `build_path`, asserts every required event
(`agent.crafted`, `agent.chopped`, `agent.built`, `agent.wall_chipped`,
`agent.wall_destroyed`) actually occurred, then replays from genesis (log only) to
a byte-identical state hash. `TestPre032SnapshotLoadsUnchanged` (spec 032 T021,
research R7) proves a pre-032-shaped snapshot (no structure `hp`, no inventory/pile
`axes`) round-trips unmarshal→marshal byte-identically — the new fields are
additive `omitempty`, so an old save loads unchanged with no format-version bump.
Together these prove: same seed + same command timeline
over 30k ticks → byte-identical event sequences and equal state hashes; different
seeds diverge; replaying the logged events over genesis (then re-living the quiet
tail) reproduces the live state hash exactly; the day/night cycle behaves (nobody
moves at night).

**Loop-era replay determinism** (`internal/mind/replay_test.go`): a real `Loop` +
`loopMind` pair proves live-vs-replay byte identity above the pure-reducer layer.
`TestLoopRunReplayByteIdenticalSC002` (TASK-52, SC-002) drives cognitions, tool
calls, and a muse through the real loop, then asserts a from-genesis replay
reproduces the identical `State` with the model seam invoked zero times.
`TestJournalAndSituatedReplayByteIdentical` (spec 019 US4, T019, SC-003) extends
this to the grounded-memories feature: injected situated memories (place/why,
place/conv), a journal write→write→delete cognition sequence, and a scripted
over-budget write that the gate refuses (landing nothing but a rejected
`cog.tool_call`) — genesis replay reproduces the identical `State` *and*
byte-identical rendered `soul.md`/`journal.md` over both live and replayed
state, with the model seam invoked exactly once per live cognition and zero
times during replay.

**IPC integration** (`internal/ipc/ipc_test.go`): a real loop + server + store on a
temp world. Proves: status round trip <2 s; subscribe-from-zero delivers strictly
consecutive seqs; abrupt disconnects and wire garbage leave the loop ticking;
commands are idempotent and land in the log as events; the `state` command's
coherence contract holds (no push predates the snapshot's `last_seq`, and a replica
built from it applies subsequent pushes cleanly — the [[tui-client]] pattern); and
`llm_call` routes through a live [[llm-orchestrator]] while a killed inference
endpoint leaves the loop ticking (the package's own suite covers routing, metering,
ceiling refusal, and circuit recovery against httptest mock providers). Spec 028
(adaptive throttle) adds its own status-fold coverage here: a scripted
`Governor` fake proves debt/jobs fold into `StatusData.Clock` exactly like the
LLM snapshot, a no-governor world reports zero governor values, and a
byte-shape test pins the three new fields `omitempty` (a zero status marshals
with none of them present); a `Loop.Govern`-driven test proves status reports
both the effective and requested speed while governed and that a player
`set_speed` below the governed notch collapses `RequestedSpeed` back to empty;
and a regression test pins that `set_speed max` is still refused with an LLM
configured (FR-012) while `32x` is accepted, unchanged by the governor. Large-reply
behavior (TASK-19) is proven against a `fakeDaemon` wire harness that speaks the
protocol from canned replies: a >1 MiB `state` payload round-trips; a reply over
the 64 MiB cap is substituted server-side with an actionable `reply too large`
error (via `net.Pipe` against `session.writeResponse`); and both the substituted
error and a raw over-long line surface promptly as `ErrReplyTooLarge` — never a
hang or silent scanner death.

**E2E** (`e2e/`): `TestMain` builds the binary once and sets a package-wide
hermetic `PROMPTWORLD_HOME` (a temp dir) before running — every subprocess the
package execs inherits it, so no test can write the developer's real
`~/.promptworld` registry (TASK-49; `manager_e2e_test.go`'s `isolatedHome`
layers a per-test override on top). Worlds drop `llm.json`
right after `new` so they are pure-sim — a precondition for `speed max` under
the TASK-20 policy. Scenarios mirror
`specs/001-world-daemon/quickstart.md` — A: always-on + detach-is-not-pause; B:
pause freezes the clock, compression ratios hold (loose tolerances over short
windows; the spec's 5% applies to 5-minute windows); C: kill -9 → lossless resume
within 10 s, restart-while-paused wakes paused, graceful stop idempotent; E: a
`cp -R`'d stopped world runs. `determinism_e2e_test.go` compares two same-seed
daemons' sim histories over their common tick prefix (past tick 25000, so the
full day-1 [[governance]] meeting cycle is inside the compared window),
excluding wall-dependent `daemon.*`/`clock.*` bookkeeping.

**Miracle cost derivation** (`internal/sim/miracles_test.go`, spec 021):
`TestMiracleCostDerivedFromTool` pins `sim.miracleCost` ≡
`tool.MiracleCostsByEvent()` — the sim-side enforcement table is a derivation of
the registry's single authoritative price source, not a mirror, so a price edit
cannot half-propagate ([[tool-registry]], [[metatron-miracles]]).

**Miracle reducer suite** (`internal/sim/miracles_test.go`, spec 016,
[[metatron-miracles]]): per-arm coverage for all four types — move (villager/
structure-whole/pile-merge, impassable/absent-source rejection), remove
(villager rejected, chest spill, pile destruction, terrain routing), grant
(happy path, over-cap whole-reject, unknown kind, dead villager, non-positive
qty, spear shape), and time-snap (forward-only, duration-preserving,
whole-day-no-drift, mints-no-charges-across-skipped-boundaries, while-paused);
plus charge doctrine (insufficient-charge rejection, gratis waives only the
charge, gratis is logged visibly), and `TestRebaseTaxonomyComplete` — the build
fails if a new tick-anchored `int64` field appears anywhere in the state tree
without a SHIFT/KEEP classification in `rebaseTicks`, so the taxonomy can never
silently drift from the state struct (spec 030 extended this to
`Belief.Reinforced`, the decay-curve anchor). Byte-identity replay suites
(`TestMiracleReplayByteIdentity`, `TestMiracleSnapReplayByteIdentity`,
`TestMiracleGrantReplayByteIdentity`) prove each miracle type replays to the
same state hash as live application.

**Memory-provenance and belief-decay suites** (spec 030, [[metatron]]):
`internal/sim/origin_test.go` proves the `DirectPerception` classifier's closed
vocabulary (action/witness/omen direct; report/gist/digest/absent secondhand),
that every situated constructor stamps `Origin` at emission, that the reducer
copies it verbatim, and that a pre-030 memory (no `origin` field) round-trips
byte-identically. `internal/sim/belief_evidence_test.go` proves belief
formation stamps `Belief.Reinforced` to the formation tick regardless of
evidence, that a later revision leaves it untouched (US2 only re-anchors on
direct evidence), and that a log of coerced beliefs replays byte-identically.
`internal/sim/belief_decay_test.go` pins `EffectiveConfidence`'s half-life
curve to the tick (a pure, computed-on-read function — nothing stored ever
mutates), including a fractional-half-life midpoint proving the curve is
continuous, not integer-stepped, and a legacy no-stamp belief grandfathered to
undecayed. `internal/sim/belief_reinforced_test.go` proves the
`agent.belief_reinforced` reducer arm re-anchors a held belief's clock and is a
total no-op against a vanished belief ID, and that a log containing the event
replays byte-identically. On the mind side, `internal/mind/provenance_test.go`
proves the consolidation user prompt instructs the model to cite evidence and
reserve "witnessed" for direct perception, and that deterministic
provenance enforcement coerces rather than rejects; `internal/mind/belief_read_sites_test.go`
proves the nightly consolidation held-beliefs block is the one documented
exception that renders EFFECTIVE (not stored) confidence and marks a faded
belief while still listing it by ID.

**Walls/axes/paths unit suites** (spec 032, [[tool-registry]]): `internal/sim/wall_test.go`
covers wall build/chip/repair/demolish across both materials (plank vs stone
HP and material cost); `internal/sim/axe_test.go` covers `craft_axe` and the
axe-assisted chop/quarry yield and durability countdown to breakage;
`internal/sim/path_speed_test.go` covers a path tile's travel-speed doubling
over a deterministic grass corridor. These sit alongside
`TestReplayByteIdentityWallsAxesPaths` and `TestPre032SnapshotLoadsUnchanged`
above as the feature's full proof.

**IPC miracle round trips** (`internal/ipc/ipc_test.go`, spec 016): the
operator "miracle" command exercised over the real wire on a pure-sim world
(no LLM/angel) — a move lands, spends a charge, and is visible in the next
state fetch; `--force`/`gratis` lands a miracle against an empty bank and
leaves it untouched at zero, while a non-forced attempt against the same
empty bank is refused; a `give_item` resolves the villager by name and the
grant is visible in the next state fetch; unknown kinds/names are refused
cleanly with the connection surviving.

**Metatron behavioral suites** (`internal/metatron/metatron_test.go`,
`internal/metatron/metatron_gaps_test.go`, TASK-74): the package's own tests
now prove the economy mirror, turn serialization, and context-window
contracts, not just the TASK-64 instruction surface. `metatron_test.go`
(pre-existing) covers turn converse/degraded/fallback paths, influence
landing (charge decrement, atomicity, perception memories), zero-bank
refusal, the firewall sentinel, charter fallbacks, skill-file
eligibility/ordering, the fixed-frame non-negotiables under an adversarial
battery, and capability-manifest gating; spec 029 (TASK-27) extended it with the
agency surface — vision landing and its single-target rejection
(`TestVisionLands`/`TestVisionRejectsMultiTarget`), omen group targeting and
dead-target/day-deferral behavior (`TestOmenLandsOnNamedGroup`,
`TestOmenDeadTargetRefused`, `TestOmenDayDefersToNightfall`,
`TestOmenDayDeferralCapExempt`), the meta tools driving the `LoopControl` seam
including the start-with-speed set_speed-then-resume order and the
converse-never-touches-the-clock guarantee (`TestMetaToolsLandThroughLoopControl`,
`TestMetaToolStartSpeedFailureStopsBeforeResume`, `TestConverseTurnNeverTouchesTheClock`,
`TestMetaToolLoopError`), the extended handler-firewall audit (`TestHandlerFirewallAudit`,
SC-007), the fixed `metatronInitiativeFrame` (`TestInitiativeFrameFixed`), the
clock-speed ladder drift guard (`TestClockSpeedsMirrorLadder`), and the
single-origin directive label — a console prompt carries exactly one
"The player says:" and a system turn's carries none
(`TestTurnDirectiveLabelSingleOrigin`); spec 025 (TASK-72)
extended it with
turn retry-visibility tests (a turn whose loop consumed its transport retry
emits the non-terminal `cog.outcome` retried marker; a clean turn emits none)
and turn token-budget plumbing tests (`metatron.New` stores and passes the
`max_tokens.metatron_turn` budget; the default reproduces 1024). The
tool-loop retry matrix itself lives in `internal/toolloop/retry_test.go`
([[tool-loop]]), with the mind-side twins in `internal/mind/mind_test.go`. `metatron_gaps_test.go` closes what
that suite left untested: `TestChargeMirrorAccrualAndCap` drives
`metatron.charge_regenerated`/`metatron.nudged` through `Observe` → `run()` →
`mirrorState` and proves the bank accrues and caps at `sim.MetatronChargeCap`
without a sim executor; `TestTurnBusyConcurrent` runs two real goroutines
against the `turnBusy` CAS (channel-gated, meaningful under `-race`) to prove
exactly one `Turn` proceeds at a time; `TestObserveNeverBlocks` proves the
notify path drops rather than wedges the caller; `TestAbsorbRefreshesMirrors`
proves an observed batch's effects (alive map, chronicle story tail capped at
8) are visible to the very next turn; and `TestTailOfFile`/
`TestSoulTailWindow`/`TestTranscriptTailTurns` pin the soul/transcript
tail-window truncation rules (`tailOfFile`, the 4000-byte `soulTail`, the
6-whole-turn `transcriptTail`). All new concurrency tests are channel-gated,
never sleep-as-the-only-gate (the TASK-69 flake lesson).

**Standing-order suites** (spec 029, TASK-27, [[metatron-orders]]): the lifecycle
is proven on both sides of the door. Reducer-side (`internal/sim/metatron_test.go`):
`TestMetatronOrderPlacedRejections` and `TestMetatronPlayerOrderCap` pin the
door validation (duplicate id, bad origin, empty event_types, TTL bounds, agent
range, over-long condition/action, and the 3-active player cap with system-origin
exemption); `TestMetatronOrderLifecycle` walks active→terminal transitions and the
cancel/expiry/trigger race (exactly one terminal lands); `TestMetatronOrderExpiryExecutor`
proves the executor emits `metatron.order_expired` as a pure function of state+tick;
`TestMetatronOrdersSnapshotUpgrade` proves a pre-029 snapshot loads with empty order
state; `TestMetatronOrdersReplayIdentically` proves from-genesis replay reconstructs
the order set identically; `TestMetatronOrderPrune` pins the retain-32 rule.
Metatron-side (`internal/metatron/orders_test.go`, 23 tests): the pure matcher and
agent probe (`TestOrderMatches`, `TestEventConcernsAgent`), id sequencing, placement/
cancel/expiry mirroring and prompt block, handler grant-gating, the end-to-end trigger
firing and its serialization with a console turn through the shared `turnBusy`
(`TestTriggerFiresEndToEnd`, `TestTriggerSerializesWithConsoleTurn`), the cancelled/raced
door resolution, the empty-bank precheck spending nothing and the budget-exhausted
one-moment-no-retry degradation (`TestEmptyBankPrecheckSpendsNothing`,
`TestTriggerBudgetExhaustedOneMomentNoRetry`), `TestReplayReconstructsWithoutFiring`
(replay rebuilds state but fires zero triggers, SC-002), the daytime-omen deferral
landing at nightfall and its cancel-before-night path
(`TestDeferredOmenTriggersAtNightfall`, `TestDeferredOmenCancelledNeverLands`), and the
fuzzy-confirm matrix — no-hit silence, rate-cap skipping, negative/failed verdict leaves
armed with no retry, and a yes verdict triggers (`TestFuzzyNoConfirmWithoutHit`,
`TestFuzzyRateCapSkipsExcess`, `TestFuzzyNegativeVerdictLeavesArmed`,
`TestFuzzyConfirmFailureNoRetry`, `TestFuzzyYesTriggers`). Channel-gated throughout.

**Persona lifecycle suite** (`internal/persona/persona_test.go`, TASK-74): on
top of the pre-existing genesis-once/0444/missing-file-load coverage,
`TestPersonaMapsSweepAligned` proves the four index-aligned maps (`Texts`,
`Anchors`, `DriftMarkers`, `Secrets`) stay in lockstep with `sim.AgentNames` —
gaining or losing an entry in any one map fails the sweep;
`TestAnchorsMatchTemperamentLine` pins the documented "deliberately
identical" invariant between `Anchors` and each persona's `**Temperament:**`
line; `TestLoadUnreadableDegrades` proves an unreadable persona file degrades
`Load` to an empty string for that agent only, mirroring the missing-file
contract; `TestGenesisSeedsCharterAndJournal` proves fresh genesis seeds
`charter.md` (= `DefaultCharter`) and a rune-budgeted `journal.md` per agent,
and that an existing `charter.md` is never overwritten; and `TestSecretEvents`
proves the genesis `social.secret_seeded` events are index-aligned,
tick-0, tone `-70`, one per agent.

The whole suite runs under `-race`; it caught a real race (store `lastSeq`, loop
writer vs IPC readers — now atomic).

## Connections

Exercises [[sim-loop]], [[sim-state-reducer]], [[deterministic-rng]] (unit),
[[ipc-server]]/[[ipc-client]] (integration), and [[cli-promptworld]]/
[[daemon-lifecycle]] (e2e). [[metatron-miracles]] and [[metatron-orders]] cover the
reducer arms and doors these suites exercise; [[tool-registry]]'s spec-032 world verbs
(walls/axe/path) are what the new whole-feature and unit suites drive.
[[agent-mind]]/[[tool-loop]] are what the
loop-era replay suite drives through a real `Loop` + `loopMind`; the
provenance/belief-decay suites prove the substrate [[metatron]]'s omen/vision/miracle
memories now stamp. Manual
validation results live in `specs/001-world-daemon/quickstart-results.md`.

## Operational notes

`go test -race ./...` runs everything in ~3 min (e2e dominates at ~187 s; measured
2026-07-23 during TASK-74 — the note's earlier ~25 s figure predates the e2e suite's
growth). E2E timing assertions
use deliberately loose bounds against CI jitter; tighten only with longer windows.
The executor behavior suites are seed-pinned: policy tuning that changes behavior
legitimately requires re-verifying (not deleting) the survival assertions.
