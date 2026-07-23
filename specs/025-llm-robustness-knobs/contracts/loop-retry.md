# Contract: tool-loop transport retry (TASK-72)

Amends the loop guarantees documented at `Run` (`internal/toolloop/loop.go:99-107`
and spec 017's `contracts/loop-api.md`). All existing guarantees stand; this adds
one.

## The retry guarantee

> A cognition run retries a failed model call **exactly once** when and only when
> the failure classifies as a transport-level provider error
> (`terminationForSubmitErr` → `provider_error`). The retry re-submits the
> identical transcript. On a second transport failure — or on the first, if the
> run's retry is already spent — the run terminates `provider_error` with the
> latest error propagated, exactly as a single failure terminates today.

## What never retries

| Failure | Classification | Behavior (unchanged) |
|---------|----------------|----------------------|
| context cancelled / deadline | `ctx_done` | terminate immediately |
| budget exhausted, queue full, tier busy, tier down | `admission_refused` | terminate immediately — the governor spoke; retrying would violate busy-is-not-down |
| tool-handler infrastructure failure (model call succeeded, dispatch broke) | `provider_error` at `loop.go:269/287` | terminate immediately — not a transport failure; handlers are side-effectful and must not be re-dispatched |

## Preserved invariants (each locked by a test)

1. **Round-cap accounting**: a failed attempt consumes no round — `rounds` counts
   model responses (`loop.go:200`). A run that fails once may still complete
   `MaxRounds` full rounds.
2. **Estimator feeding**: internal Submits ride `SkipObserve` (`loop.go:193`);
   the whole-Run wall-time observation stays a single deferred successes-only exit
   path (`loop.go:125-131`). Recovered run → exactly 1 `ObserveCognition` call;
   twice-failed run → 0. Never more.
3. **Breaker semantics**: no orchestrator changes; each Submit strikes exactly as
   an independent call does today. The retry introduces no new strike class.
4. **CallRecord density**: every model tool call still yields exactly one
   CallRecord with dense 1-based ordinals; the retry adds NO synthetic record.
5. **Transcript integrity**: a failed Submit appends nothing, so the retried
   request is byte-identical (one-assistant-turn-per-round invariant untouched).

## Result surface

`toolloop.Result` gains `Retried bool` / `RetryReason string` (see
`data-model.md` §2). Consumers MUST surface `Retried` in the recorded trail as a
non-terminal `cog.outcome` with `sim.OutcomeRetried` — a silent retry is a
contract violation (spec FR-004, SC-003: retries countable from the trail alone).

## Timeout envelope

No new deadlines: the retry runs inside the caller's existing context
(`callTimeout` for planner, `turnTimeout` for metatron). A retry that outlives the
deadline terminates `ctx_done` as today.
