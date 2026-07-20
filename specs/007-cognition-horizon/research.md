# Research: The Cognition Horizon

**Feature**: specs/007-cognition-horizon | **Date**: 2026-07-20

All Technical Context unknowns resolved. Decisions below are grounded in the existing
codebase (wiki notes verified against 8f24c13: [[sim-loop]], [[agent-mind]],
[[llm-orchestrator]], [[event-log]], [[world-save-directory]], [[game-clock]]) and the
pre-session decisions recorded on TASK-32 / decision-4.

## R1 — Where the registry and router live

**Decision**: new leaf package `internal/cognition`, stdlib-only, imported by both
`internal/mind` (routing before enqueue) and `internal/sim` (budget lookup at the
injection doors). Registry is a static Go table keyed by decision class; a
completeness check runs at daemon start: every `llm.Kind` the orchestrator accepts
must map to a registered class or startup fails naming the offender (FR-002).

**Rationale**: mind and sim must not import each other (existing boundary: mind is
I/O, sim is deterministic). A leaf package breaks the cycle. Static Go data (not a
config file) makes the registry versioned with the binary and the world
`format_version`, per decision-4 ("event types become intentional").

**Alternatives considered**: registry as JSON in the save dir — rejected: it would
make authority tunable per-world without a version gate, and invite silent drift
between worlds; points/budgets are doctrine, not preferences. Registry inside
`internal/llm` — rejected: sim would then import llm (which imports the Anthropic
SDK) just to read budgets.

## R2 — Telemetry shape: how thought lifecycle enters the event log

**Decision**: two new whitelisted reducer-no-op event types ride the existing
`inject_social` door (same pattern as `agent.thought`): `cog.thought` (request:
job id, agent, class, points, snapshot tick, trigger event seq, predicted wall ms,
predicted landing tick) and `cog.outcome` (job id, outcome enum, landing tick,
measured staleness ticks, actual wall ms, reason). Router suppressions emit a
`cog.outcome` with outcome `suppressed` and the routing arithmetic — no request
event, since no call was made. Enforcement rejections at `inject_intent` are emitted
by the loop itself in the same command handling that rejected them (so rejection and
its record are one atomic append).

**Rationale**: the event log is already the complete input record of a run
([[event-log]]); `wall_time`-style observability data inside payloads is recorded
data, so replay stays byte-identical (FR-019). Reducer no-ops mean zero state
impact. Causality = trigger event seq + job id + snapshot/landing ticks satisfies
FR-020 (stimulus → thought → intent → action chains) without a new store table.

**Alternatives considered**: separate SQLite table — rejected: breaks the "one log
is the whole truth" property and the existing subscribe/tail tooling. Emitting
telemetry only for failures — rejected: SC-002 requires every thought to terminate
in exactly one recorded outcome.

## R3 — Staleness measurement and enforcement point

**Decision**: staleness ≡ `landing_tick − snapshot_tick` (game ticks actually
elapsed), computed inside the loop's `inject_intent` handler, which gains
`SnapshotTick`, `Generation`, and `Guards` fields on its args. Over-budget or
guard-failed intents do not execute; the handler emits `agent.intent_rejected`
(reducer no-op) + `cog.outcome` in the same batch. The pause property (FR-018) needs
no code: ticks don't advance while paused, so staleness is zero by construction.

**Rationale**: tick-difference is exact under any speed trajectory, mid-flight speed
changes, and pauses — the edge cases collapse into arithmetic. The loop is the only
place that knows the authoritative current tick at the moment of landing.

**Alternatives considered**: wall-clock staleness with speed integration — rejected:
requires reconstructing the speed trajectory over the flight window; tick difference
is the integral, already computed by the world itself.

## R4 — Calibration: reference workload, profile file, live estimator

**Decision**: `scriptworld calibrate` (new CLI subcommand) runs a fixed reference
workload against the configured tiers — N=5 calls per representative shape (1-point
musing shape, 3-point planner shape) per tier, fixed prompts, MaxTokens matching the
real call kinds — and writes `calibration.json` to the save dir
(`world.CalibrationPath()`): per-tier `seconds_per_point` baseline, per-shape
samples, timestamp, model/host identity strings. At daemon start the orchestrator
seeds an in-memory estimator from the file; missing file → documented pessimistic
bootstrap default (20 s/point local, 10 s/point cloud) until live samples converge.
Live estimator: EWMA (α = 0.2) over per-point-normalized call durations measured by
the existing orchestrator worker (which already times calls for `workerCallCap`);
samples > 3× the current estimate are excluded from the EWMA but counted; > 30%
spike rate over a rolling 20-sample window emits a `cog.recalibration_recommended`
telemetry event. The file is only ever written by explicit `calibrate` runs — live
estimates are process-lifetime (re-seeded on restart).

**Rationale**: the orchestrator is the one place every call's true duration is
already observed. Explicit-write-only keeps the profile auditable (matches the
"human retune, never auto-widen" doctrine — the estimator adapts, the recorded
baseline doesn't drift silently). Pessimistic bootstrap fails toward reflex, never
toward stale action (spec edge case).

**Alternatives considered**: persisting the live estimate continuously to the meta
table — rejected: invisible self-tuning state, harder to reason about after
restarts. Calibrating per `llm.Kind` instead of per point-shape — rejected: points
exist precisely to make the scale uniform; per-kind calibration would let the
mapping fragment.

## R5 — Router consultation points and degrade actions

**Decision**: `cognition.Route(class, speed, estimate) Verdict` is consulted in the
mind driver at every enqueue site: planner due/trigger checks (`checkDue`),
musing scheduling, conversation founding (`convo.go` — the scene is one 13-point
decision), meeting rephrase, and the narrator's chapter handoff. Degrade actions per
class: planner → skip (reflex grace is the floor); musing → skip; conversation →
don't found the scene; meeting rephrase → template stands (existing behavior);
consolidation/chronicle → proceed (day-scale budgets; router effectively always
passes ≤32x); a `faster-tier` degrade is registry-expressible but not wired in v1.
Trigger-class gating (session idea D) needs no mechanism: a suppressed class simply
doesn't enqueue from any trigger.

**Rationale**: consult-at-enqueue keeps the router out of the absorb loop's hot path
and means a suppressed decision costs one arithmetic check plus one telemetry event.
Speed is already on the mind's replica state; the estimator handle comes from the
orchestrator at construction.

**Alternatives considered**: routing inside `llm.Submit` — rejected: by then the
prompt snapshot is built and the job queued; the mind also loses the ability to run
the class's degrade action. Cadence-scaling with speed (session idea E) — deferred:
routing already prevents doomed calls; cadence retune is a tuning follow-up once
telemetry exists (recorded as out-of-scope in tasks).

## R6 — Guards and the generation counter

**Decision**: guards are typed, deterministic predicates carried on the intent:
v1 set is `target_alive`, `target_present` (re-resolvable via the existing
`resolveGoal` — the *adapt* rung), `not_superseded` (generation match), and
`after_tick`/`before_tick` (timing window, used by conditional plan steps).
`Agent.Generation` (int64) joins sim `State`, bumped by the reducer on a fixed
high-salience event set: the agent is attacked, witnesses a death, or their tile/
adjacent tile catches an emergency (initial set mirrors the salience-≥9 memory
rows). The mind snapshots generation into every job; the loop compares at landing.

**Rationale**: generation must live in deterministic state (not the mind replica) so
the supersede verdict is replayable from the log. Reusing `resolveGoal` for the
adapt rung means the repair path is the code that already resolves goals — no
second resolver to drift.

**Alternatives considered**: free-form guard expressions from the model — rejected:
guards must be deterministic and enumerable; the model picks from a vocabulary, it
doesn't author predicates.

## R7 — Conditional plans and act-at-time-T

**Decision**: the planner reply vocabulary grows an optional guarded form: instead
of one goal object, a `plan` array of ≤3 steps, each `{goal, when?, until?}` where
`when`/`until` are guard vocabulary terms (timed or state guards). Steps persist in
deterministic state via a new `agent.plan_set` event; the executor evaluates the
head step's guards each tick (cheap: ≤3 steps, 8 agents) and promotes/expires steps
deterministically (`agent.plan_step_started` / `agent.plan_expired`). Default
validity window when `until` is absent: 2 game-hours. Timed guards are the only
act-at-time-T mechanism (FR-017).

**Rationale**: plan steps are recorded events, so replay is untouched; the executor
already owns per-tick agent behavior. Capping at 3 steps bounds both prompt size
(the model must express it in 256 tokens) and per-tick evaluation cost.

**Alternatives considered**: a separate scheduler component — rejected per spec
(timed guards subsume it). Unbounded plans — rejected: budget-hostile and unparseable
at MaxTokens 256.

## R8 — Future-dated prompts

**Decision**: `prompt.go`'s situation block gains one line when a prediction exists:
"It is now {Format(snapshot tick)}. Your decision will take effect around
{Format(snapshot tick + predicted ticks)}." — predicted ticks = predicted wall
seconds × current speed. Musings are not future-dated (no goal effect).

**Rationale**: FR-016 verbatim; uses the router's own prediction so prompt and gate
never disagree.

## R9 — Testing strategy

**Decision**: table-driven unit tests for registry completeness, router purity, and
the estimator (spike rejection, drift following, bootstrap); loop tests for
staleness rejection, guard ladder rungs, generation supersede, and pause-landing
(extend the existing pause-inject test); mock-provider latency injection for the
orchestrator sampling path; an e2e scenario running the same world at 1x and 32x
asserting SC-001/SC-002/SC-006 from the event log; the existing replay harness
asserts byte-equality (SC-003) on a run with cognition telemetry present.

**Rationale**: matches the established testing strategy (httptest mocks, replay
byte-comparison, e2e log audits) — no new test infrastructure.
