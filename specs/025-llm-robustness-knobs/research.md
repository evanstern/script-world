# Phase 0 Research: llm.json robustness knobs (TASK-72)

All Technical Context entries resolved from the codebase directly; no external
research required. Each decision below is grounded in a verified code path
(file:line as of the 025 spec cut, root at `ea97d31`).

## R1. Where the retry lives

**Decision**: inside `internal/toolloop/loop.go run()`, at the Submit error branch
(`loop.go:195-199`), guarded by `terminationForSubmitErr(serr) == TermProviderError`
and a `retried` flag scoped to the whole run (one retry per cognition run). On first
transport failure: set the flag, stash the reason, `continue` the round loop (the
transcript is untouched — a failed Submit appended nothing, so re-submission is
byte-identical). On second failure, or any non-transport failure: terminate exactly
as today.

**Rationale**: the Submit error branch is the single choke point where all three
provider_error paths named on TASK-72 converge for *transport* failures. The other
two `TermProviderError` sites (`loop.go:269` and `loop.go:287`) are tool-handler
infrastructure failures — the model call *succeeded* and a handler broke; retrying
those would re-dispatch side-effectful handlers. TASK-72 says "in-loop **transport**
retry", so those sites are explicitly out of scope (spec FR-002).

**Alternatives considered**:
- *Retry inside the orchestrator worker (`internal/llm/llm.go`)*: rejected — it
  would silently retry every call kind (conversations would stack on their TASK-42
  retry), and the estimator/meter/breaker live there, making the "no doctrine
  change" argument harder instead of structural.
- *Retry in the consumers (`mind.runPlan` / `metatron.Turn`) by re-running the whole
  loop*: rejected — re-runs completed rounds (wasted provider work, duplicate
  CallRecords), and the task explicitly says "in-loop".

## R2. Retry trigger classification

**Decision**: reuse `terminationForSubmitErr` (`loop.go:322-334`) as the
classifier. Retry fires only when it returns `TermProviderError` — the default arm.
`TermCtxDone` (context cancelled/deadline) and `TermAdmissionRefused`
(`ErrBudgetExhausted`/`ErrQueueFull`/`ErrTierBusy`/`ErrTierDown`) never retry.

**Rationale**: admission refusals are the governor speaking (backpressure, budget,
breaker) — retrying them would fight the ladder and double-tap a busy tier, exactly
what busy-is-not-down doctrine forbids. The conversation precedent
(`mind/convo.go` `isTransport`) draws the same line: backpressure abandons
immediately, never retried.

## R3. Estimator and breaker invariance (spec FR-005/FR-006)

**Decision**: no estimator or breaker code changes; invariance is structural and
gets locked by tests, not by new mechanism.

**Rationale** (verified):
- Every internal Submit already sets `SkipObserve: true` (`loop.go:193`), so a
  retried Submit feeds no per-call latency sample — same as any other loop round
  (`llm.go:488-492`).
- The whole-Run wall-time observation is a single deferred exit path
  (`loop.go:125-131`) that fires **successes-only** (landed / model_done /
  cap_exhausted). A recovered run terminates in the success family → exactly one
  observation, same as a never-failed run. A twice-failed run terminates
  provider_error → zero observations, same as today's once-failed run.
- Breaker strikes happen inside `Submit` per call, identically for every caller; a
  retry is just one more independent Submit — the same thing a *fresh* cognition
  would do a moment later. No new strike classification exists to get wrong.

**Test consequence**: scripted-stub tests assert `ObserveCognition` call counts
(exactly 1 on recovery, 0 on double failure) — the stub already implements the
`submitter` interface (`loop.go:94-97`), so this is observable without touching
`internal/llm`.

## R4. Retry observability surface (spec FR-004)

**Decision**: extend `toolloop.Result` with `Retried bool` and `RetryReason string`.
Consumers surface it as a **non-terminal `cog.outcome` event carrying
`sim.OutcomeRetried`** — mind via its existing `cogOutcomeEvent` family
(`mind/telemetry.go:112`), metatron via its own InjectSocial door (the same channel
its `cog.tool_call` batches ride, `metatron/toolcalls.go`).

**Rationale**: `sim.OutcomeRetried` is already defined as a non-terminal marker
(`sim/cognition.go:24-28`, TASK-42) and already flows for conversation retries, so
the digest catalog and decision-trace view need **no new event type** — avoiding
exactly the catalog-drift failure TASK-62 fixed (`TestCatalogSweep` tripwire).
Result-flag-plus-consumer-event also matches the architecture: the loop returns raw
data; consumers own event emission.

**Alternatives considered**:
- *Synthetic `CallRecord` with a "retried" verdict*: rejected — violates the
  "every model tool call yields exactly one CallRecord" density contract
  (`loop.go:99-107`), pollutes `cog.tool_call` with a non-tool row, and forces a
  digest-grammar/catalog change.
- *Log-only (`log.Printf`)*: rejected — spec FR-004 says the trail, not the logs;
  logs are not the recorded trail (SC-003 requires counting retries from the trail
  alone).

## R5. Config shape for the token budgets

**Decision**: one nested object on `llm.Config`:

```json
{
  "max_tokens": {
    "planner": 768,
    "metatron_turn": 2048,
    "consolidation": 1500
  }
}
```

Go shape: `MaxTokens TokenBudgets \`json:"max_tokens,omitempty"\`` where
`TokenBudgets` has `Planner`, `MetatronTurn`, `Consolidation int64` fields, each
normalized by a method returning `(effective int64, warn string)` mirroring
`Rounds()` (`config.go:43-54`). Defaults 512 / 1024 / 1024; legal range 1–4096;
negative → default + warn; > 4096 → 4096 + warn; absent/0 → default, silent.

**Rationale**: a nested object keeps the three sibling knobs discoverable and leaves
room for future kinds without top-level key sprawl; per-field normalizer methods are
the established, table-tested pattern (`Rounds()`, `Workers()`,
`resolveToolMode()`). The key `metatron_turn` (not `metatron`) is deliberate: the
metatron digest budget (400, `metatron/digest.go:26`) shares the metatron call kind
but is NOT governed by this knob (spec Assumption 2), and the key name should not
imply otherwise.

**Alternatives considered**:
- *Flat top-level keys* (`planner_max_tokens`, …): rejected — three siblings plus
  future growth reads worse than one object; `loop_max_rounds` stayed flat because
  it is a single knob.
- *Per-tier rather than per-kind*: rejected — the task says per-kind, and the same
  local tier serves kinds with intentionally different budgets (planner 512 vs
  conversation 128).

## R6. Plumbing route

**Decision**: `daemon.go` boot resolves the three budgets alongside the existing
knob block (`daemon.go:158-175`), prints any clamp warnings through the same
`"daemon: %s"` channel, and passes resolved values into `mind.New` (planner +
consolidation) and `metatron.New` (turn) as constructor params — exactly how
`loopRounds` travels today (`daemon.go:182`, `mind.go:122`, `metatron.go:100`).
The consts `loopMaxTokens` (`mind.go:378`), `turnMaxTokens` (`turn.go:41`) and the
inline consolidation 1024 (`consolidate.go:133`) become the defaults inside the
normalizers, with the call sites reading injected fields.

**Rationale**: normalization at the config boundary (values arrive pre-clamped;
packages never see raw operator input) is the established discipline; constructor
injection keeps `mind`/`metatron` free of config parsing and keeps tests explicit.
`cmd/promptworld/calibrate.go` constructs minds too (`calibrate.go:269` already
resolves `Rounds()`) and follows the signature change mechanically.

**Alternatives considered**: passing `*llm.Config` into the packages — rejected;
today's layering deliberately hands packages resolved scalars, not the config.

## R7. Failed attempt must not consume a loop round

**Decision**: no code needed — `rounds++` happens only after a successful Submit
(`loop.go:200`), so a failed attempt naturally consumes no round. Locked by a test
(fail on round 1, recover, then run to the cap: total successful rounds must still
reach `MaxRounds`).

## R8. Upper clamp bound 4096

**Decision**: 4096 for all three knobs (single shared bound constant).

**Rationale**: 4–8× the current defaults — real headroom for verbose local models —
while bounding pathological configs, mirroring how `maxLocalWorkers` (16) and
`maxLoopMaxRounds` (16) bound their knobs an order of magnitude above the sweet
spot. Provider-side truncation (`StopMaxTokens`) already handles any model that
cannot honor a large budget, and the existing mid-call-truncation adversarial test
(`toolloop/adversarial_test.go:190`) proves the loop survives it.
