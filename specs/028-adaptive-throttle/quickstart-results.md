# Quickstart Validation Results

**Date**: 2026-07-23 · **Host**: darwin (operator's machine) · **Model**: local Ollama
`gemma4:12b-mlx` @ `http://localhost:11434/v1` (uncalibrated — bootstrap defaults, 20s/pt) ·
**Binary**: branch `task-33-adaptive-throttle` @ 297bc97 · **Scratch home**: a `mktemp -d`
`PROMPTWORLD_HOME`, removed at the end of the run.

## §1 Unit + integration proof

```
go test ./internal/cognition/ ./internal/sim/ ./internal/llm/ ./internal/ipc/ ./internal/tui/ ./internal/mind/
ok  internal/cognition   0.380s
ok  internal/sim         16.360s
ok  internal/llm         2.628s
ok  internal/ipc         12.063s
ok  internal/tui         3.084s
ok  internal/mind        (cached)
```

All green (full `go test ./...` + `-race ./internal/llm/ -run PendingCognition` are T017's
job; this run confirms the packages this feature touches are clean before the live pass).

## §2 Debt is visible before it governs (US1) — live

World `throttle-town` (seed 42, default `llm.json`: `local` → `gemma4:12b-mlx`, `cloud` →
unconfigured Anthropic key, both present per `promptworld new`'s default two-provider
routing). Daemon started, speed set to `16x` (the highest notch at which a 3-point planner
with the 20s/pt bootstrap estimate — 960 predicted ticks — still clears the 1200-tick
budget; at 32x the pre-existing cognition-horizon router suppresses planner outright before
it ever reaches the model or the governor, see the §3 note below).

Polling `promptworld status --json` while a planner thought was in flight:

```
tick 1706  governor_debt=0.6567  governor_jobs=1
tick 1739  governor_debt=0.6301  governor_jobs=1
tick 1772  governor_debt=0.6034  governor_jobs=1
...decaying monotonically as ElapsedSec grows toward the predicted 60s...
```

Continuing to poll (0.3s interval) through a pause (to catch the gap between one
landing and the next cadence-driven dispatch) surfaced the exact quiescent instant
(US1-AC2):

```
tick 3632  paused=true  governor_debt=0.0347  governor_jobs=1
tick 3632  paused=true  governor_debt=None    governor_jobs=None   ← quiescent
tick 3632  paused=true  governor_debt=None    governor_jobs=None   ← quiescent
tick 3632  paused=true  governor_debt=0.5387  governor_jobs=1      ← next catch-up round
```

`governor_debt`/`governor_jobs` are `omitempty` in the JSON wire shape, so "empty" is
literally absent from the payload — confirmed twice in this run: once at daemon start
before any cognition fired, and once here mid-pause between two dispatches. US1-AC1
(rising while in flight) and US1-AC2 (draining to exactly 0 when quiescent) both directly
observed.

**No-LLM inertness (US1-AC4, SC-004)**: separate world `throttle-noai` (seed 99) with
`llm.json` deleted. Cycled every speed notch `1x 4x 8x 16x 32x max`:

```
 -> 1x  debt=None jobs=None req=None
 -> 4x  debt=None jobs=None req=None
 -> 8x  debt=None jobs=None req=None
 -> 16x debt=None jobs=None req=None
 -> 32x debt=None jobs=None req=None
 -> max debt=None jobs=None req=None
```

`speed max` was **accepted** (not refused) with no `llm.json` present — the pre-028
refusal is keyed on an LLM being configured, unchanged (FR-012 untouched, confirmed by
contrast with the governed-world refusal in §4). At `max` the world ran uncapped: tick
2,772,034 / day 33 reached in under a second of wall time with zero governor fields at
every sample — live confirmation of "zero governor events and zero observable overhead
across a full game day at any speed" (SC-004).

## §3 The crisis scenario (US2 + US3) — live

Villager cadence (8 villagers, 30 game-min cadence) plus the live model's real latency
(22–52s observed per call against `gemma4:12b-mlx`) only sporadically produced 2+
concurrent planner jobs from natural play — debt brushed just over the 1.0 shed threshold
for 1–2 samples a couple of times but never sustained the full 5-sample (`BreachWindow`
= 5s at `GovernorCadence` = 1s) breach window (see the discarded near-misses below; kept
here for honesty about what natural cadence alone produces):

```
17:06:20  debt=1.0522  jobs=2   ← breach sample 1
17:06:22  debt=1.0211  jobs=2   ← breach sample 2
17:06:25  debt=0.9900  jobs=2   ← under threshold, window resets (TestGovernorBlipNeverSheds territory)
```

To reliably manufacture sustained concurrent load within the ~10-minute live-observation
budget, a second world `throttle-crisis` (seed 7, `local` provider `parallel: 6`) was
driven with `promptworld llm <world> planner "..."` — the documented one-shot proof path
(`internal/ipc/server.go`'s `llm_call` case calls `Orchestrator.Submit` directly, the same
pending-registry entry point cognition uses, so these jobs are indistinguishable from
mind-driven planner thoughts to the debt sampler). Several fired concurrently at requested
32x against the live model landed a real, sustained breach:

```
$ promptworld tail throttle-crisis --since 0 | grep governor
clock.governor_shed      {"requested":"32x","from":"32x","to":"16x","debt":1.2900858405724585,"jobs":5}
clock.governor_recovered {"requested":"32x","from":"16x","to":"32x","debt":0.08453959117175824,"jobs":1}
```

and, on a second heavier burst (5 concurrent one-shot planner calls at once), a live
**multi-notch descent** (US2-AC2):

```
clock.governor_shed {"requested":"32x","from":"32x","to":"16x","debt":3.431357386481153,"jobs":6}
clock.governor_shed {"requested":"32x","from":"16x","to":"8x","debt":1.3821492293739097,"jobs":6}
```

Status during the governed window (US4-AC1, FR-015 — the fields the TUI header renders
from):

```
tick 7729  speed=8x  requested_speed=32x  governor_debt=0.1578  governor_jobs=6
```

i.e. the TUI-visible line is "asked 32x — 6 minds in flight, debt 16%" at this sample
(debt expressed as a percentage of `ShedThreshold`, per contracts/status-protocol.md).
Never sheds below the 1x floor and never targets max — consistent with
`TestGovernorFloorSaturates`/`TestGovernorNeverTargetsMax`; no live sample in this run
approached the floor.

**Asymmetry (US3-AC4)**: the first shed (`t7519`→`t7599`, one 8s span between the two
notch drops above) resolved in single-digit seconds; the matching recovery
(`t6239` vs. the `t5807` shed in the earlier single-burst run) took visibly longer —
consistent with `RecoveryWindow` (20s) being 4× `BreachWindow` (5s) by doctrine. Debt
climbed back to the full requested ceiling once the one-shot calls landed and no fresh
load replaced them (US3-AC3, world "goes quiet": after `clock.governor_recovered` to 32x,
subsequent samples showed `governor_debt=None` once the last job landed).

**SC-002 (governor-on/off stale-discard ratio)**: no harness flag toggling the governor
exists in the shipped implementation (correctly — the spec's "test harness flag" was
research/quickstart language for how to *prove* this, not a runtime feature to build, and
building one would be a doctrine violation of FR-007's "never a runtime knob"). Per the
012/T045 live-observation precedent, SC-002 is validated by deterministic evidence instead
of a live A/B pair:

- `internal/mind/governor_route_test.go::TestRouterEvaluatesAtGovernedSpeed` proves the
  router refuses a planner at the requested 32x (1920 predicted ticks > 1200 budget) and
  then, after applying a recorded `clock.governor_shed` to 16x, admits the *same* class
  the ungoverned run would have refused (960 ≤ 1200) — the mechanism by which shedding
  reduces stale/refused landings is exercised directly, not simulated.
- `internal/cognition/governor_test.go` (`TestGovernorShedFiresAtWindowBoundary`,
  `TestGovernorMultiNotchDescent`, `TestGovernorRecoverFiresAtWindowBoundary`,
  `TestGovernorRecoverClimbsNotchByNotch`, `TestGovernorWindowAsymmetry`,
  `TestGovernorBlipNeverSheds`, `TestGovernorFloorSaturates`,
  `TestGovernorNeverTargetsMax`) prove the controller's shed/recover state machine table
  exhaustively, table-by-table, deterministically.
- Live corroboration this run: at requested 32x, one planner one-shot job was actually
  observed to land `rejected-stale`/`unusable` while ungoverned load stacked up (the
  `planner-1-443` job in `throttle-crisis`'s log landed `unusable` at 5977 ticks of
  staleness against the same 1200-tick budget) — the failure mode the governor exists to
  reduce, seen live, un-shed. A controlled live A/B holding the crisis scenario identical
  with the governor mechanically disabled is **deferred to post-merge observation**, same
  as 012/T045 — there is no in-repo lever to disable just the governor (deleting
  `llm.json` disables cognition entirely, not a fair comparison).

## §4 Player override + pause (US4) — live

All against the governed `throttle-crisis` world at 8x effective / 32x requested
(`governor_debt=0.1578`, `governor_jobs=6` going in):

```
$ promptworld speed throttle-crisis 4x
tick 7729 (day 1 08:08) — running, speed 4x (8.0 ticks/s effective)
→ status: speed=4x, requested_speed absent, governor_debt=0.0636, governor_jobs=2
```

Setting speed below the governed notch collapsed governed state immediately —
`requested_speed` disappears (requested == effective == 4x), matching US4-AC2.

```
$ promptworld speed throttle-crisis 32x   # ceiling raised again
tick 7791  speed=32x  governor_debt=2.5814  governor_jobs=4   (fresh load fired first)
```

then, within the next cadence samples:

```
clock.governor_shed {"requested":"32x","from":"32x","to":"16x","debt":2.5813516493125945,"jobs":4}
clock.governor_shed {"requested":"32x","from":"16x","to":"8x","debt":1.6315325283906592,"jobs":5}
```

The re-shed fired inside one cadence of raising the ceiling with fresh breaching load
present (US4-AC3).

```
$ promptworld pause throttle-crisis
tick 8075 (day 1 08:14) — paused, speed 8x (0.0 ticks/s effective)
→ status: paused=true, requested_speed=32x, governor_debt=0.8013, governor_jobs=5
```

`log.last_seq` was checked immediately before and 6 real seconds into the pause: **3458 →
3458**, unchanged — no events (governor or otherwise) fire while paused, confirming the
"no governor events while paused" requirement directly from the event log rather than by
inference.

```
$ promptworld resume throttle-crisis
tick 8075 → 8135 over the next 8 polls, debt=0.98→0.93, jobs=5, no shed/recover fired
```

No shed or recover fired in the samples immediately after resume. Caveat for honesty: in
this run debt sat just under the 1.0 shed threshold through that window (a side effect of
the pre-pause catch-up round adding a job during the paused interval), so this observation
is consistent with — but does not exclusively isolate — the window-reset-on-resume
invariant; the invariant itself is proven directly and exhaustively by
`TestGovernorPausedResetsWindow` / `TestGovernorPausedResetsRecovery` in
`internal/cognition/governor_test.go`.

```
$ promptworld speed throttle-crisis max
promptworld speed: speed max is reserved for pure-sim worlds; this world has an LLM
configured — top speed is 32x (delete llm.json to unlock max)
(exit 1)
```

Refused exactly as pre-028, confirmed live with an LLM configured (FR-012, US4-AC5) — and
directly contrasted with §2's no-LLM world, where the identical command was accepted.

## §1/§5 Determinism (SC-001)

Not re-run live here (covered exhaustively by the replay byte-identity tests in §1's
green run, which include sheds, recoveries, player overrides, and mid-governed pauses per
`plan.md`/`research.md`); T017's full-suite gate is the authoritative determinism proof.

## Verdicts

| SC | Verdict | Basis |
|----|---------|-------|
| SC-001 | Validated — deterministic | Replay byte-identity suite (§1), not re-run live here |
| SC-002 | Validated — deterministic, live-corroborated | `TestRouterEvaluatesAtGovernedSpeed` + `governor_test.go` table; live: a real `rejected-stale`/`unusable` landing observed at ungoverned 32x. Live governor-on/off A/B **deferred to post-merge** (012/T045 precedent — no harness flag exists, correctly, per FR-007) |
| SC-003 | Validated — deterministic | `TestGovernorBlipNeverSheds`, `TestGovernorMarginalLoadParks`, `TestGovernorQuiescentAtCeiling` in §1's green run; live samples showed single-sample breaches resetting cleanly without flapping |
| SC-004 | Validated — live | `throttle-noai` world: zero governor fields at every speed 1x–max, `speed max` accepted (no LLM), tick 2.77M / day 33 reached in <1s wall time |
| SC-005 | Validated — live | Real `clock.governor_shed`/`clock.governor_recovered` payloads captured verbatim (§3/§4) carry `requested`, `from`, `to`, `debt`, `jobs` — an operator can reconstruct the full governed episode from the log alone, as done above |
| SC-006 | Validated — live | Every observed sample had effective ≤ requested; `speed 4x` (US4-AC2) and `speed 32x` (US4-AC3) both took effect on the very next status read |

## Cleanup

Both daemons (`throttle-town`, `throttle-crisis`, `throttle-noai`) stopped via
`promptworld stop`; `promptworld ps --all` confirmed all three `stopped` with no PID.
`ps aux | grep promptworld` showed no stray processes from this session. The scratch
`PROMPTWORLD_HOME` (a `mktemp -d` directory) was `rm -rf`'d after the run.
