---
name: testing-strategy
description: How correctness is proven — unit determinism harness, in-process IPC integration, binary-level e2e quickstart scenarios, race detector
kind: pattern
sources:
  - internal/sim/sim_test.go
  - internal/sim/migrate_test.go
  - internal/sim/whole_feature_test.go
  - internal/world/migrate_test.go
  - internal/ipc/ipc_test.go
  - e2e/daemon_e2e_test.go
  - e2e/determinism_e2e_test.go
verified_against: 1d1cc6ff8cad2414108f7e768f61eb0faaea3088
---

# Testing strategy

The spec's success criteria (determinism, crash-lossless resume, detach-isolation)
are only provable by tests, so the suite is layered: pure-logic harnesses at the
package level, an in-process integration layer, and binary-level e2e that execs the
real `scriptworld`.

## How it works

**Unit determinism harness** (`internal/sim/sim_test.go`): `driveTicks` replicates
the loop's semantics minus the real-time scheduler — commands injected at exact tick
boundaries, terrain threaded through exactly as the live loop does. Now proven over
the full [[executor]]: 30k–40k-tick determinism and replay harnesses, plus behavior
suites — multi-step intent chains with zero input (AC#1), needs decay + self-feeding
and starvation death with recorded cause (AC#2), night warmth mechanics and exposure
death (AC#3), and a two-day unattended village survival run on multiple seeds.
(Terrain generation has its own determinism/AC suite in `internal/worldmap`, covered
by [[worldmap-generation]].) Spec 012 added its own fixture suite spanning both
save-format packages — `internal/sim/migrate_test.go` and `internal/world/migrate_test.go`
build representative v1 states and prove the transform's carry/reset/re-place rules
([[world-migration]]) — plus `whole_feature_test.go`, a single scripted-agent run
chaining every new resources/food/crafting event kind (quarrying, water, the full
craft chain, both cook stations, bathing, refueling, a spear breaking, a fire burning
out) that replays from genesis to a byte-identical state hash (SC-004). Proves: same
seed + same command timeline over 30k ticks → byte-identical
event sequences and equal state hashes; different seeds diverge; replaying the logged
events over genesis (then re-living the quiet tail) reproduces the live state hash
exactly; the day/night cycle behaves (nobody moves at night).

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

**E2E** (`e2e/`): `TestMain` builds the binary once; worlds drop `llm.json`
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

The whole suite runs under `-race`; it caught a real race (store `lastSeq`, loop
writer vs IPC readers — now atomic).

## Connections

Exercises [[sim-loop]], [[sim-state-reducer]], [[deterministic-rng]] (unit),
[[ipc-server]]/[[ipc-client]] (integration), and [[cli-scriptworld]]/
[[daemon-lifecycle]] (e2e). Manual validation results live in
`specs/001-world-daemon/quickstart-results.md`.

## Operational notes

`go test -race ./...` runs everything in ~25 s (e2e dominates). E2E timing assertions
use deliberately loose bounds against CI jitter; tighten only with longer windows.
The executor behavior suites are seed-pinned: policy tuning that changes behavior
legitimately requires re-verifying (not deleting) the survival assertions.
