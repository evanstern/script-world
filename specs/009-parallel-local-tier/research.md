# Phase 0 Research: Parallel Local Tier

All unknowns from the plan's Technical Context resolved against the actual code
(`internal/llm` @ current `main`) and the live measurements recorded on TASK-45.
No NEEDS CLARIFICATION items remained in the spec; the one explicitly delegated
decision (concurrency cap) is R2.

## R1 — Concurrency mechanism: N copies of the existing worker loop

**Decision**: `Orchestrator.New` spawns `t.slots` goroutines of the existing
`worker(t)` loop for the local tier (cloud stays at 1). No semaphore, no worker
pool abstraction, no per-job goroutines.

**Rationale**: the worker loop already carries every per-call invariant this
feature must preserve — two-level priority drain (`prio` before `queue`),
stale-caller skip without breaker strikes, worker-side `workerCallCap`, exactly-once
`health.fail()/succeed()`, estimator sampling, and meter adds. Running N identical
loops against the same two channels preserves all of them per job with a near-zero
diff. Go channels guarantee FIFO delivery to competing receivers, so "overflow waits
in today's priority-then-FIFO order" (FR-002) holds: waiting jobs are handed to free
workers in queue order, and every idle worker still prefers `prio`.

**Alternatives considered**:
- *Semaphore inside a single dispatcher + per-job goroutines*: more invasive (moves
  the call off the worker loop), reintroduces the risk of unbounded goroutines on a
  wedged transport, and buys nothing the N-loop shape doesn't.
- *errgroup / worker-pool library*: dependency and abstraction for a loop the
  package already owns.

**Known subtlety (accepted)**: with N workers, strict global ordering between one
`prio` job and one `queue` job that arrive simultaneously is not total — two free
workers may pick one of each. That matches the spec: priority is a drain preference
for scarce capacity, and with free slots there is no scarcity. When capacity is
scarce (all slots busy), every freed worker prefers `prio` first, which is the
ordering FR-002 protects.

## R2 — Cap and normalization: parallel ∈ [1,16], clamp with boot warning

**Decision**: `LocalConfig.Parallel` (`json:"parallel,omitempty"`), normalized via a
single helper `LocalConfig.Workers() (n int, warn string)`:
- absent / `0` → 1, no warning (byte-for-byte compatibility, FR-001, SC-003)
- negative → 1, warning
- `> 16` → 16, warning
- otherwise → value, no warning

`Orchestrator.New` uses `Workers()` for slot count; `internal/daemon` calls the same
helper at boot and prints the warning (the daemon already owns the boot-line surface,
e.g. the calibration-seed messages). `LoadConfig` never errors on this field (FR-007:
a world never fails to boot over it).

**Rationale for 16**: `queueCap` is 32 — more than 16 slots would let in-flight work
exceed half the queue and mostly measures server-side queueing, not parallelism; the
measured benefit on the reference server was demonstrated at 4. 16 leaves an order of
magnitude of headroom over the measured sweet spot while bounding pathological
configs (`parallel: 10000`).

**Alternatives considered**: erroring on invalid values (violates FR-007); no cap
(pathological configs spawn thousands of goroutines and stampede the server);
cap = NumCPU (server capacity, not daemon CPU, is the real constraint — a fixed
documented number is more predictable).

## R3 — Best-effort admission: free-slot accounting

**Decision**: `tier` gains `slots int` and `inflight atomic.Int32`. Each worker
increments `inflight` when it dequeues a job and decrements when it replies (including
the stale-skip and error paths). `Submit`'s best-effort check becomes:

```
refuse iff len(t.queue) > 0 || len(t.prio) > 0 || t.inflight.Load() >= int32(t.slots)
```

**Rationale**: FR-003 — refuse only when all N slots are occupied. Queue-nonempty
implies all slots busy (a free worker would have drained it), so the queue checks
remain as fast-path refusals and preserve today's pinned behavior. At `parallel: 1`
this implements the long-documented contract ("admitted only when the local tier is
otherwise quiet" — llm.go's own comment; `Request.BestEffort`'s doc says refused
"whenever its tier has work waiting"). The existing suite
(`TestMusingBestEffort`) pins exactly two cases — queued work ⇒ refuse, fully quiet ⇒
serve — both unchanged (SC-003).

**Accepted race**: between the check and the enqueue another job may take the slot;
the musing then briefly waits for the next free worker. Best-effort admission is a
heuristic drop, not a reservation — the same benign race exists today between the
queue-length check and `q <- j`.

## R4 — Estimator under concurrency: no mechanism change

**Decision**: keep the existing per-completion `t.est.Sample(wall/points)` in the
worker; no correction factor, no concurrency-aware model.

**Rationale**: FR-004 asks that estimates be fed "from calls as they actually
complete under concurrency". Per-call wall time under concurrent load already
embeds server-side contention — that IS the true concurrent-rate latency the
governor should predict with. `Estimator` is mutex-protected (`estimate.go`), so N
concurrent `Sample` calls are safe; EWMA order among near-simultaneous completions
is immaterial. The recalibrate-hook fires under the same once-per-breach discipline
(`Estimator.Sample` returns the breach verdict under its own lock — exactly one
worker observes `true`).

**Alternatives considered**: dividing observed wall by in-flight count to estimate
"capacity" (re-creates the sequential-calibration optimism TASK-40 documented —
rejected as dishonest for landing-tick prediction).

## R5 — Health, breaker, meter: prove with -race tests, don't rebuild

**Decision**: no changes to `health.go` / `meter.go`. Add concurrency tests driving
N parallel successes and failures asserting: each outcome counted exactly once
(breaker opens after exactly `failuresToOpen` consecutive failures; one failing call
never corrupts others' accounting — FR-005), and meter total equals the exact sum of
per-call costs after concurrent completions (FR-006, SC-005). Run the package under
`go test -race`.

**Rationale**: both types are already fully mutex-protected; the risk under
concurrency is interleaving semantics (e.g. `fails` counting "consecutive" across
interleaved workers), which tests — not new locks — expose. Note: "consecutive
failures" under concurrency means globally-ordered outcomes across workers; a
success landing between two failures resets the count. That is the same semantics
as today observed at higher frequency, and it is the desired behavior (a healthy
call proves the tier lives).

## R6 — Cloud tier: pinned at one worker

**Decision**: `slots` is hard-coded 1 for `TierCloud`; the `parallel` field exists
only in `LocalConfig` (FR-008). Cloud parallelism, if ever wanted, is its own
feature with meter-ceiling admission implications (budget `Allow()` is checked at
submit, not per-slot).

## R7 — Shutdown: existing discipline suffices

**Decision**: no change. `Close()` closes `o.done`; every worker's selects observe
it between jobs and exit. In-flight provider calls are bounded by `workerCallCap`
(2 min) and their `Submit` callers are released by `<-o.done`. N workers change the
count of goroutines draining, not the shape (spec edge case: shutdown never hangs
beyond existing call-timeout discipline).

## R8 — Live-validation configs (operator machine)

**Decision**: quickstart validation uses the operator's known-good configs rather
than fresh calibration runs: `~/.scratch/calibration.json` (gemma4:12b-mlx local
profile, calibrated 2026-07-21) copied into the test world, and
`~/worlds/village03/llm.json` as the llm.json template with `"parallel": 4` added
to `local`. Rationale: reproduces the exact environment the queue-wait pathology
was measured in, and skips a ~10-minute calibrate run per validation world.
