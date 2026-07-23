---
name: testing-strategy
description: How correctness is proven — unit determinism harness, in-process IPC integration, binary-level e2e quickstart scenarios, race detector
kind: pattern
sources:
  - internal/sim/sim_test.go
  - internal/sim/migrate_test.go
  - internal/sim/whole_feature_test.go
  - internal/sim/miracles_test.go
  - internal/world/migrate_test.go
  - internal/ipc/ipc_test.go
  - internal/mind/replay_test.go
  - e2e/daemon_e2e_test.go
  - e2e/determinism_e2e_test.go
verified_against: fdd311a7f7e8b0f5d2c759318a486cc8edd4a06f
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

`internal/sim/whole_feature_test.go` carries two byte-identity suites (SC-004/SC-005):
the original spec-012 run, a single scripted-agent script chaining every
resources/food/crafting event kind (quarrying, water, the full craft chain, both
cook stations, bathing, refueling, a spear breaking, a fire burning out) — rebalanced
under spec 013's bulk cap (24) to consume-as-it-goes rather than hoard a large seeded
larder — that replays from genesis to a byte-identical state hash; and a spec-013
storage suite (`TestReplayByteIdentityWholeFeatureStorage`) exercising every new
013 event type in one run — `agent.dropped`, `agent.picked_up`, `agent.deposited`,
`agent.withdrew` (both an owner fetch and a non-owner theft with its full companion
batch: `social.chest_taken`, a reason-`theft` `social.relation_changed`, and owner +
witness `agent.memory_added`), `sim.food_rotted`, `agent.built{kind: chest}`, and a
death spill — that also replays to a byte-identical hash. The same file also proves
every new 013 event type no-ops under a pre-013 reducer stub (the unknown-type
convention: an event type the reducer's switch doesn't match falls through to a
total no-op, never an error), so old logs stay safely replayable by builds that
predate a given event kind. Together these prove: same seed + same command timeline
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
ceiling refusal, and circuit recovery against httptest mock providers). Large-reply
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
silently drift from the state struct. Byte-identity replay suites
(`TestMiracleReplayByteIdentity`, `TestMiracleSnapReplayByteIdentity`,
`TestMiracleGrantReplayByteIdentity`) prove each miracle type replays to the
same state hash as live application.

**IPC miracle round trips** (`internal/ipc/ipc_test.go`, spec 016): the
operator "miracle" command exercised over the real wire on a pure-sim world
(no LLM/angel) — a move lands, spends a charge, and is visible in the next
state fetch; `--force`/`gratis` lands a miracle against an empty bank and
leaves it untouched at zero, while a non-forced attempt against the same
empty bank is refused; a `give_item` resolves the villager by name and the
grant is visible in the next state fetch; unknown kinds/names are refused
cleanly with the connection surviving.

The whole suite runs under `-race`; it caught a real race (store `lastSeq`, loop
writer vs IPC readers — now atomic).

## Connections

Exercises [[sim-loop]], [[sim-state-reducer]], [[deterministic-rng]] (unit),
[[ipc-server]]/[[ipc-client]] (integration), and [[cli-promptworld]]/
[[daemon-lifecycle]] (e2e). [[metatron-miracles]] covers the reducer arms and
doors these suites exercise. [[agent-mind]]/[[tool-loop]] are what the
loop-era replay suite drives through a real `Loop` + `loopMind`. Manual
validation results live in `specs/001-world-daemon/quickstart-results.md`.

## Operational notes

`go test -race ./...` runs everything in ~25 s (e2e dominates). E2E timing assertions
use deliberately loose bounds against CI jitter; tighten only with longer windows.
The executor behavior suites are seed-pinned: policy tuning that changes behavior
legitimately requires re-verifying (not deleting) the survival assertions.
