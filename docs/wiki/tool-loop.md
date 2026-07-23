---
name: tool-loop
description: The bounded agent tool-use loop driver (spec 017) — submit/dispatch/feed-back to one landed action or a hard cap, transport-agnostic and sim-agnostic, shared by the villager planner and Metatron's console turn
kind: component
sources:
  - internal/toolloop/loop.go
  - internal/toolloop/record.go
verified_against: 8ada1050cc5b108790d0e48640dba0b985632e25
---

# Tool-use loop

`internal/toolloop` (spec 017, TASK-52) is the bounded loop driver every agent
cognition that "acts by calling tools" runs through: submit → dispatch → feed
results back → repeat, until an acting call lands, the model finishes on its
own, the round cap trips, or the transport fails. It replaces the pre-spec-017
pattern of one bare model call whose free-text reply a consumer package
hand-parsed against a hand-maintained vocabulary. The package is deliberately
transport-agnostic and sim-agnostic: it imports only `internal/llm` (the wire)
and `internal/tool` (the schema/roster source), and leaves handlers, artifact
recording, and event emission to the consumer — a shared leaf below both
[[agent-mind]] and [[metatron]] (research R1).

## How it works

**Doctrine, preserved verbatim from the TASK-52 design decisions**: a tool
call is a REQUEST; an event is the FACT; the gate decides; the executor
grounds work in time and space. The driver enforces bounds and RECORDS
requests — it never mutates world state itself. Every durable effect flows
through a handler that wraps an existing landing door (`InjectIntent`, the
`InjectSocial` whitelist), so the loop cannot manufacture a fact the gates
would not admit. Reads return data and ground nothing. Speaking, musing, and
thinking are tools too — game-state integrity applies to expression, not only
world mutation.

**`Run` contract** (`Run(ctx, orch *llm.Orchestrator, j Job) (Result, error)`,
delegating to an unexported `run` over a `submitter` interface — `Submit` +
`ObserveCognition` — so the control flow is unit-testable against a scripted
stub with no network or real orchestrator): a `Job` carries `JobID` (the
existing cognition job identifier, threading every `CallRecord`), `Kind`,
`System`, `Seed` (the initial user turn), `Roster []tool.Tool`, `Handlers
map[string]Handler`, `MaxRounds`, `MaxTokens`, and `Record func(CallRecord)`
(the artifact sink; the consumer buffers/lands records — never touched by the
driver beyond calling it). `MaxRounds <= 0` is defensively treated as 1 (the
real normalization is `llm.Config.Rounds()`, upstream). `Run` guarantees
(contracts/loop-api.md): it terminates within `MaxRounds` provider rounds; at
most one acting call lands; every model tool call yields exactly one
`CallRecord` via `j.Record` (ordinals 1-based, dense, emission-ordered); a
read-effect tool never consumes the action; `SkipObserve` rides every internal
`Submit`; the governor estimator is fed the whole-`Run` wall time only on
a completed termination (successes-only, below); and a transport-level
provider failure is retried EXACTLY ONCE per run (spec 025, below) before it
terminates.

**Transport retry — one per run** (spec 025, TASK-72,
`specs/025-llm-robustness-knobs/contracts/loop-retry.md`): when a `Submit`
fails and `terminationForSubmitErr` classifies it `provider_error` (transport;
NOT the admission-ladder sentinels, NOT context death), the loop re-submits
the identical transcript once — a failed `Submit` appended nothing, so the
retry is byte-identical, and it consumes no round (`rounds` counts model
responses). On a second transport failure, or the first after the run's retry
is spent, the loop terminates `provider_error` with the latest error exactly
as a single failure did pre-025. Admission refusals and ctx-done never retry
(the governor spoke; busy-is-not-down), and a handler infrastructure failure
is not a transport failure (the model call succeeded; handlers are
side-effectful) — those paths are unchanged. `Result.Retried` /
`Result.RetryReason` (the FIRST failure's text; non-empty iff `Retried`)
report the consumed retry for the consumer to surface as a NON-terminal
`cog.outcome` carrying `sim.OutcomeRetried` — the TASK-42 conversation
vocabulary, so no new event type — making every recovery countable from the
trail alone. Estimator/breaker doctrine is untouched structurally: the
retried `Submit` rides `SkipObserve` like any round, a recovered run ends in
the success family and feeds exactly one `ObserveCognition`, a twice-failed
run feeds zero, and each `Submit` strikes the breaker as an independent call.

**Cardinality — one landed acting call, reads exempt**: a tool is "acting"
(`isActing`) iff its `tool.EffectClass` is `World` or `Expressive`; a `Read`
tool does not consume the cognition's one action. Once an acting call has
landed within a response, EVERY remaining call in that same response —
including further reads — is rejected `rejected_cardinality`: the cognition's
one action is spent (FR-004, R8). A read-effect tool dispatched on a non-final
round returns its data and grounds nothing; dispatched on the FINAL round (at
the cap) it is instead recorded `unlanded` without ever calling its handler —
the loop is out of rounds to make use of what it would learn. An acting call,
by contrast, is dispatched on every round including the last — it can land as
the terminal answer without needing a follow-up round.

**Verdict taxonomy** (`Verdict`, data-model.md §5): the DRIVER owns
`rejected_unknown` (the call names a tool not on this cognition's roster, or
one with no registered handler), `rejected_malformed` (driver-side schema/
param validation — `validateArgs`, catching missing required args, wrong
scalar types, enum membership, number bounds, text caps; a tool with an
authored `InputSchemaJSON` override instead gets `validateSetPlan`'s
structural check), and `rejected_cardinality`. A handler's returned `Outcome`
owns `landed`, `rejected_gate` (the door refused — stale, guard, scene,
charge), `read_ok`, and `read_error`. `unlanded` covers a call the loop
terminated before dispatching (cap reached, or a trailing call after an
infrastructure failure in the same batch). Every model tool call ends with
exactly one of these.

**`CallRecord`/`Record` sink** (`record.go`): `CallRecord{JobID, Ordinal,
Tool, Args, Verdict, Reason, Tier}` is the first-class artifact for one model
tool call (FR-007); `{JobID, Ordinal}` is the correlation key. `Args` is a
capped copy (`capArgs`, 2 KiB `maxArgsBytes`) — within the cap, a fresh byte
copy (never aliasing the transcript's buffer); over the cap, it collapses to a
valid JSON string `{"_truncated":true,"prefix":"…"}` with a UTF-8-clean
prefix (a byte-boundary cut that splits a multi-byte rune drops the dangling
partial rather than let `json.Marshal` substitute `U+FFFD`). The driver calls
`j.Record` for every dispatch decision it makes — landed, every rejection
kind, every read outcome, every `unlanded` — so a consumer's telemetry (both
[[agent-mind]]'s mind and [[metatron]] land these as `cog.tool_call` events
via the shared `sim.NewCogToolCallPayload`, [[event-types]], [[cognition]])
can reconstruct the complete call trace even for a cognition where nothing
ever landed.

**Termination taxonomy** (`Termination`, data-model.md §4): `TermLanded` /
`TermModelDone` (the model produced no tool call — Run reports this honestly;
the CONSUMER decides how to record the failure, FR-015) / `TermCapExhausted`
return a nil error; `TermAdmissionRefused` (the submit-side admission ladder —
budget/queue/circuit/best-effort sentinels) / `TermProviderError` /
`TermCtxDone` (context canceled or deadline exceeded) return the underlying
error alongside. `terminationForSubmitErr` maps a `Submit` failure onto one of
the latter three (a `provider_error` `Submit` failure first passes through the
one-per-run transport retry above); a handler's infrastructure failure (`Outcome.Err != nil`)
always terminates the loop with `TermProviderError`, recording the failing
call and every trailing call in the same batch as `unlanded`
(`recordInfraFailure`) — every model tool call still yields exactly one
record even when the loop dies mid-batch.

**Successes-only whole-loop estimator feeding**: `Run`'s deferred exit hook
always sets `res.TotalMillis` (part of `Result` regardless of outcome), but
feeds `Orchestrator.ObserveCognition(j.Kind, res.TotalMillis)` ONLY on a
completed termination — `TermLanded`, `TermModelDone`, `TermCapExhausted` —
each of which measured completed model work (`TermCapExhausted` did N full
provider rounds). The failure family (`TermAdmissionRefused`/
`TermProviderError`/`TermCtxDone`) did no completed thought and feeds nothing,
so a refused or errored loop cannot skew the governor's EWMA toward zero —
mirroring [[llm-orchestrator]]'s own per-call worker doctrine ("a fast
failure is not a latency observation of completed thought"). Every
per-round `Submit` inside the loop sets `Request.SkipObserve: true` so no
fractional per-round sample separately reaches the estimator; the whole-`Run`
observation is the ONLY sample a loop cognition contributes, in the SAME unit
([[cognition]]'s `TierProfile.SecondsPerPoint` doctrine) a single-shot kind's
one-call wall time is.

**Transcript invariant — one assistant turn per round**: the transcript
(`[]llm.Turn`) opens with the seed user turn and each round appends exactly
ONE assistant turn (`assistantEcho`: the model's prose, if any, then one
`llm.Block{ToolUse: ...}` per emitted call, in emission order) followed by one
user turn carrying that round's tool results (`resultBlock` per call). This
one-assistant-turn-per-round shape is load-bearing, not cosmetic: the
`openaiCompat` json-mode fallback ([[llm-orchestrator]]'s `callJSON`)
synthesizes a per-round call ID as `"env-<round>"` from the COUNT OF
ASSISTANT TURNS already in the transcript (`jsonModeRound`), since the flat
envelope carries no ID of its own — any deviation from exactly one assistant
turn per round would collide synthesized IDs across rounds.

**Roster and schema wiring**: `Run` builds the wire-level `[]llm.ToolDecl`
from `j.Roster` — `Name`, `Description: t.PromptGloss`, `InputSchema:
tool.InputSchema(t)` ([[tool-registry]]) — once per invocation; the roster
itself (`tool.LoopRosterVillager()` / `tool.LoopRosterMetatron()`) and its
authored or derived schemas are the tool registry's responsibility, not this
package's.

## Connections

[[llm-orchestrator]] is the transport `Run` drives: `Request.Tools`/`Turns`/
`SkipObserve` out, `Response.ToolCalls`/`Stop` back, and
`Orchestrator.ObserveCognition` for the whole-loop latency feed.
[[tool-registry]] supplies the declared roster (`LoopRosterVillager`/
`LoopRosterMetatron`) and each tool's wire schema (`InputSchema`,
`InputSchemaJSON` overrides). [[agent-mind]]'s `runPlan` is the villager
consumer: it builds a `villagerDispatch`, wraps every acting tool's landing
door in `villagerHandlers` (`internal/mind/handlers.go`), and reads `res.Term`
to decide the terminal `cog.outcome` and rearm exactly as the pre-loop
rejection/failure paths did. [[metatron]]'s `Turn` is the console consumer:
its `turnHandlers` wrap `landNudge`/`landMiracle`, and `converse` is
deliberately NOT a declared tool — the model's closing prose
(`Result.Final`) is the transcript-only answer channel. [[cognition]] owns
the decision-class registry and staleness router both consumers gate on
before ever calling `Run`; [[event-types]] catalogs `cog.tool_call`, the
event both consumers land from buffered `CallRecord`s.

## Operational notes

The package has no environment variables and no persisted state of its own —
the transcript is ephemeral (never persisted, never replayed) and every
durable trace is the consumer's `CallRecord` emission. `contracts/loop-api.md`
and `data-model.md` (`specs/017-agent-tool-loop/`) are the authored contract
this note grounds; `loop_test.go`/`equivalence_test.go`/`governor_test.go`/
`adversarial_test.go` exercise the cardinality rule, the termination
taxonomy, the successes-only estimator feed, and adversarial model behavior
(over-cap calls, malformed args, an unknown tool name) respectively;
`retry_test.go` (spec 025) locks the transport-retry matrix — fail-once
recovery, fail-twice termination, admission/ctx/handler failures never
retried, round-cap and estimator invariance.
